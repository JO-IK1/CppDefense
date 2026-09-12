package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Secure    bool
	SpoolDir  string
}

type Storage struct {
	client   *minio.Client
	bucket   string
	spoolDir string
}

func New(config Config) (*Storage, error) {
	if config.Endpoint == "" || config.Bucket == "" || config.AccessKey == "" || config.SecretKey == "" {
		return nil, errors.New("S3 endpoint, bucket and credentials are required")
	}
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.Secure,
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return &Storage{client: client, bucket: config.Bucket, spoolDir: config.SpoolDir}, nil
}

func (storage *Storage) EnsurePrivateBucket(ctx context.Context) error {
	exists, err := storage.client.BucketExists(ctx, storage.bucket)
	if err != nil {
		return unavailable("check bucket", err)
	}
	if !exists {
		if err := storage.client.MakeBucket(ctx, storage.bucket, minio.MakeBucketOptions{}); err != nil {
			return unavailable("create bucket", err)
		}
	}
	policy, err := storage.client.GetBucketPolicy(ctx, storage.bucket)
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.Code != "NoSuchBucketPolicy" && response.Code != "NoSuchPolicy" {
			return unavailable("read bucket policy", err)
		}
		return nil
	}
	if strings.TrimSpace(policy) != "" {
		return errors.New("S3 bucket must not have a public or custom bucket policy")
	}
	return nil
}

func (storage *Storage) Put(ctx context.Context, key appstorage.Key, source io.Reader, options appstorage.PutOptions) (appstorage.Object, error) {
	if options.Size < 0 {
		return appstorage.Object{}, errors.New("object size cannot be negative")
	}
	temporary, err := os.CreateTemp(storage.spoolDir, "cppdefense-upload-*")
	if err != nil {
		return appstorage.Object{}, fmt.Errorf("create upload spool: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck

	hash := sha256.New()
	written, copyErr := copyContext(ctx, io.MultiWriter(temporary, hash), io.LimitReader(source, options.Size+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return appstorage.Object{}, fmt.Errorf("spool upload: %w", copyErr)
	}
	if closeErr != nil {
		return appstorage.Object{}, fmt.Errorf("close upload spool: %w", closeErr)
	}
	if written != options.Size {
		return appstorage.Object{}, fmt.Errorf("object size mismatch: expected %d, received %d", options.Size, written)
	}
	digest := hash.Sum(nil)

	reader, err := os.Open(temporaryPath)
	if err != nil {
		return appstorage.Object{}, fmt.Errorf("open upload spool: %w", err)
	}
	defer reader.Close()
	putOptions := minio.PutObjectOptions{
		ContentType:  options.ContentType,
		UserMetadata: map[string]string{"sha256": hex.EncodeToString(digest)},
	}
	putOptions.SetMatchETagExcept("*")
	info, err := storage.client.PutObject(ctx, storage.bucket, key.String(), reader, options.Size, putOptions)
	if err != nil {
		return appstorage.Object{}, classify("put object", err)
	}
	var sum [32]byte
	copy(sum[:], digest)
	return appstorage.Object{Key: key, Size: info.Size, SHA256: sum, ContentType: options.ContentType, CreatedAt: time.Now().UTC()}, nil
}

func (storage *Storage) Open(ctx context.Context, key appstorage.Key) (io.ReadCloser, appstorage.Object, error) {
	metadata, err := storage.Stat(ctx, key)
	if err != nil {
		return nil, appstorage.Object{}, err
	}
	reader, err := storage.client.GetObject(ctx, storage.bucket, key.String(), minio.GetObjectOptions{})
	if err != nil {
		return nil, appstorage.Object{}, classify("get object", err)
	}
	return reader, metadata, nil
}

func (storage *Storage) Stat(ctx context.Context, key appstorage.Key) (appstorage.Object, error) {
	info, err := storage.client.StatObject(ctx, storage.bucket, key.String(), minio.StatObjectOptions{})
	if err != nil {
		return appstorage.Object{}, classify("stat object", err)
	}
	digestHex := info.Metadata.Get("X-Amz-Meta-Sha256")
	digest, err := hex.DecodeString(digestHex)
	if err != nil || len(digest) != sha256.Size {
		return appstorage.Object{}, errors.New("stored object has invalid SHA-256 metadata")
	}
	var sum [32]byte
	copy(sum[:], digest)
	return appstorage.Object{Key: key, Size: info.Size, SHA256: sum, ContentType: info.ContentType, CreatedAt: info.LastModified.UTC()}, nil
}

func (storage *Storage) List(ctx context.Context, namespace appstorage.Namespace) ([]appstorage.Object, error) {
	result := make([]appstorage.Object, 0)
	for item := range storage.client.ListObjects(ctx, storage.bucket, minio.ListObjectsOptions{Prefix: string(namespace) + "/", Recursive: true}) {
		if item.Err != nil {
			return nil, classify("list objects", item.Err)
		}
		key, err := appstorage.ParseKey(item.Key)
		if err != nil {
			return nil, fmt.Errorf("invalid key returned by S3: %w", err)
		}
		metadata, err := storage.Stat(ctx, key)
		if err != nil {
			return nil, err
		}
		result = append(result, metadata)
	}
	return result, nil
}

func (storage *Storage) Delete(ctx context.Context, key appstorage.Key) error {
	err := storage.client.RemoveObject(ctx, storage.bucket, key.String(), minio.RemoveObjectOptions{})
	if err != nil {
		return classify("delete object", err)
	}
	return nil
}

func (storage *Storage) Check(ctx context.Context) error {
	exists, err := storage.client.BucketExists(ctx, storage.bucket)
	if err != nil {
		return unavailable("check bucket", err)
	}
	if !exists {
		return fmt.Errorf("%w: bucket %s does not exist", appstorage.ErrUnavailable, storage.bucket)
	}
	return nil
}

func (storage *Storage) TemporaryGetURL(ctx context.Context, key appstorage.Key, expiry time.Duration) (string, error) {
	if expiry < time.Second || expiry > 15*time.Minute {
		return "", errors.New("temporary URL expiry must be between 1 second and 15 minutes")
	}
	result, err := storage.client.PresignedGetObject(ctx, storage.bucket, key.String(), expiry, url.Values{})
	if err != nil {
		return "", classify("presign object", err)
	}
	return result.String(), nil
}

func classify(operation string, err error) error {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NoSuchBucket":
		return fmt.Errorf("%s: %w", operation, appstorage.ErrNotFound)
	case "PreconditionFailed":
		return fmt.Errorf("%s: %w", operation, appstorage.ErrExists)
	}
	return unavailable(operation, err)
}

func unavailable(operation string, err error) error {
	return fmt.Errorf("%s: %w: %v", operation, appstorage.ErrUnavailable, err)
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
