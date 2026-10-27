package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestRealSandboxIncrementalSnapshotIsolation(t *testing.T) {
	image := os.Getenv("CPPDEFENSE_TEST_SANDBOX_IMAGE")
	if image == "" {
		t.Skip("CPPDEFENSE_TEST_SANDBOX_IMAGE not set")
	}
	runtime := env("CPPDEFENSE_TEST_CONTAINER_RUNTIME", "docker")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	t.Cleanup(func() { removeSnapshotTree(root) })
	original := filepath.Join(root, "original")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"CMakeLists.txt": `cmake_minimum_required(VERSION 3.20)
project(SnapshotFixture LANGUAGES CXX)
include(FetchContent)
FetchContent_Declare(googletest URL https://invalid.invalid/googletest.zip)
FetchContent_MakeAvailable(googletest)
enable_testing()
add_library(calculator calculator.cpp untouched.cpp)
target_include_directories(calculator PUBLIC "${CMAKE_CURRENT_SOURCE_DIR}")
add_executable(calculator_tests tests.cpp)
target_link_libraries(calculator_tests PRIVATE calculator GTest::gtest_main)
add_test(NAME calculator_tests COMMAND calculator_tests)
`,
		"calculator.hpp": "#pragma once\nint add(int,int);\n",
		"calculator.cpp": "#include \"calculator.hpp\"\nint add(int a,int b){return a+b;}\n",
		"untouched.cpp":  "#include \"calculator.hpp\"\nint untouched(){return add(1,2);}\n",
		"tests.cpp": `#include "calculator.hpp"
#include <gtest/gtest.h>
#include <fstream>
TEST(Calculator,Add){EXPECT_EQ(add(2,3),5);}
TEST(Snapshot,FreshState){
 std::ifstream marker("student-marker");EXPECT_FALSE(marker.good());
 std::ofstream("student-marker")<<"must not persist";
 std::ofstream("build.ninja")<<"must not poison the next attempt";
 std::ofstream seed("/seed/poison");EXPECT_FALSE(seed.good());
 std::ofstream cache("/ccache/test-state");EXPECT_FALSE(cache.good());
}
`,
	}
	writeProject := func(name string, changes map[string]string) string {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, data := range files {
			if v, ok := changes[name]; ok {
				data = v
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	// original was created above; the helper creates isolated attempt sources.
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(original, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := agent{config: config{Runtime: runtime, Image: image, Timeout: 3 * time.Minute, BuildCacheDir: filepath.Join(root, "snapshots"), BuildCacheTTL: time.Hour, BuildCacheBytes: 2 << 30}}
	lease := postgres.Lease{SessionID: "session", SubmissionObjectKey: "normalized-submissions/version.zip"}
	b, err := a.newBuildContext(ctx, original, lease)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, baseline, err := a.ensureSnapshot(ctx, original, lease, b, true)
	if err != nil {
		t.Fatalf("baseline export: %v\n%#v", err, baseline)
	}
	if snapshot == nil || !baseline.OK() {
		t.Fatalf("baseline failed: %#v", baseline)
	}
	if baseline.CompileMS == nil || baseline.LinkMS == nil || baseline.CompileCommands < 3 {
		t.Fatalf("command measurements missing: %#v", baseline)
	}
	if strings.Contains(baseline.Build.Log, "gtest-all.cc") {
		t.Fatal("GoogleTest rebuilt instead of prebuilt")
	}
	if _, err := os.Stat(filepath.Join(snapshot.Dir, "student-marker")); !os.IsNotExist(err) {
		t.Fatal("test state entered snapshot")
	}
	if _, err := loadSnapshot(a.config.BuildCacheDir, b, time.Hour); err != nil {
		t.Fatalf("published snapshot invalid: %v", err)
	}
	run := func(name, body string) sandboxResults {
		t.Helper()
		project := writeProject(name, map[string]string{"calculator.cpp": body})
		if err := makeSandboxReadable(project); err != nil {
			t.Fatal(err)
		}
		defer restoreSandboxOwnerAccess(project)
		return a.runWithSnapshot(ctx, project, lease, b)
	}
	wrong := run("wrong", "#include \"calculator.hpp\"\nint add(int a,int b){return a-b;}\n")
	if !wrong.Configure.Reused || !wrong.Build.OK || wrong.CTest.OK || wrong.CompileCommands != 1 {
		t.Fatalf("incorrect incremental check: %#v", wrong)
	}
	correct := run("correct", "#include \"calculator.hpp\"\nint add(int a,int b){return b+a;}\n")
	if !correct.OK() || !correct.Configure.Reused || correct.CompileCommands != 1 {
		t.Fatalf("correct answer/state isolation failed: %#v", correct)
	}
	t.Logf("baseline build %.1f ms, warm build %.1f ms; baseline compiler commands %d, warm compiler commands %d", *baseline.Build.DurationMS, *correct.Build.DurationMS, baseline.CompileCommands, correct.CompileCommands)
	again := run("again", "#include \"calculator.hpp\"\nint add(int a,int b){return b+a;}\n")
	if !again.OK() || again.CompileCommands != 1 {
		t.Fatalf("attempt contaminated snapshot: %#v", again)
	}
	// Header changes must rebuild their dependents, not reuse stale objects.
	changedHeader := writeProject("header", map[string]string{"calculator.hpp": "#pragma once\n#define add subtract\nint add(int,int);\n"})
	if err := makeSandboxReadable(changedHeader); err != nil {
		t.Fatal(err)
	}
	header := a.runWithSnapshot(ctx, changedHeader, lease, b)
	restoreSandboxOwnerAccess(changedHeader)
	if header.CompileCommands < 3 {
		t.Fatalf("header dependency not rebuilt: %#v", header)
	}
	// A new agent (process restart) can validate and reuse the persisted snapshot.
	restarted := agent{config: a.config}
	proof, result := restarted.verifyOriginal(ctx, original, lease)
	if proof == nil || !result.OK() || result.CompileCommands != 0 {
		t.Fatalf("persisted snapshot lost: %#v %#v", proof, result)
	}
	completed := make(chan postgres.Completion, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value postgres.Completion
		if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
			t.Error(err)
		}
		completed <- value
		w.Write([]byte("{}"))
	}))
	defer server.Close()
	a.config.Backend = server.URL
	a.client = server.Client()
	worker := filepath.Join(root, "materializer")
	script := `#!/bin/sh
set -eu
cp -R "$CPPDEFENSE_TEST_PROJECT" "$CPPDEFENSE_TEST_ATTEMPT"
printf '%s' "$CPPDEFENSE_TEST_ALTERNATE_SOURCE" > "$CPPDEFENSE_TEST_ATTEMPT/calculator.cpp"
printf '{"status":"ok"}\n'
`
	if err := os.WriteFile(worker, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a.config.Worker = worker
	workspace := filepath.Join(root, "worker")
	source := writeProject("worker/session/project", nil)
	attempt := filepath.Join(workspace, "session", "attempt")
	t.Setenv("CPPDEFENSE_TEST_PROJECT", source)
	t.Setenv("CPPDEFENSE_TEST_ATTEMPT", attempt)
	wrongSource := "#include \"calculator.hpp\"\nint add(int a,int b){return a-b;}\n"
	t.Setenv("CPPDEFENSE_TEST_ALTERNATE_SOURCE", wrongSource)
	answer := "return a-b;"
	lease.Answer = &answer
	digest := sha256.Sum256([]byte(files["calculator.cpp"]))
	lease.SelectedFunction = &postgres.SelectedFunction{FilePath: "calculator.cpp", BodyBegin: int64(strings.Index(files["calculator.cpp"], "{")), BodyEnd: int64(strings.Index(files["calculator.cpp"], "}") + 1), SourceSHA256: hex.EncodeToString(digest[:])}
	jobCtx := context.WithValue(ctx, timingKey{}, &jobProfile{started: time.Now(), value: postgres.JobTimings{QueueMS: 17}})
	if err := a.check(jobCtx, jobCtx, attempt, workspace, lease); err != nil {
		t.Fatal(err)
	}
	checked := <-completed
	if checked.Outcome != "failed" || checked.ReferenceResult != nil || checked.Timings == nil || !checked.Timings.SnapshotHit || checked.Timings.CompileCommands != 1 || checked.Timings.QueueMS != 17 {
		t.Fatalf("restore pipeline: %#v", checked)
	}
	source = writeProject("hide/project", nil)
	lease.DefenseMode = "hide_seek"
	lease.HideMutations = []hideseek.Mutation{{Path: "calculator.cpp", Start: strings.Index(files["calculator.cpp"], "+"), Before: "+", After: "-"}}
	edit, _ := json.Marshal(map[string]string{"calculator.cpp": "#include \"calculator.hpp\"\nint add(int a,int b){return b+a;}\n"})
	answer = string(edit)
	lease.Answer = &answer
	jobCtx = context.WithValue(ctx, timingKey{}, &jobProfile{started: time.Now()})
	if err := a.checkHide(jobCtx, jobCtx, source, lease); err != nil {
		t.Fatal(err)
	}
	checked = <-completed
	if checked.Outcome != "passed" || checked.ReferenceResult != nil || checked.Timings == nil || !checked.Timings.SnapshotHit || checked.Timings.CompileCommands != 1 {
		t.Fatalf("hide pipeline: %#v", checked)
	}
}
