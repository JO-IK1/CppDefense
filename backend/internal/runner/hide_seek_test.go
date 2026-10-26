package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

// The test runtime executes only this test's C fixture on the host. Production
// continues using the constrained Podman sandbox. This tests real CMake/CTest
// behavior without requiring Docker on developer computers.
const fixtureRuntime = `#!/bin/sh
set -eu
project=""
for arg do
 case "$arg" in *:/workspace:ro) project="${arg%:/workspace:ro}" ;; esac
done
test -n "$project"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
emit() {
 printf '__CPPDEFENSE_STAGE__ %s %s\n' "$1" "$2"
 base64 < "$work/log" | tr -d '\n'
 printf '\n__CPPDEFENSE_END__\n'
}
code=0
cmake -S "$project" -B "$work/build" > "$work/log" 2>&1 || code=$?
emit configure "$code"
test "$code" -eq 0 || exit 0
code=0
cmake --build "$work/build" > "$work/log" 2>&1 || code=$?
emit build "$code"
test "$code" -eq 0 || exit 0
code=0
ctest --test-dir "$work/build" --output-on-failure > "$work/log" 2>&1 || code=$?
emit ctest "$code"
`

func TestHideSeekValidatesLogicalMutationAndChecksStudentProject(t *testing.T) {
	if _, err := exec.LookPath("cmake"); err != nil {
		t.Skip("cmake is required")
	}
	directory := t.TempDir()
	project := filepath.Join(directory, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	cmake := "cmake_minimum_required(VERSION 3.20)\nproject(HideFixture C)\nenable_testing()\nadd_executable(check main.c)\nadd_test(NAME logic COMMAND check)\n"
	original := "int main(void) { int n = 3; return n > 2 ? 0 : 1; }\n"
	for name, data := range map[string]string{"CMakeLists.txt": cmake, "main.c": original} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime := filepath.Join(directory, "fixture-runtime")
	if err := os.WriteFile(runtime, []byte(fixtureRuntime), 0o700); err != nil {
		t.Fatal(err)
	}
	completions := make(chan postgres.Completion, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var completed postgres.Completion
		if err := json.NewDecoder(r.Body).Decode(&completed); err != nil {
			t.Error(err)
		}
		completions <- completed
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	a := agent{config: config{Backend: server.URL, Runtime: runtime, Image: "fixture"}, client: server.Client()}
	mutation := hideseek.Mutation{Path: "main.c", Start: strings.Index(original, ">"), Before: ">", After: "<"}
	lease := postgres.Lease{JobID: "job", SessionID: "session", HideMutations: []hideseek.Mutation{mutation}}
	if err := a.validateHide(context.Background(), context.Background(), project, lease); err != nil {
		t.Fatal(err)
	}
	completed := <-completions
	if completed.HideValidation == nil || !completed.HideValidation.Valid {
		t.Fatalf("logical mutation rejected: %#v", completed.HideValidation)
	}
	answer, _ := json.Marshal(map[string]string{"main.c": original})
	text := string(answer)
	lease.Answer = &text
	if err := a.checkHide(context.Background(), context.Background(), project, lease); err != nil {
		t.Fatal(err)
	}
	completed = <-completions
	if completed.Outcome != "passed" {
		t.Fatalf("correct repair: %#v", completed)
	}
	empty := "{}"
	lease.Answer = &empty
	if err := a.checkHide(context.Background(), context.Background(), project, lease); err != nil {
		t.Fatal(err)
	}
	completed = <-completions
	if completed.Outcome != "failed" {
		t.Fatalf("unrepaired project: %#v", completed)
	}
	// Configure-time validation must reject a change that does not compile.
	if err := os.WriteFile(filepath.Join(project, "main.c"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	lease.HideMutations[0].After = "???"
	if err := a.validateHide(context.Background(), context.Background(), project, lease); err != nil {
		t.Fatal(err)
	}
	completed = <-completions
	if completed.HideValidation == nil || completed.HideValidation.Valid || !strings.Contains(completed.HideValidation.Message, "сборку") {
		t.Fatalf("compile error not rejected: %#v", completed)
	}
	// A missing runtime is infrastructure failure, never a wrong student answer.
	lease.HideMutations[0].After = "<"
	if err := os.WriteFile(filepath.Join(project, "main.c"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	a.config.Runtime = filepath.Join(directory, "missing-runtime")
	if err := a.checkHide(context.Background(), context.Background(), project, lease); err != nil {
		t.Fatal(err)
	}
	completed = <-completions
	if completed.Outcome != "error" {
		t.Fatalf("infrastructure failure counted as wrong answer: %#v", completed)
	}
}
