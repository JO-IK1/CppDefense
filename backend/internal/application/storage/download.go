package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrAccessDenied = errors.New("storage access denied")
var ErrTemporaryURLUnsupported = errors.New("temporary storage URL is unsupported")

type AccessRepository interface {
	ReadableSubmissionKey(ctx context.Context, actorUserID, submissionVersionID string) (Key, error)
}

type DownloadService struct {
	storage FileStorage
	access  AccessRepository
}

func NewDownloadService(fileStorage FileStorage, access AccessRepository) *DownloadService {
	if fileStorage == nil || access == nil {
		panic("download service dependencies are required")
	}
	return &DownloadService{storage: fileStorage, access: access}
}

func (service *DownloadService) OpenSubmission(ctx context.Context, actorUserID, submissionVersionID string) (io.ReadCloser, Object, error) {
	key, err := service.access.ReadableSubmissionKey(ctx, actorUserID, submissionVersionID)
	if err != nil {
		return nil, Object{}, err
	}
	return service.storage.Open(ctx, key)
}

func (service *DownloadService) TemporarySubmissionURL(ctx context.Context, actorUserID, submissionVersionID string, expiry time.Duration) (string, error) {
	key, err := service.access.ReadableSubmissionKey(ctx, actorUserID, submissionVersionID)
	if err != nil {
		return "", err
	}
	temporaryStorage, ok := service.storage.(TemporaryURLStorage)
	if !ok {
		return "", ErrTemporaryURLUnsupported
	}
	return temporaryStorage.TemporaryGetURL(ctx, key, expiry)
}
