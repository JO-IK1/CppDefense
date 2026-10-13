package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/application/importer"
	"github.com/JO-IK1/CppDefense/backend/internal/application/roster"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

func database(t *testing.T) *postgres.Database {
	t.Helper()
	url := os.Getenv("CPPDEFENSE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CPPDEFENSE_TEST_DATABASE_URL is not set")
	}
	db, err := postgres.Open(context.Background(), config.Database{
		URL: url, MaxConnections: 4, MinConnections: 1,
		MaxConnectionLife: time.Minute, MaxConnectionIdle: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := postgres.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestGitHubLoginAutomaticallyClaimsUniqueStudent(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	suffix, _ := domain.NewUUIDv7()
	groupID, _ := domain.NewUUIDv7()
	recordID, _ := domain.NewUUIDv7()
	login := "oauth-" + suffix
	if _, err := db.Pool().Exec(ctx, `insert into groups(id,code,name) values($1,$2,$3)`, groupID, "group-"+suffix, "OAuth test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `insert into student_records(id,group_id,github_login_expected,created_source) values($1,$2,$3,'manual')`, recordID, groupID, login); err != nil {
		t.Fatal(err)
	}
	repository := postgres.NewAuthRepository(db, 1)
	githubID := time.Now().UnixNano()
	user, err := repository.LoginGitHub(ctx, appauth.Profile{ID: githubID, Login: strings.ToUpper(login), DisplayName: "Student"})
	if err != nil {
		t.Fatal(err)
	}
	if user.Status != "active" || user.Role == nil || *user.Role != "student" {
		t.Fatalf("unexpected user: %#v", user)
	}
	var linkedUser string
	if err := db.Pool().QueryRow(ctx, `select user_id from student_records where id=$1 and status='claimed'`, recordID).Scan(&linkedUser); err != nil {
		t.Fatal(err)
	}
	if linkedUser != user.ID {
		t.Fatalf("linked user = %q, want %q", linkedUser, user.ID)
	}
	repeated, err := repository.LoginGitHub(ctx, appauth.Profile{ID: githubID, Login: login + "-renamed", DisplayName: "Student"})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != user.ID || repeated.Identity.Subject != user.Identity.Subject {
		t.Fatalf("repeat created another user: %#v", repeated)
	}
}

func TestMigrationsAreRepeatable(t *testing.T) {
	db := database(t)
	if err := postgres.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Pool().QueryRow(context.Background(), "select count(*) from cppdefense_schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 11 {
		t.Fatalf("migration count = %d", count)
	}
	var legacyTable *string
	if err := db.Pool().QueryRow(context.Background(), "select to_regclass('public.legacy_telegram_identities')::text").Scan(&legacyTable); err != nil {
		t.Fatal(err)
	}
	if legacyTable == nil {
		t.Fatal("legacy Telegram identity data was not retained")
	}
}

func TestRosterImportUpdatesStudentsWithoutDuplicatesOrStaffClaims(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	suffix, _ := domain.NewUUIDv7()
	token := strings.ReplaceAll(suffix, "-", "")[:16]
	adminID, _ := domain.NewUUIDv7()
	adminIdentityID, _ := domain.NewUUIDv7()
	staffID, _ := domain.NewUUIDv7()
	staffIdentityID, _ := domain.NewUUIDv7()
	adminLogin := "admin-" + token
	staffLogin := "staff-" + token
	studentLogin := "student-" + token

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `insert into users(id,status,role) values($1,'active','admin'),($2,'active','teacher')`, adminID, staffID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,clock_timestamp()),($5,$6,$7,$8,clock_timestamp())`, adminIdentityID, adminID, time.Now().UnixNano(), adminLogin, staffIdentityID, staffID, time.Now().UnixNano()+1, staffLogin); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	document := roster.Document{
		SchemaVersion: 1,
		Organization:  roster.Organization{GitHubLogin: "organization-" + token, Name: "Organization"},
		Groups: []roster.Group{{
			Code: "group-" + token,
			Name: "Initial name",
			Students: []roster.Student{
				{FullName: "Initial Student", GitHubLogin: studentLogin},
				{FullName: "Staff Account", GitHubLogin: staffLogin},
			},
		}},
	}
	repository := postgres.NewAdminRepository(db)
	if _, err = repository.ApplyRoster(ctx, adminID, document); err != nil {
		t.Fatal(err)
	}
	document.Groups[0].Name = "Updated name"
	document.Groups[0].Students[0].FullName = "Updated Student"
	if _, err = repository.ApplyRoster(ctx, adminID, document); err != nil {
		t.Fatal(err)
	}

	var count int
	if err = db.Pool().QueryRow(ctx, `select count(*) from student_records where github_login_expected in($1,$2) and status<>'archived'`, studentLogin, staffLogin).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("student record count = %d, want 2", count)
	}
	var fullName string
	if err = db.Pool().QueryRow(ctx, `select full_name from student_records where github_login_expected=$1`, studentLogin).Scan(&fullName); err != nil {
		t.Fatal(err)
	}
	if fullName != "Updated Student" {
		t.Fatalf("student name = %q", fullName)
	}
	var claimed bool
	if err = db.Pool().QueryRow(ctx, `select user_id is not null from student_records where github_login_expected=$1`, staffLogin).Scan(&claimed); err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("teacher account was claimed as a student")
	}
}

func TestStudentZIPRequiresTeacherApprovalBeforeCreatingVersion(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	suffix, _ := domain.NewUUIDv7()
	token := strings.ReplaceAll(suffix, "-", "")[:16]
	teacherID, _ := domain.NewUUIDv7()
	teacherIdentityID, _ := domain.NewUUIDv7()
	studentID, _ := domain.NewUUIDv7()
	studentIdentityID, _ := domain.NewUUIDv7()
	groupID, _ := domain.NewUUIDv7()
	recordID, _ := domain.NewUUIDv7()
	labID, _ := domain.NewUUIDv7()
	teacherLogin := "teacher-" + token
	studentLogin := "student-" + token
	groupCode := "group-" + token

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `insert into users(id,status,role) values($1,'active','teacher'),($2,'active','student')`, teacherID, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,clock_timestamp()),($5,$6,$7,$8,clock_timestamp())`, teacherIdentityID, teacherID, time.Now().UnixNano(), teacherLogin, studentIdentityID, studentID, time.Now().UnixNano()+1, studentLogin); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into groups(id,code,name) values($1,$2,'Student ZIP test')`, groupID, groupCode); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into group_teachers(group_id,teacher_user_id,assigned_by) values($1,$2,$2)`, groupID, teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into student_records(id,group_id,user_id,github_login_expected,status,created_source,full_name) values($1,$2,$3,$4,'claimed','group_import','Student')`, recordID, groupID, studentID, studentLogin); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into labs(id,group_id,code,name,time_limit_seconds,top_n) values($1,$2,'labwork1','Labwork 1',900,5)`, labID, groupID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	objectID, _ := domain.NewUUIDv7()
	originalKey, err := appstorage.NewKey(appstorage.OriginalArchives, objectID, ".zip")
	if err != nil {
		t.Fatal(err)
	}
	originalDigest := sha256.Sum256([]byte("student archive"))
	manifestJSON := []byte(`{"schema_version":1,"kind":"lab","group_code":"` + groupCode + `","lab_code":"labwork1","student":{"github_login":"` + studentLogin + `"},"project_path":"project","version_label":"deadline_0","source":"student_zip","source_reference":"labwork1-` + studentLogin + `-deadline_0.zip"}`)
	analysis := importer.Analysis{
		Manifest: importer.Manifest{
			SchemaVersion: 1, Kind: "lab", GroupCode: groupCode, LabCode: "labwork1",
			Student: importer.Student{GitHubLogin: studentLogin}, ProjectPath: "project",
			VersionLabel: "deadline_0", Source: "student_zip",
		},
		ManifestJSON: manifestJSON,
		Projects: []importer.Project{{
			Item:      importer.Item{LabCode: "labwork1", Student: importer.Student{GitHubLogin: studentLogin}, ProjectPath: "project"},
			FileCount: 2,
			Size:      128,
		}},
		TotalSize: 128,
	}
	repository := postgres.NewImportRepository(db)
	created, err := repository.CreateStudentReview(ctx, studentID, "labwork1-"+studentLogin+"-deadline_0.zip", appstorage.Object{Key: originalKey, Size: 256, SHA256: originalDigest}, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "review_pending" || len(created.Items) != 1 {
		t.Fatalf("unexpected pending import: %#v", created)
	}
	var versionCount int
	if err = db.Pool().QueryRow(ctx, `select count(*) from submission_versions where student_record_id=$1 and lab_id=$2`, recordID, labID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 0 {
		t.Fatalf("version existed before approval: %d", versionCount)
	}

	if _, err = repository.Review(ctx, teacherID, created.ID, "approve", hex.EncodeToString(originalDigest[:]), map[string]any{"paths_checked": true, "ownership_checked": true}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.BeginApply(ctx, teacherID, created.ID); err != nil {
		t.Fatal(err)
	}
	normalizedID, _ := domain.NewUUIDv7()
	normalizedKey, _ := appstorage.NewKey(appstorage.NormalizedSubmissions, normalizedID, ".zip")
	normalizedDigest := sha256.Sum256([]byte("normalized archive"))
	if _, err = repository.CommitApply(ctx, teacherID, created.ID, []postgres.ApplyItem{{
		ItemID: created.Items[0].ID, StudentRecordID: recordID, LabID: labID,
		ObjectKey: normalizedKey, SHA256: normalizedDigest, SourceManifestJSON: manifestJSON,
		VersionLabel: "deadline_0", SourceReference: "labwork1-" + studentLogin + "-deadline_0.zip",
	}}); err != nil {
		t.Fatal(err)
	}
	var source, versionLabel, approvedBy string
	if err = db.Pool().QueryRow(ctx, `select source::text,version_label,approved_by from submission_versions where student_record_id=$1 and lab_id=$2`, recordID, labID).Scan(&source, &versionLabel, &approvedBy); err != nil {
		t.Fatal(err)
	}
	if source != "student_zip" || versionLabel != "deadline_0" || approvedBy != teacherID {
		t.Fatalf("unexpected version metadata: source=%q label=%q approved_by=%q", source, versionLabel, approvedBy)
	}
}

func TestTeacherGitHubImportUsesSiteGroupAndPersistsCommit(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	suffix, _ := domain.NewUUIDv7()
	token := strings.ReplaceAll(suffix, "-", "")[:16]
	organizationID, _ := domain.NewUUIDv7()
	teacherID, _ := domain.NewUUIDv7()
	groupID, _ := domain.NewUUIDv7()
	recordID, _ := domain.NewUUIDv7()
	labID, _ := domain.NewUUIDv7()
	organizationLogin := "org-" + token
	groupCode := "group-" + token
	studentLogin := "student-" + token

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `insert into users(id,status,role) values($1,'active','teacher')`, teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into organizations(id,github_login,name) values($1,$2,'GitHub import organization')`, organizationID, organizationLogin); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into groups(id,code,name,organization_id) values($1,$2,'GitHub import group',$3)`, groupID, groupCode, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into group_teachers(group_id,teacher_user_id,assigned_by) values($1,$2,$2)`, groupID, teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into student_records(id,group_id,github_login_expected,full_name,created_source) values($1,$2,$3,'Student Name','group_import')`, recordID, groupID, studentLogin); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `insert into labs(id,group_id,code,name,time_limit_seconds,top_n) values($1,$2,'labwork1','Labwork 1',900,5)`, labID, groupID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	repository := postgres.NewImportRepository(db)
	contextValue, err := repository.GitHubImportContext(ctx, teacherID)
	if err != nil {
		t.Fatal(err)
	}
	var managed *postgres.GitHubImportGroup
	for index := range contextValue.Groups {
		if contextValue.Groups[index].ID == groupID {
			managed = &contextValue.Groups[index]
			break
		}
	}
	if managed == nil || managed.OrganizationLogin != organizationLogin || len(managed.Students) != 1 || len(managed.Labs) != 1 {
		t.Fatalf("unexpected GitHub import context: %#v", contextValue)
	}

	commitSHA := "0123456789abcdef0123456789abcdef01234567"
	repositoryName := organizationLogin + "/labwork1-" + studentLogin
	manifestJSON := []byte(`{"schema_version":1,"kind":"lab","group_code":"` + groupCode + `","lab_code":"labwork1","student":{"github_login":"` + studentLogin + `"},"project_path":"project","version_label":"deadline_0","source":"teacher_import","source_reference":"https://github.com/` + repositoryName + `/tree/` + commitSHA + `","commit_sha":"` + commitSHA + `"}`)
	analysis := importer.Analysis{
		Manifest: importer.Manifest{
			SchemaVersion: 1, Kind: "lab", GroupCode: groupCode, LabCode: "labwork1",
			Student: importer.Student{GitHubLogin: studentLogin}, ProjectPath: "project",
			VersionLabel: "deadline_0", Source: "teacher_import",
			SourceReference: "https://github.com/" + repositoryName + "/tree/" + commitSHA,
			CommitSHA:       commitSHA,
		},
		ManifestJSON: manifestJSON,
		Projects: []importer.Project{{
			Item:      importer.Item{LabCode: "labwork1", Student: importer.Student{GitHubLogin: studentLogin}, ProjectPath: "project"},
			FileCount: 2,
			Size:      128,
		}},
		TotalSize: 128,
	}
	objectID, _ := domain.NewUUIDv7()
	originalKey, _ := appstorage.NewKey(appstorage.OriginalArchives, objectID, ".zip")
	originalDigest := sha256.Sum256([]byte("github archive"))
	created, err := repository.CreateGitHubReview(ctx, teacherID, groupID, repositoryName, appstorage.Object{Key: originalKey, Size: 256, SHA256: originalDigest}, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "review_pending" || len(created.Items) != 1 {
		t.Fatalf("unexpected pending import: %#v", created)
	}
	if _, err = repository.CreateGitHubReview(ctx, teacherID, groupID, organizationLogin+"/labwork1-someone-else", appstorage.Object{Key: originalKey, Size: 256, SHA256: originalDigest}, analysis); err == nil {
		t.Fatal("mismatched repository was accepted")
	}
	if _, err = repository.Review(ctx, teacherID, created.ID, "approve", hex.EncodeToString(originalDigest[:]), map[string]any{"paths_checked": true, "ownership_checked": true}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.BeginApply(ctx, teacherID, created.ID); err != nil {
		t.Fatal(err)
	}
	normalizedID, _ := domain.NewUUIDv7()
	normalizedKey, _ := appstorage.NewKey(appstorage.NormalizedSubmissions, normalizedID, ".zip")
	normalizedDigest := sha256.Sum256([]byte("normalized github archive"))
	if _, err = repository.CommitApply(ctx, teacherID, created.ID, []postgres.ApplyItem{{
		ItemID: created.Items[0].ID, StudentRecordID: recordID, LabID: labID,
		ObjectKey: normalizedKey, SHA256: normalizedDigest, SourceManifestJSON: manifestJSON,
		VersionLabel: "deadline_0", SourceReference: analysis.Manifest.SourceReference, CommitSHA: commitSHA,
	}}); err != nil {
		t.Fatal(err)
	}
	var source, storedCommit string
	if err = db.Pool().QueryRow(ctx, `select source::text,commit_sha from submission_versions where student_record_id=$1 and lab_id=$2`, recordID, labID).Scan(&source, &storedCommit); err != nil {
		t.Fatal(err)
	}
	if source != "teacher_import" || storedCommit != commitSHA {
		t.Fatalf("unexpected GitHub version metadata: source=%q commit=%q", source, storedCommit)
	}
}

func TestIdentityUniquenessAndStateTransition(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	userA, _ := domain.NewUUIDv7()
	userB, _ := domain.NewUUIDv7()
	identityA, _ := domain.NewUUIDv7()
	identityB, _ := domain.NewUUIDv7()
	loginSuffix, _ := domain.NewUUIDv7()
	githubUserID := time.Now().UnixNano()
	if _, err := db.Pool().Exec(ctx, "insert into users(id) values ($1),($2)", userA, userB); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,clock_timestamp())`, identityA, userA, githubUserID, "stage2-"+loginSuffix); err != nil {
		t.Fatal(err)
	}
	_, err := db.Pool().Exec(ctx, `insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,clock_timestamp())`, identityB, userB, githubUserID, "other-"+loginSuffix)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected unique violation, got %v", err)
	}

	groupID, _ := domain.NewUUIDv7()
	importID, _ := domain.NewUUIDv7()
	if _, err := db.Pool().Exec(ctx, "insert into groups(id,code,name) values($1,$2,$3)", groupID, "g-"+loginSuffix, "Stage 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `insert into imports(id,group_id,uploaded_by,kind,original_object_key,original_sha256,compressed_size,schema_version) values($1,$2,$3,'group',$4,$5,0,1)`, importID, groupID, userA, "test/"+importID, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	_, err = db.Pool().Exec(ctx, "update imports set state='completed' where id=$1", importID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23000" {
		t.Fatalf("expected transition violation, got %v", err)
	}
}
