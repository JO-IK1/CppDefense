package postgres

import (
	"context"
	"fmt"
	"strings"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/application/roster"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type RosterPreview struct {
	Organization roster.Organization  `json:"organization"`
	Groups       []RosterGroupPreview `json:"groups"`
	Summary      RosterSummary        `json:"summary"`
}

type RosterSummary struct {
	GroupsCreated   int `json:"groups_created"`
	GroupsUpdated   int `json:"groups_updated"`
	StudentsCreated int `json:"students_created"`
	StudentsUpdated int `json:"students_updated"`
	Unchanged       int `json:"unchanged"`
}

type RosterGroupPreview struct {
	Code     string                 `json:"code"`
	Name     string                 `json:"name"`
	Action   string                 `json:"action"`
	Students []RosterStudentPreview `json:"students"`
}

type RosterStudentPreview struct {
	FullName    string `json:"full_name"`
	GitHubLogin string `json:"github_login"`
	Action      string `json:"action"`
}

func (r *AdminRepository) PreviewRoster(ctx context.Context, actor string, document roster.Document) (RosterPreview, error) {
	if ok, err := r.canManageRosters(ctx, actor); err != nil || !ok {
		return RosterPreview{}, appauth.ErrForbidden
	}
	preview := RosterPreview{Organization: document.Organization, Groups: make([]RosterGroupPreview, 0, len(document.Groups))}
	for _, inputGroup := range document.Groups {
		groupPreview := RosterGroupPreview{Code: inputGroup.Code, Name: inputGroup.Name, Action: "create", Students: make([]RosterStudentPreview, 0, len(inputGroup.Students))}
		var currentName string
		err := r.db.pool.QueryRow(ctx, `select name from groups where code=$1`, inputGroup.Code).Scan(&currentName)
		if err == nil {
			if currentName == inputGroup.Name {
				groupPreview.Action = "unchanged"
			} else {
				groupPreview.Action = "update"
			}
		} else if err != pgx.ErrNoRows {
			return RosterPreview{}, err
		}
		if groupPreview.Action == "create" {
			preview.Summary.GroupsCreated++
		} else if groupPreview.Action == "update" {
			preview.Summary.GroupsUpdated++
		}
		for _, inputStudent := range inputGroup.Students {
			studentPreview := RosterStudentPreview{FullName: inputStudent.FullName, GitHubLogin: inputStudent.GitHubLogin, Action: "create"}
			var currentName, currentGroup string
			err = r.db.pool.QueryRow(ctx, `select coalesce(sr.full_name,''),g.code::text from student_records sr join groups g on g.id=sr.group_id where sr.github_login_expected=$1 and sr.status<>'archived' order by sr.updated_at desc limit 1`, inputStudent.GitHubLogin).Scan(&currentName, &currentGroup)
			if err == nil {
				if currentName == inputStudent.FullName && strings.EqualFold(currentGroup, inputGroup.Code) {
					studentPreview.Action = "unchanged"
					preview.Summary.Unchanged++
				} else {
					studentPreview.Action = "update"
					preview.Summary.StudentsUpdated++
				}
			} else if err == pgx.ErrNoRows {
				preview.Summary.StudentsCreated++
			} else {
				return RosterPreview{}, err
			}
			groupPreview.Students = append(groupPreview.Students, studentPreview)
		}
		preview.Groups = append(preview.Groups, groupPreview)
	}
	return preview, nil
}

func (r *AdminRepository) ApplyRoster(ctx context.Context, actor string, document roster.Document) (RosterPreview, error) {
	if ok, err := r.canManageRosters(ctx, actor); err != nil || !ok {
		return RosterPreview{}, appauth.ErrForbidden
	}
	preview, err := r.PreviewRoster(ctx, actor, document)
	if err != nil {
		return RosterPreview{}, err
	}
	tx, err := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RosterPreview{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	organizationID, err := domain.NewUUIDv7()
	if err != nil {
		return RosterPreview{}, err
	}
	err = tx.QueryRow(ctx, `
		insert into organizations(id,github_login,name) values($1,$2,$3)
		on conflict(github_login) do update set name=excluded.name,updated_at=clock_timestamp()
		returning id`, organizationID, document.Organization.GitHubLogin, document.Organization.Name).Scan(&organizationID)
	if err != nil {
		return RosterPreview{}, fmt.Errorf("upsert organization: %w", err)
	}

	for _, inputGroup := range document.Groups {
		groupID, idErr := domain.NewUUIDv7()
		if idErr != nil {
			return RosterPreview{}, idErr
		}
		err = tx.QueryRow(ctx, `
			insert into groups(id,organization_id,code,name) values($1,$2,$3,$4)
			on conflict(code) do update set organization_id=excluded.organization_id,name=excluded.name,status='active',archived_at=null,updated_at=clock_timestamp(),version=groups.version+1
			returning id`, groupID, organizationID, inputGroup.Code, inputGroup.Name).Scan(&groupID)
		if err != nil {
			return RosterPreview{}, fmt.Errorf("upsert group %s: %w", inputGroup.Code, err)
		}
		if _, err = tx.Exec(ctx, `insert into group_teachers(group_id,teacher_user_id,assigned_by) values($1,$2,$2) on conflict do nothing`, groupID, actor); err != nil {
			return RosterPreview{}, fmt.Errorf("assign group teacher: %w", err)
		}
		for _, inputStudent := range inputGroup.Students {
			var recordID string
			err = tx.QueryRow(ctx, `select id from student_records where github_login_expected=$1 and status<>'archived' order by updated_at desc limit 1 for update`, inputStudent.GitHubLogin).Scan(&recordID)
			if err == pgx.ErrNoRows {
				recordID, err = domain.NewUUIDv7()
				if err == nil {
					_, err = tx.Exec(ctx, `insert into student_records(id,group_id,github_login_expected,full_name,created_source) values($1,$2,$3,$4,'group_import')`, recordID, groupID, inputStudent.GitHubLogin, inputStudent.FullName)
				}
			} else if err == nil {
				_, err = tx.Exec(ctx, `update student_records set group_id=$2,full_name=$3,updated_at=clock_timestamp(),version=version+1 where id=$1`, recordID, groupID, inputStudent.FullName)
			}
			if err != nil {
				return RosterPreview{}, fmt.Errorf("upsert student %s: %w", inputStudent.GitHubLogin, err)
			}
			// If this GitHub account already signed in, claim the imported record
			// immediately. A claimed record is never reassigned to another user.
			_, err = tx.Exec(ctx, `
				update student_records sr set user_id=gi.user_id,status='claimed',updated_at=clock_timestamp(),version=sr.version+1
				from github_identities gi join users u on u.id=gi.user_id
				where sr.id=$1 and sr.user_id is null and gi.login=$2
				  and u.status in ('pending','active') and (u.role is null or u.role='student')
				  and not exists(select 1 from student_records linked where linked.user_id=gi.user_id and linked.id<>sr.id)
			`, recordID, inputStudent.GitHubLogin)
			if err != nil {
				return RosterPreview{}, err
			}
			_, err = tx.Exec(ctx, `update users u set status='active',role='student',updated_at=clock_timestamp(),version=version+1 where exists(select 1 from student_records sr where sr.id=$1 and sr.user_id=u.id) and (u.status='pending' or u.role='student')`, recordID)
			if err != nil {
				return RosterPreview{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return RosterPreview{}, err
	}
	return preview, nil
}

func (r *AdminRepository) canManageRosters(ctx context.Context, actor string) (bool, error) {
	var allowed bool
	err := r.db.pool.QueryRow(ctx, `select exists(select 1 from users where id=$1 and status='active' and role in('teacher','admin'))`, actor).Scan(&allowed)
	return allowed, err
}
