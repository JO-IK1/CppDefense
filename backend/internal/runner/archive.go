package runner

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extract(source io.ReadCloser, destination string) error {
	data, err := io.ReadAll(io.LimitReader(source, 500<<20))
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, entry := range archive.File {
		name := filepath.Clean(entry.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || strings.Contains(entry.Name, "\\") || entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe normalized archive")
		}
		target := filepath.Join(destination, name)
		if !strings.HasPrefix(target, destination+string(filepath.Separator)) {
			return errors.New("archive escape")
		}
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, openErr := entry.Open()
		if openErr != nil {
			return openErr
		}
		out, copyErr := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if copyErr == nil {
			_, copyErr = io.Copy(out, io.LimitReader(in, 100<<20))
			out.Close()
		}
		in.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
