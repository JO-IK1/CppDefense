package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func snapshotReusedResult() sandboxResults {
	reused := commandResult{OK: true, Reused: true, Log: "Использован проверенный снимок сборки", DurationMS: timingPointer(0)}
	return sandboxResults{Configure: reused, Build: reused, CTest: commandResult{OK: true, Reused: true, Log: "Оригинал ранее проверен", DurationMS: timingPointer(0)}}
}
func snapshotFailure(err error) sandboxResults {
	result := commandResult{ExitCode: -1, Log: err.Error()}
	return sandboxResults{InfrastructureError: true, Configure: result, Build: result, CTest: result}
}

// Only this original-validation path can publish artifacts. The build export
// happens before CTest; tests always receive a fresh, private tmpfs copy.
func (a *agent) ensureSnapshot(ctx context.Context, original string, lease postgres.Lease, b *buildContext, refresh bool) (*buildSnapshot, sandboxResults, error) {
	root, err := a.cacheRoot()
	if err != nil {
		return nil, sandboxResults{}, err
	}
	unlock, err := snapshotLock(ctx, root, b.Key)
	if err != nil {
		return nil, sandboxResults{}, err
	}
	defer unlock()
	if current, err := loadSnapshot(root, b, a.config.BuildCacheTTL); err == nil {
		if !refresh || time.Since(current.Proof.VerifiedAt) < referenceTTL {
			return current, snapshotReusedResult(), nil
		}
		if err = normalizeProjectTimes(original, b.Sources, 0); err != nil {
			return nil, sandboxResults{}, err
		}
		if err = makeSandboxReadable(original); err != nil {
			return nil, sandboxResults{}, err
		}
		defer restoreSandboxOwnerAccess(original)
		result := runSandboxOptions(ctx, a.config.Runtime, b.Image, original, lease.SessionID, current.Dir, "reuse", nil)
		if !result.OK() {
			removeSnapshotTree(filepath.Join(root, b.Key))
			return nil, result, nil
		}
		current.Proof.VerifiedAt = time.Now().UTC()
		raw, _ := json.Marshal(current)
		if err = os.WriteFile(filepath.Join(root, b.Key, "metadata.new"), raw, 0600); err != nil {
			return nil, result, err
		}
		if err = os.Rename(filepath.Join(root, b.Key, "metadata.new"), filepath.Join(root, b.Key, "metadata.json")); err != nil {
			return nil, result, err
		}
		return current, result, nil
	}
	removeSnapshotTree(filepath.Join(root, b.Key))
	if err = normalizeProjectTimes(original, b.Sources, 0); err != nil {
		return nil, sandboxResults{}, err
	}
	if err = makeSandboxReadable(original); err != nil {
		return nil, sandboxResults{}, err
	}
	defer restoreSandboxOwnerAccess(original)
	staging, err := os.MkdirTemp(root, ".staging-"+b.Key+"-")
	if err != nil {
		return nil, sandboxResults{}, err
	}
	defer removeSnapshotTree(staging)
	artifact, err := os.CreateTemp(staging, "export-")
	if err != nil {
		return nil, sandboxResults{}, err
	}
	artifactName := artifact.Name()
	built := runSandboxOptions(ctx, a.config.Runtime, b.Image, original, lease.SessionID, "", "build", artifact)
	closeErr := artifact.Close()
	if !built.Configure.OK || !built.Build.OK {
		return nil, built, nil
	}
	if closeErr != nil || !built.SnapshotExportOK {
		return nil, built, fmt.Errorf("sandbox did not export a complete build snapshot")
	}
	started := time.Now()
	buildDir := filepath.Join(staging, "build")
	if err = os.Mkdir(buildDir, 0700); err != nil {
		return nil, built, err
	}
	exported, err := os.Open(artifactName)
	if err != nil {
		return nil, built, err
	}
	files, size, err := unpackSnapshot(exported, buildDir)
	exported.Close()
	os.Remove(artifactName)
	if err != nil {
		return nil, built, err
	}
	if err = sealSnapshot(buildDir, files); err != nil {
		return nil, built, err
	}
	addTiming(ctx, func(t *postgres.JobTimings) { t.SnapshotMS += elapsedMS(started) })
	checked := runSandboxOptions(ctx, a.config.Runtime, b.Image, original, lease.SessionID, buildDir, "test", nil)
	built.CTest = checked.CTest
	built.InfrastructureError = built.InfrastructureError || checked.InfrastructureError
	if !built.OK() {
		return nil, built, nil
	}
	proof := b.Proof
	proof.VerifiedAt = time.Now().UTC()
	value := &buildSnapshot{Schema: snapshotSchema, Key: b.Key, Proof: proof, Files: files, Bytes: size, Dir: buildDir}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, built, err
	}
	if len(raw) > 32<<20 {
		return nil, built, fmt.Errorf("snapshot metadata exceeds 32 MiB")
	}
	if err = os.WriteFile(filepath.Join(staging, "metadata.json"), raw, 0600); err != nil {
		return nil, built, err
	}
	if err = os.Rename(staging, filepath.Join(root, b.Key)); err != nil {
		return nil, built, err
	}
	value.Dir = filepath.Join(root, b.Key, "build")
	a.pruneSnapshots(root, b.Key)
	return value, built, nil
}

func (a *agent) prepareBuildContext(ctx context.Context, original string, lease postgres.Lease) (*buildContext, *sandboxResults) {
	if a.config.BuildCacheDir == "" {
		return nil, nil
	}
	b, err := a.newBuildContext(ctx, original, lease)
	if err != nil {
		return nil, nil
	}
	_, result, err := a.ensureSnapshot(ctx, original, lease, b, false)
	if err != nil {
		return nil, nil
	}
	return b, &result
}

func (a *agent) runWithSnapshot(ctx context.Context, project string, lease postgres.Lease, b *buildContext) sandboxResults {
	if b == nil {
		return runSandboxCached(ctx, a.config.Runtime, a.config.Image, project, lease.SessionID)
	}
	root, err := a.cacheRoot()
	if err != nil {
		return runSandboxCached(ctx, a.config.Runtime, b.Image, project, lease.SessionID)
	}
	unlock, err := snapshotLock(ctx, root, b.Key)
	if err != nil {
		return snapshotFailure(err)
	}
	defer unlock()
	snapshot, err := loadSnapshot(root, b, a.config.BuildCacheTTL)
	if err != nil {
		return runSandboxCached(ctx, a.config.Runtime, b.Image, project, lease.SessionID)
	}
	var latest int64
	for _, file := range snapshot.Files {
		if file.MTime > latest {
			latest = file.MTime
		}
	}
	if err = normalizeProjectTimes(project, b.Sources, latest); err != nil {
		return snapshotFailure(err)
	}
	addTiming(ctx, func(t *postgres.JobTimings) { t.SnapshotHit = true })
	return runSandboxOptions(ctx, a.config.Runtime, b.Image, project, lease.SessionID, snapshot.Dir, "reuse", nil)
}
