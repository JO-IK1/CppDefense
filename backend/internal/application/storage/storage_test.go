package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
)

type memoryStorage struct{ objects map[string][]byte }

func newMemoryStorage() *memoryStorage { return &memoryStorage{objects: map[string][]byte{}} }
func (storage *memoryStorage) Put(_ context.Context, key Key, reader io.Reader, options PutOptions) (Object, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Object{}, err
	}
	if int64(len(contents)) != options.Size {
		return Object{}, errors.New("size mismatch")
	}
	storage.objects[key.String()] = contents
	return Object{Key: key, Size: int64(len(contents)), SHA256: sha256.Sum256(contents)}, nil
}
func (storage *memoryStorage) Open(_ context.Context, key Key) (io.ReadCloser, Object, error) {
	contents, ok := storage.objects[key.String()]
	if !ok {
		return nil, Object{}, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(contents)), Object{Key: key, Size: int64(len(contents)), SHA256: sha256.Sum256(contents)}, nil
}
func (storage *memoryStorage) Stat(_ context.Context, key Key) (Object, error) {
	contents, ok := storage.objects[key.String()]
	if !ok {
		return Object{}, ErrNotFound
	}
	return Object{Key: key, Size: int64(len(contents)), SHA256: sha256.Sum256(contents)}, nil
}
func (storage *memoryStorage) List(_ context.Context, namespace Namespace) ([]Object, error) {
	result := []Object{}
	for value, contents := range storage.objects {
		key, _ := ParseKey(value)
		if strings.HasPrefix(value, string(namespace)+"/") {
			result = append(result, Object{Key: key, Size: int64(len(contents)), SHA256: sha256.Sum256(contents)})
		}
	}
	return result, nil
}
func (storage *memoryStorage) Delete(_ context.Context, key Key) error {
	delete(storage.objects, key.String())
	return nil
}
func (storage *memoryStorage) Check(context.Context) error { return nil }

type versionRepository struct {
	duplicate bool
	result    SubmissionVersion
}

func (repository *versionRepository) CreateOrGetByDigest(_ context.Context, command CreateVersion) (SubmissionVersion, bool, error) {
	if repository.duplicate {
		return repository.result, true, nil
	}
	return SubmissionVersion{ID: command.ID, StudentRecordID: command.StudentRecordID, LabID: command.LabID, VersionNumber: 1}, false, nil
}

func TestSubmissionVersionStoresOpaqueKey(t *testing.T) {
	files := newMemoryStorage()
	service := NewSubmissionService(files, &versionRepository{})
	version, duplicate, err := service.StoreVersion(context.Background(), StoreVersionRequest{
		StudentRecordID: "student", LabID: "lab", ImportID: "import", CreatedBy: "teacher",
		Size: 3, SourceManifestJSON: []byte(`{"schema_version":1}`), Archive: bytes.NewReader([]byte("zip")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("new content marked duplicate")
	}
	if len(files.objects) != 1 {
		t.Fatalf("object count = %d", len(files.objects))
	}
	if version.Object.Key.String() == "" {
		t.Fatal("version has no object key")
	}
	if _, err := ParseKey(version.Object.Key.String()); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateVersionRemovesRedundantObject(t *testing.T) {
	files := newMemoryStorage()
	existingKey, _ := NewKey(NormalizedSubmissions, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".zip")
	files.objects[existingKey.String()] = []byte("existing")
	repository := &versionRepository{duplicate: true, result: SubmissionVersion{ID: "existing", Object: Object{Key: existingKey}}}
	service := NewSubmissionService(files, repository)
	version, duplicate, err := service.StoreVersion(context.Background(), StoreVersionRequest{
		StudentRecordID: "student", LabID: "lab", ImportID: "import", CreatedBy: "teacher",
		Size: 3, SourceManifestJSON: []byte(`{}`), Archive: bytes.NewReader([]byte("zip")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate || version.ID != "existing" {
		t.Fatal("existing version not returned")
	}
	if len(files.objects) != 1 {
		t.Fatal("redundant object was not removed or existing object was changed")
	}
}

type referenceRepository struct{ keys []Reference }

func (repository referenceRepository) ListObjectReferences(context.Context) ([]Reference, error) {
	return repository.keys, nil
}

func TestReconciliationFindsMissingAndOrphaned(t *testing.T) {
	files := newMemoryStorage()
	referenced, _ := NewKey(OriginalArchives, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".zip")
	missing, _ := NewKey(NormalizedSubmissions, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a53", ".zip")
	orphan, _ := NewKey(SafeLogs, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a54", ".log")
	files.objects[referenced.String()] = []byte("a")
	files.objects[orphan.String()] = []byte("b")
	report, err := Reconcile(context.Background(), files, referenceRepository{[]Reference{
		{Key: referenced, SHA256: sha256.Sum256([]byte("a")), HasDigest: true},
		{Key: missing, HasDigest: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Missing) != 1 || report.Missing[0] != missing {
		t.Fatalf("missing = %v", report.Missing)
	}
	if len(report.Orphaned) != 1 || report.Orphaned[0] != orphan {
		t.Fatalf("orphaned = %v", report.Orphaned)
	}
}

type deniedAccess struct{}

func (deniedAccess) ReadableSubmissionKey(context.Context, string, string) (Key, error) {
	return Key{}, ErrAccessDenied
}

func TestDownloadChecksAccessBeforeStorage(t *testing.T) {
	service := NewDownloadService(newMemoryStorage(), deniedAccess{})
	if _, _, err := service.OpenSubmission(context.Background(), "student-a", "student-b-version"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("open error = %v", err)
	}
	if _, err := service.TemporarySubmissionURL(context.Background(), "student-a", "student-b-version", 10); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("temporary URL error = %v", err)
	}
}

func TestObjectKeyRejectsPersonalDataAndUnsafeExtension(t *testing.T) {
	if _, err := NewKey(NormalizedSubmissions, "student-login", ".zip"); err == nil {
		t.Fatal("personal identifier accepted as object id")
	}
	if _, err := NewKey(NormalizedSubmissions, "018f47de-6c8b-7c41-9f5d-c68c7fbf6a52", ".ZIP"); err == nil {
		t.Fatal("non-canonical extension accepted")
	}
}
