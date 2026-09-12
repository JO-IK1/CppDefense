package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	s3storage "github.com/JO-IK1/CppDefense/backend/internal/infrastructure/objectstore/s3"
)

func objectStorage(t *testing.T) *s3storage.Storage {
	t.Helper()
	endpoint := os.Getenv("CPPDEFENSE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("CPPDEFENSE_TEST_S3_ENDPOINT is not set")
	}
	storage, err := s3storage.New(s3storage.Config{
		Endpoint: endpoint, Region: "us-east-1", Bucket: "cppdefense-test",
		AccessKey: os.Getenv("CPPDEFENSE_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("CPPDEFENSE_TEST_S3_SECRET_KEY"),
		Secure: false, SpoolDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := storage.EnsurePrivateBucket(ctx); err != nil {
		t.Fatal(err)
	}
	return storage
}

func TestS3RoundTripAndTemporaryURL(t *testing.T) {
	storage := objectStorage(t)
	key, _ := appstorage.NewKey(appstorage.NormalizedSubmissions, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".zip")
	_ = storage.Delete(context.Background(), key)
	contents := []byte("stage-three")
	stored, err := storage.Put(context.Background(), key, bytes.NewReader(contents), appstorage.PutOptions{Size: int64(len(contents)), ContentType: "application/zip"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Delete(context.Background(), key) })
	reader, opened, err := storage.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(read, contents) || opened.SHA256 != stored.SHA256 {
		t.Fatal("S3 round trip mismatch")
	}
	url, err := storage.TemporaryGetURL(context.Background(), key, time.Minute)
	if err != nil || url == "" {
		t.Fatalf("temporary URL: %q, %v", url, err)
	}
	if _, err := storage.Stat(context.Background(), key); err != nil {
		t.Fatalf("presigning changed object: %v", err)
	}
	if _, err := storage.Put(context.Background(), key, bytes.NewReader(contents), appstorage.PutOptions{Size: int64(len(contents))}); !errors.Is(err, appstorage.ErrExists) {
		t.Fatalf("overwrite was not rejected: %v", err)
	}
}
