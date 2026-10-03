package runner

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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
