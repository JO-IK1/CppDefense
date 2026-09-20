package httpapi

import (
	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"net/http"
)

type adminHTTP struct {
	repository *postgres.AdminRepository
	auth       *authHTTP
	audit      *audit.Service
}

func (h *adminHTTP) users(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	v, e := h.repository.ListUsers(r.Context(), a.ID)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}
func (h *adminHTTP) pendingLinks(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	v, e := h.repository.ListPendingLinks(r.Context(), a.ID)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}
func (h *adminHTTP) setRole(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &in) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Role is invalid")
		return
	}
	v, e := h.repository.SetRole(r.Context(), a.ID, r.PathValue("user_id"), in.Role)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	target := r.PathValue("user_id")
	if e = h.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "admin.user.role", TargetType: "user", TargetID: &target, RequestID: RequestID(r.Context()), Metadata: map[string]any{"role": in.Role}}); e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *adminHTTP) block(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &in) || in.Reason == "" {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Reason is required")
		return
	}
	v, e := h.repository.Block(r.Context(), a.ID, r.PathValue("user_id"), in.Reason)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	target := r.PathValue("user_id")
	if e = h.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "admin.user.block", TargetType: "user", TargetID: &target, RequestID: RequestID(r.Context()), Reason: optionalString(in.Reason)}); e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *adminHTTP) assignTeacher(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		UserID string `json:"user_id"`
	}
	if !decodeJSON(w, r, &in) || !uuidPattern.MatchString(in.UserID) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "User is invalid")
		return
	}
	if e := h.repository.AssignTeacher(r.Context(), a.ID, in.UserID, r.PathValue("group_id")); e != nil {
		h.fail(w, r, e)
		return
	}
	target := r.PathValue("group_id")
	if e := h.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "admin.group.assign_teacher", TargetType: "group", TargetID: &target, RequestID: RequestID(r.Context()), Metadata: map[string]any{"teacher_user_id": in.UserID}}); e != nil {
		h.fail(w, r, e)
		return
	}
	w.WriteHeader(204)
}
func (h *adminHTTP) view(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		Role            string `json:"role"`
		GroupID         string `json:"group_id"`
		StudentRecordID string `json:"student_record_id"`
	}
	if !decodeJSON(w, r, &in) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "View mode is invalid")
		return
	}
	target := in.GroupID
	if in.Role == "student" {
		target = in.StudentRecordID
	}
	if e := h.auth.sessions.SetViewAs(r.Context(), s.ID, a.ID, in.Role, target); e != nil {
		h.fail(w, r, e)
		return
	}
	if e := h.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "admin.view_as.start", TargetType: "session", TargetID: &s.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"role": in.Role, "target_id": target}}); e != nil {
		h.fail(w, r, e)
		return
	}
	value := presentUser(a)
	value.ViewAs = presentViewAs(in.Role, &target)
	writeJSON(w, 200, value)
}
func (h *adminHTTP) clearView(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeCSRF(w, r, s, c) {
		return
	}
	if e := h.auth.sessions.ClearViewAs(r.Context(), s.ID, a.ID); e != nil {
		h.fail(w, r, e)
		return
	}
	if e := h.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "admin.view_as.stop", TargetType: "session", TargetID: &s.ID, RequestID: RequestID(r.Context())}); e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, presentUser(a))
}
func (h *adminHTTP) fail(w http.ResponseWriter, r *http.Request, e error) {
	if e == appauth.ErrForbidden {
		writeProblem(w, r, 403, "FORBIDDEN", "Operation is not allowed")
		return
	}
	if e == appauth.ErrConflict {
		writeProblem(w, r, 409, "STATE_CONFLICT", "Resource state has changed")
		return
	}
	writeProblem(w, r, 500, "ADMIN_ERROR", "Administrative operation failed")
}
