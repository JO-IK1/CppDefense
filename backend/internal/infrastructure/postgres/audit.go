package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type AuditRepository struct{ db *Database }

func NewAuditRepository(db *Database) *AuditRepository { return &AuditRepository{db: db} }

func (repository *AuditRepository) Append(ctx context.Context, event audit.Event) error {
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return err
	}
	tx, err := repository.db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", int64(43677068020201)); err != nil {
		return fmt.Errorf("lock audit chain: %w", err)
	}

	var previous []byte
	err = tx.QueryRow(ctx, `
		select event_hash from audit_events order by sequence_no desc limit 1 for update
	`).Scan(&previous)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("read audit chain: %w", err)
	}
	payload := fmt.Sprintf("%x|%s|%s|%s|%s|%s|%s", previous, event.OccurredAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), event.ActorKind, event.Action, event.TargetType, event.RequestID, metadata)
	digest := sha256.Sum256([]byte(payload))
	if _, err := tx.Exec(ctx, `
		insert into audit_events(
			id, occurred_at, actor_user_id, actor_kind, action, target_type,
			target_id, request_id, reason, metadata, prev_event_hash, event_hash
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, id, event.OccurredAt, event.ActorID, event.ActorKind, event.Action, event.TargetType,
		event.TargetID, event.RequestID, event.Reason, metadata, nullableBytes(previous), digest[:]); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return tx.Commit(ctx)
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
