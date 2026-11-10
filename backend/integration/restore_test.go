package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestMultiRestoreLifecycleAndSignaturePrivacy(t *testing.T) {
	db := isolatedDatabase(t)
	ctx := context.Background()
	uuid := func() string { id, _ := domain.NewUUIDv7(); return id }
	student, admin, group, record, lab, imp, submission, defense, runner := uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid()
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
	for i, user := range []string{student, admin} {
		role := "student"
		if i == 1 {
			role = "admin"
		}
		exec("insert into users(id,status,role) values($1,'active',$2)", user, role)
		exec("insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,now())", uuid(), user, time.Now().UnixNano(), "restore-"+user)
	}
	exec("insert into groups(id,code,name) values($1,$2,'Restore')", group, group)
	exec("insert into student_records(id,group_id,user_id,github_login_expected,status,created_source) values($1,$2,$3,$4,'claimed','manual')", record, group, student, "restore-"+student)
	exec("insert into labs(id,group_id,code,name,time_limit_seconds,top_n) values($1,$2,'restore','Restore',900,5)", lab, group)
	exec("insert into imports(id,group_id,kind,uploaded_by,original_object_key,original_sha256,compressed_size,schema_version) values($1,$2,'lab',$3,$4,$5,1,1)", imp, group, student, "original-archives/"+imp+".zip", make([]byte, 32))
	exec("insert into submission_versions(id,student_record_id,lab_id,version_no,source,original_import_id,normalized_object_key,normalized_sha256,source_manifest,created_by) values($1,$2,$3,1,'student_zip',$4,$5,$6,'{}',$7)", submission, record, lab, imp, "normalized-submissions/"+submission+".zip", make([]byte, 32), student)
	exec("insert into defenses(id,session_id,submission_version_id,status,seed,time_limit_seconds) values($1,$2,$3,'ready',42,900)", defense, uuid(), submission)
	ids := []string{uuid(), uuid(), uuid()}
	for i, id := range ids {
		kind := "function"
		if i == 0 {
			kind = "struct"
		}
		exec("insert into defense_candidates(id,defense_id,rank,function_name,signature,file_path,signature_begin_offset,body_start_offset,body_end_offset,start_line,end_line,line_count,source_sha256,original_body_sha256,is_selected,entity_type) values($1,$2,$3,$4,$4,'main.cpp',$5,$6,$7,$3,$3,1,$8,$8,false,$9)", id, defense, i+1, "secret"+id, i*30, i*30+10, i*30+20, make([]byte, 32), kind)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := postgres.NewDefenseRepository(db)
	configured, err := r.Configure(ctx, admin, defense, "manual", "", "functions_classes_structs", 900, 3, postgres.RestoreOptions{HiddenCount: 3, SignaturesVisible: false, CandidateIDs: ids})
	if err != nil || configured.Status != "preparing" {
		t.Fatal(configured, err)
	}
	pending, err := r.Get(ctx, student, defense)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pending)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("signature leaked before activation", string(raw))
	}
	hash := sha256.Sum256([]byte("runner"))
	if _, err := r.Lease(ctx, runner, 1, hash[:]); err == nil {
		t.Fatal("legacy runner received a v3 job")
	}
	lease, err := r.Lease(ctx, runner, 1, hash[:], false, true)
	if err != nil || len(lease.RestoreTargets) != 3 || lease.SelectedIndex == nil {
		t.Fatal(lease, err)
	}
	// Worker preparation still verifies the baseline once; server masks all targets.
	completion := postgres.Completion{SelectedIndex: 0, MaskedSource: "masked", Candidates: []postgres.Candidate{{EntityType: "struct"}}}
	if _, err = r.Complete(ctx, lease.JobID, lease.LeaseToken, completion); err != nil {
		t.Fatal(err)
	}
	active, err := r.Get(ctx, student, defense)
	if err != nil || len(active.Challenges) != 3 || active.SignaturesVisible || active.Status != "active" {
		t.Fatal(active, err)
	}
	raw, _ = json.Marshal(active)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("signature leaked", string(raw))
	}
	access, err := r.SourceAccess(ctx, student, defense)
	if err != nil || len(access.Targets) != 3 {
		t.Fatal(access, err)
	}
	answers := map[string]string{}
	for _, id := range ids {
		answers[id] = "answer"
	}
	data, _ := json.Marshal(answers)
	if _, _, err = r.SaveDraft(ctx, student, defense, string(data), 1); err != nil {
		t.Fatal(err)
	}
	attempt, err := r.Attempt(ctx, student, defense, uuid(), string(data))
	if err != nil {
		t.Fatal(err)
	}
	check, err := r.Lease(ctx, runner, 1, hash[:], false, true)
	if err != nil || check.AttemptID == nil || *check.AttemptID != attempt.ID || len(check.RestoreTargets) != 3 {
		t.Fatal(check, err)
	}
	if _, err = r.Complete(ctx, check.JobID, check.LeaseToken, postgres.Completion{Outcome: "passed"}); err != nil {
		t.Fatal(err)
	}
	passed, err := r.Get(ctx, student, defense)
	if err != nil || passed.Status != "passed" {
		t.Fatal(passed, err)
	}
}
