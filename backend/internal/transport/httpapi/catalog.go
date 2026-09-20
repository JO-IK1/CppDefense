package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type catalogHTTP struct {
	repository *postgres.CatalogRepository
	auth       *authHTTP
	downloads  *appstorage.DownloadService
}

func (h *catalogHTTP) groups(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	v, e := h.repository.ListGroups(r.Context(), a.ID)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil})
}
func (h *catalogHTTP) createGroup(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		IsDemo bool   `json:"is_demo"`
	}
	if !decodeJSON(w, r, &in) || !validIdempotencyKey(r) || !codeOK(in.Code) || strings.TrimSpace(in.Name) == "" {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Request is invalid")
		return
	}
	v, e := h.repository.CreateGroup(r.Context(), a.ID, in.Code, in.Name, in.IsDemo)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "group.create", TargetType: "group", TargetID: &v.ID, RequestID: RequestID(r.Context())}); e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 201, v)
}
func (h *catalogHTTP) labs(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	g := r.PathValue("group_id")
	if !uuidPattern.MatchString(g) {
		writeProblem(w, r, 400, "INVALID_GROUP_ID", "Group ID is invalid")
		return
	}
	v, e := h.repository.ListLabs(r.Context(), a.ID, g)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *catalogHTTP) submissions(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	group := r.PathValue("group_id")
	if !uuidPattern.MatchString(group) {
		writeProblem(w, r, 400, "INVALID_GROUP_ID", "Group ID is invalid")
		return
	}
	values, e := h.repository.GroupSubmissions(r.Context(), a.ID, group)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": values})
}
func (h *catalogHTTP) download(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	id := r.PathValue("submission_id")
	if !uuidPattern.MatchString(id) {
		writeProblem(w, r, 404, "NOT_FOUND", "Submission not found")
		return
	}
	reader, object, e := h.downloads.OpenSubmission(r.Context(), a.ID, id)
	if e != nil {
		writeProblem(w, r, 404, "NOT_FOUND", "Submission not found")
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="submission-%s.zip"`, id))
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	_, _ = io.Copy(w, reader)
}
func (h *catalogHTTP) createLab(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	g := r.PathValue("group_id")
	var in struct {
		Code             string `json:"code"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		TimeLimitSeconds int    `json:"time_limit_seconds"`
		TopN             int    `json:"top_n"`
	}
	if !decodeJSON(w, r, &in) || !validIdempotencyKey(r) || !uuidPattern.MatchString(g) || !codeOK(in.Code) || strings.TrimSpace(in.Name) == "" || in.TimeLimitSeconds < 1 || in.TopN < 1 || in.TopN > 50 {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Request is invalid")
		return
	}
	v, e := h.repository.CreateLab(r.Context(), a.ID, g, in.Code, in.Name, in.Description, in.TimeLimitSeconds, in.TopN)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "lab.create", TargetType: "lab", TargetID: &v.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"group_id": g}}); e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 201, v)
}
func (h *catalogHTTP) fail(w http.ResponseWriter, r *http.Request, e error) {
	if e == appauth.ErrForbidden {
		writeProblem(w, r, 403, "FORBIDDEN", "Operation is not allowed")
		return
	}
	writeProblem(w, r, 500, "DATABASE_ERROR", "Database operation failed")
}
func codeOK(v string) bool {
	if len(v) < 1 || len(v) > 64 || v[0] == '-' || v[len(v)-1] == '-' {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}
