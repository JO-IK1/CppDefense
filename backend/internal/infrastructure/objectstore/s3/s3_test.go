package s3

import (
	"context"
	"errors"
	"testing"
	"time"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
)

func TestUnavailableEndpointIsRetryable(t *testing.T) {
	storage, err := New(Config{Endpoint: "127.0.0.1:1", Region: "us-east-1", Bucket: "cppdefense", AccessKey: "test", SecretKey: "test", Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err = storage.Check(ctx)
	if !errors.Is(err, appstorage.ErrUnavailable) {
		t.Fatalf("error is not retryable: %v", err)
	}
}

func TestTemporaryURLExpiryIsBounded(t *testing.T) {
	storage, err := New(Config{Endpoint: "localhost:9000", Region: "us-east-1", Bucket: "cppdefense", AccessKey: "test", SecretKey: "test", Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := appstorage.NewKey(appstorage.NormalizedSubmissions, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".zip")
	if _, err := storage.TemporaryGetURL(context.Background(), key, 16*time.Minute); err == nil {
		t.Fatal("overlong URL accepted")
	}
	url, err := storage.TemporaryGetURL(context.Background(), key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if url == "" {
		t.Fatal("empty temporary URL")
	}
}
