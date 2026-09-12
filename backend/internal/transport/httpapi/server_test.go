package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
)

type discardAudit struct{}

func (discardAudit) Append(context.Context, audit.Event) error { return nil }

func handler(ready func(context.Context) error) http.Handler {
	return New(Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Ready:  ready,
		Audit:  audit.New(discardAudit{}),
		Config: config.HTTP{ReadyTimeout: time.Second},
	})
}

func TestLiveness(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	response := httptest.NewRecorder()
	handler(func(context.Context) error { return nil }).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
}

func TestReadinessFailureUsesProblemDetails(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	handler(func(context.Context) error { return errors.New("offline") }).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type = %q", got)
	}
}

func TestInvalidRequestIDIsReplaced(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.Header.Set("X-Request-ID", "attacker-controlled")
	response := httptest.NewRecorder()
	handler(func(context.Context) error { return nil }).ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") == "attacker-controlled" {
		t.Fatal("invalid request id was reflected")
	}
}
