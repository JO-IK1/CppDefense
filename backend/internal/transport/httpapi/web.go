package httpapi

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

//go:embed templates/*.html static/*
var webFiles embed.FS

type webHTTP struct {
	auth      *authHTTP
	catalog   *postgres.CatalogRepository
	templates *template.Template
	static    http.Handler
}
type appPage struct {
	User        appauth.User
	Role        string
	Viewing     bool
	Groups      []postgres.Group
	Submissions []postgres.Submission
}

func newWebHTTP(auth *authHTTP, catalog *postgres.CatalogRepository) *webHTTP {
	templates := template.Must(template.ParseFS(webFiles, "templates/*.html"))
	assets, _ := fs.Sub(webFiles, "static")
	return &webHTTP{auth: auth, catalog: catalog, templates: templates, static: http.StripPrefix("/static/", http.FileServer(http.FS(assets)))}
}
func (h *webHTTP) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "index.html", nil)
}
func (h *webHTTP) app(w http.ResponseWriter, r *http.Request) {
	session, user, _, ok := h.auth.authenticate(w, r)
	if !ok {
		return
	}
	page := appPage{User: user}
	if user.Role != nil {
		page.Role = *user.Role
	}
	if session.ViewAsRole != nil {
		page.Role = *session.ViewAsRole
		page.Viewing = true
	}
	if user.Status == "active" {
		if page.Role == "student" {
			if session.ViewAsRole != nil && session.ViewAsTargetID != nil {
				page.Submissions, _ = h.catalog.SubmissionsByRecord(r.Context(), user.ID, *session.ViewAsTargetID)
			} else {
				page.Submissions, _ = h.catalog.StudentSubmissions(r.Context(), user.ID)
			}
		} else {
			if session.ViewAsRole != nil && *session.ViewAsRole == "teacher" && session.ViewAsTargetID != nil {
				page.Groups, _ = h.catalog.GroupForAdminView(r.Context(), user.ID, *session.ViewAsTargetID)
			} else {
				page.Groups, _ = h.catalog.ListGroups(r.Context(), user.ID)
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "app.html", page)
}
