package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/domain"
)

type SubmissionVersion struct {
	ID              string
	StudentRecordID string
	LabID           string
	VersionNumber   int
	Object          Object
	CreatedAt       time.Time
}

type CreateVersion struct {
	ID                 string
	StudentRecordID    string
	LabID              string
	ImportID           string
	CreatedBy          string
	NormalizedKey      Key
	NormalizedSHA256   [32]byte
	SourceManifestJSON []byte
}

type VersionRepository interface {
	CreateOrGetByDigest(context.Context, CreateVersion) (SubmissionVersion, bool, error)
}

type SubmissionService struct {
	storage  FileStorage
	versions VersionRepository
}

func NewSubmissionService(fileStorage FileStorage, versions VersionRepository) *SubmissionService {
	if fileStorage == nil || versions == nil {
		panic("submission service dependencies are required")
	}
	return &SubmissionService{storage: fileStorage, versions: versions}
}

type StoreVersionRequest struct {
	StudentRecordID    string
	LabID              string
	ImportID           string
	CreatedBy          string
	Size               int64
	SourceManifestJSON []byte
	Archive            io.Reader
}

func (service *SubmissionService) StoreVersion(ctx context.Context, request StoreVersionRequest) (SubmissionVersion, bool, error) {
	if request.Archive == nil || request.Size < 0 {
		return SubmissionVersion{}, false, errors.New("normalized archive and non-negative size are required")
	}
	objectID, err := domain.NewUUIDv7()
	if err != nil {
		return SubmissionVersion{}, false, err
	}
	versionID, err := domain.NewUUIDv7()
	if err != nil {
		return SubmissionVersion{}, false, err
	}
	key, err := NewKey(NormalizedSubmissions, objectID, ".zip")
	if err != nil {
		return SubmissionVersion{}, false, err
	}
	object, err := service.storage.Put(ctx, key, request.Archive, PutOptions{Size: request.Size, ContentType: "application/zip"})
	if err != nil {
		return SubmissionVersion{}, false, err
	}

	created, duplicate, err := service.versions.CreateOrGetByDigest(ctx, CreateVersion{
		ID: versionID, StudentRecordID: request.StudentRecordID, LabID: request.LabID,
		ImportID: request.ImportID, CreatedBy: request.CreatedBy, NormalizedKey: key,
		NormalizedSHA256: object.SHA256, SourceManifestJSON: request.SourceManifestJSON,
	})
	if err != nil || duplicate {
		cleanupErr := service.storage.Delete(context.WithoutCancel(ctx), key)
		if err != nil {
			if cleanupErr != nil {
				return SubmissionVersion{}, false, errors.Join(err, fmt.Errorf("cleanup uploaded object: %w", cleanupErr))
			}
			return SubmissionVersion{}, false, err
		}
		if cleanupErr != nil {
			return SubmissionVersion{}, false, fmt.Errorf("cleanup duplicate object: %w", cleanupErr)
		}
		existingObject, statErr := service.storage.Stat(ctx, created.Object.Key)
		if statErr != nil {
			return SubmissionVersion{}, false, fmt.Errorf("stat existing duplicate: %w", statErr)
		}
		created.Object = existingObject
		return created, true, nil
	}
	created.Object = object
	return created, false, nil
}

func StoreOriginalArchive(ctx context.Context, fileStorage FileStorage, size int64, archive io.Reader) (Object, error) {
	if fileStorage == nil || archive == nil || size < 0 {
		return Object{}, errors.New("storage, archive and non-negative size are required")
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return Object{}, err
	}
	key, err := NewKey(OriginalArchives, id, ".zip")
	if err != nil {
		return Object{}, err
	}
	return fileStorage.Put(ctx, key, archive, PutOptions{Size: size, ContentType: "application/zip"})
}
