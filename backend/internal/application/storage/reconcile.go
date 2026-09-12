package storage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

type ReferenceRepository interface {
	ListObjectReferences(context.Context) ([]Reference, error)
}

type Reference struct {
	Key       Key
	SHA256    [32]byte
	HasDigest bool
}

type ReconciliationReport struct {
	Checked  int
	Missing  []Key
	Orphaned []Key
}

func Reconcile(ctx context.Context, fileStorage FileStorage, references ReferenceRepository) (ReconciliationReport, error) {
	if fileStorage == nil || references == nil {
		return ReconciliationReport{}, fmt.Errorf("reconciliation dependencies are required")
	}
	keys, err := references.ListObjectReferences(ctx)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("list database references: %w", err)
	}
	referenced := make(map[string]Key, len(keys))
	report := ReconciliationReport{Checked: len(keys)}
	for _, reference := range keys {
		key := reference.Key
		referenced[key.String()] = key
		reader, object, err := fileStorage.Open(ctx, key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				report.Missing = append(report.Missing, key)
				continue
			}
			return ReconciliationReport{}, fmt.Errorf("open %s: %w", key.String(), err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return ReconciliationReport{}, fmt.Errorf("hash %s: %w", key.String(), copyErr)
		}
		if closeErr != nil {
			return ReconciliationReport{}, fmt.Errorf("close %s: %w", key.String(), closeErr)
		}
		var actual [32]byte
		copy(actual[:], hash.Sum(nil))
		if actual != object.SHA256 || (reference.HasDigest && actual != reference.SHA256) {
			return ReconciliationReport{}, fmt.Errorf("object %s has a checksum mismatch", key.String())
		}
	}
	for _, namespace := range []Namespace{OriginalArchives, NormalizedSubmissions, SafeLogs} {
		objects, err := fileStorage.List(ctx, namespace)
		if err != nil {
			return ReconciliationReport{}, fmt.Errorf("list %s: %w", namespace, err)
		}
		for _, object := range objects {
			if _, ok := referenced[object.Key.String()]; !ok {
				report.Orphaned = append(report.Orphaned, object.Key)
			}
		}
	}
	return report, nil
}
