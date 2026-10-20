package httpapi

import (
	"bytes"
	"compress/zlib"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	"github.com/JO-IK1/CppDefense/backend/internal/application/importer"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
)

type localImportItem struct {
	ID         string `json:"id"`
	GroupID    string `json:"group_id"`
	GroupCode  string `json:"group_code"`
	Login      string `json:"github_login"`
	Lab        string `json:"lab_code"`
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
}
type localImportPlan struct {
	Actor   string            `json:"actor"`
	Expires int64             `json:"expires"`
	Items   []localImportItem `json:"items"`
}

var localDeadline = regexp.MustCompile(`^deadline_[0-9]+$`)

func signLocalPlan(plan localImportPlan, key []byte) string {
	data, _ := json.Marshal(plan)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write(data)
	_ = writer.Close()
	body := base64.RawURLEncoding.EncodeToString(compressed.Bytes())
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("local-import-v1:" + body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func readLocalPlan(token string, key []byte) (localImportPlan, error) {
	var plan localImportPlan
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(token) > 500000 {
		return plan, errors.New("invalid ticket")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return plan, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("local-import-v1:" + parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return plan, errors.New("invalid ticket")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return plan, err
	}
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return plan, err
	}
	defer reader.Close()
	data, err = io.ReadAll(io.LimitReader(reader, 512001))
	if err != nil || len(data) > 512000 {
		return plan, errors.New("invalid plan")
	}
	if err = json.Unmarshal(data, &plan); err != nil {
		return plan, err
	}
	if plan.Expires <= time.Now().Unix() || len(plan.Items) == 0 {
		return plan, errors.New("expired ticket")
	}
	return plan, nil
}
func (h *importHTTP) createLocalPlan(w http.ResponseWriter, r *http.Request) {
	session, actor, claims, ok := h.auth.authenticate(w, r)
	if !ok || !h.auth.authorizeMutation(w, r, session, claims) {
		return
	}
	if actor.Role == nil || (*actor.Role != "teacher" && *actor.Role != "admin") {
		writeProblem(w, r, 403, "FORBIDDEN", "Нужна роль преподавателя")
		return
	}
	var input struct {
		GroupID  string   `json:"group_id"`
		Students []string `json:"students"`
		Labs     []string `json:"labs"`
		Branch   string   `json:"branch"`
		All      bool     `json:"all_students"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&input) != nil || (input.Branch != "latest" && !localDeadline.MatchString(input.Branch)) {
		writeProblem(w, r, 400, "INVALID_PLAN", "Выберите ветку deadline_N или последний deadline")
		return
	}
	context, err := h.repository.GitHubImportContext(r.Context(), actor.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	plan := localImportPlan{Actor: actor.ID, Expires: time.Now().Add(2 * time.Hour).Unix()}
	contains := func(values []string, target string) bool {
		for _, v := range values {
			if strings.EqualFold(v, target) {
				return true
			}
		}
		return false
	}
	for _, group := range context.Groups {
		if group.ID != input.GroupID {
			continue
		}
		for _, student := range group.Students {
			if !input.All && !contains(input.Students, student.GitHubLogin) {
				continue
			}
			for _, lab := range group.Labs {
				if !contains(input.Labs, lab.Code) {
					continue
				}
				id, e := domain.NewUUIDv7()
				if e != nil {
					h.fail(w, r, e)
					return
				}
				plan.Items = append(plan.Items, localImportItem{ID: id, GroupID: group.ID, GroupCode: group.Code, Login: student.GitHubLogin, Lab: lab.Code, Repository: group.OrganizationLogin + "/" + lab.Code + "-" + student.GitHubLogin, Branch: input.Branch})
			}
		}
	}
	if len(plan.Items) == 0 || len(plan.Items) > 200 {
		writeProblem(w, r, 422, "INVALID_PLAN", "Выберите от 1 до 200 работ")
		return
	}
	writeJSON(w, 201, map[string]any{"ticket": signLocalPlan(plan, h.auth.csrfKey), "count": len(plan.Items), "expires_at": time.Unix(plan.Expires, 0)})
}
func (h *importHTTP) localPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := readLocalPlan(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), h.auth.csrfKey)
	if err != nil {
		writeProblem(w, r, 401, "TICKET_EXPIRED", "Создайте новую команду на сайте")
		return
	}
	writeJSON(w, 200, plan)
}
func (h *importHTTP) uploadLocalArchive(w http.ResponseWriter, r *http.Request) {
	plan, err := readLocalPlan(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), h.auth.csrfKey)
	if err != nil {
		writeProblem(w, r, 401, "TICKET_EXPIRED", "Создайте новую команду на сайте")
		return
	}
	var selected *localImportItem
	for i := range plan.Items {
		if plan.Items[i].ID == r.PathValue("item_id") {
			selected = &plan.Items[i]
			break
		}
	}
	if selected == nil {
		writeProblem(w, r, 403, "FORBIDDEN", "Работа не включена в команду")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 101<<20)
	if r.ParseMultipartForm(8<<20) != nil {
		writeProblem(w, r, 413, "ARCHIVE_TOO_LARGE", "Лимит ZIP — 100 МБ")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	branch := r.FormValue("branch")
	if !localDeadline.MatchString(branch) || (selected.Branch != "latest" && branch != selected.Branch) {
		writeProblem(w, r, 422, "INVALID_BRANCH", "Ветка не соответствует команде")
		return
	}
	file, _, err := r.FormFile("archive")
	if err != nil {
		writeProblem(w, r, 400, "ARCHIVE_REQUIRED", "Не передан ZIP")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (100<<20)+1))
	if err != nil || len(data) > 100<<20 {
		writeProblem(w, r, 413, "ARCHIVE_TOO_LARGE", "Лимит ZIP — 100 МБ")
		return
	}
	canonical, analysis, err := importer.PrepareGitHubArchive(data, importer.ProjectArchiveMetadata{GroupCode: selected.GroupCode, LabCode: selected.Lab, GitHubLogin: selected.Login, VersionLabel: branch, SourceReference: "https://github.com/" + selected.Repository + "/tree/" + r.FormValue("commit_sha"), CommitSHA: r.FormValue("commit_sha")}, importer.DefaultLimits())
	if err != nil {
		writeProblem(w, r, 422, "ARCHIVE_INVALID", err.Error())
		return
	}
	object, err := appstorage.StoreOriginalArchive(r.Context(), h.files, int64(len(canonical)), bytes.NewReader(canonical))
	if err != nil {
		writeProblem(w, r, 503, "STORAGE_UNAVAILABLE", "Не удалось сохранить ZIP")
		return
	}
	value, err := h.repository.CreateGitHubReview(r.Context(), plan.Actor, selected.GroupID, selected.Repository, object, analysis, selected.ID)
	if err != nil {
		_ = h.files.Delete(r.Context(), object.Key)
		h.fail(w, r, err)
		return
	}
	// Repeated uploads return the original import and discard the redundant object.
	if original, _, e := h.repository.OriginalObject(r.Context(), plan.Actor, value.ID); e == nil && original.String() != object.Key.String() {
		_ = h.files.Delete(r.Context(), object.Key)
	}
	if err = h.auth.audit.Record(r.Context(), audit.Event{ActorID: &plan.Actor, ActorKind: "user", Action: "teacher.local_import.upload", TargetType: "import", TargetID: &value.ID, RequestID: RequestID(r.Context()), Metadata: map[string]any{"repository": selected.Repository, "branch": branch}}); err != nil {
		writeProblem(w, r, 500, "AUDIT_WRITE_FAILED", "Не удалось записать событие импорта")
		return
	}
	writeJSON(w, 202, map[string]any{"import_id": value.ID, "state": value.State})
}
