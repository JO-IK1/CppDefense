package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestMultiRestoreReferenceRequiresEveryOriginal(t *testing.T) {
	project := t.TempDir()
	source := "struct Box {int x;};\nint F(){return 1;}\n"
	if err := os.WriteFile(filepath.Join(project, "main.cpp"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(source))
	end := strings.Index(source, "}") + 1
	begin := strings.Index(source, "int F")
	targets := []postgres.RestoreTarget{
		{ID: "box", SelectedFunction: postgres.SelectedFunction{FilePath: "main.cpp", EntityType: "struct", SignatureBegin: 0, BodyEnd: int64(end), SourceSHA256: hex.EncodeToString(digest[:])}},
		{ID: "f", SelectedFunction: postgres.SelectedFunction{FilePath: "main.cpp", EntityType: "function", SignatureBegin: int64(begin), BodyEnd: int64(len(source) - 1), SourceSHA256: hex.EncodeToString(digest[:])}},
	}
	answers := map[string]string{"box": "struct Box {int x;};", "f": "int F(){return 1;}"}
	raw, _ := json.Marshal(answers)
	answer := string(raw)
	lease := postgres.Lease{RestoreTargets: targets, Answer: &answer}
	if !restoreMatches(project, lease) {
		t.Fatal("all original definitions should match")
	}
	answers["f"] = "int F(){return 2;}"
	raw, _ = json.Marshal(answers)
	answer = string(raw)
	if restoreMatches(project, lease) {
		t.Fatal("one matching target accepted the entire answer")
	}
}

func TestRestorePayloadUsesOpaqueIDsAndOneMaterialization(t *testing.T) {
	targets := []postgres.RestoreTarget{{ID: "one", Label: "func_1", SelectedFunction: postgres.SelectedFunction{FunctionName: "f"}}, {ID: "two", Label: "func_2", SelectedFunction: postgres.SelectedFunction{FunctionName: "Box", EntityType: "struct"}}}
	answer := `{"one":"int f(){return 1;}","two":"struct Box {int x;};"}`
	lease := postgres.Lease{RestoreTargets: targets, Answer: &answer}
	payload, err := restorePayload(lease)
	if err != nil {
		t.Fatal(err)
	}
	if payload["full_definition"] != true || len(payload["additional_replacements"].([]map[string]any)) != 1 {
		t.Fatal(payload)
	}
	data, _ := json.Marshal(payload)
	if string(data) == "" {
		t.Fatal("not serializable")
	}
	for _, bad := range []string{`{"one":"x"}`, `{"one":"x","two":"y","extra":"z"}`, `{"one":"x","one":"z","two":"y"}`, `{"one":"x","two":2}`, `{"one":"x","two":"y"} {}`} {
		lease.Answer = &bad
		if _, err := restorePayload(lease); err == nil {
			t.Fatal("invalid answers accepted", bad)
		}
	}
}
