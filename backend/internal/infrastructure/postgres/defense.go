package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type DefenseRepository struct{ db *Database }

func NewDefenseRepository(db *Database) *DefenseRepository { return &DefenseRepository{db: db} }

type Defense struct {
	ID                  string     `json:"id"`
	SessionID           string     `json:"session_id"`
	SubmissionVersionID string     `json:"submission_version_id"`
	Status              string     `json:"status"`
	Seed                string     `json:"seed"`
	TimeLimitSeconds    int        `json:"time_limit_seconds"`
	StartedAt           *time.Time `json:"started_at"`
	DeadlineAt          *time.Time `json:"deadline_at"`
	FinishedAt          *time.Time `json:"finished_at"`
	CurrentDraft        string     `json:"current_draft"`
	DraftVersion        int64      `json:"draft_version"`
	SelectedCandidateID *string    `json:"selected_candidate_id"`
	Challenge           *Challenge `json:"challenge,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}
type Challenge struct {
	FunctionName string `json:"function_name"`
	FilePath     string `json:"file_path"`
	Signature    string `json:"signature"`
	BeginLine    int    `json:"begin_line"`
	EndLine      int    `json:"end_line"`
	MaskedSource string `json:"masked_source"`
}
type Attempt struct {
	ID              string     `json:"id"`
	DefenseID       string     `json:"defense_id"`
	AttemptNo       int        `json:"attempt_no"`
	Outcome         string     `json:"outcome"`
	AcceptedAt      time.Time  `json:"accepted_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	ConfigureResult any        `json:"configure_result"`
	BuildResult     any        `json:"build_result"`
	CTestResult     any        `json:"ctest_result"`
}

func (r *DefenseRepository) Create(ctx context.Context, actor, submission, key string) (Defense, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return Defense{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var seconds, topN int
	var owned bool
	e = tx.QueryRow(ctx, `select l.time_limit_seconds,l.top_n,exists(select 1 from student_records sr join users u on u.id=$1 and u.status='active' and u.role='student' where sr.id=sv.student_record_id and sr.user_id=u.id) from submission_versions sv join labs l on l.id=sv.lab_id where sv.id=$2`, actor, submission).Scan(&seconds, &topN, &owned)
	if e != nil || !owned {
		return Defense{}, appauth.ErrForbidden
	}
	var existing string
	e = tx.QueryRow(ctx, `select id from defenses where submission_version_id=$1 and start_idempotency_key=$2`, submission, key).Scan(&existing)
	if e == nil {
		_ = tx.Rollback(ctx)
		return r.Get(ctx, actor, existing)
	}
	if e != pgx.ErrNoRows {
		return Defense{}, e
	}
	e = tx.QueryRow(ctx, `select id from defenses where submission_version_id=$1 and status in('ready','preparing','active')`, submission).Scan(&existing)
	if e == nil {
		_ = tx.Rollback(ctx)
		return r.Get(ctx, actor, existing)
	}
	if e != pgx.ErrNoRows {
		return Defense{}, e
	}
	id, _ := domain.NewUUIDv7()
	session, _ := domain.NewUUIDv7()
	job, _ := domain.NewUUIDv7()
	limit := new(big.Int).Lsh(big.NewInt(1), 64)
	seedN, e := rand.Int(rand.Reader, limit)
	if e != nil {
		return Defense{}, e
	}
	seed := seedN.String()
	_, e = tx.Exec(ctx, `insert into defenses(id,session_id,submission_version_id,seed,time_limit_seconds,start_idempotency_key)values($1,$2,$3,$4,$5,$6)`, id, session, submission, seed, seconds, key)
	if e != nil {
		return Defense{}, e
	}
	_, e = tx.Exec(ctx, `update defenses set status='preparing' where id=$1`, id)
	if e != nil {
		return Defense{}, e
	}
	_, e = tx.Exec(ctx, `insert into runner_jobs(id,kind,defense_id,timeout_seconds)values($1,'prepare_defense',$2,300)`, job, id)
	if e != nil {
		return Defense{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Defense{}, e
	}
	_ = topN
	return r.Get(ctx, actor, id)
}
func (r *DefenseRepository) Get(ctx context.Context, actor, id string) (Defense, error) {
	_, _ = r.db.pool.Exec(ctx, `update defenses set status='expired',finished_at=clock_timestamp(),terminal_reason='deadline reached' where id=$1 and status='active' and deadline_at<=clock_timestamp()`, id)
	var v Defense
	e := r.db.pool.QueryRow(ctx, `select d.id,d.session_id,d.submission_version_id,d.status::text,d.seed::text,d.time_limit_seconds,d.started_at,d.deadline_at,d.finished_at,coalesce(d.current_draft,''),d.draft_version,d.selected_candidate_id,d.created_at from defenses d join submission_versions sv on sv.id=d.submission_version_id join student_records sr on sr.id=sv.student_record_id join users u on u.id=$1 and u.status='active' where d.id=$2 and (u.role='admin' or (u.role='student' and sr.user_id=u.id) or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=sr.group_id and gt.teacher_user_id=u.id)))`, actor, id).Scan(&v.ID, &v.SessionID, &v.SubmissionVersionID, &v.Status, &v.Seed, &v.TimeLimitSeconds, &v.StartedAt, &v.DeadlineAt, &v.FinishedAt, &v.CurrentDraft, &v.DraftVersion, &v.SelectedCandidateID, &v.CreatedAt)
	if e != nil {
		return Defense{}, appauth.ErrForbidden
	}
	if v.SelectedCandidateID != nil {
		var challenge Challenge
		if e = r.db.pool.QueryRow(ctx, `select function_name,file_path,signature,start_line,end_line,coalesce(masked_source,'') from defense_candidates where id=$1`, *v.SelectedCandidateID).Scan(&challenge.FunctionName, &challenge.FilePath, &challenge.Signature, &challenge.BeginLine, &challenge.EndLine, &challenge.MaskedSource); e == nil {
			v.Challenge = &challenge
		}
	}
	return v, nil
}
func (r *DefenseRepository) SaveDraft(ctx context.Context, actor, id, answer string, version int64) (int64, time.Time, error) {
	var newVersion int64
	var saved time.Time
	e := r.db.pool.QueryRow(ctx, `update defenses d set current_draft=$3,draft_version=draft_version+1,updated_at=clock_timestamp() from submission_versions sv,student_records sr where d.id=$1 and d.submission_version_id=sv.id and sv.student_record_id=sr.id and sr.user_id=$2 and d.status='active' and d.draft_version=$4 and d.deadline_at>clock_timestamp() returning d.draft_version,d.updated_at`, id, actor, answer, version).Scan(&newVersion, &saved)
	if e != nil {
		return 0, time.Time{}, appauth.ErrConflict
	}
	return newVersion, saved, nil
}
func (r *DefenseRepository) Cancel(ctx context.Context, actor, id string) (Defense, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return Defense{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var allowed bool
	e = tx.QueryRow(ctx, `select exists(select 1 from defenses d join submission_versions sv on sv.id=d.submission_version_id join student_records sr on sr.id=sv.student_record_id join users u on u.id=$2 and u.status='active' where d.id=$1 and d.status in('ready','preparing','active') and (u.role='admin' or (u.role='student' and sr.user_id=u.id) or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=sr.group_id and gt.teacher_user_id=u.id))))`, id, actor).Scan(&allowed)
	if e != nil || !allowed {
		return Defense{}, appauth.ErrConflict
	}
	if _, e = tx.Exec(ctx, `update defenses set status='cancelled',finished_at=clock_timestamp(),terminal_reason='cancelled by user' where id=$1 and status in('ready','preparing','active')`, id); e != nil {
		return Defense{}, e
	}
	if _, e = tx.Exec(ctx, `update check_attempts set outcome='error',finished_at=clock_timestamp(),configure_result='{"error":"defense cancelled"}'::jsonb where defense_id=$1 and outcome='pending'`, id); e != nil {
		return Defense{}, e
	}
	if _, e = tx.Exec(ctx, `update runner_jobs set state='cancelled',lease_owner_runner_id=null,lease_token_hash=null,lease_expires_at=null where defense_id=$1 and state in('queued','leased','running')`, id); e != nil {
		return Defense{}, e
	}
	_, _ = tx.Exec(ctx, `update job_leases set released_at=clock_timestamp(),release_reason='cancelled' where job_id in(select id from runner_jobs where defense_id=$1) and released_at is null`, id)
	if e = tx.Commit(ctx); e != nil {
		return Defense{}, e
	}
	return r.Get(ctx, actor, id)
}
func (r *DefenseRepository) Attempt(ctx context.Context, actor, id, key, answer string) (Attempt, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return Attempt{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var allowed bool
	e = tx.QueryRow(ctx, `select exists(select 1 from defenses d join submission_versions sv on sv.id=d.submission_version_id join student_records sr on sr.id=sv.student_record_id where d.id=$1 and sr.user_id=$2 and d.status='active' and d.deadline_at>clock_timestamp())`, id, actor).Scan(&allowed)
	if e != nil || !allowed {
		return Attempt{}, appauth.ErrConflict
	}
	var existing string
	e = tx.QueryRow(ctx, `select id from check_attempts where defense_id=$1 and idempotency_key=$2`, id, key).Scan(&existing)
	if e == nil {
		_ = tx.Rollback(ctx)
		return r.GetAttempt(ctx, actor, existing)
	}
	var no int
	e = tx.QueryRow(ctx, `select coalesce(max(attempt_no),0)+1 from check_attempts where defense_id=$1`, id).Scan(&no)
	if e != nil {
		return Attempt{}, e
	}
	attemptID, _ := domain.NewUUIDv7()
	jobID, _ := domain.NewUUIDv7()
	digest := sha256.Sum256([]byte(answer))
	_, e = tx.Exec(ctx, `insert into check_attempts(id,defense_id,attempt_no,idempotency_key,answer,answer_sha256)values($1,$2,$3,$4,$5,$6)`, attemptID, id, no, key, answer, digest[:])
	if e != nil {
		return Attempt{}, appauth.ErrConflict
	}
	_, e = tx.Exec(ctx, `insert into runner_jobs(id,kind,defense_id,check_attempt_id,timeout_seconds)values($1,'check_attempt',$2,$3,300)`, jobID, id, attemptID)
	if e != nil {
		return Attempt{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Attempt{}, e
	}
	return r.GetAttempt(ctx, actor, attemptID)
}
func (r *DefenseRepository) GetAttempt(ctx context.Context, actor, id string) (Attempt, error) {
	var v Attempt
	var configure, build, ctest []byte
	e := r.db.pool.QueryRow(ctx, `select a.id,a.defense_id,a.attempt_no,a.outcome::text,a.accepted_at,a.finished_at,a.configure_result,a.build_result,a.ctest_result from check_attempts a join defenses d on d.id=a.defense_id join submission_versions sv on sv.id=d.submission_version_id join student_records sr on sr.id=sv.student_record_id join users u on u.id=$1 and u.status='active' where a.id=$2 and (u.role='admin' or (u.role='student' and sr.user_id=u.id) or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=sr.group_id and gt.teacher_user_id=u.id)))`, actor, id).Scan(&v.ID, &v.DefenseID, &v.AttemptNo, &v.Outcome, &v.AcceptedAt, &v.FinishedAt, &configure, &build, &ctest)
	if e != nil {
		return Attempt{}, appauth.ErrForbidden
	}
	v.ConfigureResult = jsonRaw(configure)
	v.BuildResult = jsonRaw(build)
	v.CTestResult = jsonRaw(ctest)
	return v, nil
}
func jsonRaw(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(v, &value) != nil {
		return nil
	}
	return value
}

type Lease struct {
	JobID               string            `json:"job_id"`
	Kind                string            `json:"kind"`
	DefenseID           string            `json:"defense_id"`
	AttemptID           *string           `json:"attempt_id"`
	LeaseToken          string            `json:"lease_token"`
	LeaseExpiresAt      time.Time         `json:"lease_expires_at"`
	SessionID           string            `json:"session_id"`
	SubmissionObjectKey string            `json:"submission_object_key"`
	Seed                string            `json:"seed"`
	TopN                int               `json:"top_n"`
	Answer              *string           `json:"answer"`
	SelectedFunction    *SelectedFunction `json:"selected_function,omitempty"`
}
type SelectedFunction struct {
	FunctionName   string `json:"function_name"`
	FilePath       string `json:"file_path"`
	SignatureBegin int64  `json:"signature_begin"`
	BodyBegin      int64  `json:"body_begin"`
	BodyEnd        int64  `json:"body_end"`
	SourceSHA256   string `json:"source_sha256"`
}

func (r *DefenseRepository) Lease(ctx context.Context, runnerID string, slots int, credentialHash []byte) (Lease, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return Lease{}, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	_, e = tx.Exec(ctx, `insert into runners(id,name,status,slots,agent_version,credential_hash,last_heartbeat_at)values($1,$2,'active',$3,'2.0',$4,clock_timestamp()) on conflict(id)do update set status='active',slots=excluded.slots,last_heartbeat_at=clock_timestamp()`, runnerID, "runner-"+runnerID, slots, credentialHash)
	if e != nil {
		return Lease{}, e
	}
	expired, e := tx.Query(ctx, `update runner_jobs set state=case when retry_count+1>max_retries then 'dead'::runner_job_state else 'queued'::runner_job_state end,retry_count=retry_count+1,lease_owner_runner_id=null,lease_token_hash=null,lease_expires_at=null where state='leased' and lease_expires_at<clock_timestamp() returning id,kind::text,defense_id,check_attempt_id,state::text`)
	if e != nil {
		return Lease{}, e
	}
	type expiredLease struct {
		job, kind, defense, state string
		attempt                   *string
	}
	var expiredLeases []expiredLease
	for expired.Next() {
		var item expiredLease
		if e = expired.Scan(&item.job, &item.kind, &item.defense, &item.attempt, &item.state); e != nil {
			expired.Close()
			return Lease{}, e
		}
		expiredLeases = append(expiredLeases, item)
	}
	if e = expired.Err(); e != nil {
		expired.Close()
		return Lease{}, e
	}
	expired.Close()
	for _, item := range expiredLeases {
		_, _ = tx.Exec(ctx, `update job_leases set released_at=clock_timestamp(),release_reason='expired' where job_id=$1 and released_at is null`, item.job)
		if item.state == "dead" && item.kind == "prepare_defense" {
			_, _ = tx.Exec(ctx, `update defenses set status='error',terminal_reason='runner retries exhausted' where id=$1 and status='preparing'`, item.defense)
		}
		if item.state == "dead" && item.attempt != nil {
			_, _ = tx.Exec(ctx, `update check_attempts set outcome='error',finished_at=clock_timestamp(),configure_result='{"error":"runner retries exhausted"}'::jsonb where id=$1 and outcome='pending'`, *item.attempt)
		}
	}
	var v Lease
	e = tx.QueryRow(ctx, `select j.id,j.kind::text,j.defense_id,j.check_attempt_id,d.session_id,sv.normalized_object_key,d.seed::text,l.top_n,a.answer from runner_jobs j join defenses d on d.id=j.defense_id join submission_versions sv on sv.id=d.submission_version_id join labs l on l.id=sv.lab_id left join check_attempts a on a.id=j.check_attempt_id where j.state in('queued','retry_wait') and j.available_at<=clock_timestamp() order by j.priority desc,j.created_at for update skip locked limit 1`).Scan(&v.JobID, &v.Kind, &v.DefenseID, &v.AttemptID, &v.SessionID, &v.SubmissionObjectKey, &v.Seed, &v.TopN, &v.Answer)
	if e == pgx.ErrNoRows {
		return Lease{}, e
	}
	if e != nil {
		return Lease{}, e
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return Lease{}, e
	}
	v.LeaseToken = base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(v.LeaseToken))
	v.LeaseExpiresAt = time.Now().UTC().Add(2 * time.Minute)
	_, e = tx.Exec(ctx, `update runner_jobs set state='leased',lease_owner_runner_id=$2,lease_token_hash=$3,lease_expires_at=$4 where id=$1`, v.JobID, runnerID, hash[:], v.LeaseExpiresAt)
	if e != nil {
		return Lease{}, e
	}
	leaseID, _ := domain.NewUUIDv7()
	_, e = tx.Exec(ctx, `insert into job_leases(id,job_id,runner_id,lease_no,token_fingerprint,leased_at,expires_at)values($1,$2,$3,(select count(*)+1 from job_leases where job_id=$2),$4,clock_timestamp(),$5)`, leaseID, v.JobID, runnerID, fmt.Sprintf("%x", hash[:8]), v.LeaseExpiresAt)
	if e != nil {
		return Lease{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Lease{}, e
	}
	if v.Kind == "check_attempt" {
		var selected SelectedFunction
		var digest []byte
		e = r.db.pool.QueryRow(ctx, `select function_name,file_path,signature_begin_offset,body_start_offset,body_end_offset,source_sha256 from defense_candidates where defense_id=$1 and is_selected`, v.DefenseID).Scan(&selected.FunctionName, &selected.FilePath, &selected.SignatureBegin, &selected.BodyBegin, &selected.BodyEnd, &digest)
		if e != nil {
			return Lease{}, e
		}
		selected.SourceSHA256 = fmt.Sprintf("%x", digest)
		v.SelectedFunction = &selected
	}
	return v, nil
}
func parseVersion(header string) (int64, error) {
	return strconv.ParseInt(strings.Trim(header, "\""), 10, 64)
}

type Candidate struct {
	FunctionName       string `json:"function_name"`
	FilePath           string `json:"file_path"`
	Signature          string `json:"signature"`
	SignatureBegin     int64  `json:"signature_begin"`
	BodyBegin          int64  `json:"body_begin"`
	BodyEnd            int64  `json:"body_end"`
	BeginLine          int    `json:"begin_line"`
	EndLine            int    `json:"end_line"`
	LineCount          int    `json:"line_count"`
	SourceSHA256       string `json:"source_sha256"`
	OriginalBodySHA256 string `json:"original_body_sha256"`
}
type Completion struct {
	Outcome         string      `json:"outcome"`
	Candidates      []Candidate `json:"candidates"`
	SelectedIndex   int         `json:"selected_index"`
	MaskedSource    string      `json:"masked_source"`
	ConfigureResult any         `json:"configure_result"`
	BuildResult     any         `json:"build_result"`
	CTestResult     any         `json:"ctest_result"`
}

func (r *DefenseRepository) Heartbeat(ctx context.Context, job, token string) error {
	hash := sha256.Sum256([]byte(token))
	result, e := r.db.pool.Exec(ctx, `update runner_jobs set lease_expires_at=clock_timestamp()+interval '2 minutes',updated_at=clock_timestamp() where id=$1 and state in('leased','running') and lease_token_hash=$2 and lease_expires_at>clock_timestamp()`, job, hash[:])
	if e != nil || result.RowsAffected() != 1 {
		return appauth.ErrConflict
	}
	return nil
}

func (r *DefenseRepository) Complete(ctx context.Context, job, token string, input Completion) (*Attempt, error) {
	tx, e := r.db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	hash := sha256.Sum256([]byte(token))
	var kind, defense, state string
	var attempt *string
	e = tx.QueryRow(ctx, `select kind::text,defense_id,check_attempt_id,state::text from runner_jobs where id=$1 and lease_token_hash=$2 and (state='completed' or (state in('leased','running') and lease_expires_at>clock_timestamp())) for update`, job, hash[:]).Scan(&kind, &defense, &attempt, &state)
	if e != nil {
		return nil, appauth.ErrConflict
	}
	if state == "completed" {
		if attempt == nil {
			return nil, nil
		}
		value, getErr := r.getAttemptUnscoped(ctx, *attempt)
		return &value, getErr
	}
	_, e = tx.Exec(ctx, `update runner_jobs set state='running' where id=$1 and state='leased'`, job)
	if e != nil {
		return nil, e
	}
	if kind == "prepare_defense" {
		if input.Outcome == "error" {
			_, e = tx.Exec(ctx, `update defenses set status='error',terminal_reason='worker preparation failed' where id=$1 and status='preparing'`, defense)
			if e != nil {
				return nil, e
			}
		} else if len(input.Candidates) == 0 || input.SelectedIndex < 0 || input.SelectedIndex >= len(input.Candidates) || len(input.MaskedSource) == 0 || len(input.MaskedSource) > 4<<20 {
			return nil, fmt.Errorf("invalid candidates")
		} else {
			selected := ""
			for index, c := range input.Candidates {
				source, e := hex.DecodeString(c.SourceSHA256)
				if e != nil || len(source) != 32 {
					return nil, fmt.Errorf("invalid source digest")
				}
				body, e := hex.DecodeString(c.OriginalBodySHA256)
				if e != nil || len(body) != 32 {
					return nil, fmt.Errorf("invalid body digest")
				}
				id, _ := domain.NewUUIDv7()
				chosen := index == input.SelectedIndex
				if chosen {
					selected = id
				}
				var maskedSource *string
				if chosen {
					maskedSource = &input.MaskedSource
				}
				_, e = tx.Exec(ctx, `insert into defense_candidates(id,defense_id,rank,function_name,file_path,signature,signature_begin_offset,body_start_offset,body_end_offset,start_line,end_line,line_count,source_sha256,original_body_sha256,is_selected,masked_source)values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id, defense, index+1, c.FunctionName, c.FilePath, c.Signature, c.SignatureBegin, c.BodyBegin, c.BodyEnd, c.BeginLine, c.EndLine, c.LineCount, source, body, chosen, maskedSource)
				if e != nil {
					return nil, e
				}
			}
			_, e = tx.Exec(ctx, `update defenses set selected_candidate_id=$2,status='active',started_at=clock_timestamp(),deadline_at=clock_timestamp()+make_interval(secs=>time_limit_seconds) where id=$1 and status='preparing'`, defense, selected)
			if e != nil {
				return nil, e
			}
		}
	} else {
		if attempt == nil {
			return nil, fmt.Errorf("attempt missing")
		}
		if input.Outcome != "passed" && input.Outcome != "failed" && input.Outcome != "error" {
			return nil, fmt.Errorf("invalid outcome")
		}
		configure, _ := json.Marshal(input.ConfigureResult)
		build, _ := json.Marshal(input.BuildResult)
		ctest, _ := json.Marshal(input.CTestResult)
		_, e = tx.Exec(ctx, `update check_attempts set outcome=$2,configure_result=$3::jsonb,build_result=$4::jsonb,ctest_result=$5::jsonb,finished_at=clock_timestamp() where id=$1 and outcome='pending'`, *attempt, input.Outcome, string(configure), string(build), string(ctest))
		if e != nil {
			return nil, e
		}
		// A failed or infrastructure-error attempt does not end the defense: the
		// student may submit another answer until the deadline. Only success is
		// terminal; expiration is applied when the defense is read or edited.
		if input.Outcome == "passed" {
			_, e = tx.Exec(ctx, `update defenses set status='passed',finished_at=clock_timestamp(),terminal_reason='attempt passed' where id=$1 and status='active'`, defense)
			if e != nil {
				return nil, e
			}
		}
	}
	// Keep the hashed lease credentials on a completed job so a runner can
	// safely retry a completion whose HTTP response was lost. The raw token is
	// never persisted and no heartbeat accepts a completed job.
	_, e = tx.Exec(ctx, `update runner_jobs set state='completed' where id=$1`, job)
	if e != nil {
		return nil, e
	}
	_, _ = tx.Exec(ctx, `update job_leases set released_at=clock_timestamp(),release_reason='completed' where job_id=$1 and released_at is null`, job)
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	if attempt == nil {
		return nil, nil
	}
	value, e := r.getAttemptUnscoped(ctx, *attempt)
	return &value, e
}

func (r *DefenseRepository) getAttemptUnscoped(ctx context.Context, id string) (Attempt, error) {
	var v Attempt
	var configure, build, ctest []byte
	e := r.db.pool.QueryRow(ctx, `select id,defense_id,attempt_no,outcome::text,accepted_at,finished_at,configure_result,build_result,ctest_result from check_attempts where id=$1`, id).Scan(&v.ID, &v.DefenseID, &v.AttemptNo, &v.Outcome, &v.AcceptedAt, &v.FinishedAt, &configure, &build, &ctest)
	v.ConfigureResult = jsonRaw(configure)
	v.BuildResult = jsonRaw(build)
	v.CTestResult = jsonRaw(ctest)
	return v, e
}

func (r *DefenseRepository) History(ctx context.Context, actor, groupID, studentRecordID, labID, outcome string, limit int) ([]Attempt, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, e := r.db.pool.Query(ctx, `select a.id,a.defense_id,a.attempt_no,a.outcome::text,a.accepted_at,a.finished_at,a.configure_result,a.build_result,a.ctest_result from check_attempts a join defenses d on d.id=a.defense_id join submission_versions sv on sv.id=d.submission_version_id join student_records sr on sr.id=sv.student_record_id join users u on u.id=$1 and u.status='active' where ($2='' or sr.group_id::text=$2) and ($3='' or sr.id::text=$3) and ($4='' or sv.lab_id::text=$4) and ($5='' or a.outcome::text=$5) and (u.role='admin' or (u.role='teacher' and exists(select 1 from group_teachers gt where gt.group_id=sr.group_id and gt.teacher_user_id=u.id))) order by a.accepted_at desc limit $6`, actor, groupID, studentRecordID, labID, outcome, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		var v Attempt
		var configure, build, ctest []byte
		if e = rows.Scan(&v.ID, &v.DefenseID, &v.AttemptNo, &v.Outcome, &v.AcceptedAt, &v.FinishedAt, &configure, &build, &ctest); e != nil {
			return nil, e
		}
		v.ConfigureResult = jsonRaw(configure)
		v.BuildResult = jsonRaw(build)
		v.CTestResult = jsonRaw(ctest)
		out = append(out, v)
	}
	return out, rows.Err()
}
