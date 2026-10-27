package integration

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestHideSeekLifecycleAndOriginalAccess(t *testing.T) {
	t.Run("sandbox", func(t *testing.T) { testHideSeekLifecycle(t, false) })
	t.Run("reference", func(t *testing.T) { testHideSeekLifecycle(t, true) })
}

func testHideSeekLifecycle(t *testing.T, useReference bool) {
	testHideSeekLifecycleWithWait(t, useReference, false)
}

func testHideSeekLifecycleWithWait(t *testing.T, useReference, withWait bool) {
	var db *postgres.Database
	if withWait {
		db = isolatedDatabase(t)
	} else {
		db = database(t)
	}
	ctx := context.Background()
	uuid := func() string {
		v, e := domain.NewUUIDv7()
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	teacher, student, group, record, lab, imp, submission, defense, session, runner := uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid()
	tx, e := db.Pool().Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := tx.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec("insert into users(id,status,role) values($1,'active','teacher'),($2,'active','student')", teacher, student)
	exec("insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,now()),($5,$6,$7,$8,now())", uuid(), teacher, time.Now().UnixNano(), "hide-teacher-"+teacher, uuid(), student, time.Now().UnixNano()+1, "hide-student-"+student)
	exec("insert into groups(id,code,name) values($1,$2,'Hide test')", group, "hide-"+group)
	exec("insert into group_teachers(group_id,teacher_user_id,assigned_by) values($1,$2,$2)", group, teacher)
	exec("insert into student_records(id,group_id,user_id,github_login_expected,status,created_source) values($1,$2,$3,$4,'claimed','manual')", record, group, student, "hide-student-"+student)
	exec("insert into labs(id,group_id,code,name,time_limit_seconds,top_n) values($1,$2,'hide','Hide',900,5)", lab, group)
	originalKey, _ := appstorage.NewKey(appstorage.OriginalArchives, imp, ".zip")
	normalizedKey, _ := appstorage.NewKey(appstorage.NormalizedSubmissions, submission, ".zip")
	digest := sha256.Sum256([]byte(submission))
	exec("insert into imports(id,group_id,kind,uploaded_by,original_object_key,original_sha256,compressed_size,schema_version) values($1,$2,'lab',$3,$4,$5,100,1)", imp, group, student, originalKey.String(), digest[:])
	exec("insert into submission_versions(id,student_record_id,lab_id,version_no,source,original_import_id,normalized_object_key,normalized_sha256,source_manifest,created_by) values($1,$2,$3,1,'teacher_import',$4,$5,$6,'{}',$7)", submission, record, lab, imp, normalizedKey.String(), digest[:], teacher)
	exec("insert into defenses(id,session_id,submission_version_id,status,seed,time_limit_seconds) values($1,$2,$3,'ready',42,900)", defense, session, submission)
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	r := postgres.NewDefenseRepository(db)
	changes := []hideseek.Mutation{{Path: "main.cpp", Start: 12, Before: ">", After: "<"}}
	if _, e = r.ConfigureHide(ctx, student, defense, 900, changes); !errors.Is(e, appauth.ErrForbidden) {
		t.Fatalf("student configured hide: %v", e)
	}
	configured, e := r.ConfigureHide(ctx, teacher, defense, 900, changes)
	if e != nil {
		t.Fatal(e)
	}
	if configured.Status != "preparing" || configured.StartedAt != nil || configured.ErrorCount != 1 {
		t.Fatalf("premature start: %#v", configured)
	}
	hash := sha256.Sum256([]byte(runner))
	lease := func() postgres.Lease {
		t.Helper()
		// Other integration tests may leave queued fixtures in the same database.
		if _, e := db.Pool().Exec(ctx, "update runner_jobs set priority=1000000 where defense_id=$1 and state='queued'", defense); e != nil {
			t.Fatal(e)
		}
		v, e := r.Lease(ctx, runner, 1, hash[:])
		if e != nil {
			t.Fatal(e)
		}
		if v.DefenseID != defense || v.DefenseMode != "hide_seek" || len(v.HideMutations) != 1 || v.SelectedFunction != nil {
			t.Fatalf("wrong lease: %#v", v)
		}
		return v
	}
	job := lease()
	if _, e = r.Complete(ctx, job.JobID, job.LeaseToken, postgres.Completion{Outcome: "error", HideValidation: &postgres.HideValidation{Message: "compile failed"}}); e != nil {
		t.Fatal(e)
	}
	v, e := r.Get(ctx, teacher, defense)
	if e != nil {
		t.Fatal(e)
	}
	if v.Status != "ready" || v.DeadlineAt != nil || v.HideReport == nil {
		t.Fatalf("invalid task started: %#v", v)
	}
	type leasedResult struct {
		value postgres.Lease
		err   error
	}
	waiting := make(chan leasedResult, 1)
	if withWait {
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		go func() {
			value, err := r.LeaseWaiting(waitCtx, runner, 1, hash[:], 5*time.Second)
			waiting <- leasedResult{value, err}
		}()
		until := time.Now().Add(3 * time.Second)
		for {
			var count int
			if err := db.Pool().QueryRow(ctx, "select count(*) from pg_stat_activity where application_name='cppdefense-runner-wait' and state='idle'").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count > 0 {
				break
			}
			if time.Now().After(until) {
				t.Fatal("long poll did not subscribe")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	queuedAt := time.Now()
	if _, e = r.ConfigureHide(ctx, teacher, defense, 900, changes); e != nil {
		t.Fatal(e)
	}
	if withWait {
		select {
		case result := <-waiting:
			if result.err != nil || result.value.DefenseID != defense {
				t.Fatalf("notified lease: %#v %v", result.value, result.err)
			}
			job = result.value
			if time.Since(queuedAt) > 800*time.Millisecond {
				t.Fatal("job waited for polling instead of notification")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("committed insertion did not wake runner")
		}
	} else {
		job = lease()
	}
	completion := postgres.Completion{HideValidation: &postgres.HideValidation{Valid: true, Message: "validated"}, Timings: &postgres.JobTimings{TotalMS: 123, ConfigureMS: 17, BuildMS: 80, TestMS: 10}}
	if _, e = r.Complete(ctx, job.JobID, job.LeaseToken, completion); e != nil {
		t.Fatal(e)
	}
	v, e = r.Get(ctx, student, defense)
	if e != nil {
		t.Fatal(e)
	}
	if v.Status != "active" || v.StartedAt == nil || v.DeadlineAt == nil || v.HideReport != nil || len(v.TeacherMutations) != 0 || v.Challenge != nil || v.SelectedCandidateID != nil || len(v.WheelCandidates) != 0 {
		t.Fatalf("student view leaks or invalid state: %#v", v)
	}
	if v.PreparationTimings == nil || v.PreparationTimings.ConfigureMS != 17 {
		t.Fatalf("preparation timings lost: %#v", v.PreparationTimings)
	}
	started := *v.StartedAt
	if _, e = r.Complete(ctx, job.JobID, job.LeaseToken, completion); e != nil {
		t.Fatal(e)
	}
	v, e = r.Get(ctx, teacher, defense)
	if e != nil || !v.StartedAt.Equal(started) || v.HideReport == nil || len(v.TeacherMutations) != 1 {
		t.Fatalf("non-idempotent completion: %#v %v", v, e)
	}
	storage := postgres.NewStorageRepository(db)
	if _, e = storage.ReadableSubmissionKey(ctx, student, submission); e == nil {
		t.Fatal("student downloaded original")
	}
	if _, e = storage.ReadableSubmissionKey(ctx, teacher, submission); e != nil {
		t.Fatal(e)
	}
	imports := postgres.NewImportRepository(db)
	if _, _, e = imports.OriginalObject(ctx, student, imp); e == nil {
		t.Fatal("student downloaded original import")
	}
	if _, _, e = imports.OriginalObject(ctx, teacher, imp); e != nil {
		t.Fatal(e)
	}
	access, e := r.SourceAccess(ctx, student, defense)
	if e != nil || !access.StudentView || access.MaskSelected || len(access.HideMutations) != 1 {
		t.Fatalf("source access: %#v %v", access, e)
	}
	answer := `{"main.cpp":"int main() { return 0; }"}`
	if _, _, e = r.SaveDraft(ctx, student, defense, answer, v.DraftVersion); e != nil {
		t.Fatal(e)
	}
	for _, outcome := range []string{"error", "failed", "passed"} {
		attempt, e := r.Attempt(ctx, student, defense, uuid(), answer)
		if e != nil {
			t.Fatal(e)
		}
		job = lease()
		if job.Answer == nil || *job.Answer != answer || job.Kind != "check_attempt" {
			t.Fatalf("answer not leased: %#v", job)
		}
		result := postgres.Completion{Outcome: outcome, ConfigureResult: map[string]any{"ok": true}, BuildResult: map[string]any{"ok": true}, CTestResult: map[string]any{"ok": outcome == "passed"}}
		if useReference && outcome == "passed" {
			proof := &postgres.ReferenceResult{ProjectSHA256: strings.Repeat("a", 64), PolicySHA256: strings.Repeat("b", 64), ImageID: "sha256:" + strings.Repeat("c", 64), VerifiedAt: time.Now().UTC()}
			bad := postgres.Completion{Outcome: "failed", ReferenceResult: proof}
			if _, err := r.Complete(ctx, job.JobID, job.LeaseToken, bad); err == nil {
				t.Fatal("reference accepted for failed answer")
			}
			bad.Outcome = "passed"
			bad.ConfigureResult = map[string]any{"ok": true}
			if _, err := r.Complete(ctx, job.JobID, job.LeaseToken, bad); err == nil {
				t.Fatal("reference mixed with executed stages")
			}
			result = postgres.Completion{Outcome: "passed", ReferenceResult: proof}
		}
		result.Timings = &postgres.JobTimings{QueueMS: 5, BuildMS: 50, TestMS: 9, TotalMS: 100}
		if _, e = r.Complete(ctx, job.JobID, job.LeaseToken, result); e != nil {
			t.Fatal(e)
		}
		got, e := r.GetAttempt(ctx, student, attempt.ID)
		if e != nil || got.Outcome != outcome {
			t.Fatalf("attempt: %#v %v", got, e)
		}
		if got.Timings == nil || got.Timings.QueueMS != 5 {
			t.Fatalf("attempt timings lost: %#v", got.Timings)
		}
		if useReference && outcome == "passed" {
			if got.ReferenceResult == nil || got.ConfigureResult != nil || got.BuildResult != nil || got.CTestResult != nil {
				t.Fatalf("incorrect reference result: %#v", got)
			}
			repeated, err := r.Complete(ctx, job.JobID, job.LeaseToken, result)
			if err != nil || repeated.ReferenceResult == nil {
				t.Fatalf("reference lost on retry: %#v %v", repeated, err)
			}
			history, err := r.History(ctx, teacher, group, "", "", "passed", 10)
			if err != nil || len(history) != 1 || history[0].ReferenceResult == nil {
				t.Fatalf("teacher history lost reference: %#v %v", history, err)
			}
		}
		v, e = r.Get(ctx, student, defense)
		if e != nil {
			t.Fatal(e)
		}
		expected := "active"
		if outcome == "passed" {
			expected = "passed"
		}
		if v.Status != expected {
			t.Fatalf("status=%s after %s", v.Status, outcome)
		}
	}
}
