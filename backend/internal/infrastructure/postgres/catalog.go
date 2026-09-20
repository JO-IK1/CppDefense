package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
)

type CatalogRepository struct{ db *Database }

func NewCatalogRepository(db *Database) *CatalogRepository { return &CatalogRepository{db: db} }

type Group struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsDemo    bool      `json:"is_demo"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
type Lab struct {
	ID               string    `json:"id"`
	GroupID          string    `json:"group_id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Description      *string   `json:"description"`
	TimeLimitSeconds int       `json:"time_limit_seconds"`
	TopN             int       `json:"top_n"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

type Submission struct {
	ID            string    `json:"id"`
	LabID         string    `json:"lab_id"`
	LabCode       string    `json:"lab_code"`
	LabName       string    `json:"lab_name"`
	VersionNumber int       `json:"version_number"`
	CreatedAt     time.Time `json:"created_at"`
}
type GroupSubmission struct {
	Submission
	StudentRecordID string `json:"student_record_id"`
	GitHubLogin     string `json:"github_login"`
}

func (r *CatalogRepository) StudentSubmissions(ctx context.Context, actor string) ([]Submission, error) {
	rows, e := r.db.pool.Query(ctx, `select sv.id,sv.lab_id,l.code::text,l.name,sv.version_no,sv.created_at from submission_versions sv join student_records sr on sr.id=sv.student_record_id join labs l on l.id=sv.lab_id join users u on u.id=$1 and u.status='active' and u.role='student' where sr.user_id=u.id order by l.code,sv.version_no desc`, actor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Submission
	for rows.Next() {
		var v Submission
		if e = rows.Scan(&v.ID, &v.LabID, &v.LabCode, &v.LabName, &v.VersionNumber, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *CatalogRepository) SubmissionsByRecord(ctx context.Context, actor, record string) ([]Submission, error) {
	var admin bool
	if e := r.db.pool.QueryRow(ctx, `select exists(select 1 from users where id=$1 and status='active' and role='admin')`, actor).Scan(&admin); e != nil || !admin {
		return nil, appauth.ErrForbidden
	}
	rows, e := r.db.pool.Query(ctx, `select sv.id,sv.lab_id,l.code::text,l.name,sv.version_no,sv.created_at from submission_versions sv join labs l on l.id=sv.lab_id join student_records sr on sr.id=sv.student_record_id join groups g on g.id=sr.group_id where sr.id=$1 and g.is_demo order by l.code,sv.version_no desc`, record)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Submission
	for rows.Next() {
		var v Submission
		if e = rows.Scan(&v.ID, &v.LabID, &v.LabCode, &v.LabName, &v.VersionNumber, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *CatalogRepository) GroupSubmissions(ctx context.Context, actor, group string) ([]GroupSubmission, error) {
	ok, e := r.canManage(ctx, actor, group)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, appauth.ErrForbidden
	}
	rows, e := r.db.pool.Query(ctx, `select sv.id,sv.lab_id,l.code::text,l.name,sv.version_no,sv.created_at,sr.id,sr.github_login_expected::text from submission_versions sv join student_records sr on sr.id=sv.student_record_id join labs l on l.id=sv.lab_id where sr.group_id=$1 order by sr.github_login_expected,l.code,sv.version_no desc`, group)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []GroupSubmission
	for rows.Next() {
		var v GroupSubmission
		if e = rows.Scan(&v.ID, &v.LabID, &v.LabCode, &v.LabName, &v.VersionNumber, &v.CreatedAt, &v.StudentRecordID, &v.GitHubLogin); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *CatalogRepository) ListGroups(ctx context.Context, actor string) ([]Group, error) {
	rows, e := r.db.pool.Query(ctx, `select g.id,g.code::text,g.name,g.is_demo,g.status::text,g.created_at from groups g join users u on u.id=$1 and u.status='active' where u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=g.id and gt.teacher_user_id=u.id)) order by g.code`, actor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var v Group
		if e = rows.Scan(&v.ID, &v.Code, &v.Name, &v.IsDemo, &v.Status, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *CatalogRepository) GroupForAdminView(ctx context.Context, actor, group string) ([]Group, error) {
	var v Group
	e := r.db.pool.QueryRow(ctx, `select g.id,g.code::text,g.name,g.is_demo,g.status::text,g.created_at from groups g join users u on u.id=$1 and u.status='active' and u.role='admin' where g.id=$2`, actor, group).Scan(&v.ID, &v.Code, &v.Name, &v.IsDemo, &v.Status, &v.CreatedAt)
	if e != nil {
		return nil, appauth.ErrForbidden
	}
	return []Group{v}, nil
}
func (r *CatalogRepository) CreateGroup(ctx context.Context, actor, code, name string, demo bool) (Group, error) {
	var role string
	if e := r.db.pool.QueryRow(ctx, `select role::text from users where id=$1 and status='active'`, actor).Scan(&role); e != nil || role != "admin" {
		return Group{}, appauth.ErrForbidden
	}
	id, e := domain.NewUUIDv7()
	if e != nil {
		return Group{}, e
	}
	var v Group
	e = r.db.pool.QueryRow(ctx, `insert into groups(id,code,name,is_demo)values($1,$2,$3,$4)returning id,code::text,name,is_demo,status::text,created_at`, id, strings.ToLower(code), strings.TrimSpace(name), demo).Scan(&v.ID, &v.Code, &v.Name, &v.IsDemo, &v.Status, &v.CreatedAt)
	if e != nil {
		return Group{}, fmt.Errorf("create group: %w", e)
	}
	return v, nil
}
func (r *CatalogRepository) ListLabs(ctx context.Context, actor, group string) ([]Lab, error) {
	ok, e := r.canRead(ctx, actor, group)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, appauth.ErrForbidden
	}
	rows, e := r.db.pool.Query(ctx, `select id,group_id,code::text,name,description,time_limit_seconds,top_n,status::text,created_at from labs where group_id=$1 order by code`, group)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Lab
	for rows.Next() {
		var v Lab
		if e = rows.Scan(&v.ID, &v.GroupID, &v.Code, &v.Name, &v.Description, &v.TimeLimitSeconds, &v.TopN, &v.Status, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *CatalogRepository) CreateLab(ctx context.Context, actor, group, code, name, description string, seconds, topN int) (Lab, error) {
	ok, e := r.canManage(ctx, actor, group)
	if e != nil {
		return Lab{}, e
	}
	if !ok {
		return Lab{}, appauth.ErrForbidden
	}
	id, e := domain.NewUUIDv7()
	if e != nil {
		return Lab{}, e
	}
	var v Lab
	e = r.db.pool.QueryRow(ctx, `insert into labs(id,group_id,code,name,description,time_limit_seconds,top_n)values($1,$2,$3,$4,$5,$6,$7)returning id,group_id,code::text,name,description,time_limit_seconds,top_n,status::text,created_at`, id, group, strings.ToLower(code), strings.TrimSpace(name), nullableString(description), seconds, topN).Scan(&v.ID, &v.GroupID, &v.Code, &v.Name, &v.Description, &v.TimeLimitSeconds, &v.TopN, &v.Status, &v.CreatedAt)
	if e != nil {
		return Lab{}, fmt.Errorf("create lab: %w", e)
	}
	return v, nil
}
func (r *CatalogRepository) canManage(ctx context.Context, actor, group string) (bool, error) {
	var ok bool
	e := r.db.pool.QueryRow(ctx, `select exists(select 1 from users u where u.id=$1 and u.status='active' and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=$2 and gt.teacher_user_id=u.id))))`, actor, group).Scan(&ok)
	return ok, e
}
func (r *CatalogRepository) canRead(ctx context.Context, actor, group string) (bool, error) {
	var ok bool
	e := r.db.pool.QueryRow(ctx, `select exists(select 1 from users u where u.id=$1 and u.status='active' and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=$2 and gt.teacher_user_id=u.id)) or (u.role='student' and exists(select 1 from student_records sr where sr.group_id=$2 and sr.user_id=u.id))))`, actor, group).Scan(&ok)
	return ok, e
}
