package runner

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

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
printf '%s\n' '{"status":"ok","result":{"selected_index":0,"masked_source":"int main() { /* TODO */ }","candidates":[{"function_name":"main","file_path":"main.cpp","signature":"int main()","signature_begin":0,"body_begin":10,"body_end":12,"begin_line":1,"end_line":1,"line_count":1,"source_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","original_body_sha256":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}]}}'
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
