package runner

import (
	"os"
	"path/filepath"
	"testing"
)

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
