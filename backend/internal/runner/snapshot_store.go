package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

const snapshotSchema = 1

type buildContext struct {
	Key, Image, Policy string
	Proof              postgres.ReferenceResult
	Sources            map[string]string
}
type buildSnapshot struct {
	Schema int                      `json:"schema"`
	Key    string                   `json:"key"`
	Proof  postgres.ReferenceResult `json:"proof"`
	Files  map[string]snapshotFile  `json:"files"`
	Bytes  int64                    `json:"bytes"`
	Dir    string                   `json:"-"`
}

func (a *agent) newBuildContext(ctx context.Context, original string, lease postgres.Lease) (*buildContext, error) {
	image, policy, err := a.referenceEnvironment(ctx)
	if err != nil {
		return nil, err
	}
	key, proof, err := referenceIdentity(original, lease, image, policy)
	if err != nil {
		return nil, err
	}
	sources, err := sourceIndex(original)
	if err != nil {
		return nil, err
	}
	return &buildContext{Key: key, Image: image, Policy: policy, Proof: *proof, Sources: sources}, nil
}
func (a *agent) cacheRoot() (string, error) {
	if a.config.BuildCacheDir == "" {
		return "", fmt.Errorf("snapshots disabled")
	}
	root, err := filepath.Abs(a.config.BuildCacheDir)
	if err != nil || root == string(filepath.Separator) {
		return "", fmt.Errorf("invalid cache root")
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("cache is not a private directory")
	}
	marker := filepath.Join(root, ".cppdefense-build-cache-v1")
	expected := "CppDefense build snapshots 1\n"
	if raw, err := os.ReadFile(marker); err == nil {
		if string(raw) != expected {
			return "", fmt.Errorf("invalid cache ownership marker")
		}
	} else if os.IsNotExist(err) {
		entries, err := os.ReadDir(root)
		if err != nil {
			return "", err
		}
		if len(entries) != 0 {
			return "", fmt.Errorf("refusing to manage a nonempty unmarked directory")
		}
		file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		_, err = file.WriteString(expected)
		file.Close()
		if err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	if err = os.Chmod(root, 0700); err != nil {
		return "", err
	}
	a.cleanupStages(root)
	return root, nil
}

func (a *agent) cleanupStages(root string) {
	entries, _ := os.ReadDir(root)
	age := 24 * time.Hour
	if a.config.Timeout+time.Hour > age {
		age = a.config.Timeout + time.Hour
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".staging-") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(entry.Name(), ".staging-"), "-", 2)
		if len(parts) != 2 || !validSnapshotKey(parts[0]) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < age {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		unlock, err := snapshotLock(ctx, root, parts[0])
		cancel()
		if err != nil {
			continue
		}
		removeSnapshotTree(filepath.Join(root, entry.Name()))
		unlock()
	}
}
func validSnapshotKey(key string) bool {
	return len(key) == 64 && strings.Trim(key, "0123456789abcdef") == ""
}

// A crash releases flock; lock files are not unlinked underneath waiters.
func snapshotLock(ctx context.Context, root, key string) (func(), error) {
	if !validSnapshotKey(key) {
		return nil, fmt.Errorf("invalid snapshot key")
	}
	f, err := os.OpenFile(filepath.Join(root, key+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			f.Close()
			return nil, err
		}
		if !wait(ctx, 25*time.Millisecond) {
			f.Close()
			return nil, ctx.Err()
		}
	}
}
func loadSnapshot(root string, b *buildContext, ttl time.Duration) (*buildSnapshot, error) {
	directory := filepath.Join(root, b.Key)
	f, err := os.Open(filepath.Join(directory, "metadata.json"))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	f.Close()
	if err != nil || len(data) > 32<<20 {
		return nil, fmt.Errorf("invalid snapshot metadata")
	}
	var v buildSnapshot
	if err = json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	age := time.Since(v.Proof.VerifiedAt)
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if v.Schema != snapshotSchema || v.Key != b.Key || v.Proof.ProjectSHA256 != b.Proof.ProjectSHA256 || v.Proof.PolicySHA256 != b.Policy || v.Proof.ImageID != b.Image || age < 0 || age > ttl || len(v.Files) > maxSnapshotFiles || v.Bytes > maxSnapshotBytes {
		return nil, fmt.Errorf("stale snapshot metadata")
	}
	if err = v.Proof.Validate(); err != nil {
		return nil, err
	}
	v.Dir = filepath.Join(directory, "build")
	var actual int64
	seen := map[string]bool{}
	err = filepath.WalkDir(v.Dir, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == v.Dir {
			return nil
		}
		rel, err := filepath.Rel(v.Dir, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		expected, ok := v.Files[rel]
		if !ok {
			if e.IsDir() {
				return nil
			}
			return fmt.Errorf("unexpected snapshot file")
		}
		seen[rel] = true
		info, err := e.Info()
		if err != nil {
			return err
		}
		switch expected.Kind {
		case "dir":
			if !e.IsDir() {
				return fmt.Errorf("snapshot directory changed")
			}
		case "link":
			if e.Type()&os.ModeSymlink == 0 {
				return fmt.Errorf("snapshot link changed")
			}
			target, err := os.Readlink(name)
			if err != nil || target != expected.Link || !safeSnapshotLink(rel, target) {
				return fmt.Errorf("snapshot link changed")
			}
		case "file":
			if expected.MTime < 0 || expected.MTime > time.Now().Add(24*time.Hour).UnixNano() || !info.Mode().IsRegular() || info.Size() != expected.Size || info.ModTime().UnixNano() != expected.MTime || uint32(info.Mode().Perm()&0111) != expected.Mode || info.Mode().Perm()&0222 != 0 {
				return fmt.Errorf("snapshot file changed")
			}
			file, err := os.Open(name)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, err = io.Copy(hash, file)
			file.Close()
			if err != nil || hex.EncodeToString(hash.Sum(nil)) != expected.SHA {
				return fmt.Errorf("snapshot content changed")
			}
			actual += expected.Size
		default:
			return fmt.Errorf("invalid snapshot file kind")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(seen) != len(v.Files) || actual != v.Bytes {
		return nil, fmt.Errorf("incomplete snapshot")
	}
	return &v, nil
}
func (a *agent) pruneSnapshots(root, keep string) {
	entries, _ := os.ReadDir(root)
	type candidate struct {
		key  string
		size int64
		at   time.Time
	}
	var values []candidate
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() || !validSnapshotKey(entry.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name(), "metadata.json"))
		if err != nil || len(raw) > 32<<20 {
			continue
		}
		var v buildSnapshot
		if json.Unmarshal(raw, &v) != nil {
			continue
		}
		cost := v.Bytes + int64(len(raw)) + int64(len(v.Files))*4096
		total += cost
		values = append(values, candidate{entry.Name(), cost, v.Proof.VerifiedAt})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].at.Before(values[j].at) })
	limit := a.config.BuildCacheBytes
	if limit <= 0 {
		limit = 2 << 30
	}
	ttl := a.config.BuildCacheTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	count := len(values)
	for _, v := range values {
		if v.key == keep || total <= limit && count <= 64 && time.Since(v.at) < ttl {
			continue
		}
		attempt, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		unlock, err := snapshotLock(attempt, root, v.key)
		cancel()
		if err != nil {
			continue
		}
		removeSnapshotTree(filepath.Join(root, v.key))
		unlock()
		total -= v.size
		count--
	}
}
