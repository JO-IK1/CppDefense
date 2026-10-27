package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

// Proofs are private, bounded and process-local. Restarting a runner or moving
// a job to another runner requires validation again, never optimistic acceptance.
const referenceTTL = time.Hour

func normalizedCode(value []byte) []byte {
	return bytes.ReplaceAll(value, []byte("\r\n"), []byte("\n"))
}

func projectDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			relative, _ := filepath.Rel(root, name)
			fmt.Fprintf(hash, "dir:%d:%s:", len(filepath.ToSlash(relative)), filepath.ToSlash(relative))
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular reference file")
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%d:%s:%d:", len(filepath.ToSlash(relative)), filepath.ToSlash(relative), info.Size())
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(hash, file)
		file.Close()
		return err
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *agent) referenceEnvironment(ctx context.Context) (string, string, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(inspectCtx, a.config.Runtime, "image", "inspect", "--format", "{{.Id}}", a.config.Image).Output()
	if err != nil {
		return "", "", err
	}
	id := strings.TrimSpace(string(output))
	id = strings.TrimPrefix(id, "sha256:")
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != 32 {
		return "", "", fmt.Errorf("cannot resolve immutable sandbox image")
	}
	image := "sha256:" + strings.ToLower(id)
	// Derived from the actual execution arguments and script, not a manually
	// maintained version label. Project location is not an execution setting.
	runtimeSignature := ""
	if binary, err := exec.LookPath(a.config.Runtime); err == nil {
		if info, err := os.Stat(binary); err == nil {
			runtimeSignature = fmt.Sprintf("%s:%d:%d", binary, info.Size(), info.ModTime().UnixNano())
		}
	}
	configurationSignature := ""
	if conf := os.Getenv("CONTAINERS_CONF"); conf != "" {
		raw, err := os.ReadFile(conf)
		if err != nil {
			return "", "", fmt.Errorf("cannot read runtime configuration")
		}
		hash := sha256.Sum256(raw)
		configurationSignature = hex.EncodeToString(hash[:])
	}
	policy, _ := json.Marshal([]any{snapshotSchema, a.config.Runtime, runtimeSignature, configurationSignature, a.config.Timeout, sandboxArguments(image, "REFERENCE_PROJECT"), sandboxScript})
	digest := sha256.Sum256(policy)
	return image, hex.EncodeToString(digest[:]), nil
}

func referenceIdentity(project string, lease postgres.Lease, image, policy string) (string, *postgres.ReferenceResult, error) {
	digest, err := projectDigest(project)
	if err != nil {
		return "", nil, err
	}
	data, _ := json.Marshal([]string{lease.SubmissionObjectKey, lease.SessionID, digest, image, policy})
	key := sha256.Sum256(data)
	return hex.EncodeToString(key[:]), &postgres.ReferenceResult{ProjectSHA256: digest, ImageID: image, PolicySHA256: policy}, nil
}

func (a *agent) rememberReference(project string, lease postgres.Lease, image, policy string) {
	if image == "" || policy == "" {
		return
	}
	key, proof, err := referenceIdentity(project, lease, image, policy)
	if err != nil {
		return
	}
	proof.VerifiedAt = time.Now().UTC()
	a.referenceMu.Lock()
	defer a.referenceMu.Unlock()
	if a.references == nil || len(a.references) >= 256 {
		a.references = make(map[string]postgres.ReferenceResult)
	}
	a.references[key] = *proof
}

func (a *agent) verifyOriginal(ctx context.Context, project string, lease postgres.Lease) (*postgres.ReferenceResult, sandboxResults) {
	if a.config.BuildCacheDir != "" {
		if b, err := a.newBuildContext(ctx, project, lease); err == nil {
			snapshot, result, err := a.ensureSnapshot(ctx, project, lease, b, true)
			if err == nil {
				if snapshot == nil {
					return nil, result
				}
				a.referenceMu.Lock()
				if a.references == nil || len(a.references) >= 256 {
					a.references = make(map[string]postgres.ReferenceResult)
				}
				a.references[b.Key] = snapshot.Proof
				a.referenceMu.Unlock()
				proof := snapshot.Proof
				return &proof, result
			}
		}
	}
	image, policy, envErr := a.referenceEnvironment(ctx)
	var key string
	if envErr == nil {
		var err error
		key, _, err = referenceIdentity(project, lease, image, policy)
		if err != nil {
			return nil, sandboxResults{InfrastructureError: true, Configure: commandResult{ExitCode: -1, Log: err.Error()}}
		}
		a.referenceMu.Lock()
		proof, found := a.references[key]
		a.referenceMu.Unlock()
		if found && time.Since(proof.VerifiedAt) >= 0 && time.Since(proof.VerifiedAt) < referenceTTL {
			return &proof, sandboxResults{Configure: commandResult{OK: true}, Build: commandResult{OK: true}, CTest: commandResult{OK: true}}
		}
	} else {
		image = a.config.Image
	}
	if err := makeSandboxReadable(project); err != nil {
		return nil, sandboxResults{InfrastructureError: true, Configure: commandResult{ExitCode: -1, Log: err.Error()}}
	}
	defer restoreSandboxOwnerAccess(project)
	results := runSandboxCached(ctx, a.config.Runtime, image, project, lease.SessionID)
	if !results.OK() || envErr != nil {
		return nil, results
	}
	a.rememberReference(project, lease, image, policy)
	a.referenceMu.Lock()
	proof, found := a.references[key]
	a.referenceMu.Unlock()
	if !found {
		return nil, results
	}
	return &proof, results
}

func restoreMatches(project string, lease postgres.Lease) bool {
	s := lease.SelectedFunction
	if s == nil || lease.Answer == nil || s.FilePath == "" || path.Clean(s.FilePath) != s.FilePath || strings.HasPrefix(s.FilePath, "/") || strings.HasPrefix(s.FilePath, "../") || strings.ContainsAny(s.FilePath, "\\\x00") {
		return false
	}
	source, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(s.FilePath)))
	if err != nil {
		return false
	}
	digest := sha256.Sum256(source)
	if hex.EncodeToString(digest[:]) != s.SourceSHA256 || s.BodyBegin < 0 || s.BodyEnd <= s.BodyBegin+1 || s.BodyEnd > int64(len(source)) || source[s.BodyBegin] != '{' || source[s.BodyEnd-1] != '}' {
		return false
	}
	return bytes.Equal(normalizedCode(source[s.BodyBegin+1:s.BodyEnd-1]), normalizedCode([]byte(*lease.Answer)))
}

func sameSources(original, changed map[string][]byte) bool {
	if len(original) != len(changed) {
		return false
	}
	for name, data := range original {
		value, found := changed[name]
		if !found || !bytes.Equal(normalizedCode(data), normalizedCode(value)) {
			return false
		}
	}
	return true
}

func (a *agent) acceptReference(parent, ctx context.Context, project string, lease postgres.Lease) (bool, error) {
	proof, _ := a.verifyOriginal(ctx, project, lease)
	if proof == nil {
		return false, nil
	}
	return true, a.complete(parent, lease, postgres.Completion{Outcome: "passed", ReferenceResult: proof})
}
