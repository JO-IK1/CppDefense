package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type commandResult struct {
	OK         bool     `json:"ok"`
	ExitCode   int      `json:"exit_code"`
	Log        string   `json:"log"`
	TimedOut   bool     `json:"timed_out"`
	DurationMS *float64 `json:"duration_ms,omitempty"`
	Reused     bool     `json:"reused,omitempty"`
}

type sandboxResults struct {
	InfrastructureError bool          `json:"-"`
	Configure           commandResult `json:"configure"`
	Build               commandResult `json:"build"`
	CTest               commandResult `json:"ctest"`
	CompileMS           *float64      `json:"compile_ms,omitempty"`
	LinkMS              *float64      `json:"link_ms,omitempty"`
	CompileCommands     int           `json:"compile_commands"`
	LinkCommands        int           `json:"link_commands"`
	SnapshotMS          float64       `json:"snapshot_ms"`
	SnapshotExportOK    bool          `json:"-"`
}

func (r sandboxResults) OK() bool {
	return r.Configure.OK && r.Build.OK && r.CTest.OK
}

// makeSandboxReadable lets the unprivileged user inside the rootless
// container traverse and read the materialized project. The bind mount stays
// read-only, and no write permission is granted to any user.
func makeSandboxReadable(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("sandbox project contains symbolic link: %s", path)
		}
		mode := fs.FileMode(0o444)
		if entry.IsDir() {
			mode = 0o555
		}
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("prepare sandbox path %s: %w", path, err)
		}
		return nil
	})
}

func restoreSandboxOwnerAccess(root string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		mode := fs.FileMode(0o600)
		if entry.IsDir() {
			mode = 0o700
		}
		return os.Chmod(path, mode)
	})
}

const sandboxScript = `
printf '__CPPDEFENSE_READY__\n'
now() { date +%s%N; }
emit_stage() {
  name="$1"; code="$2"; elapsed="$3"
  log="/tmp/cppdefense-${name}.log"
  printf '__CPPDEFENSE_STAGE__ %s %s\n' "$name" "$code"
  if [ -s "$log" ]; then tail -c 200000 "$log" | base64 -w 0; fi
  printf '\n__CPPDEFENSE_END__\n'
  printf '__CPPDEFENSE_TIMING__ %s %s\n' "$name" "$((elapsed/1000000))"
}
command_times() {
  if [ -x /opt/cppdefense/timed.sh ]; then
    for command_phase in compile link; do
      awk -v phase="$command_phase" '$1==phase {sum+=$2; n++} END {printf "__CPPDEFENSE_COMMAND_TIME__ %s %.3f %d\n",phase,sum/1000000,n}' /tmp/cppdefense-command-times
    done
  fi
}
phase="${CPPDEFENSE_PHASE:-all}"
touch /tmp/cppdefense-command-times
if [ -d /seed ]; then
  start=$(now)
  find /seed -mindepth 1 -maxdepth 1 -print0 | xargs -0 -r cp -R --preserve=mode,timestamps -t /build/ -- || exit 125
  find /build -mindepth 1 ! -type l -exec chmod u+rwX {} + || exit 125
  end=$(now)
  printf '__CPPDEFENSE_TIMING__ snapshot_copy %s\n' "$(((end-start)/1000000))"
fi
if [ "$phase" != test ]; then
  if [ "$phase" = reuse ]; then
    printf 'Использован проверенный снимок CMake/Ninja.\n' >/tmp/cppdefense-configure.log
    emit_stage configure 0 0
  else
    cache_args=""
    if command -v ccache >/dev/null 2>&1 && [ -d /ccache ]; then
      export CCACHE_DIR=/ccache CCACHE_MAXSIZE=256M CCACHE_COMPILERCHECK=content
      cache_args="-DCMAKE_C_COMPILER_LAUNCHER=ccache -DCMAKE_CXX_COMPILER_LAUNCHER=ccache"
    fi
    gtest=/usr/src/googletest
    if [ -r /opt/cppdefense/googletest/CMakeLists.txt ]; then gtest=/opt/cppdefense/googletest; fi
    timing=""
    if [ -r /opt/cppdefense/timing.cmake ]; then timing="-DCMAKE_PROJECT_INCLUDE_BEFORE=/opt/cppdefense/timing.cmake"; fi
    start=$(now)
    cmake -S /workspace -B /build -G Ninja $cache_args $timing \
      -DCMAKE_BUILD_TYPE=Release \
      -DFETCHCONTENT_SOURCE_DIR_GOOGLETEST="$gtest" \
      -DFETCHCONTENT_FULLY_DISCONNECTED=ON >/tmp/cppdefense-configure.log 2>&1
    code=$?;end=$(now);emit_stage configure "$code" "$((end-start))"
    if [ "$code" -ne 0 ]; then command_times;exit 0;fi
  fi
  start=$(now)
  cmake --build /build --parallel 2 >/tmp/cppdefense-build.log 2>&1
  code=$?;end=$(now);emit_stage build "$code" "$((end-start))"
  command_times
  if [ "$code" -ne 0 ]; then exit 0;fi
  if [ "$phase" = build ]; then
    printf '\n__CPPDEFENSE_SNAPSHOT__\n'
    tar --format=pax -C /build -cf - .
    exit $?
  fi
fi
start=$(now)
ctest --test-dir /build --output-on-failure --no-tests=error >/tmp/cppdefense-ctest.log 2>&1
code=$?;end=$(now);emit_stage ctest "$code" "$((end-start))"
exit 0
`

func runSandbox(ctx context.Context, runtime, image, project string) sandboxResults {
	return runSandboxCached(ctx, runtime, image, project, "")
}

func sandboxArguments(image, project string) []string {
	return []string{"run", "--rm", "--network", "none", "--cpus", "1.0", "--memory", "512m", "--memory-swap", "512m", "--pids-limit", "128", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=256m", "--tmpfs", "/build:rw,exec,nosuid,nodev,size=768m", "--user", "65532:65532", "-v", project + ":/workspace:ro", image, "sh", "-lc", sandboxScript}
}

func runSandboxCached(ctx context.Context, runtime, image, project, sessionID string) sandboxResults {
	return runSandboxOptions(ctx, runtime, image, project, sessionID, "", "all", nil)
}

func runSandboxOptions(ctx context.Context, runtime, image, project, sessionID, seed, phase string, artifact io.Writer) sandboxResults {
	absolute, _ := filepath.Abs(project)
	args := sandboxArguments(image, absolute)
	extra := []string{"--env", "CPPDEFENSE_PHASE=" + phase}
	if phase != "build" {
		extra = append(extra, "--env", "CCACHE_DISABLE=1")
	}
	if seed != "" {
		extra = append(extra, "-v", seed+":/seed:ro")
	}
	position := len(args) - 4
	args = append(args[:position], append(extra, args[position:]...)...)
	if phase == "build" && sessionID != "" && filepath.Base(runtime) == "podman" {
		// Only original preparation can access a persistent compiler cache.
		// Student builds and tests never mount it, so test state cannot survive.
		digest := sha256.Sum256([]byte("snapshot-original-v1\x00" + image + "\x00" + sessionID))
		volume := fmt.Sprintf("cppdefense-ccache-%x", digest[:16])
		create := exec.CommandContext(ctx, runtime, "volume", "create", "--label", "cppdefense.cache=compiler", volume)
		if err := create.Run(); err == nil {
			position := len(args) - 4
			args = append(args[:position], append([]string{"-v", volume + ":/ccache:U"}, args[position:]...)...)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cid, err := os.CreateTemp("", "cppdefense-cid-*")
	if err != nil {
		return sandboxResults{InfrastructureError: true, Configure: commandResult{ExitCode: -1, Log: err.Error()}}
	}
	cidPath := cid.Name()
	cid.Close()
	os.Remove(cidPath)
	defer os.Remove(cidPath)
	position = len(args) - 4
	args = append(args[:position], append([]string{"--cidfile", cidPath}, args[position:]...)...)
	command := exec.CommandContext(runCtx, runtime, args...)
	output := sandboxOutput{started: time.Now(), artifact: artifact, cancel: cancel}
	var stderr limitedBuffer
	command.Stdout, command.Stderr = &output, &stderr
	err = command.Run()
	output.finish()
	if runCtx.Err() != nil {
		raw, _ := os.ReadFile(cidPath)
		id := strings.TrimSpace(string(raw))
		if len(id) == 64 && strings.Trim(id, "0123456789abcdef") == "" {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			_ = exec.CommandContext(cleanup, runtime, "rm", "--force", id).Run()
			stop()
		}
	}
	exitCode := 0
	if exit, ok := err.(*exec.ExitError); ok {
		exitCode = exit.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	result := parseSandboxResults(output.String()+"\n"+stderr.String(), exitCode, ctx.Err() != nil)
	result.SnapshotExportOK = artifact != nil && err == nil && output.exporting && output.exported > 0
	recordSandboxTimings(ctx, result, output.containerMS())
	return result
}

type sandboxOutput struct {
	sync.Mutex
	data      limitedBuffer
	started   time.Time
	readyMS   *float64
	artifact  io.Writer
	pending   []byte
	exporting bool
	exported  int64
	cancel    context.CancelFunc
}

func (o *sandboxOutput) Write(data []byte) (int, error) {
	o.Lock()
	defer o.Unlock()
	if o.artifact == nil {
		return o.writeLog(data)
	}
	if o.exporting {
		return o.writeArtifact(data)
	}
	o.pending = append(o.pending, data...)
	marker := []byte("\n__CPPDEFENSE_SNAPSHOT__\n")
	if at := bytes.Index(o.pending, marker); at >= 0 {
		o.writeLog(o.pending[:at])
		tail := o.pending[at+len(marker):]
		o.exporting = true
		_, err := o.writeArtifact(tail)
		o.pending = nil
		return len(data), err
	}
	if len(o.pending) > len(marker) {
		n := len(o.pending) - len(marker)
		o.writeLog(o.pending[:n])
		o.pending = append([]byte{}, o.pending[n:]...)
	}
	return len(data), nil
}
func (o *sandboxOutput) writeLog(data []byte) (int, error) {
	n, err := o.data.Write(data)
	if o.readyMS == nil && strings.Contains(o.data.String(), "__CPPDEFENSE_READY__") {
		o.readyMS = timingPointer(elapsedMS(o.started))
	}
	return n, err
}
func (o *sandboxOutput) writeArtifact(data []byte) (int, error) {
	if o.exported+int64(len(data)) > 800<<20 {
		if o.cancel != nil {
			o.cancel()
		}
		return 0, fmt.Errorf("snapshot exceeds 800 MiB")
	}
	n, err := o.artifact.Write(data)
	o.exported += int64(n)
	if err != nil && o.cancel != nil {
		o.cancel()
	}
	return n, err
}
func (o *sandboxOutput) finish()        { o.Lock(); defer o.Unlock(); o.writeLog(o.pending); o.pending = nil }
func (o *sandboxOutput) String() string { o.Lock(); defer o.Unlock(); return o.data.String() }
func (o *sandboxOutput) containerMS() float64 {
	o.Lock()
	defer o.Unlock()
	if o.readyMS != nil {
		return *o.readyMS
	}
	return elapsedMS(o.started)
}
func recordSandboxTimings(ctx context.Context, r sandboxResults, container float64) {
	addTiming(ctx, func(t *postgres.JobTimings) {
		t.ContainerMS += container
		t.SnapshotMS += r.SnapshotMS
		if r.Configure.DurationMS != nil {
			t.ConfigureMS += *r.Configure.DurationMS
		}
		if r.Build.DurationMS != nil {
			t.BuildMS += *r.Build.DurationMS
		}
		if r.CTest.DurationMS != nil {
			t.TestMS += *r.CTest.DurationMS
		}
		if r.CompileMS != nil {
			if t.CompileMS == nil {
				t.CompileMS = timingPointer(0)
			}
			*t.CompileMS += *r.CompileMS
		}
		if r.LinkMS != nil {
			if t.LinkMS == nil {
				t.LinkMS = timingPointer(0)
			}
			*t.LinkMS += *r.LinkMS
		}
		t.CompileCommands += r.CompileCommands
		t.LinkCommands += r.LinkCommands
	})
}

func parseSandboxResults(output string, containerExitCode int, timedOut bool) sandboxResults {
	parsed := make(map[string]commandResult, 3)
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var stage string
	var exitCode int
	var encoded strings.Builder
	durations := map[string]float64{}
	var compile, link *float64
	var compileCount, linkCount int
	var snapshotMS float64
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "__CPPDEFENSE_TIMING__" {
			value, err := strconv.ParseFloat(fields[2], 64)
			if err == nil && value >= 0 && value < 86400000 {
				durations[fields[1]] = value
				if fields[1] == "snapshot_copy" {
					snapshotMS = value
				}
			}
			continue
		}
		if len(fields) == 4 && fields[0] == "__CPPDEFENSE_COMMAND_TIME__" {
			value, err := strconv.ParseFloat(fields[2], 64)
			count, countErr := strconv.Atoi(fields[3])
			if err == nil && countErr == nil && value >= 0 && value < 86400000 && count >= 0 && count < 1000000 {
				if fields[1] == "compile" {
					compile = timingPointer(value)
					compileCount = count
				}
				if fields[1] == "link" {
					link = timingPointer(value)
					linkCount = count
				}
			}
			continue
		}
		if fields := strings.Fields(line); len(fields) == 3 && fields[0] == "__CPPDEFENSE_STAGE__" {
			stage = fields[1]
			exitCode, _ = strconv.Atoi(fields[2])
			encoded.Reset()
			continue
		}
		if line == "__CPPDEFENSE_END__" && stage != "" {
			log, decodeErr := base64.StdEncoding.DecodeString(encoded.String())
			if decodeErr != nil {
				log = []byte("Не удалось прочитать журнал этапа: " + decodeErr.Error())
				exitCode = -1
			}
			parsed[stage] = commandResult{OK: exitCode == 0, ExitCode: exitCode, Log: string(log)}
			stage = ""
			continue
		}
		if stage != "" {
			encoded.WriteString(strings.TrimSpace(line))
		}
	}

	fallback := commandResult{ExitCode: containerExitCode, Log: strings.TrimSpace(output), TimedOut: timedOut}
	if fallback.Log == "" {
		fallback.Log = "Sandbox завершился без результата"
	}
	if len(parsed) == 0 {
		return sandboxResults{Configure: fallback, Build: fallback, CTest: fallback, InfrastructureError: !timedOut}
	}

	skipped := func(after string) commandResult {
		message := "Этап не выполнялся, потому что завершился ошибкой этап «" + after + "»"
		if timedOut {
			message = "Этап не завершён: превышен лимит времени проверки"
		}
		return commandResult{ExitCode: -1, Log: message, TimedOut: timedOut}
	}
	configure, ok := parsed["configure"]
	if !ok {
		configure = fallback
	}
	build, ok := parsed["build"]
	if !ok {
		build = skipped("Конфигурация CMake")
	}
	ctest, ok := parsed["ctest"]
	if !ok {
		ctest = skipped("Сборка проекта")
	}
	if value, ok := durations["configure"]; ok {
		configure.DurationMS = timingPointer(value)
	}
	configure.Reused = strings.HasPrefix(configure.Log, "Использован проверенный снимок CMake/Ninja.")
	if value, ok := durations["build"]; ok {
		build.DurationMS = timingPointer(value)
	}
	if value, ok := durations["ctest"]; ok {
		ctest.DurationMS = timingPointer(value)
	}
	return sandboxResults{Configure: configure, Build: build, CTest: ctest, CompileMS: compile, LinkMS: link, CompileCommands: compileCount, LinkCommands: linkCount, SnapshotMS: snapshotMS}
}
