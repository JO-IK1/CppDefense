package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/security"
	"github.com/jackc/pgx/v5"
)

type SessionRepository struct {
	db      *Database
	hashKey []byte
	csrfKey []byte
}

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type NewSession struct {
	Session
	Token     string
	CSRFToken string
}

func NewSessionRepository(db *Database, hashKey, csrfKey []byte) *SessionRepository {
	return &SessionRepository{db: db, hashKey: hashKey, csrfKey: csrfKey}
}

func (repository *SessionRepository) Create(ctx context.Context, userID string, ttl time.Duration) (NewSession, error) {
	id, err := domain.NewUUIDv7()
	if err != nil {
		return NewSession{}, err
	}
	token, err := security.NewToken()
	if err != nil {
		return NewSession{}, fmt.Errorf("session token: %w", err)
	}
	csrfToken, csrfHash, err := security.NewCSRFToken(repository.csrfKey, id)
	if err != nil {
		return NewSession{}, fmt.Errorf("csrf token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(ttl)
	_, err = repository.db.pool.Exec(ctx, `
		insert into web_sessions(id, user_id, token_hash, csrf_secret_hash, expires_at)
		values ($1, $2, $3, $4, $5)
	`, id, userID, security.HashToken(repository.hashKey, token), csrfHash, expiresAt)
	if err != nil {
		return NewSession{}, fmt.Errorf("insert session: %w", err)
	}
	return NewSession{Session: Session{ID: id, UserID: userID, ExpiresAt: expiresAt}, Token: token, CSRFToken: csrfToken}, nil
}

func (repository *SessionRepository) FindActive(ctx context.Context, token string) (Session, []byte, error) {
	var session Session
	var csrfHash []byte
	err := repository.db.pool.QueryRow(ctx, `
		select id, user_id, expires_at, revoked_at, csrf_secret_hash
		from web_sessions
		where token_hash = $1 and revoked_at is null and expires_at > clock_timestamp()
	`, security.HashToken(repository.hashKey, token)).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.RevokedAt, &csrfHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Session{}, nil, err
		}
		return Session{}, nil, fmt.Errorf("find session: %w", err)
	}
	return session, csrfHash, nil
}

func (repository *SessionRepository) Revoke(ctx context.Context, sessionID string) error {
	_, err := repository.db.pool.Exec(ctx, `
		update web_sessions set revoked_at = coalesce(revoked_at, clock_timestamp()) where id = $1
	`, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
