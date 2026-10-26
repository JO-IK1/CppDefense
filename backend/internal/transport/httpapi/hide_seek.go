package httpapi

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
)

func archiveSources(archive *zip.Reader) (map[string][]byte, error) {
	files := map[string][]byte{}
	total := 0
	for _, file := range archive.File {
		if file.FileInfo().IsDir() || !hideseek.Editable(file.Name) || file.UncompressedSize64 > 2<<20 {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
		reader.Close()
		if err != nil {
			return nil, err
		}
		if len(data) <= 2<<20 && utf8.Valid(data) {
			total += len(data)
			if total > 64<<20 {
				return nil, fmt.Errorf("слишком много исходного кода для режима Прятки")
			}
			files[file.Name] = data
		}
	}
	return files, nil
}

func (h *defenseHTTP) configureHide(w http.ResponseWriter, r *http.Request) {
	session, actor, claims, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, session, claims) {
		return
	}
	if actor.Role == nil || (*actor.Role != "teacher" && *actor.Role != "admin") {
		writeProblem(w, r, 403, "FORBIDDEN", "Нужна роль преподавателя")
		return
	}
	var input struct {
		TimeLimitSeconds int                 `json:"time_limit_seconds"`
		Mutations        []hideseek.Mutation `json:"mutations"`
	}
	if !decodeJSONLimit(w, r, &input, 256<<10) {
		return
	}
	if !validIdempotencyKey(r) {
		writeProblem(w, r, 400, "INVALID_REQUEST", "Idempotency key is required")
		return
	}
	if input.TimeLimitSeconds < 60 || input.TimeLimitSeconds > 1800 {
		writeProblem(w, r, 422, "INVALID_TIME", "Время защиты — от 1 до 30 минут")
		return
	}
	archive, _, err := h.openDefenseArchive(r, actor.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	files, err := archiveSources(archive)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err = hideseek.Validate(input.Mutations, files); err != nil {
		writeProblem(w, r, 422, "INVALID_MUTATIONS", err.Error())
		return
	}
	value, err := h.repository.ConfigureHide(r.Context(), actor.ID, r.PathValue("defense_id"), input.TimeLimitSeconds, input.Mutations)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &actor.ID, ActorKind: "user", Action: "defense.configure_hide_seek", TargetType: "defense", TargetID: &value.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"error_count": len(input.Mutations), "time_limit_seconds": input.TimeLimitSeconds}}); err != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Could not record defense configuration")
		return
	}
	writeJSON(w, 202, value)
}

func (h *defenseHTTP) validateAnswer(r *http.Request, actor, id, answer string) error {
	defense, err := h.repository.Get(r.Context(), actor, id)
	if err != nil {
		return err
	}
	if defense.DefenseMode != "hide_seek" {
		return nil
	}
	archive, _, err := h.openDefenseArchive(r, actor)
	if err != nil {
		return err
	}
	files, err := archiveSources(archive)
	if err != nil {
		return err
	}
	_, err = hideseek.DecodeEdits(answer, files)
	return err
}
