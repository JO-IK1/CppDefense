package runner

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const maxSnapshotBytes int64 = 768 << 20
const maxSnapshotFiles = 100000

var sourceEpoch = time.Unix(946684800, 0)

type snapshotFile struct {
	Kind  string `json:"kind"`
	SHA   string `json:"sha,omitempty"`
	Link  string `json:"link,omitempty"`
	Size  int64  `json:"size"`
	Mode  uint32 `json:"mode"`
	MTime int64  `json:"mtime_ns"`
}

func sourceIndex(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular project file")
		}
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, f)
		f.Close()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = hex.EncodeToString(hash.Sum(nil))
		return nil
	})
	return out, err
}
func relativeSnapshotPath(name string) (string, error) {
	name = strings.TrimPrefix(name, "./")
	if name == "" || name == "." {
		return "", nil
	}
	if strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("unsafe snapshot path")
	}
	return strings.TrimSuffix(name, "/"), nil
}
func safeSnapshotLink(name, target string) bool {
	if target == "" || strings.ContainsAny(target, "\\\x00") || path.IsAbs(target) {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return resolved != ".." && !strings.HasPrefix(resolved, "../")
}

// Export is from a bounded tmpfs; never preserve owners or host hard links.
// Links are created last so later tar entries cannot write through them.
func unpackSnapshot(input io.Reader, destination string) (map[string]snapshotFile, int64, error) {
	files := map[string]snapshotFile{}
	var total int64
	reader := tar.NewReader(input)
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		name, err := relativeSnapshotPath(h.Name)
		if err != nil {
			return nil, 0, err
		}
		if name == "" {
			continue
		}
		if _, ok := files[name]; ok {
			return nil, 0, fmt.Errorf("duplicate snapshot path")
		}
		if len(files) >= maxSnapshotFiles {
			return nil, 0, fmt.Errorf("too many snapshot files")
		}
		if h.ModTime.Before(time.Unix(0, 0)) || h.ModTime.After(time.Now().Add(24*time.Hour)) {
			return nil, 0, fmt.Errorf("unsafe snapshot timestamp")
		}
		v := snapshotFile{Size: h.Size, Mode: uint32(h.Mode) & 0111, MTime: h.ModTime.UnixNano()}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, 0, err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			v.Kind = "dir"
			v.Size = 0
			if err = os.MkdirAll(target, 0700); err != nil {
				return nil, 0, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > maxSnapshotBytes-total {
				return nil, 0, fmt.Errorf("snapshot exceeds 768 MiB")
			}
			total += h.Size
			v.Kind = "file"
			f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return nil, 0, err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(io.MultiWriter(f, hash), reader)
			closeErr := f.Close()
			if copyErr != nil {
				return nil, 0, copyErr
			}
			if closeErr != nil {
				return nil, 0, closeErr
			}
			v.SHA = hex.EncodeToString(hash.Sum(nil))
			if err = os.Chtimes(target, h.ModTime, h.ModTime); err != nil {
				return nil, 0, err
			}
		case tar.TypeSymlink:
			if !safeSnapshotLink(name, h.Linkname) {
				return nil, 0, fmt.Errorf("unsafe snapshot symlink")
			}
			v.Kind = "link"
			v.Link = h.Linkname
			v.Size = 0
		default:
			return nil, 0, fmt.Errorf("unsupported snapshot entry type")
		}
		files[name] = v
	}
	for name, v := range files {
		if v.Kind == "link" {
			if err := os.Symlink(v.Link, filepath.Join(destination, filepath.FromSlash(name))); err != nil {
				return nil, 0, err
			}
		}
	}
	if _, ok := files["build.ninja"]; !ok {
		return nil, 0, fmt.Errorf("missing Ninja build")
	}
	if _, ok := files["CMakeCache.txt"]; !ok {
		return nil, 0, fmt.Errorf("missing CMake cache")
	}
	return files, total, nil
}
func sealSnapshot(root string, files map[string]snapshotFile) error {
	return filepath.WalkDir(root, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			return nil
		}
		mode := fs.FileMode(0555)
		if !e.IsDir() {
			rel, _ := filepath.Rel(root, name)
			mode = 0444 | fs.FileMode(files[filepath.ToSlash(rel)].Mode)
		}
		return os.Chmod(name, mode)
	})
}
func removeSnapshotTree(root string) {
	_ = filepath.WalkDir(root, func(name string, e fs.DirEntry, err error) error {
		if e == nil {
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if e.IsDir() {
			return os.Chmod(name, 0700)
		}
		return os.Chmod(name, 0600)
	})
	_ = os.RemoveAll(root)
}
func normalizeProjectTimes(root string, original map[string]string, latest int64) error {
	current, err := sourceIndex(root)
	if err != nil {
		return err
	}
	dirty := time.Now().Add(2 * time.Second)
	if candidate := time.Unix(0, latest).Add(2 * time.Second); candidate.After(dirty) {
		dirty = candidate
	}
	return filepath.WalkDir(root, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, name)
		stamp := sourceEpoch
		if !e.IsDir() && current[filepath.ToSlash(rel)] != original[filepath.ToSlash(rel)] {
			stamp = dirty
		}
		return os.Chtimes(name, stamp, stamp)
	})
}
