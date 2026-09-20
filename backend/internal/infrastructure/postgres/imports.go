package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/application/importer"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type ImportRepository struct{ db *Database }

func NewImportRepository(db *Database) *ImportRepository { return &ImportRepository{db: db} }

type Import struct {
	ID               string       `json:"id"`
	GroupID          string       `json:"group_id"`
	Kind             string       `json:"kind"`
	State            string       `json:"state"`
	OriginalSHA256   string       `json:"original_sha256"`
	CompressedSize   int64        `json:"compressed_size"`
	UncompressedSize int64        `json:"uncompressed_size"`
	CreatedAt        time.Time    `json:"created_at"`
	Items            []ImportItem `json:"items"`
}
type ImportItem struct {
	ID                  string  `json:"id"`
	StudentRecordID     string  `json:"student_record_id"`
	LabID               string  `json:"lab_id"`
	GitHubLogin         string  `json:"github_login"`
	ProjectPath         string  `json:"project_path"`
	Action              string  `json:"action"`
	FileCount           int     `json:"file_count"`
	UncompressedSize    int64   `json:"uncompressed_size"`
	SubmissionVersionID *string `json:"submission_version_id"`
}

func (r *ImportRepository) CreateReview(ctx context.Context, actor, groupID, kind string, object appstorage.Object, analysis importer.Analysis) (Import, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return Import{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var code string
	var allowed bool
	e = tx.QueryRow(ctx, `select g.code::text,exists(select 1 from users u where u.id=$1 and u.status='active' and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=g.id and gt.teacher_user_id=u.id)))) from groups g where g.id=$2 for update`, actor, groupID).Scan(&code, &allowed)
	if e != nil || !allowed {
		return Import{}, appauth.ErrForbidden
	}
	if !strings.EqualFold(code, analysis.Manifest.GroupCode) {
		return Import{}, fmt.Errorf("manifest group mismatch")
	}
	id, e := domain.NewUUIDv7()
	if e != nil {
		return Import{}, e
	}
	_, e = tx.Exec(ctx, `insert into imports(id,group_id,kind,uploaded_by,original_object_key,original_sha256,compressed_size,uncompressed_size,schema_version)values($1,$2,$3,$4,$5,$6,$7,$8,1)`, id, groupID, kind, actor, object.Key.String(), object.SHA256[:], object.Size, analysis.TotalSize)
	if e != nil {
		return Import{}, e
	}
	for _, state := range []string{"stored", "validating"} {
		if _, e = tx.Exec(ctx, `update imports set state=$2 where id=$1`, id, state); e != nil {
			return Import{}, e
		}
	}
	for _, project := range analysis.Projects {
		var labID string
		e = tx.QueryRow(ctx, `select id from labs where group_id=$1 and code=$2 and status='active'`, groupID, project.Item.LabCode).Scan(&labID)
		if e != nil {
			return Import{}, fmt.Errorf("unknown lab %s", project.Item.LabCode)
		}
		var studentID string
		created := false
		e = tx.QueryRow(ctx, `select id from student_records where group_id=$1 and github_login_expected=$2 and status<>'archived'`, groupID, strings.ToLower(project.Item.Student.GitHubLogin)).Scan(&studentID)
		if e == pgx.ErrNoRows {
			studentID, e = domain.NewUUIDv7()
			if e == nil {
				_, e = tx.Exec(ctx, `insert into student_records(id,group_id,github_login_expected,created_source)values($1,$2,$3,'group_import')`, studentID, groupID, strings.ToLower(project.Item.Student.GitHubLogin))
				created = true
			}
		}
		if e != nil {
			return Import{}, e
		}
		itemID, _ := domain.NewUUIDv7()
		action := "create_version"
		if created {
			action = "create_student_and_version"
		}
		_, e = tx.Exec(ctx, `insert into import_items(id,import_id,student_record_id,lab_id,github_login_input,project_path,action,file_count,uncompressed_size)values($1,$2,$3,$4,$5,$6,$7,$8,$9)`, itemID, id, studentID, labID, project.Item.Student.GitHubLogin, project.Item.ProjectPath, action, project.FileCount, project.Size)
		if e != nil {
			return Import{}, e
		}
	}
	if _, e = tx.Exec(ctx, `update imports set state='review_pending' where id=$1`, id); e != nil {
		return Import{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Import{}, e
	}
	return r.Get(ctx, actor, id)
}

func (r *ImportRepository) Get(ctx context.Context, actor, id string) (Import, error) {
	var v Import
	var digest []byte
	e := r.db.pool.QueryRow(ctx, `select i.id,i.group_id,i.kind::text,i.state::text,i.original_sha256,i.compressed_size,coalesce(i.uncompressed_size,0),i.created_at from imports i join users u on u.id=$1 and u.status='active' where i.id=$2 and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=i.group_id and gt.teacher_user_id=u.id)))`, actor, id).Scan(&v.ID, &v.GroupID, &v.Kind, &v.State, &digest, &v.CompressedSize, &v.UncompressedSize, &v.CreatedAt)
	if e != nil {
		return Import{}, appauth.ErrForbidden
	}
	v.OriginalSHA256 = fmt.Sprintf("%x", digest)
	rows, e := r.db.pool.Query(ctx, `select id,student_record_id,lab_id,github_login_input::text,project_path,action::text,coalesce(file_count,0),coalesce(uncompressed_size,0),submission_version_id from import_items where import_id=$1 order by project_path`, id)
	if e != nil {
		return Import{}, e
	}
	defer rows.Close()
	for rows.Next() {
		var x ImportItem
		if e = rows.Scan(&x.ID, &x.StudentRecordID, &x.LabID, &x.GitHubLogin, &x.ProjectPath, &x.Action, &x.FileCount, &x.UncompressedSize, &x.SubmissionVersionID); e != nil {
			return Import{}, e
		}
		v.Items = append(v.Items, x)
	}
	return v, rows.Err()
}

func (r *ImportRepository) Review(ctx context.Context, actor, id, decision, sha string, checklist map[string]any, reason string) (Import, error) {
	raw, e := json.Marshal(checklist)
	if e != nil {
		return Import{}, e
	}
	var allowed bool
	e = r.db.pool.QueryRow(ctx, `select exists(select 1 from imports i join users u on u.id=$1 and u.status='active' where i.id=$2 and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=i.group_id and gt.teacher_user_id=u.id))))`, actor, id).Scan(&allowed)
	if e != nil || !allowed {
		return Import{}, appauth.ErrForbidden
	}
	digest, e := hex.DecodeString(sha)
	if e != nil || len(digest) != 32 {
		return Import{}, fmt.Errorf("invalid digest")
	}
	state := "approved"
	if decision == "reject" {
		state = "rejected"
		if strings.TrimSpace(reason) == "" {
			return Import{}, fmt.Errorf("reason required")
		}
	}
	result, e := r.db.pool.Exec(ctx, `update imports set state=$3,reviewed_by=$1,reviewed_at=clock_timestamp(),reviewed_sha256=$4,review_checklist=$5::jsonb,rejection_reason=$6 where id=$2 and state='review_pending' and original_sha256=$4`, actor, id, state, digest, string(raw), nullableString(reason))
	if e != nil {
		return Import{}, e
	}
	if result.RowsAffected() != 1 {
		return Import{}, appauth.ErrConflict
	}
	return r.Get(ctx, actor, id)
}

type ApplySource struct {
	Import    Import
	ObjectKey appstorage.Key
}

type ApplyItem struct {
	ItemID             string
	StudentRecordID    string
	LabID              string
	ObjectKey          appstorage.Key
	SHA256             [32]byte
	SourceManifestJSON []byte
}

func (r *ImportRepository) CommitApply(ctx context.Context, actor, importID string, items []ApplyItem) ([]appstorage.Key, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state string
	if e = tx.QueryRow(ctx, `select state::text from imports where id=$1 for update`, importID).Scan(&state); e != nil || state != "applying" {
		return nil, appauth.ErrConflict
	}
	var duplicateObjects []appstorage.Key
	for _, item := range items {
		if !json.Valid(item.SourceManifestJSON) {
			return nil, fmt.Errorf("invalid source manifest")
		}
		lockKey := item.StudentRecordID + ":" + item.LabID
		if _, e = tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); e != nil {
			return nil, e
		}
		var versionID string
		e = tx.QueryRow(ctx, `select id from submission_versions where student_record_id=$1 and lab_id=$2 and normalized_sha256=$3 order by version_no desc limit 1`, item.StudentRecordID, item.LabID, item.SHA256[:]).Scan(&versionID)
		duplicate := e == nil
		if e != nil && e != pgx.ErrNoRows {
			return nil, e
		}
		if duplicate {
			duplicateObjects = append(duplicateObjects, item.ObjectKey)
		} else {
			var versionNumber int
			if e = tx.QueryRow(ctx, `select coalesce(max(version_no),0)+1 from submission_versions where student_record_id=$1 and lab_id=$2`, item.StudentRecordID, item.LabID).Scan(&versionNumber); e != nil {
				return nil, e
			}
			versionID, e = domain.NewUUIDv7()
			if e != nil {
				return nil, e
			}
			_, e = tx.Exec(ctx, `insert into submission_versions(id,student_record_id,lab_id,version_no,source,original_import_id,normalized_object_key,normalized_sha256,source_manifest,created_by)values($1,$2,$3,$4,'teacher_import',$5,$6,$7,$8::jsonb,$9)`, versionID, item.StudentRecordID, item.LabID, versionNumber, importID, item.ObjectKey.String(), item.SHA256[:], string(item.SourceManifestJSON), actor)
			if e != nil {
				return nil, e
			}
		}
		action := "create_version"
		if duplicate {
			action = "skip_duplicate"
		}
		result, updateErr := tx.Exec(ctx, `update import_items set submission_version_id=$3,action=$4 where id=$1 and import_id=$2 and submission_version_id is null`, item.ItemID, importID, versionID, action)
		if updateErr != nil || result.RowsAffected() != 1 {
			return nil, appauth.ErrConflict
		}
	}
	result, e := tx.Exec(ctx, `update imports set state='completed' where id=$1 and state='applying'`, importID)
	if e != nil || result.RowsAffected() != 1 {
		return nil, appauth.ErrConflict
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return duplicateObjects, nil
}

func (r *ImportRepository) BeginApply(ctx context.Context, actor, id string) (ApplySource, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return ApplySource{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var key string
	var kind string
	var group string
	var allowed bool
	e = tx.QueryRow(ctx, `select i.original_object_key,i.kind::text,i.group_id,exists(select 1 from users u where u.id=$1 and u.status='active' and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=i.group_id and gt.teacher_user_id=u.id)))) from imports i where i.id=$2 and i.state='approved' for update`, actor, id).Scan(&key, &kind, &group, &allowed)
	if e != nil || !allowed {
		return ApplySource{}, appauth.ErrForbidden
	}
	result, e := tx.Exec(ctx, `update imports set state='applying' where id=$1 and state='approved'`, id)
	if e != nil {
		return ApplySource{}, e
	}
	if result.RowsAffected() != 1 {
		return ApplySource{}, appauth.ErrConflict
	}
	if e = tx.Commit(ctx); e != nil {
		return ApplySource{}, e
	}
	parsed, e := appstorage.ParseKey(key)
	return ApplySource{Import: Import{ID: id, GroupID: group, Kind: kind}, ObjectKey: parsed}, e
}
func (r *ImportRepository) ItemTarget(ctx context.Context, importID, path string) (string, string, string, error) {
	var item, student, lab string
	e := r.db.pool.QueryRow(ctx, `select id,student_record_id,lab_id from import_items where import_id=$1 and project_path=$2`, importID, path).Scan(&item, &student, &lab)
	return item, student, lab, e
}
func (r *ImportRepository) AttachVersion(ctx context.Context, item, version string) error {
	_, e := r.db.pool.Exec(ctx, `update import_items set submission_version_id=$2 where id=$1 and submission_version_id is null`, item, version)
	return e
}
func (r *ImportRepository) Finish(ctx context.Context, id string, success bool) error {
	state := "completed"
	if !success {
		state = "failed"
	}
	_, e := r.db.pool.Exec(ctx, `update imports set state=$2 where id=$1 and state='applying'`, id, state)
	return e
}
