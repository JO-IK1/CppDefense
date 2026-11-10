package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5"
)

func TestSubmissionWarmupLeasePriorityPreemptionAndCompletion(t *testing.T) {
	db := isolatedDatabase(t)
	ctx := context.Background()
	uuid := func() string { id, _ := domain.NewUUIDv7(); return id }
	student, group, record, lab, imp, submission, runner := uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid()
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("insert into users(id,status,role) values($1,'active','student')", student)
	exec("insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,now())", uuid(), student, time.Now().UnixNano(), "warm-"+student)
	exec("insert into groups(id,code,name) values($1,$2,'Warm')", group, "warm-"+group)
	exec("insert into student_records(id,group_id,user_id,github_login_expected,status,created_source) values($1,$2,$3,$4,'claimed','manual')", record, group, student, "warm-"+student)
	exec("insert into labs(id,group_id,code,name,time_limit_seconds,top_n) values($1,$2,'warm','Warm',900,5)", lab, group)
	exec("insert into imports(id,group_id,kind,uploaded_by,original_object_key,original_sha256,compressed_size,schema_version) values($1,$2,'lab',$3,$4,$5,1,1)", imp, group, student, "original-archives/"+imp+".zip", make([]byte, 32))
	exec("insert into submission_versions(id,student_record_id,lab_id,version_no,source,original_import_id,normalized_object_key,normalized_sha256,source_manifest,created_by) values($1,$2,$3,1,'student_zip',$4,$5,$6,'{}',$7)", submission, record, lab, imp, "normalized-submissions/"+submission+".zip", make([]byte, 32), student)
	exec("insert into runner_jobs(id,kind,submission_version_id,priority,timeout_seconds) values($1,'warm_submission',$2,-100,300)", uuid(), submission)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := postgres.NewDefenseRepository(db)
	hash := sha256.Sum256([]byte("test credential"))
	if _, err = r.Lease(ctx, runner, 1, hash[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("legacy runner received warmup: %v", err)
	}
	warm, err := r.Lease(ctx, runner, 1, hash[:], true)
	if err != nil || warm.Kind != "warm_submission" || warm.DefenseID != "" || warm.SessionID != submission {
		t.Fatalf("warm lease: %#v %v", warm, err)
	}
	var count int
	db.Pool().QueryRow(ctx, "select count(*) from defenses").Scan(&count)
	if count != 0 {
		t.Fatal("warmup created a defense")
	}
	if err = r.Heartbeat(ctx, warm.JobID, warm.LeaseToken); err != nil {
		t.Fatal(err)
	}
	// Creating an interactive defense is idempotent with the existing warmup.
	defense, err := r.Create(ctx, student, submission, uuid())
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Heartbeat(ctx, warm.JobID, warm.LeaseToken); !errors.Is(err, appauth.ErrConflict) {
		t.Fatalf("warmup not preempted: %v", err)
	}
	interactive, err := r.Lease(ctx, runner, 1, hash[:], true)
	if err != nil || interactive.DefenseID != defense.ID || interactive.Kind != "prepare_defense" {
		t.Fatalf("priority lost: %#v %v", interactive, err)
	}
	completion := postgres.Completion{Candidates: []postgres.Candidate{{FunctionName: "f", EntityType: "function", FilePath: "main.cpp", Signature: "int f()", SignatureBegin: 0, BodyBegin: 8, BodyEnd: 20, BeginLine: 1, EndLine: 1, LineCount: 1, SourceSHA256: strings.Repeat("a", 64), OriginalBodySHA256: strings.Repeat("b", 64)}}}
	if _, err = r.Complete(ctx, interactive.JobID, interactive.LeaseToken, completion); err != nil {
		t.Fatal(err)
	}
	warmAgain, err := r.Lease(ctx, runner, 1, hash[:], true)
	if err != nil || warmAgain.JobID != warm.JobID || warmAgain.LeaseToken == warm.LeaseToken {
		t.Fatalf("preempted warmup lost: %#v %v", warmAgain, err)
	}
	if _, err = r.Complete(ctx, warm.JobID, warm.LeaseToken, postgres.Completion{Outcome: "passed"}); !errors.Is(err, appauth.ErrConflict) {
		t.Fatal("stale warmup token accepted")
	}
	input := postgres.Completion{Outcome: "passed", Timings: &postgres.JobTimings{BuildMS: 12, TotalMS: 20}}
	if _, err = r.Complete(ctx, warmAgain.JobID, warmAgain.LeaseToken, input); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Complete(ctx, warmAgain.JobID, warmAgain.LeaseToken, input); err != nil {
		t.Fatal("completion not idempotent", err)
	}
	var raw []byte
	if err = db.Pool().QueryRow(ctx, "select preparation_result from runner_jobs where id=$1", warm.JobID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Outcome string
		Timings postgres.JobTimings
	}
	if err = json.Unmarshal(raw, &result); err != nil || result.Outcome != "passed" || result.Timings.BuildMS != 12 {
		t.Fatalf("warm result lost: %s %v", raw, err)
	}
	view, err := r.Get(ctx, student, defense.ID)
	if err != nil || view.Status != "ready" || view.StartedAt != nil || view.DeadlineAt != nil {
		t.Fatalf("warmup started timer: %#v %v", view, err)
	}
	if err = db.Pool().QueryRow(ctx, "select count(*) from runner_jobs where kind='warm_submission' and submission_version_id=$1", submission).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate warmup: %d %v", count, err)
	}
}
