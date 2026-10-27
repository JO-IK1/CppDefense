package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func snapshotTar(t *testing.T, entries []*tar.Header) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, header := range entries {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg && header.Size < 1024 {
			writer.Write(bytes.Repeat([]byte("x"), int(header.Size)))
		}
	}
	writer.Close()
	return buffer.Bytes()
}
func TestSnapshotArchiveRejectsEscapesLinksAndQuota(t *testing.T) {
	for name, headers := range map[string][]*tar.Header{
		"parent":        {{Name: "../escape", Typeflag: tar.TypeReg}},
		"absolute":      {{Name: "/escape", Typeflag: tar.TypeReg}},
		"symlink":       {{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}},
		"absolute-link": {{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}},
		"hardlink":      {{Name: "link", Typeflag: tar.TypeLink, Linkname: "target"}},
		"device":        {{Name: "device", Typeflag: tar.TypeChar}},
		"duplicate":     {{Name: "same", Typeflag: tar.TypeReg}, {Name: "same", Typeflag: tar.TypeReg}},
		"link-parent":   {{Name: "folder", Typeflag: tar.TypeSymlink, Linkname: "target"}, {Name: "folder/child", Typeflag: tar.TypeReg}},
		"quota":         {{Name: "huge", Typeflag: tar.TypeReg, Size: maxSnapshotBytes + 1}},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if _, _, err := unpackSnapshot(bytes.NewReader(snapshotTar(t, headers)), root); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestSnapshotIntegrityAndVersionBinding(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { removeSnapshotTree(root) })
	key := strings.Repeat("a", 64)
	dir := filepath.Join(root, key, "build")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	archive := snapshotTar(t, []*tar.Header{
		{Name: "CMakeCache.txt", Typeflag: tar.TypeReg, Size: 1, Mode: 0600, ModTime: time.Unix(123, 0)},
		{Name: "build.ninja", Typeflag: tar.TypeReg, Size: 1, Mode: 0600, ModTime: time.Unix(123, 0)},
	})
	files, size, err := unpackSnapshot(bytes.NewReader(archive), dir)
	if err != nil {
		t.Fatal(err)
	}
	b := &buildContext{Key: key, Image: "sha256:" + strings.Repeat("b", 64), Policy: strings.Repeat("c", 64)}
	b.Proof.ProjectSHA256 = strings.Repeat("d", 64)
	b.Proof.ImageID = b.Image
	b.Proof.PolicySHA256 = b.Policy
	b.Proof.VerifiedAt = time.Now().UTC()
	value := buildSnapshot{Schema: snapshotSchema, Key: key, Proof: b.Proof, Files: files, Bytes: size}
	raw, _ := json.Marshal(value)
	if err := os.WriteFile(filepath.Join(root, key, "metadata.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := sealSnapshot(dir, files); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSnapshot(root, b, time.Hour); err != nil {
		t.Fatal(err)
	}
	different := *b
	different.Image = "sha256:" + strings.Repeat("e", 64)
	if _, err := loadSnapshot(root, &different, time.Hour); err == nil {
		t.Fatal("different image reused snapshot")
	}
	different = *b
	different.Policy = strings.Repeat("e", 64)
	if _, err := loadSnapshot(root, &different, time.Hour); err == nil {
		t.Fatal("different settings reused snapshot")
	}
	different = *b
	different.Key = strings.Repeat("f", 64)
	if _, err := loadSnapshot(root, &different, time.Hour); err == nil {
		t.Fatal("different version/session reused snapshot")
	}
	target := filepath.Join(dir, "build.ninja")
	os.Chmod(target, 0600)
	os.WriteFile(target, []byte("z"), 0600)
	os.Chtimes(target, time.Unix(123, 0), time.Unix(123, 0))
	os.Chmod(target, 0444)
	if _, err := loadSnapshot(root, b, time.Hour); err == nil {
		t.Fatal("corrupted content reused snapshot")
	}
}
func TestSnapshotLockCancellationAndPruning(t *testing.T) {
	root := t.TempDir()
	key := strings.Repeat("a", 64)
	unlock, err := snapshotLock(context.Background(), root, key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if release, err := snapshotLock(ctx, root, key); err == nil {
		release()
		t.Fatal("concurrent lock acquired")
	}
	unlock()
	for _, entry := range []string{key, strings.Repeat("b", 64), "unrelated"} {
		dir := filepath.Join(root, entry)
		os.Mkdir(dir, 0700)
		raw, _ := json.Marshal(buildSnapshot{Bytes: 100, Proof: postgres.ReferenceResult{VerifiedAt: time.Now().Add(-48 * time.Hour)}})
		os.WriteFile(filepath.Join(dir, "metadata.json"), raw, 0600)
	}
	a := agent{config: config{BuildCacheBytes: 100, BuildCacheTTL: time.Hour}}
	a.pruneSnapshots(root, key)
	if _, err := os.Stat(filepath.Join(root, key)); err != nil {
		t.Fatal("active snapshot removed")
	}
	if _, err := os.Stat(filepath.Join(root, strings.Repeat("b", 64))); !os.IsNotExist(err) {
		t.Fatal("expired snapshot retained")
	}
	if _, err := os.Stat(filepath.Join(root, "unrelated")); err != nil {
		t.Fatal("unrelated path removed")
	}
}
