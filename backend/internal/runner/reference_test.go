package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestReferenceComparisonIsExactExceptCRLF(t *testing.T) {
	project := t.TempDir()
	source := "int f() {\r\n return 42;\r\n}\r\n"
	if err := os.WriteFile(filepath.Join(project, "main.cpp"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(source))
	answer := "\n return 42;\n"
	lease := postgres.Lease{Answer: &answer, SelectedFunction: &postgres.SelectedFunction{FilePath: "main.cpp", BodyBegin: int64(strings.Index(source, "{")), BodyEnd: int64(strings.Index(source, "}") + 1), SourceSHA256: hex.EncodeToString(digest[:])}}
	if !restoreMatches(project, lease) {
		t.Fatal("CRLF normalization rejected")
	}
	for _, wrong := range []string{"return 42;", "\n return 43;\n", "\n return  42;\n", "\n return 42; /* comment */\n"} {
		lease.Answer = &wrong
		if restoreMatches(project, lease) {
			t.Fatalf("non-identical answer accepted: %q", wrong)
		}
	}
	lease.Answer = &answer
	lease.SelectedFunction.SourceSHA256 = strings.Repeat("0", 64)
	if restoreMatches(project, lease) {
		t.Fatal("stale selected source accepted")
	}
	if sameSources(map[string][]byte{"a.cpp": []byte("x"), "b.hpp": []byte("y")}, map[string][]byte{"a.cpp": []byte("x"), "b.hpp": []byte("z")}) {
		t.Fatal("unrelated header edit ignored")
	}
}

func TestVerifiedReferenceSkipsSandboxAndInvalidatesContext(t *testing.T) {
	if _, err := exec.LookPath("cmake"); err != nil {
		t.Skip("cmake required")
	}
	ctx := context.Background()
	directory := t.TempDir()
	session := "session"
	project := filepath.Join(directory, session, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	original := "int main(void) { int n = 3; return n > 2 ? 0 : 1; }\n"
	cmake := "cmake_minimum_required(VERSION 3.20)\nproject(ReferenceFixture C)\nenable_testing()\nadd_executable(check main.c)\nadd_test(NAME logic COMMAND check)\n"
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(project, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("main.c", original)
	write("other.hpp", "// header\n")
	write("CMakeLists.txt", cmake)
	runtime := filepath.Join(directory, "runtime")
	if err := os.WriteFile(runtime, []byte(fixtureRuntime), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(directory, "runs")
	t.Setenv("CPPDEFENSE_TEST_RUN_LOG", log)
	runs := func() int { t.Helper(); data, _ := os.ReadFile(log); return strings.Count(string(data), "run\n") }
	completed := make(chan postgres.Completion, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value postgres.Completion
		if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
			t.Error(err)
		}
		completed <- value
		w.Write([]byte("{}"))
	}))
	defer server.Close()
	a := agent{config: config{Runtime: runtime, Image: "fixture", Backend: server.URL, Worker: filepath.Join(directory, "missing-worker")}, client: server.Client()}
	digest := sha256.Sum256([]byte(original))
	body := original[strings.Index(original, "{")+1 : strings.Index(original, "}")]
	lease := postgres.Lease{JobID: "job", SessionID: session, SubmissionObjectKey: "normalized-submissions/version.zip", Answer: &body, SelectedFunction: &postgres.SelectedFunction{FilePath: "main.c", BodyBegin: int64(strings.Index(original, "{")), BodyEnd: int64(strings.Index(original, "}") + 1), SourceSHA256: hex.EncodeToString(digest[:])}}
	proof, result := a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 1 {
		t.Fatalf("original not validated: %#v %#v", proof, result)
	}
	// Real jobs extract the same immutable archive into different temp roots.
	relocated := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(relocated, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"main.c": original, "other.hpp": "// header\n", "CMakeLists.txt": cmake} {
		if err := os.WriteFile(filepath.Join(relocated, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if v, r := a.verifyOriginal(ctx, relocated, lease); v == nil || !r.OK() || runs() != 1 {
		t.Fatal("same archive in a new workspace lost proof")
	}
	if err := a.check(ctx, ctx, filepath.Join(directory, session, "attempt"), directory, lease); err != nil {
		t.Fatal(err)
	}
	value := <-completed
	if value.Outcome != "passed" || value.ReferenceResult == nil || value.ConfigureResult != nil || runs() != 1 {
		t.Fatalf("restore did not use reference: %#v, runs=%d", value, runs())
	}
	// Multiple changes in different files must all be repaired. No patch metadata leaks.
	lease.DefenseMode = "hide_seek"
	lease.HideMutations = []hideseek.Mutation{{Path: "main.c", Start: strings.Index(original, ">"), Before: ">", After: "<"}, {Path: "other.hpp", Start: 3, Before: "header", After: "changed"}}
	answer, _ := json.Marshal(map[string]string{"main.c": original, "other.hpp": "// header\n"})
	text := string(answer)
	lease.Answer = &text
	if err := a.checkHide(ctx, ctx, project, lease); err != nil {
		t.Fatal(err)
	}
	value = <-completed
	if value.ReferenceResult == nil || runs() != 1 {
		t.Fatalf("hide repair not fast: %#v", value)
	}
	// Missing one repair takes the normal sandbox path (even if that particular
	// change is harmless and the tests still pass).
	answer, _ = json.Marshal(map[string]string{"main.c": original})
	text = string(answer)
	lease.Answer = &text
	if err := a.checkHide(ctx, ctx, project, lease); err != nil {
		t.Fatal(err)
	}
	value = <-completed
	if value.ReferenceResult != nil || runs() != 2 {
		t.Fatalf("partial repair used reference: %#v", value)
	}
	write("other.hpp", "// header\n")
	// New image ID forces a fresh baseline; execution uses that immutable ID.
	t.Setenv("CPPDEFENSE_TEST_IMAGE_ID", strings.Repeat("b", 64))
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 3 {
		t.Fatal("image change did not invalidate")
	}
	// All files, including CMake and tests, are part of the proof identity.
	write("CMakeLists.txt", cmake+"# settings changed\n")
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 4 {
		t.Fatal("settings change did not invalidate")
	}
	write("test-data.txt", "new input")
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 5 {
		t.Fatal("test input change did not invalidate")
	}
	lease.SubmissionObjectKey = "normalized-submissions/another-version.zip"
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 6 {
		t.Fatal("new version did not invalidate")
	}
	a.referenceMu.Lock()
	for key, v := range a.references {
		v.VerifiedAt = time.Now().Add(-2 * referenceTTL)
		a.references[key] = v
	}
	a.referenceMu.Unlock()
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 7 {
		t.Fatal("expired proof used")
	}
	// A process restart loses proofs and must validate again.
	a.references = nil
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof == nil || !result.OK() || runs() != 8 {
		t.Fatal("restart skipped baseline")
	}
	// Failing originals must never acquire proof.
	write("main.c", "int main(void) { return 1; }\n")
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof != nil || result.OK() {
		t.Fatal("failing original certified")
	}
	// Concurrent readers are safe; all should hit the existing proof.
	write("main.c", original)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, r := a.verifyOriginal(ctx, project, lease)
			if v == nil || !r.OK() {
				t.Error("lost proof")
			}
		}()
	}
	wg.Wait()
	// An original with zero registered tests is not a verified reference.
	write("CMakeLists.txt", "cmake_minimum_required(VERSION 3.20)\nproject(NoTests C)\nenable_testing()\n")
	proof, result = a.verifyOriginal(ctx, project, lease)
	if proof != nil || result.OK() {
		t.Fatal("test-free original certified")
	}
	selected := 0
	preparationLease := lease
	preparationLease.SelectedIndex = &selected
	if err := a.prepare(ctx, ctx, directory, preparationLease); err != nil {
		t.Fatal(err)
	}
	preparation := <-completed
	if preparation.Outcome != "error" || preparation.CTestResult == nil {
		t.Fatalf("unverified original reached challenge materialization: %#v", preparation)
	}
	write("CMakeLists.txt", cmake)
	// A different, valid implementation must still be built and tested. The
	// materializer fixture is limited to this known, benign C test program.
	worker := filepath.Join(directory, "materializer")
	workerScript := `#!/bin/sh
set -eu
cp -R "$CPPDEFENSE_TEST_PROJECT" "$CPPDEFENSE_TEST_ATTEMPT"
printf '%s' "$CPPDEFENSE_TEST_ALTERNATE_SOURCE" > "$CPPDEFENSE_TEST_ATTEMPT/main.c"
printf '{"status":"ok"}\n'
`
	if err := os.WriteFile(worker, []byte(workerScript), 0700); err != nil {
		t.Fatal(err)
	}
	a.config.Worker = worker
	attemptRoot := filepath.Join(directory, session, "alternate-attempt")
	t.Setenv("CPPDEFENSE_TEST_PROJECT", project)
	t.Setenv("CPPDEFENSE_TEST_ATTEMPT", attemptRoot)
	t.Setenv("CPPDEFENSE_TEST_ALTERNATE_SOURCE", "int main(void) { return 0; }\n")
	alternate := " return 0; "
	lease.Answer = &alternate
	before := runs()
	if err := a.check(ctx, ctx, attemptRoot, directory, lease); err != nil {
		t.Fatal(err)
	}
	value = <-completed
	if value.ReferenceResult != nil || value.Outcome != "passed" || value.CTestResult == nil || runs() != before+1 {
		t.Fatalf("alternative answer skipped sandbox: %#v", value)
	}
	// Failed tests for a different answer still fail, rather than receiving
	// credit because some other attempt had matched the reference.
	t.Setenv("CPPDEFENSE_TEST_ALTERNATE_SOURCE", "int main(void) { return 1; }\n")
	wrongRoot := filepath.Join(directory, session, "wrong-attempt")
	t.Setenv("CPPDEFENSE_TEST_ATTEMPT", wrongRoot)
	if err := a.check(ctx, ctx, wrongRoot, directory, lease); err != nil {
		t.Fatal(err)
	}
	value = <-completed
	if value.ReferenceResult != nil || value.Outcome != "failed" {
		t.Fatalf("wrong answer accepted: %#v", value)
	}
	if v, r := a.verifyOriginal(ctx, project, lease); v == nil || !r.OK() {
		t.Fatal("baseline missing")
	}
	before = runs()
	a.config.Timeout = time.Minute
	if v, r := a.verifyOriginal(ctx, project, lease); v == nil || !r.OK() || runs() != before+1 {
		t.Fatal("time-limit policy change did not invalidate proof")
	}
	// Failure to resolve an immutable image disables the shortcut, even when
	// an older proof exists and the current original itself still passes.
	t.Setenv("CPPDEFENSE_TEST_IMAGE_ID", "invalid-image")
	if v, r := a.verifyOriginal(ctx, project, lease); v != nil || !r.OK() {
		t.Fatal("unresolved image created a proof")
	}
}
