package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/jackc/pgx/v5"
)

type StorageRepository struct{ db *Database }

func NewStorageRepository(db *Database) *StorageRepository { return &StorageRepository{db: db} }

func (repository *StorageRepository) CreateOrGetByDigest(ctx context.Context, command appstorage.CreateVersion) (appstorage.SubmissionVersion, bool, error) {
	if !json.Valid(command.SourceManifestJSON) {
		return appstorage.SubmissionVersion{}, false, errors.New("source manifest is not valid JSON")
	}
	tx, err := repository.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return appstorage.SubmissionVersion{}, false, fmt.Errorf("begin version transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	lockKey := command.StudentRecordID + ":" + command.LabID
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return appstorage.SubmissionVersion{}, false, fmt.Errorf("lock version sequence: %w", err)
	}
	var existing appstorage.SubmissionVersion
	var keyValue string
	var digest []byte
	err = tx.QueryRow(ctx, `
		select id, version_no, normalized_object_key, normalized_sha256, created_at
		from submission_versions
		where student_record_id = $1 and lab_id = $2 and normalized_sha256 = $3
		order by version_no desc limit 1
	`, command.StudentRecordID, command.LabID, command.NormalizedSHA256[:]).Scan(
		&existing.ID, &existing.VersionNumber, &keyValue, &digest, &existing.CreatedAt,
	)
	if err == nil {
		key, parseErr := appstorage.ParseKey(keyValue)
		if parseErr != nil {
			return appstorage.SubmissionVersion{}, false, fmt.Errorf("parse stored key: %w", parseErr)
		}
		existing.StudentRecordID = command.StudentRecordID
		existing.LabID = command.LabID
		existing.Object.Key = key
		copy(existing.Object.SHA256[:], digest)
		if err := tx.Commit(ctx); err != nil {
			return appstorage.SubmissionVersion{}, false, fmt.Errorf("commit duplicate lookup: %w", err)
		}
		return existing, true, nil
	}
	if err != pgx.ErrNoRows {
		return appstorage.SubmissionVersion{}, false, fmt.Errorf("find duplicate version: %w", err)
	}

	var versionNumber int
	if err := tx.QueryRow(ctx, `
		select coalesce(max(version_no), 0) + 1
		from submission_versions where student_record_id = $1 and lab_id = $2
	`, command.StudentRecordID, command.LabID).Scan(&versionNumber); err != nil {
		return appstorage.SubmissionVersion{}, false, fmt.Errorf("allocate version number: %w", err)
	}
	var createdAt appstorage.SubmissionVersion
	err = tx.QueryRow(ctx, `
		insert into submission_versions(
			id, student_record_id, lab_id, version_no, source, original_import_id,
			normalized_object_key, normalized_sha256, source_manifest, created_by
		) values ($1,$2,$3,$4,'teacher_import',$5,$6,$7,$8::jsonb,$9)
		returning created_at
	`, command.ID, command.StudentRecordID, command.LabID, versionNumber, command.ImportID,
		command.NormalizedKey.String(), command.NormalizedSHA256[:], string(command.SourceManifestJSON), command.CreatedBy,
	).Scan(&createdAt.CreatedAt)
	if err != nil {
		return appstorage.SubmissionVersion{}, false, fmt.Errorf("insert submission version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return appstorage.SubmissionVersion{}, false, fmt.Errorf("commit submission version: %w", err)
	}
	createdAt.ID = command.ID
	createdAt.StudentRecordID = command.StudentRecordID
	createdAt.LabID = command.LabID
	createdAt.VersionNumber = versionNumber
	createdAt.Object.Key = command.NormalizedKey
	createdAt.Object.SHA256 = command.NormalizedSHA256
	return createdAt, false, nil
}

func (repository *StorageRepository) ReadableSubmissionKey(ctx context.Context, actorUserID, submissionVersionID string) (appstorage.Key, error) {
	var keyValue string
	err := repository.db.pool.QueryRow(ctx, `
		select sv.normalized_object_key
		from submission_versions sv
		join student_records sr on sr.id = sv.student_record_id
		join users actor on actor.id = $1 and actor.status = 'active'
		where sv.id = $2 and (
			actor.role = 'admin'
			or (actor.role = 'student' and sr.user_id = actor.id)
			or (actor.role = 'teacher' and exists (
				select 1 from group_teachers gt where gt.group_id = sr.group_id and gt.teacher_user_id = actor.id
			))
		)
	`, actorUserID, submissionVersionID).Scan(&keyValue)
	if err == pgx.ErrNoRows {
		return appstorage.Key{}, appstorage.ErrAccessDenied
	}
	if err != nil {
		return appstorage.Key{}, fmt.Errorf("resolve readable submission: %w", err)
	}
	key, err := appstorage.ParseKey(keyValue)
	if err != nil {
		return appstorage.Key{}, fmt.Errorf("parse submission key: %w", err)
	}
	return key, nil
}

func (repository *StorageRepository) ListObjectReferences(ctx context.Context) ([]appstorage.Reference, error) {
	rows, err := repository.db.pool.Query(ctx, `
		select original_object_key, original_sha256 from imports
		union all select normalized_object_key, normalized_sha256 from submission_versions
		union all select safe_log_object_key, null::bytea from check_attempts where safe_log_object_key is not null
	`)
	if err != nil {
		return nil, fmt.Errorf("query object references: %w", err)
	}
	defer rows.Close()
	result := make([]appstorage.Reference, 0)
	for rows.Next() {
		var value string
		var digest []byte
		if err := rows.Scan(&value, &digest); err != nil {
			return nil, fmt.Errorf("scan object reference: %w", err)
		}
		key, err := appstorage.ParseKey(value)
		if err != nil {
			return nil, fmt.Errorf("parse object reference %q: %w", value, err)
		}
		reference := appstorage.Reference{Key: key, HasDigest: len(digest) != 0}
		if len(digest) != 0 && len(digest) != 32 {
			return nil, fmt.Errorf("object reference %q has invalid digest", value)
		}
		copy(reference.SHA256[:], digest)
		result = append(result, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate object references: %w", err)
	}
	return result, nil
}
