package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/application/importer"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type importHTTP struct {
	repository  *postgres.ImportRepository
	files       appstorage.FileStorage
	submissions *appstorage.SubmissionService
	auth        *authHTTP
}

func (h *importHTTP) create(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	if !validIdempotencyKey(r) {
		writeProblem(w, r, 400, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 101<<20)
	if e := r.ParseMultipartForm(101 << 20); e != nil {
		writeProblem(w, r, 413, "ARCHIVE_TOO_LARGE", "Archive is too large")
		return
	}
	group, kind := r.FormValue("group_id"), r.FormValue("kind")
	if !uuidPattern.MatchString(group) || (kind != "group" && kind != "lab") {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Group and kind are required")
		return
	}
	file, header, e := r.FormFile("archive")
	if e != nil {
		writeProblem(w, r, 400, "ARCHIVE_REQUIRED", "ZIP archive is required")
		return
	}
	defer file.Close()
	if header.Size < 0 || header.Size > 100<<20 {
		writeProblem(w, r, 413, "ARCHIVE_TOO_LARGE", "Archive is too large")
		return
	}
	data, e := io.ReadAll(io.LimitReader(file, 100<<20+1))
	if e != nil || len(data) > 100<<20 {
		writeProblem(w, r, 413, "ARCHIVE_TOO_LARGE", "Archive is too large")
		return
	}
	analysis, e := importer.Analyze(data, kind, importer.DefaultLimits())
	if e != nil {
		writeProblem(w, r, 422, "ARCHIVE_INVALID", e.Error())
		return
	}
	object, e := appstorage.StoreOriginalArchive(r.Context(), h.files, int64(len(data)), bytes.NewReader(data))
	if e != nil {
		writeProblem(w, r, 503, "STORAGE_UNAVAILABLE", "Could not store archive")
		return
	}
	value, e := h.repository.CreateReview(r.Context(), a.ID, group, kind, object, analysis)
	if e != nil {
		_ = h.files.Delete(r.Context(), object.Key)
		h.fail(w, r, e)
		return
	}
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "import.upload", TargetType: "import", TargetID: &value.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"group_id": group, "kind": kind}}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record import")
		return
	}
	writeJSON(w, 202, value)
}
func (h *importHTTP) get(w http.ResponseWriter, r *http.Request) {
	_, a, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	v, e := h.repository.Get(r.Context(), a.ID, r.PathValue("import_id"))
	if e != nil {
		h.fail(w, r, e)
		return
	}
	writeJSON(w, 200, v)
}
func (h *importHTTP) review(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	var in struct {
		Decision      string         `json:"decision"`
		ArchiveSHA256 string         `json:"archive_sha256"`
		Checklist     map[string]any `json:"checklist"`
		Reason        string         `json:"reason"`
	}
	if !decodeJSON(w, r, &in) || !validIdempotencyKey(r) || (in.Decision != "approve" && in.Decision != "reject") {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Review is invalid")
		return
	}
	pathsChecked, pathsOK := in.Checklist["paths_checked"].(bool)
	ownershipChecked, ownershipOK := in.Checklist["ownership_checked"].(bool)
	if !pathsOK || !ownershipOK || !pathsChecked || !ownershipChecked || len(in.Checklist) != 2 {
		writeProblem(w, r, 400, "REVIEW_CHECKLIST_INCOMPLETE", "Review checklist must be confirmed")
		return
	}
	v, e := h.repository.Review(r.Context(), a.ID, r.PathValue("import_id"), in.Decision, in.ArchiveSHA256, in.Checklist, in.Reason)
	if e != nil {
		h.fail(w, r, e)
		return
	}
	target := r.PathValue("import_id")
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "import.review." + in.Decision, TargetType: "import", TargetID: &target, RequestID: RequestID(r.Context()), Reason: optionalString(in.Reason)}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record review")
		return
	}
	writeJSON(w, 200, v)
}
func (h *importHTTP) apply(w http.ResponseWriter, r *http.Request) {
	s, a, c, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, s, c) {
		return
	}
	source, e := h.repository.BeginApply(r.Context(), a.ID, r.PathValue("import_id"))
	if e != nil {
		h.fail(w, r, e)
		return
	}
	success := false
	defer func() {
		if !success {
			_ = h.repository.Finish(context.WithoutCancel(r.Context()), source.Import.ID, false)
		}
	}()
	reader, _, e := h.files.Open(r.Context(), source.ObjectKey)
	if e != nil {
		writeProblem(w, r, 503, "STORAGE_UNAVAILABLE", "Could not read archive")
		return
	}
	data, e := io.ReadAll(io.LimitReader(reader, 100<<20+1))
	reader.Close()
	if e != nil {
		writeProblem(w, r, 500, "ARCHIVE_READ_FAILED", "Could not read archive")
		return
	}
	analysis, e := importer.Analyze(data, source.Import.Kind, importer.DefaultLimits())
	if e != nil {
		writeProblem(w, r, 422, "ARCHIVE_INVALID", e.Error())
		return
	}
	var applyItems []postgres.ApplyItem
	var uploaded []appstorage.Key
	cleanup := func() {
		for _, key := range uploaded {
			_ = h.files.Delete(context.WithoutCancel(r.Context()), key)
		}
	}
	for _, project := range analysis.Projects {
		item, student, lab, e := h.repository.ItemTarget(r.Context(), source.Import.ID, project.Item.ProjectPath)
		if e != nil {
			cleanup()
			writeProblem(w, r, 409, "IMPORT_CHANGED", "Import review no longer matches")
			return
		}
		objectID, idErr := domain.NewUUIDv7()
		if idErr != nil {
			cleanup()
			writeProblem(w, r, 500, "IMPORT_APPLY_FAILED", "Could not allocate object")
			return
		}
		key, keyErr := appstorage.NewKey(appstorage.NormalizedSubmissions, objectID, ".zip")
		if keyErr != nil {
			cleanup()
			writeProblem(w, r, 500, "IMPORT_APPLY_FAILED", "Could not allocate object")
			return
		}
		object, e := h.files.Put(r.Context(), key, bytes.NewReader(project.Archive), appstorage.PutOptions{Size: int64(len(project.Archive)), ContentType: "application/zip"})
		if e != nil {
			cleanup()
			writeProblem(w, r, 500, "IMPORT_APPLY_FAILED", "Could not create submission version")
			return
		}
		uploaded = append(uploaded, key)
		applyItems = append(applyItems, postgres.ApplyItem{ItemID: item, StudentRecordID: student, LabID: lab, ObjectKey: key, SHA256: object.SHA256, SourceManifestJSON: analysis.ManifestJSON})
	}
	duplicates, e := h.repository.CommitApply(r.Context(), a.ID, source.Import.ID, applyItems)
	if e != nil {
		cleanup()
		writeProblem(w, r, 500, "IMPORT_APPLY_FAILED", "Could not complete import")
		return
	}
	for _, key := range duplicates {
		_ = h.files.Delete(context.WithoutCancel(r.Context()), key)
	}
	success = true
	if e = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &a.ID, ActorKind: "user", Action: "import.apply", TargetType: "import", TargetID: &source.Import.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"items": len(applyItems)}}); e != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record import application")
		return
	}
	writeJSON(w, 200, map[string]any{"id": source.Import.ID, "state": "completed"})
}
func (h *importHTTP) fail(w http.ResponseWriter, r *http.Request, e error) {
	if e == appauth.ErrForbidden {
		writeProblem(w, r, 403, "FORBIDDEN", "Import is not visible")
		return
	}
	message := e.Error()
	if strings.Contains(message, "mismatch") || strings.Contains(message, "unknown lab") || strings.Contains(message, "invalid") {
		writeProblem(w, r, 422, "IMPORT_INVALID", message)
		return
	}
	writeProblem(w, r, 409, "IMPORT_CONFLICT", "Import cannot be changed")
}
