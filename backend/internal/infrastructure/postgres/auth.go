package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type AuthRepository struct {
	db               *Database
	bootstrapAdminID int64
}

func NewAuthRepository(db *Database, bootstrapAdminID int64) *AuthRepository {
	return &AuthRepository{db: db, bootstrapAdminID: bootstrapAdminID}
}

func (repository *AuthRepository) CreateFlow(ctx context.Context, flow appauth.Flow) error {
	id, err := appauth.NewFlowID()
	if err != nil {
		return err
	}
	_, err = repository.db.pool.Exec(ctx, `
		insert into auth_flows(id, provider, state_hash, pkce_verifier_encrypted, return_path, expires_at)
		values($1, 'github', $2, $3, $4, $5)
	`, id, flow.StateHash, flow.EncryptedVerifier, flow.ReturnPath, flow.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create OAuth flow: %w", err)
	}
	return nil
}

func (repository *AuthRepository) ConsumeFlow(ctx context.Context, stateHash []byte) (appauth.Flow, error) {
	var flow appauth.Flow
	err := repository.db.pool.QueryRow(ctx, `
		update auth_flows set consumed_at = clock_timestamp()
		where state_hash = $1 and provider = 'github' and consumed_at is null and expires_at > clock_timestamp()
		returning state_hash, pkce_verifier_encrypted, return_path, expires_at
	`, stateHash).Scan(&flow.StateHash, &flow.EncryptedVerifier, &flow.ReturnPath, &flow.ExpiresAt)
	if err != nil {
		return appauth.Flow{}, err
	}
	return flow, nil
}

func (repository *AuthRepository) LoginGitHub(ctx context.Context, profile appauth.Profile) (appauth.User, error) {
	tx, err := repository.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return appauth.User{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	user, err := queryUserByGitHubID(ctx, tx, profile.ID)
	if err == nil {
		_, err = tx.Exec(ctx, `
			update github_identities set login=$2, display_name=$3, avatar_url=$4,
			verified_at=clock_timestamp(), updated_at=clock_timestamp() where github_user_id=$1
		`, profile.ID, strings.ToLower(profile.Login), nullableString(profile.DisplayName), nullableString(profile.AvatarURL))
		if err != nil {
			return appauth.User{}, err
		}
		if profile.ID == repository.bootstrapAdminID && user.Status != "blocked" && (user.Role == nil || *user.Role != "admin") {
			if _, err := tx.Exec(ctx, `update users set status='active',role='admin',rejection_reason=null where id=$1`, user.ID); err != nil {
				return appauth.User{}, err
			}
			admin := "admin"
			user.Status = "active"
			user.Role = &admin
		}
		if err := tx.Commit(ctx); err != nil {
			return appauth.User{}, err
		}
		user.Identity.Login = strings.ToLower(profile.Login)
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return appauth.User{}, err
	}

	rows, err := tx.Query(ctx, `
		select id from student_records
		where status='unclaimed' and github_login_expected=$1
		order by id for update
	`, strings.ToLower(profile.Login))
	if err != nil {
		return appauth.User{}, err
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return appauth.User{}, err
		}
		candidates = append(candidates, id)
	}
	rows.Close()

	userID, err := domain.NewUUIDv7()
	if err != nil {
		return appauth.User{}, err
	}
	identityID, err := domain.NewUUIDv7()
	if err != nil {
		return appauth.User{}, err
	}
	status := "pending"
	var role *string
	if profile.ID == repository.bootstrapAdminID {
		status = "active"
		admin := "admin"
		role = &admin
	} else if len(candidates) == 1 {
		status = "active"
		student := "student"
		role = &student
	}
	_, err = tx.Exec(ctx, `insert into users(id,status,role,display_name) values($1,$2,$3,$4)`, userID, status, role, nullableString(profile.DisplayName))
	if err != nil {
		return appauth.User{}, err
	}
	_, err = tx.Exec(ctx, `
		insert into github_identities(id,user_id,github_user_id,login,display_name,avatar_url,verified_at)
		values($1,$2,$3,$4,$5,$6,clock_timestamp())
	`, identityID, userID, profile.ID, strings.ToLower(profile.Login), nullableString(profile.DisplayName), nullableString(profile.AvatarURL))
	if err != nil {
		return appauth.User{}, err
	}
	if len(candidates) == 1 && profile.ID != repository.bootstrapAdminID {
		result, err := tx.Exec(ctx, `
			update student_records set user_id=$1,status='claimed',updated_at=clock_timestamp(),version=version+1
			where id=$2 and status='unclaimed' and user_id is null
		`, userID, candidates[0])
		if err != nil || result.RowsAffected() != 1 {
			return appauth.User{}, fmt.Errorf("claim student record: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return appauth.User{}, err
	}
	return repository.UserByID(ctx, userID)
}

func (repository *AuthRepository) UserByID(ctx context.Context, id string) (appauth.User, error) {
	return queryUser(ctx, repository.db.pool, id)
}

func (repository *AuthRepository) ApproveStudent(ctx context.Context, actorID, userID, studentRecordID, idempotencyKey string) (appauth.User, error) {
	tx, err := repository.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return appauth.User{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var actorRole, targetStatus, recordStatus, groupID string
	if err := tx.QueryRow(ctx, `select role::text from users where id=$1 and status='active' for update`, actorID).Scan(&actorRole); err != nil {
		return appauth.User{}, appauth.ErrForbidden
	}
	if actorRole != "admin" && actorRole != "teacher" {
		return appauth.User{}, appauth.ErrForbidden
	}
	replayUserID, replay, err := beginIdempotency(ctx, tx, actorID, "pending-link.approve", idempotencyKey, userID+"|"+studentRecordID)
	if err != nil {
		return appauth.User{}, err
	}
	if replay {
		return queryUser(ctx, tx, replayUserID)
	}
	if err := tx.QueryRow(ctx, `select status::text from users where id=$1 for update`, userID).Scan(&targetStatus); err != nil || targetStatus != "pending" {
		return appauth.User{}, appauth.ErrConflict
	}
	if err := tx.QueryRow(ctx, `select status::text,group_id from student_records where id=$1 for update`, studentRecordID).Scan(&recordStatus, &groupID); err != nil || recordStatus != "unclaimed" {
		return appauth.User{}, appauth.ErrConflict
	}
	if actorRole == "teacher" {
		var allowed bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from group_teachers where group_id=$1 and teacher_user_id=$2)`, groupID, actorID).Scan(&allowed); err != nil || !allowed {
			return appauth.User{}, appauth.ErrForbidden
		}
	}
	if _, err := tx.Exec(ctx, `update student_records set user_id=$1,status='claimed',updated_at=clock_timestamp(),version=version+1 where id=$2`, userID, studentRecordID); err != nil {
		return appauth.User{}, appauth.ErrConflict
	}
	if _, err := tx.Exec(ctx, `update users set status='active',role='student',rejection_reason=null where id=$1`, userID); err != nil {
		return appauth.User{}, appauth.ErrConflict
	}
	if err := finishIdempotency(ctx, tx, actorID, "pending-link.approve", idempotencyKey, userID); err != nil {
		return appauth.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return appauth.User{}, appauth.ErrConflict
	}
	return repository.UserByID(ctx, userID)
}

func (repository *AuthRepository) RejectUser(ctx context.Context, actorID, userID, reason, idempotencyKey string) (appauth.User, error) {
	tx, err := repository.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return appauth.User{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var actorRole, targetStatus, login string
	if err := tx.QueryRow(ctx, `select role::text from users where id=$1 and status='active' for update`, actorID).Scan(&actorRole); err != nil || (actorRole != "admin" && actorRole != "teacher") {
		return appauth.User{}, appauth.ErrForbidden
	}
	replayUserID, replay, err := beginIdempotency(ctx, tx, actorID, "pending-link.reject", idempotencyKey, userID+"|"+reason)
	if err != nil {
		return appauth.User{}, err
	}
	if replay {
		return queryUser(ctx, tx, replayUserID)
	}
	if err := tx.QueryRow(ctx, `select u.status::text,gi.login::text from users u join github_identities gi on gi.user_id=u.id where u.id=$1 for update of u,gi`, userID).Scan(&targetStatus, &login); err != nil || targetStatus != "pending" {
		return appauth.User{}, appauth.ErrConflict
	}
	if actorRole == "teacher" {
		var allowed bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from student_records sr join group_teachers gt on gt.group_id=sr.group_id where sr.github_login_expected=$1 and gt.teacher_user_id=$2)`, login, actorID).Scan(&allowed); err != nil || !allowed {
			return appauth.User{}, appauth.ErrForbidden
		}
	}
	if _, err := tx.Exec(ctx, `update users set status='rejected',role=null,rejection_reason=$2 where id=$1`, userID, reason); err != nil {
		return appauth.User{}, appauth.ErrConflict
	}
	if err := finishIdempotency(ctx, tx, actorID, "pending-link.reject", idempotencyKey, userID); err != nil {
		return appauth.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return appauth.User{}, appauth.ErrConflict
	}
	return repository.UserByID(ctx, userID)
}

func beginIdempotency(ctx context.Context, tx pgx.Tx, actorID, route, key, payload string) (string, bool, error) {
	digest := sha256.Sum256([]byte(payload))
	var storedHash []byte
	var responseUserID *string
	err := tx.QueryRow(ctx, `
		select request_hash,response_reference->>'user_id'
		from idempotency_records
		where principal_id=$1 and http_method='POST' and canonical_route=$2 and idempotency_key=$3
		for update
	`, actorID, route, key).Scan(&storedHash, &responseUserID)
	if err == nil {
		if string(storedHash) != string(digest[:]) || responseUserID == nil {
			return "", false, appauth.ErrConflict
		}
		return *responseUserID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return "", false, err
	}
	_, err = tx.Exec(ctx, `
		insert into idempotency_records(id,principal_id,http_method,canonical_route,idempotency_key,request_hash,expires_at)
		values($1,$2,'POST',$3,$4,$5,clock_timestamp()+interval '24 hours')
	`, id, actorID, route, key, digest[:])
	return "", false, err
}

func finishIdempotency(ctx context.Context, tx pgx.Tx, actorID, route, key, userID string) error {
	_, err := tx.Exec(ctx, `
		update idempotency_records set response_status=200,response_reference=jsonb_build_object('user_id',$4::text)
		where principal_id=$1 and http_method='POST' and canonical_route=$2 and idempotency_key=$3
	`, actorID, route, key, userID)
	return err
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func queryUserByGitHubID(ctx context.Context, query rowQuerier, githubID int64) (appauth.User, error) {
	var id string
	if err := query.QueryRow(ctx, `select user_id from github_identities where github_user_id=$1`, githubID).Scan(&id); err != nil {
		return appauth.User{}, err
	}
	return queryUser(ctx, query, id)
}

func queryUser(ctx context.Context, query rowQuerier, id string) (appauth.User, error) {
	var user appauth.User
	var githubID int64
	err := query.QueryRow(ctx, `
		select u.id,u.status,u.role,u.display_name,u.created_at,
		       gi.github_user_id,gi.login,gi.verified_at
		from users u join github_identities gi on gi.user_id=u.id where u.id=$1
	`, id).Scan(&user.ID, &user.Status, &user.Role, &user.DisplayName, &user.CreatedAt,
		&githubID, &user.Identity.Login, &user.Identity.VerifiedAt)
	user.Identity.Subject = strconv.FormatInt(githubID, 10)
	return user, err
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
