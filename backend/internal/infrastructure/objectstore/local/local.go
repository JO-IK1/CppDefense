package local

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
)

type Storage struct{ root string }

func New(root string) (*Storage, error) {
	if root == "" {
		return nil, errors.New("local storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &Storage{root: absolute}, nil
}

func (storage *Storage) Put(ctx context.Context, key appstorage.Key, source io.Reader, options appstorage.PutOptions) (appstorage.Object, error) {
	if options.Size < 0 {
		return appstorage.Object{}, errors.New("object size cannot be negative")
	}
	path := storage.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return appstorage.Object{}, fmt.Errorf("create namespace: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return appstorage.Object{}, fmt.Errorf("create upload: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck

	hash := sha256.New()
	written, copyErr := copyContext(ctx, io.MultiWriter(temporary, hash), io.LimitReader(source, options.Size+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return appstorage.Object{}, fmt.Errorf("write object: %w", copyErr)
	}
	if closeErr != nil {
		return appstorage.Object{}, fmt.Errorf("close object: %w", closeErr)
	}
	if written != options.Size {
		return appstorage.Object{}, fmt.Errorf("object size mismatch: expected %d, received %d", options.Size, written)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return appstorage.Object{}, fmt.Errorf("restrict object: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return appstorage.Object{}, appstorage.ErrExists
		}
		return appstorage.Object{}, fmt.Errorf("publish object: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return appstorage.Object{}, fmt.Errorf("stat published object: %w", err)
	}
	return object(key, info, hash.Sum(nil), options.ContentType), nil
}

func (storage *Storage) Open(ctx context.Context, key appstorage.Key) (io.ReadCloser, appstorage.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, appstorage.Object{}, err
	}
	file, err := os.Open(storage.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, appstorage.Object{}, appstorage.ErrNotFound
	}
	if err != nil {
		return nil, appstorage.Object{}, fmt.Errorf("open object: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, appstorage.Object{}, fmt.Errorf("stat object: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, appstorage.Object{}, errors.New("stored object is not a regular file")
	}
	hash, err := digest(ctx, file)
	if err != nil {
		file.Close()
		return nil, appstorage.Object{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, appstorage.Object{}, err
	}
	return file, object(key, info, hash[:], "application/octet-stream"), nil
}

func (storage *Storage) Stat(ctx context.Context, key appstorage.Key) (appstorage.Object, error) {
	reader, result, err := storage.Open(ctx, key)
	if err != nil {
		return appstorage.Object{}, err
	}
	if err := reader.Close(); err != nil {
		return appstorage.Object{}, fmt.Errorf("close object: %w", err)
	}
	return result, nil
}

func (storage *Storage) List(ctx context.Context, namespace appstorage.Namespace) ([]appstorage.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory := filepath.Join(storage.root, string(namespace))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []appstorage.Object{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list namespace: %w", err)
	}
	result := make([]appstorage.Object, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, errors.New("storage namespace contains a non-regular entry")
		}
		key, err := appstorage.ParseKey(string(namespace) + "/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid stored key: %w", err)
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
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(storage.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (storage *Storage) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(storage.root)
	if err != nil {
		return fmt.Errorf("local storage: %w", err)
	}
	if !info.IsDir() {
		return errors.New("local storage root is not a directory")
	}
	return nil
}

func (storage *Storage) path(key appstorage.Key) string {
	return filepath.Join(storage.root, filepath.FromSlash(key.String()))
}

func object(key appstorage.Key, info os.FileInfo, digest []byte, contentType string) appstorage.Object {
	var sum [32]byte
	copy(sum[:], digest)
	return appstorage.Object{Key: key, Size: info.Size(), SHA256: sum, ContentType: contentType, CreatedAt: info.ModTime().UTC()}
}

func digest(ctx context.Context, reader io.Reader) ([32]byte, error) {
	hash := sha256.New()
	if _, err := copyContext(ctx, hash, reader); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
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
