package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type discardAudit struct{}

func (discardAudit) Append(context.Context, audit.Event) error { return nil }

type fakeAuthRepository struct{}

func (fakeAuthRepository) CreateFlow(context.Context, appauth.Flow) error { return nil }
func (fakeAuthRepository) ConsumeFlow(context.Context, []byte) (appauth.Flow, error) {
	return appauth.Flow{}, errors.New("not implemented")
}
func (fakeAuthRepository) LoginGitHub(context.Context, appauth.Profile) (appauth.User, error) {
	return appauth.User{}, errors.New("not implemented")
}
func (fakeAuthRepository) UserByID(context.Context, string) (appauth.User, error) {
	return appauth.User{}, errors.New("not implemented")
}
func (fakeAuthRepository) ApproveStudent(context.Context, string, string, string, string) (appauth.User, error) {
	return appauth.User{}, errors.New("not implemented")
}
func (fakeAuthRepository) RejectUser(context.Context, string, string, string, string) (appauth.User, error) {
	return appauth.User{}, errors.New("not implemented")
}

type fakeGitHub struct{}

func (fakeGitHub) Authenticate(context.Context, string, string, string, string) (appauth.Profile, error) {
	return appauth.Profile{}, errors.New("not implemented")
}

type fakeSessions struct{}

func (fakeSessions) Create(context.Context, string, time.Duration) (postgres.NewSession, error) {
	return postgres.NewSession{}, errors.New("not implemented")
}
func (fakeSessions) FindActive(context.Context, string) (postgres.Session, []byte, error) {
	return postgres.Session{}, nil, errors.New("not implemented")
}
func (fakeSessions) Revoke(context.Context, string) error                            { return nil }
func (fakeSessions) SetViewAs(context.Context, string, string, string, string) error { return nil }
func (fakeSessions) ClearViewAs(context.Context, string, string) error               { return nil }

func handler(ready func(context.Context) error) http.Handler {
	authService, err := appauth.New(appauth.Config{
		ClientID: "test", RedirectURL: "http://localhost/callback",
		AuthorizeURL: "https://github.example/authorize", FlowKey: make([]byte, 32), FlowTTL: time.Minute,
	}, fakeAuthRepository{}, fakeGitHub{})
	if err != nil {
		panic(err)
	}
	return New(Dependencies{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Ready:         ready,
		Audit:         audit.New(discardAudit{}),
		Auth:          authService,
		Sessions:      fakeSessions{},
		SessionConfig: config.Session{CSRFKey: make([]byte, 32), TTL: time.Hour},
		Catalog:       &postgres.CatalogRepository{},
		Config:        config.HTTP{ReadyTimeout: time.Second},
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

func TestGitHubLoginStartUsesPKCEAuthorizationRedirect(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/start?return_to=/labs", nil)
	response := httptest.NewRecorder()
	handler(func(context.Context) error { return nil }).ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d", response.Code)
	}
	location := response.Header().Get("Location")
	if !strings.HasPrefix(location, "https://github.example/authorize?") || !strings.Contains(location, "code_challenge_method=S256") {
		t.Fatalf("location = %q", location)
	}
}

func TestCurrentUserRequiresSession(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	response := httptest.NewRecorder()
	handler(func(context.Context) error { return nil }).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
}
