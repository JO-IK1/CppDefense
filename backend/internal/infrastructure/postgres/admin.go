package postgres

import (
	"context"
	"fmt"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
)

type AdminRepository struct{ db *Database }

func NewAdminRepository(db *Database) *AdminRepository { return &AdminRepository{db: db} }

type ManagedUser struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Role        *string   `json:"role"`
	DisplayName *string   `json:"display_name"`
	GitHubID    int64     `json:"github_id"`
	GitHubLogin string    `json:"github_login"`
	CreatedAt   time.Time `json:"created_at"`
}

type PendingLink struct {
	UserID          string  `json:"user_id"`
	DisplayName     *string `json:"display_name"`
	GitHubLogin     string  `json:"github_login"`
	StudentRecordID *string `json:"student_record_id"`
	GroupID         *string `json:"group_id"`
	GroupCode       *string `json:"group_code"`
}

func (r *AdminRepository) ListPendingLinks(ctx context.Context, actor string) ([]PendingLink, error) {
	rows, e := r.db.pool.Query(ctx, `
		select u.id,u.display_name,gi.login::text,sr.id,sr.group_id,g.code::text
		from users u
		join github_identities gi on gi.user_id=u.id
		join users actor on actor.id=$1 and actor.status='active' and actor.role in('teacher','admin')
		left join student_records sr on sr.user_id is null and sr.status='unclaimed' and sr.github_login_expected=gi.login
		  and (actor.role='admin' or exists(select 1 from group_teachers gt where gt.group_id=sr.group_id and gt.teacher_user_id=actor.id))
		left join groups g on g.id=sr.group_id
		where u.status='pending' and (actor.role='admin' or sr.id is not null)
		order by gi.login,g.code`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []PendingLink
	for rows.Next() {
		var v PendingLink
		if e = rows.Scan(&v.UserID, &v.DisplayName, &v.GitHubLogin, &v.StudentRecordID, &v.GroupID, &v.GroupCode); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *AdminRepository) ListUsers(ctx context.Context, actor string) ([]ManagedUser, error) {
	if ok, _ := r.isAdmin(ctx, actor); !ok {
		return nil, appauth.ErrForbidden
	}
	rows, e := r.db.pool.Query(ctx, `select u.id,u.status::text,u.role::text,u.display_name,gi.github_user_id,gi.login::text,u.created_at from users u join github_identities gi on gi.user_id=u.id order by u.created_at desc limit 500`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []ManagedUser
	for rows.Next() {
		var v ManagedUser
		if e = rows.Scan(&v.ID, &v.Status, &v.Role, &v.DisplayName, &v.GitHubID, &v.GitHubLogin, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *AdminRepository) SetRole(ctx context.Context, actor, target, role string) (ManagedUser, error) {
	if ok, _ := r.isAdmin(ctx, actor); !ok {
		return ManagedUser{}, appauth.ErrForbidden
	}
	if role != "student" && role != "teacher" && role != "admin" {
		return ManagedUser{}, fmt.Errorf("invalid role")
	}
	_, e := r.db.pool.Exec(ctx, `update users set status='active',role=$2,rejection_reason=null,blocked_at=null where id=$1 and status in('pending','active','rejected','blocked')`, target, role)
	if e != nil {
		return ManagedUser{}, e
	}
	return r.user(ctx, target)
}
func (r *AdminRepository) Block(ctx context.Context, actor, target, reason string) (ManagedUser, error) {
	if ok, _ := r.isAdmin(ctx, actor); !ok || actor == target {
		return ManagedUser{}, appauth.ErrForbidden
	}
	tx, e := r.db.pool.Begin(ctx)
	if e != nil {
		return ManagedUser{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	result, e := tx.Exec(ctx, `update users set status='blocked',blocked_at=clock_timestamp(),rejection_reason=null where id=$1 and status<>'blocked'`, target)
	if e != nil || result.RowsAffected() != 1 {
		return ManagedUser{}, appauth.ErrConflict
	}
	_, e = tx.Exec(ctx, `update web_sessions set revoked_at=coalesce(revoked_at,clock_timestamp()) where user_id=$1`, target)
	if e != nil {
		return ManagedUser{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return ManagedUser{}, e
	}
	_ = reason
	return r.user(ctx, target)
}
func (r *AdminRepository) AssignTeacher(ctx context.Context, actor, user, group string) error {
	if ok, _ := r.isAdmin(ctx, actor); !ok {
		return appauth.ErrForbidden
	}
	result, e := r.db.pool.Exec(ctx, `insert into group_teachers(group_id,teacher_user_id,assigned_by) select $1,u.id,$3 from users u where u.id=$2 and u.status='active' and u.role in('teacher','admin') on conflict do nothing`, group, user, actor)
	if e != nil || result.RowsAffected() != 1 {
		return appauth.ErrConflict
	}
	return nil
}
func (r *AdminRepository) isAdmin(ctx context.Context, id string) (bool, error) {
	var ok bool
	e := r.db.pool.QueryRow(ctx, `select exists(select 1 from users where id=$1 and status='active' and role='admin')`, id).Scan(&ok)
	return ok, e
}
func (r *AdminRepository) user(ctx context.Context, id string) (ManagedUser, error) {
	var v ManagedUser
	e := r.db.pool.QueryRow(ctx, `select u.id,u.status::text,u.role::text,u.display_name,gi.github_user_id,gi.login::text,u.created_at from users u join github_identities gi on gi.user_id=u.id where u.id=$1`, id).Scan(&v.ID, &v.Status, &v.Role, &v.DisplayName, &v.GitHubID, &v.GitHubLogin, &v.CreatedAt)
	return v, e
}
