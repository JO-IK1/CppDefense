package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSnapshotStreamHandlesSplitMarkerAndBinaryPayload(t *testing.T) {
	var artifact bytes.Buffer
	output := sandboxOutput{artifact: &artifact}
	payload := []byte{0, 255, '\n', 1, 0}
	for _, chunk := range [][]byte{[]byte("log\n__CPPDEFENSE_SNAP"), []byte("SHOT__\n"), payload} {
		if _, err := output.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	output.finish()
	if output.String() != "log" || !bytes.Equal(artifact.Bytes(), payload) {
		t.Fatalf("stream mixed logs and archive: %q %v", output.String(), artifact.Bytes())
	}
	canceled := false
	output.exported = 800 << 20
	output.cancel = func() { canceled = true }
	if _, err := output.Write([]byte{1}); err == nil || !canceled {
		t.Fatal("oversized archive did not cancel export")
	}
}

func TestCompilerCacheIsReusedOnlyWithinSameSessionAndImage(t *testing.T) {
	directory := t.TempDir()
	runtime := filepath.Join(directory, "podman")
	log := filepath.Join(directory, "arguments")
	t.Setenv("CPPDEFENSE_TEST_ARGS", log)
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CPPDEFENSE_TEST_ARGS\"\n"
	if err := os.WriteFile(runtime, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{{"image-a", "session-a"}, {"image-a", "session-a"}, {"image-a", "session-b"}, {"image-b", "session-a"}} {
		runSandboxOptions(context.Background(), runtime, input[0], directory, input[1], "", "build", io.Discard)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var volumes []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "volume create ") {
			fields := strings.Fields(line)
			volumes = append(volumes, fields[len(fields)-1])
		}
	}
	if len(volumes) != 4 || volumes[0] != volumes[1] || volumes[0] == volumes[2] || volumes[0] == volumes[3] {
		t.Fatalf("cache reuse/isolation failed: %v", volumes)
	}
	if strings.Count(string(data), ":/ccache:U") != 4 || !strings.Contains(string(data), "--network none") {
		t.Fatal("cache mount or sandbox restrictions missing")
	}
}

func TestStudentAttemptsDoNotMountMutableCompilerCache(t *testing.T) {
	directory := t.TempDir()
	runtime := filepath.Join(directory, "podman")
	log := filepath.Join(directory, "arguments")
	t.Setenv("CPPDEFENSE_TEST_ARGS", log)
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CPPDEFENSE_TEST_ARGS\"\n"
	if err := os.WriteFile(runtime, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runSandboxOptions(context.Background(), runtime, "image", directory, "session", "", "all", nil)
	runSandboxOptions(context.Background(), runtime, "image", directory, "session", directory, "reuse", nil)
	runSandboxOptions(context.Background(), runtime, "image", directory, "session", directory, "test", nil)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "volume create") || strings.Contains(string(data), ":/ccache") {
		t.Fatal("student attempt can change persistent cache")
	}
	if strings.Count(string(data), "CCACHE_DISABLE=1") != 3 {
		t.Fatal("cached launcher not disabled in student containers")
	}
}

func TestMakeSandboxReadable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attempt")
	t.Cleanup(func() { restoreSandboxOwnerAccess(root) })
	source := filepath.Join(root, "src")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	cmake := filepath.Join(root, "CMakeLists.txt")
	if err := os.WriteFile(cmake, []byte("cmake_minimum_required(VERSION 3.20)"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := makeSandboxReadable(root); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{root: 0o555, source: 0o555, cmake: 0o444} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
	restoreSandboxOwnerAccess(root)
	for path, want := range map[string]os.FileMode{root: 0o700, source: 0o700, cmake: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s restored mode = %o, want %o", path, got, want)
		}
	}
}

func TestParseSandboxResultsKeepsStagesSeparate(t *testing.T) {
	stage := func(name string, exitCode int, log string) string {
		return "__CPPDEFENSE_STAGE__ " + name + " " + strconv.Itoa(exitCode) + "\n" +
			base64.StdEncoding.EncodeToString([]byte(log)) + "\n__CPPDEFENSE_END__\n"
	}
	output := stage("configure", 0, "configured") +
		stage("build", 0, "built") +
		stage("ctest", 8, "tests failed")

	results := parseSandboxResults(output, 0, false)
	if !results.Configure.OK || results.Configure.Log != "configured" {
		t.Fatalf("configure = %#v", results.Configure)
	}
	if !results.Build.OK || results.Build.Log != "built" {
		t.Fatalf("build = %#v", results.Build)
	}
	if results.CTest.OK || results.CTest.ExitCode != 8 || results.CTest.Log != "tests failed" {
		t.Fatalf("ctest = %#v", results.CTest)
	}
	if results.OK() {
		t.Fatal("sandbox result must fail when CTest fails")
	}
}

func TestParseSandboxResultsMarksLaterStagesSkipped(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("configure failed"))
	results := parseSandboxResults("__CPPDEFENSE_STAGE__ configure 1\n"+encoded+"\n__CPPDEFENSE_END__\n", 0, false)
	if results.Configure.OK || results.Configure.Log != "configure failed" {
		t.Fatalf("configure = %#v", results.Configure)
	}
	if results.Build.ExitCode != -1 || !strings.Contains(results.Build.Log, "Конфигурация CMake") {
		t.Fatalf("build = %#v", results.Build)
	}
	if results.CTest.ExitCode != -1 || !strings.Contains(results.CTest.Log, "Сборка проекта") {
		t.Fatalf("ctest = %#v", results.CTest)
	}
}
