package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

type Namespace string

const (
	OriginalArchives      Namespace = "original-archives"
	NormalizedSubmissions Namespace = "normalized-submissions"
	SafeLogs              Namespace = "safe-logs"
)

var opaqueIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var extensionPattern = regexp.MustCompile(`^\.[a-z0-9]{1,11}$`)

type Key struct{ value string }

func NewKey(namespace Namespace, opaqueID, extension string) (Key, error) {
	if namespace != OriginalArchives && namespace != NormalizedSubmissions && namespace != SafeLogs {
		return Key{}, fmt.Errorf("unsupported object namespace %q", namespace)
	}
	if !opaqueIDPattern.MatchString(opaqueID) {
		return Key{}, errors.New("object id must be a canonical UUID")
	}
	if !extensionPattern.MatchString(extension) {
		return Key{}, errors.New("invalid object extension")
	}
	return Key{value: string(namespace) + "/" + opaqueID + extension}, nil
}

func ParseKey(value string) (Key, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return Key{}, errors.New("invalid object key")
	}
	dot := strings.LastIndexByte(parts[1], '.')
	if dot < 0 {
		return Key{}, errors.New("object key has no extension")
	}
	return NewKey(Namespace(parts[0]), parts[1][:dot], parts[1][dot:])
}

func (key Key) String() string { return key.value }

type Object struct {
	Key         Key
	Size        int64
	SHA256      [32]byte
	ContentType string
	CreatedAt   time.Time
}

type PutOptions struct {
	Size        int64
	ContentType string
}

var (
	ErrNotFound    = errors.New("storage object not found")
	ErrExists      = errors.New("storage object already exists")
	ErrUnavailable = errors.New("storage unavailable")
)

type FileStorage interface {
	Put(context.Context, Key, io.Reader, PutOptions) (Object, error)
	Open(context.Context, Key) (io.ReadCloser, Object, error)
	Stat(context.Context, Key) (Object, error)
	List(context.Context, Namespace) ([]Object, error)
	Delete(context.Context, Key) error
	Check(context.Context) error
}

type TemporaryURLStorage interface {
	FileStorage
	TemporaryGetURL(context.Context, Key, time.Duration) (string, error)
}

func IsRetryable(err error) bool { return errors.Is(err, ErrUnavailable) }
