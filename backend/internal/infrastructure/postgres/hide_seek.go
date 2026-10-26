package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (r *DefenseRepository) hideView(ctx context.Context, actor string, value *Defense) error {
	var report, mutations []byte
	var teacher bool
	if err := r.db.pool.QueryRow(ctx, `select d.defense_mode,jsonb_array_length(d.hide_mutations),d.hide_report,d.hide_mutations,coalesce(d.hide_error,''),u.role in('teacher','admin') from defenses d join users u on u.id=$1 where d.id=$2`, actor, value.ID).Scan(&value.DefenseMode, &value.ErrorCount, &report, &mutations, &value.PreparationError, &teacher); err != nil {
		return err
	}
	if value.DefenseMode == "hide_seek" {
		value.Challenge = nil
		value.SelectedCandidateID = nil
		value.WheelCandidates = []WheelCandidate{}
		if value.Status == "preparing" {
			value.PreparationStage = "validating_hide_seek"
		}
		if teacher {
			value.HideReport = jsonRaw(report)
			if err := json.Unmarshal(mutations, &value.TeacherMutations); err != nil {
				return err
			}
		} else if value.PreparationError != "" {
			value.PreparationError = "Преподавателю нужно скорректировать задание"
		}
	}
	if !teacher && value.DefenseMode != "hide_seek" {
		var hidden bool
		if err := r.db.pool.QueryRow(ctx, `select exists(select 1 from defenses where submission_version_id=$1 and defense_mode='hide_seek')`, value.SubmissionVersionID).Scan(&hidden); err != nil {
			return err
		}
		if hidden {
			value.Challenge = nil
			value.CurrentDraft = ""
			value.WheelCandidates = []WheelCandidate{}
		}
	}
	return nil
}

func (r *DefenseRepository) ConfigureHide(ctx context.Context, actor, id string, seconds int, mutations []hideseek.Mutation) (Defense, error) {
	if seconds < 60 || seconds > 1800 || len(mutations) < 1 || len(mutations) > 5 {
		return Defense{}, fmt.Errorf("invalid hide configuration")
	}
	tx, err := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Defense{}, err
	}
	defer tx.Rollback(ctx)
	var allowed bool
	err = tx.QueryRow(ctx, `select exists(select 1 from users u join submission_versions sv on sv.id=d.submission_version_id join student_records sr on sr.id=sv.student_record_id where u.id=$1 and u.status='active' and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=sr.group_id and gt.teacher_user_id=u.id)))) from defenses d where d.id=$2 and d.status='ready' for update`, actor, id).Scan(&allowed)
	if err != nil || !allowed {
		return Defense{}, appauth.ErrForbidden
	}
	raw, _ := json.Marshal(mutations)
	_, err = tx.Exec(ctx, `update defenses set defense_mode='hide_seek',hide_mutations=$2::jsonb,hide_report=null,hide_error=null,time_limit_seconds=$3,selected_candidate_id=null,selection_mode='manual',wheel_size=null,entity_mode=null,current_draft='',draft_version=draft_version+1,status='preparing' where id=$1`, id, string(raw), seconds)
	if err != nil {
		return Defense{}, err
	}
	job, err := domain.NewUUIDv7()
	if err != nil {
		return Defense{}, err
	}
	if _, err = tx.Exec(ctx, `insert into runner_jobs(id,kind,defense_id,timeout_seconds) values($1,'prepare_defense',$2,300)`, job, id); err != nil {
		return Defense{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Defense{}, err
	}
	return r.Get(ctx, actor, id)
}

type HideValidation struct {
	Valid    bool   `json:"valid"`
	Message  string `json:"message"`
	Baseline any    `json:"baseline,omitempty"`
	Mutated  any    `json:"mutated,omitempty"`
	Restored any    `json:"restored,omitempty"`
}

func completeHide(ctx context.Context, tx pgx.Tx, defense string, input Completion) error {
	report := input.HideValidation
	if report == nil {
		report = &HideValidation{Message: "Runner не завершил проверку задания"}
	}
	raw, _ := json.Marshal(report)
	var err error
	if report.Valid && input.Outcome != "error" {
		_, err = tx.Exec(ctx, `update defenses set hide_report=$2::jsonb,hide_error=null,status='active',started_at=statement_timestamp(),deadline_at=statement_timestamp()+make_interval(secs=>time_limit_seconds) where id=$1 and status='preparing'`, defense, string(raw))
	} else {
		// A teacher can adjust an invalid mutation and submit it again. No timer starts.
		_, err = tx.Exec(ctx, `update defenses set hide_report=$2::jsonb,hide_error=$3,status='ready' where id=$1 and status='preparing'`, defense, string(raw), report.Message)
	}
	return err
}
