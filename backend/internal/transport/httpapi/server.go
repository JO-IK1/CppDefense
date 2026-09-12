package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
)

type Dependencies struct {
	Logger *slog.Logger
	Ready  func(context.Context) error
	Audit  *audit.Service
	Config config.HTTP
}

func New(dependencies Dependencies) http.Handler {
	if dependencies.Logger == nil || dependencies.Ready == nil || dependencies.Audit == nil {
		panic("http api dependencies are required")
	}
	mux := http.NewServeMux()
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
