package runner

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
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
