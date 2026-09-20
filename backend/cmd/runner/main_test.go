package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func testZIP(t *testing.T, name, content string) *readCloser {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &readCloser{Reader: bytes.NewReader(data.Bytes())}
}

func TestLoadRejectsTooManySlots(t *testing.T) {
	t.Setenv("CPPDEFENSE_BACKEND_URL", "http://backend")
	t.Setenv("CPPDEFENSE_RUNNER_TOKEN", "01234567890123456789012345678901")
	t.Setenv("CPPDEFENSE_RUNNER_ID", "0199a123-4567-7abc-8def-0123456789ab")
	t.Setenv("CPPDEFENSE_RUNNER_SLOTS", "7")
	if _, err := load(); err == nil {
		t.Fatal("invalid slot count accepted")
	}
}

func TestHeartbeatLoopStops(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	a := &agent{config: config{Backend: "http://backend", Token: "token"}, client: client}
	done := make(chan struct{})
	close(done)
	finished := make(chan struct{})
	go func() {
		a.heartbeatLoop(context.Background(), done, postgres.Lease{JobID: "job", LeaseToken: "lease"}, func() {})
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop")
	}
}

func TestExecutePrepareUsesSessionWorkspace(t *testing.T) {
	archive := testZIP(t, "CMakeLists.txt", "cmake_minimum_required(VERSION 3.20)")
	archiveBytes, _ := io.ReadAll(archive)
	var completion bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/complete") {
			_, _ = io.Copy(&completion, request.Body)
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"status":"prepared"}`))
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	worker := filepath.Join(t.TempDir(), "worker")
	script := `#!/bin/sh
test -d 0199a123-4568-7abc-8def-0123456789ab/project || exit 2
printf '%s\n' '{"status":"ok","result":{"selected_index":0,"candidates":[{"function_name":"main","file_path":"main.cpp","signature":"int main()","signature_begin":0,"body_begin":10,"body_end":12,"begin_line":1,"end_line":1,"line_count":1,"source_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","original_body_sha256":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}]}}'
`
	if err := os.WriteFile(worker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &agent{config: config{Backend: server.URL, Token: "01234567890123456789012345678901", Worker: worker, Timeout: time.Minute}, client: server.Client(), storage: fakeStorage{data: archiveBytes}}
	lease := postgres.Lease{JobID: "0199a123-4567-7abc-8def-0123456789ab", Kind: "prepare_defense", LeaseToken: "lease", SessionID: "0199a123-4568-7abc-8def-0123456789ab", SubmissionObjectKey: "normalized-submissions/0199a123-4569-7abc-8def-0123456789ab.zip", Seed: "1", TopN: 5}
	if err := a.execute(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(completion.Bytes(), []byte(`"signature":"int main()"`)) {
		t.Fatalf("completion=%s", completion.Bytes())
	}
}

type fakeStorage struct{ data []byte }

func (f fakeStorage) Open(context.Context, appstorage.Key) (io.ReadCloser, appstorage.Object, error) {
	return io.NopCloser(bytes.NewReader(f.data)), appstorage.Object{}, nil
}
func (fakeStorage) Put(context.Context, appstorage.Key, io.Reader, appstorage.PutOptions) (appstorage.Object, error) {
	return appstorage.Object{}, errors.New("not implemented")
}
func (fakeStorage) Stat(context.Context, appstorage.Key) (appstorage.Object, error) {
	return appstorage.Object{}, errors.New("not implemented")
}
func (fakeStorage) List(context.Context, appstorage.Namespace) ([]appstorage.Object, error) {
	return nil, errors.New("not implemented")
}
func (fakeStorage) Delete(context.Context, appstorage.Key) error {
	return errors.New("not implemented")
}
func (fakeStorage) Check(context.Context) error { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type readCloser struct{ *bytes.Reader }

func (*readCloser) Close() error { return nil }

func TestExtractNormalizedArchive(t *testing.T) {
	target := t.TempDir()
	if err := extract(testZIP(t, "src/main.cpp", "int main(){}"), target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "src", "main.cpp"))
	if err != nil || string(data) != "int main(){}" {
		t.Fatalf("file=%q err=%v", data, err)
	}
}
func TestExtractRejectsTraversal(t *testing.T) {
	if err := extract(testZIP(t, "../escape.cpp", "bad"), t.TempDir()); err == nil {
		t.Fatal("archive traversal accepted")
	}
}
func TestLimitedBufferCapsOutput(t *testing.T) {
	var buffer limitedBuffer
	payload := bytes.Repeat([]byte("x"), (1<<20)+100)
	written, err := buffer.Write(payload)
	if err != nil || written != len(payload) || buffer.Len() != 1<<20 {
		t.Fatalf("written=%d len=%d err=%v", written, buffer.Len(), err)
	}
}
