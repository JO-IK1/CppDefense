package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
)

func TestPutOpenListAndPersistence(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := appstorage.NewKey(appstorage.NormalizedSubmissions, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".zip")
	contents := []byte("normalized archive")
	object, err := first.Put(context.Background(), key, bytes.NewReader(contents), appstorage.PutOptions{Size: int64(len(contents)), ContentType: "application/zip"})
	if err != nil {
		t.Fatal(err)
	}
	if object.SHA256 != sha256.Sum256(contents) {
		t.Fatal("wrong digest")
	}
	if _, err := first.Put(context.Background(), key, bytes.NewReader(contents), appstorage.PutOptions{Size: int64(len(contents))}); !errors.Is(err, appstorage.ErrExists) {
		t.Fatalf("duplicate error = %v", err)
	}

	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	reader, persisted, err := second.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	read, _ := io.ReadAll(reader)
	if !bytes.Equal(read, contents) || persisted.SHA256 != object.SHA256 {
		t.Fatal("object did not survive storage restart")
	}
	objects, err := second.List(context.Background(), appstorage.NormalizedSubmissions)
	if err != nil || len(objects) != 1 {
		t.Fatalf("list = %v, %v", objects, err)
	}
}

func TestSizeMismatchIsNotPublished(t *testing.T) {
	storage, _ := New(t.TempDir())
	key, _ := appstorage.NewKey(appstorage.OriginalArchives, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".zip")
	if _, err := storage.Put(context.Background(), key, bytes.NewReader([]byte("too long")), appstorage.PutOptions{Size: 3}); err == nil {
		t.Fatal("size mismatch accepted")
	}
	if _, err := storage.Stat(context.Background(), key); !errors.Is(err, appstorage.ErrNotFound) {
		t.Fatalf("object was published: %v", err)
	}
}
