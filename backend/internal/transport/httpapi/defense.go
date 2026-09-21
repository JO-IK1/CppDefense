package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5"
)

type defenseHTTP struct {
	repository  *postgres.DefenseRepository
	auth        *authHTTP
	files       appstorage.FileStorage
	runnerToken string
}

func (h *defenseHTTP) create(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		SubmissionVersionID string `json:"submission_version_id"`
	}
	if !decodeJSON(w, r, &in) || !uuidPattern.MatchString(in.SubmissionVersionID) || !validIdempotencyKey(r) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Request is invalid")
		return
	}
	v, e := h.repository.Create(r.Context(), a.ID, in.SubmissionVersionID, r.Header.Get("Idempotency-Key"))
	if e != nil {
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "defense.start", TargetType: "defense", TargetID: &v.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"submission_version_id": in.SubmissionVersionID}}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record defense")
		return
	}
	writeJSON(w, 202, v)
}
func (h *defenseHTTP) get(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	v, e := h.repository.Get(r.Context(), a.ID, r.PathValue("defense_id"))
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *defenseHTTP) pendingConfiguration(w http.ResponseWriter, r *http.Request) {
	_, actor, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	if actor.Role == nil || (*actor.Role != "teacher" && *actor.Role != "admin") {
		writeProblem(w, r, 403, "FORBIDDEN", "Teacher or admin role is required")
		return
	}
	values, e := h.repository.PendingConfiguration(r.Context(), actor.ID)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": values})
}
func (h *defenseHTTP) configure(w http.ResponseWriter, r *http.Request) {
	session, actor, cookie, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, session, cookie) {
		return
	}
	if actor.Role == nil || (*actor.Role != "teacher" && *actor.Role != "admin") {
		writeProblem(w, r, 403, "FORBIDDEN", "Teacher or admin role is required")
		return
	}
	var input struct {
		SelectionMode    string `json:"selection_mode"`
		CandidateID      string `json:"candidate_id"`
		TimeLimitSeconds int    `json:"time_limit_seconds"`
	}
	if !decodeJSON(w, r, &input) || !validIdempotencyKey(r) || (input.SelectionMode == "manual" && !uuidPattern.MatchString(input.CandidateID)) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Defense configuration is invalid")
		return
	}
	value, e := h.repository.Configure(r.Context(), actor.ID, r.PathValue("defense_id"), input.SelectionMode, input.CandidateID, input.TimeLimitSeconds)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &actor.ID, ActorKind: "user", Action: "defense.configure", TargetType: "defense", TargetID: &value.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"selection_mode": input.SelectionMode}}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record defense configuration")
		return
	}
	writeJSON(w, 202, value)
}

func (h *defenseHTTP) repositoryFiles(w http.ResponseWriter, r *http.Request) {
	_, actor, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	archive, _, e := h.openDefenseArchive(r, actor.ID)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	items := make([]map[string]any, 0, len(archive.File))
	for _, file := range archive.File {
		if file.FileInfo().IsDir() || !safeRepositoryPath(file.Name) {
			continue
		}
		items = append(items, map[string]any{"path": file.Name, "size": file.UncompressedSize64})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (h *defenseHTTP) repositoryFile(w http.ResponseWriter, r *http.Request) {
	_, actor, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	wanted := r.URL.Query().Get("path")
	if !safeRepositoryPath(wanted) {
		writeProblem(w, r, 400, "INVALID_PATH", "Repository path is invalid")
		return
	}
	archive, access, e := h.openDefenseArchive(r, actor.ID)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	for _, file := range archive.File {
		if file.Name != wanted || file.FileInfo().IsDir() {
			continue
		}
		if file.UncompressedSize64 > 2<<20 {
			writeProblem(w, r, 413, "FILE_TOO_LARGE", "File is too large for the browser")
			return
		}
		reader, openErr := file.Open()
		if openErr != nil {
			h.fail(w, r, openErr)
			return
		}
		content, readErr := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
		reader.Close()
		if readErr != nil || len(content) > 2<<20 || !utf8.Valid(content) {
			writeProblem(w, r, 422, "UNSUPPORTED_FILE", "Only UTF-8 text files up to 2 MiB can be viewed")
			return
		}
		if wanted == access.SelectedFile {
			content, e = maskRepositoryBody(content, access.BodyBegin, access.BodyEnd)
			if e != nil {
				h.fail(w, r, e)
				return
			}
		}
		writeJSON(w, 200, map[string]any{"path": wanted, "content": string(content), "masked": wanted == access.SelectedFile})
		return
	}
	writeProblem(w, r, 404, "NOT_FOUND", "Repository file was not found")
}

func (h *defenseHTTP) openDefenseArchive(r *http.Request, actor string) (*zip.Reader, postgres.DefenseSourceAccess, error) {
	access, e := h.repository.SourceAccess(r.Context(), actor, r.PathValue("defense_id"))
	if e != nil {
		return nil, postgres.DefenseSourceAccess{}, e
	}
	key, e := appstorage.ParseKey(access.ObjectKey)
	if e != nil {
		return nil, postgres.DefenseSourceAccess{}, e
	}
	reader, object, e := h.files.Open(r.Context(), key)
	if e != nil {
		return nil, postgres.DefenseSourceAccess{}, e
	}
	defer reader.Close()
	if object.Size < 0 || object.Size > 256<<20 {
		return nil, postgres.DefenseSourceAccess{}, errors.New("repository archive is too large")
	}
	data, e := io.ReadAll(io.LimitReader(reader, (256<<20)+1))
	if e != nil || len(data) > 256<<20 {
		return nil, postgres.DefenseSourceAccess{}, errors.New("could not read repository archive")
	}
	archive, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	return archive, access, e
}

func safeRepositoryPath(value string) bool {
	return value != "" && value == path.Clean(value) && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func maskRepositoryBody(source []byte, begin, end int64) ([]byte, error) {
	if begin < 0 || end <= begin+1 || end > int64(len(source)) || source[begin] != '{' || source[end-1] != '}' {
		return nil, errors.New("invalid selected function offsets")
	}
	masked := bytes.Clone(source)
	for i := begin + 1; i < end-1; i++ {
		if masked[i] != '\n' && masked[i] != '\r' {
			masked[i] = ' '
		}
	}
	marker := []byte("/* TODO */")
	for i := begin + 1; i+int64(len(marker)) <= end-1; i++ {
		valid := true
		for j := range marker {
			if masked[i+int64(j)] == '\n' || masked[i+int64(j)] == '\r' {
				valid = false
				break
			}
		}
		if valid {
			copy(masked[i:i+int64(len(marker))], marker)
			break
		}
	}
	return masked, nil
}
func (h *defenseHTTP) draft(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	version, e := strconv.ParseInt(strings.Trim(r.Header.Get("If-Match"), "\""), 10, 64)
	var in struct {
		Answer string `json:"answer"`
	}
	if e != nil || !decodeJSON(w, r, &in) || len(in.Answer) > 1<<20 {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Draft is invalid")
		return
	}
	next, saved, e := h.repository.SaveDraft(r.Context(), a.ID, r.PathValue("defense_id"), in.Answer, version)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"version": next, "saved_at": saved})
}
func (h *defenseHTTP) attempt(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		Answer string `json:"answer"`
	}
	if !decodeJSON(w, r, &in) || len(in.Answer) > 1<<20 || !validIdempotencyKey(r) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Attempt is invalid")
		return
	}
	v, e := h.repository.Attempt(r.Context(), a.ID, r.PathValue("defense_id"), r.Header.Get("Idempotency-Key"), in.Answer)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "defense.attempt", TargetType: "attempt", TargetID: &v.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"defense_id": v.DefenseID, "attempt_no": v.AttemptNo}}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record attempt")
		return
	}
	writeJSON(w, 202, v)
}
func (h *defenseHTTP) cancel(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	if !validIdempotencyKey(r) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Idempotency-Key is required")
		return
	}
	v, e := h.repository.Cancel(r.Context(), a.ID, r.PathValue("defense_id"))
	if e != nil {
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "defense.cancel", TargetType: "defense", TargetID: &v.ID, RequestID: RequestID(r.Context())}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record cancellation")
		return
	}
	writeJSON(w, 200, v)
}
func (h *defenseHTTP) getAttempt(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	v, e := h.repository.GetAttempt(r.Context(), a.ID, r.PathValue("attempt_id"))
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *defenseHTTP) history(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	groupID, studentID, labID, outcome := r.URL.Query().Get("group_id"), r.URL.Query().Get("student_record_id"), r.URL.Query().Get("lab_id"), r.URL.Query().Get("outcome")
	for _, id := range []string{groupID, studentID, labID} {
		if id != "" && !uuidPattern.MatchString(id) {
			writeProblem(w, r, 400, "INVALID_REQUEST", "History filter is invalid")
			return
		}
	}
	if outcome != "" && outcome != "pending" && outcome != "passed" && outcome != "failed" && outcome != "error" {
		writeProblem(w, r, 400, "INVALID_REQUEST", "History outcome is invalid")
		return
	}
	values, e := h.repository.History(r.Context(), a.ID, groupID, studentID, labID, outcome, limit)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": values, "next_cursor": nil})
}
func (h *defenseHTTP) lease(w http.ResponseWriter, r *http.Request) {
	if !h.runnerAuthorized(r) {
		writeProblem(w, r, 401, "RUNNER_UNAUTHORIZED", "Runner token is invalid")
		return
	}
	var in struct {
		RunnerID string `json:"runner_id"`
		Capacity int    `json:"capacity"`
	}
	if !decodeJSON(w, r, &in) || !uuidPattern.MatchString(in.RunnerID) || in.Capacity < 1 || in.Capacity > 6 {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Lease request is invalid")
		return
	}
	digest := sha256.Sum256([]byte(h.runnerToken))
	v, e := h.repository.Lease(r.Context(), in.RunnerID, in.Capacity, digest[:])
	if errors.Is(e, pgx.ErrNoRows) {
		w.WriteHeader(204)
		return
	}
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *defenseHTTP) heartbeat(w http.ResponseWriter, r *http.Request) {
	if !h.runnerAuthorized(r) {
		writeProblem(w, r, 401, "RUNNER_UNAUTHORIZED", "Runner token is invalid")
		return
	}
	if e := h.repository.Heartbeat(r.Context(), r.PathValue("job_id"), r.Header.Get("X-Lease-Token")); e != nil {
		h.fail(w, r, e)
		return
	}
	w.WriteHeader(204)
}
func (h *defenseHTTP) complete(w http.ResponseWriter, r *http.Request) {
	if !h.runnerAuthorized(r) {
		writeProblem(w, r, 401, "RUNNER_UNAUTHORIZED", "Runner token is invalid")
		return
	}
	var in postgres.Completion
	if !decodeJSON(w, r, &in) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Completion is invalid")
		return
	}
	v, e := h.repository.Complete(r.Context(), r.PathValue("job_id"), r.Header.Get("X-Lease-Token"), in)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	target := r.PathValue("job_id")
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorKind: "runner", Action: "runner.job.complete", TargetType: "runner_job", TargetID: &target, RequestID: RequestID(r.Context()), Metadata: map[string]any{"outcome": in.Outcome}}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record runner completion")
		return
	}
	if v == nil {
		writeJSON(w, 200, map[string]any{"status": "prepared"})
		return
	}
	writeJSON(w, 200, v)
}
func (h *defenseHTTP) runnerAuthorized(r *http.Request) bool {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return provided != "" && hmac.Equal([]byte(provided), []byte(h.runnerToken))
}
func (h *defenseHTTP) fail(w http.ResponseWriter, r *http.Request, e error) {
	if e == appauth.ErrForbidden {
		writeProblem(w, r, 404, "NOT_FOUND", "Resource not found")
		return
	}
	if e == appauth.ErrConflict {
		writeProblem(w, r, 409, "STATE_CONFLICT", "Resource state has changed")
		return
	}
	writeProblem(w, r, 500, "DEFENSE_ERROR", "Defense operation failed")
}
