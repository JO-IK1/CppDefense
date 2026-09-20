package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5"
)

type defenseHTTP struct {
	repository  *postgres.DefenseRepository
	auth        *authHTTP
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
