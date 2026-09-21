package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type Dependencies struct {
	Logger        *slog.Logger
	Ready         func(context.Context) error
	Audit         *audit.Service
	Auth          *appauth.Service
	Sessions      SessionStore
	SessionConfig config.Session
	Config        config.HTTP
	Catalog       *postgres.CatalogRepository
	Imports       *postgres.ImportRepository
	Files         appstorage.FileStorage
	Submissions   *appstorage.SubmissionService
	Downloads     *appstorage.DownloadService
	Defenses      *postgres.DefenseRepository
	RunnerToken   string
	Admin         *postgres.AdminRepository
}

func New(dependencies Dependencies) http.Handler {
	if dependencies.Logger == nil || dependencies.Ready == nil || dependencies.Audit == nil || dependencies.Auth == nil || dependencies.Sessions == nil || dependencies.Catalog == nil {
		panic("http api dependencies are required")
	}
	mux := http.NewServeMux()
	authHandler := &authHTTP{service: dependencies.Auth, sessions: dependencies.Sessions, csrfKey: dependencies.SessionConfig.CSRFKey, sessionTTL: dependencies.SessionConfig.TTL, secure: isHTTPS(dependencies.Config.PublicOrigin), audit: dependencies.Audit}
	webHandler := newWebHTTP(authHandler, dependencies.Catalog)
	mux.HandleFunc("GET /", webHandler.index)
	mux.HandleFunc("GET /app", webHandler.app)
	mux.Handle("GET /static/", webHandler.static)
	mux.HandleFunc("GET /api/v1/auth/github/start", authHandler.start)
	mux.HandleFunc("GET /api/v1/auth/github/callback", authHandler.callback)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.logout)
	mux.HandleFunc("GET /api/v1/me", authHandler.me)
	mux.HandleFunc("POST /api/v1/admin/pending-links/{user_id}/approve", authHandler.approve)
	mux.HandleFunc("POST /api/v1/admin/pending-links/{user_id}/reject", authHandler.reject)
	if dependencies.Admin != nil {
		adminHandler := &adminHTTP{repository: dependencies.Admin, auth: authHandler, audit: dependencies.Audit}
		mux.HandleFunc("GET /api/v1/admin/pending-links", adminHandler.pendingLinks)
		mux.HandleFunc("GET /api/v1/admin/users", adminHandler.users)
		mux.HandleFunc("PUT /api/v1/admin/users/{user_id}/role", adminHandler.setRole)
		mux.HandleFunc("POST /api/v1/admin/users/{user_id}/block", adminHandler.block)
		mux.HandleFunc("POST /api/v1/admin/groups/{group_id}/teachers", adminHandler.assignTeacher)
		mux.HandleFunc("PUT /api/v1/admin/view-as-role", adminHandler.view)
		mux.HandleFunc("DELETE /api/v1/admin/view-as-role", adminHandler.clearView)
	}
	catalogHandler := &catalogHTTP{repository: dependencies.Catalog, auth: authHandler, downloads: dependencies.Downloads}
	mux.HandleFunc("GET /api/v1/groups", catalogHandler.groups)
	mux.HandleFunc("POST /api/v1/groups", catalogHandler.createGroup)
	mux.HandleFunc("GET /api/v1/groups/{group_id}/labs", catalogHandler.labs)
	mux.HandleFunc("POST /api/v1/groups/{group_id}/labs", catalogHandler.createLab)
	mux.HandleFunc("GET /api/v1/groups/{group_id}/submissions", catalogHandler.submissions)
	if dependencies.Downloads != nil {
		mux.HandleFunc("GET /api/v1/submissions/{submission_id}/archive", catalogHandler.download)
	}
	if dependencies.Imports != nil && dependencies.Files != nil && dependencies.Submissions != nil {
		importHandler := &importHTTP{repository: dependencies.Imports, files: dependencies.Files, submissions: dependencies.Submissions, auth: authHandler}
		mux.HandleFunc("POST /api/v1/imports", importHandler.create)
		mux.HandleFunc("GET /api/v1/imports/{import_id}", importHandler.get)
		mux.HandleFunc("POST /api/v1/imports/{import_id}/review", importHandler.review)
		mux.HandleFunc("POST /api/v1/imports/{import_id}/apply", importHandler.apply)
	}
	if dependencies.Defenses != nil {
		defenseHandler := &defenseHTTP{repository: dependencies.Defenses, auth: authHandler, files: dependencies.Files, runnerToken: dependencies.RunnerToken}
		mux.HandleFunc("POST /api/v1/defenses", defenseHandler.create)
		mux.HandleFunc("GET /api/v1/defenses/{defense_id}", defenseHandler.get)
		mux.HandleFunc("GET /api/v1/teacher/defenses/pending", defenseHandler.pendingConfiguration)
		mux.HandleFunc("POST /api/v1/defenses/{defense_id}/configure", defenseHandler.configure)
		if dependencies.Files != nil {
			mux.HandleFunc("GET /api/v1/defenses/{defense_id}/repository", defenseHandler.repositoryFiles)
			mux.HandleFunc("GET /api/v1/defenses/{defense_id}/repository/file", defenseHandler.repositoryFile)
		}
		mux.HandleFunc("PUT /api/v1/defenses/{defense_id}/draft", defenseHandler.draft)
		mux.HandleFunc("POST /api/v1/defenses/{defense_id}/attempts", defenseHandler.attempt)
		mux.HandleFunc("POST /api/v1/defenses/{defense_id}/cancel", defenseHandler.cancel)
		mux.HandleFunc("GET /api/v1/attempts/{attempt_id}", defenseHandler.getAttempt)
		mux.HandleFunc("GET /api/v1/history/attempts", defenseHandler.history)
		mux.HandleFunc("POST /api/v1/runner/jobs/lease", defenseHandler.lease)
		mux.HandleFunc("POST /api/v1/runner/jobs/{job_id}/heartbeat", defenseHandler.heartbeat)
		mux.HandleFunc("POST /api/v1/runner/jobs/{job_id}/complete", defenseHandler.complete)
	}
	mux.HandleFunc("GET /health/live", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /health/ready", func(response http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), dependencies.Config.ReadyTimeout)
		defer cancel()
		if err := dependencies.Ready(ctx); err != nil {
			writeProblem(response, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Required dependencies are unavailable")
			return
		}
		response.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		writeProblem(response, request, http.StatusNotFound, "NOT_FOUND", "Resource not found")
	})

	return requestID(
		recoverPanic(dependencies.Logger,
			requestLog(dependencies.Logger,
				securityHeaders(mux))))
}

type problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	RequestID string `json:"request_id"`
}

func writeProblem(response http.ResponseWriter, request *http.Request, status int, code, title string) {
	response.Header().Set("Content-Type", "application/problem+json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(problem{
		Type:      "https://cppdefense.example/problems/" + code,
		Title:     title,
		Status:    status,
		Code:      code,
		RequestID: RequestID(request.Context()),
	})
}
