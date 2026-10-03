package runner

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type commandResult struct {
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exit_code"`
	Log      string `json:"log"`
	TimedOut bool   `json:"timed_out"`
}

type sandboxResults struct {
	Configure commandResult
	Build     commandResult
	CTest     commandResult
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
emit_stage() {
  name="$1"
  code="$2"
  log="/tmp/cppdefense-${name}.log"
  printf '__CPPDEFENSE_STAGE__ %s %s\n' "$name" "$code"
  if [ -s "$log" ]; then
    tail -c 200000 "$log" | base64 -w 0
  fi
  printf '\n__CPPDEFENSE_END__\n'
}

cmake -S /workspace -B /build -DCMAKE_BUILD_TYPE=Release >/tmp/cppdefense-configure.log 2>&1
configure_code=$?
emit_stage configure "$configure_code"
if [ "$configure_code" -ne 0 ]; then exit 0; fi

cmake --build /build --parallel 2 >/tmp/cppdefense-build.log 2>&1
build_code=$?
emit_stage build "$build_code"
if [ "$build_code" -ne 0 ]; then exit 0; fi

ctest --test-dir /build --output-on-failure >/tmp/cppdefense-ctest.log 2>&1
ctest_code=$?
emit_stage ctest "$ctest_code"
exit 0
`

func runSandbox(ctx context.Context, runtime, image, project string) sandboxResults {
	absolute, _ := filepath.Abs(project)
	args := []string{"run", "--rm", "--network", "none", "--cpus", "1.0", "--memory", "512m", "--memory-swap", "512m", "--pids-limit", "128", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=256m", "--tmpfs", "/build:rw,nosuid,nodev,size=768m", "--user", "65532:65532", "-v", absolute + ":/workspace:ro", image, "sh", "-lc", sandboxScript}
	command := exec.CommandContext(ctx, runtime, args...)
	var output limitedBuffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	exitCode := 0
	if exit, ok := err.(*exec.ExitError); ok {
		exitCode = exit.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return parseSandboxResults(output.String(), exitCode, ctx.Err() != nil)
}

func parseSandboxResults(output string, containerExitCode int, timedOut bool) sandboxResults {
	parsed := make(map[string]commandResult, 3)
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var stage string
	var exitCode int
	var encoded strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
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
		return sandboxResults{Configure: fallback, Build: fallback, CTest: fallback}
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
	return sandboxResults{Configure: configure, Build: build, CTest: ctest}
}
