package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/JO-IK1/CppDefense/backend/internal/security"
	"github.com/jackc/pgx/v5"
)

const (
	sessionCookie = "cppdefense_session"
	csrfCookie    = "cppdefense_csrf"
)

type SessionStore interface {
	Create(context.Context, string, time.Duration) (postgres.NewSession, error)
	FindActive(context.Context, string) (postgres.Session, []byte, error)
	Revoke(context.Context, string) error
	SetViewAs(context.Context, string, string, string, string) error
	ClearViewAs(context.Context, string, string) error
}

type authHTTP struct {
	service    *appauth.Service
	sessions   SessionStore
	csrfKey    []byte
	sessionTTL time.Duration
	secure     bool
	audit      *audit.Service
}

type userResponse struct {
	ID          string             `json:"id"`
	Status      string             `json:"status"`
	Role        *string            `json:"role"`
	DisplayName *string            `json:"display_name"`
	Identities  []identityResponse `json:"identities"`
	CreatedAt   time.Time          `json:"created_at"`
	ViewAs      any                `json:"view_as,omitempty"`
}

type identityResponse struct {
	Provider   string    `json:"provider"`
	Subject    string    `json:"subject"`
	Login      string    `json:"login"`
	VerifiedAt time.Time `json:"verified_at"`
}

func (handler *authHTTP) start(response http.ResponseWriter, request *http.Request) {
	location, err := handler.service.Start(request.Context(), request.URL.Query().Get("return_to"))
	if err != nil {
		writeProblem(response, request, http.StatusBadRequest, "INVALID_RETURN_PATH", "Invalid return path")
		return
	}
	http.Redirect(response, request, location, http.StatusFound)
}

func (handler *authHTTP) callback(response http.ResponseWriter, request *http.Request) {
	user, returnPath, err := handler.service.Complete(request.Context(), request.URL.Query().Get("code"), request.URL.Query().Get("state"))
	if err != nil {
		switch {
		case errors.Is(err, appauth.ErrInvalidFlow):
			writeProblem(response, request, http.StatusBadRequest, "AUTH_STATE_INVALID", "OAuth flow is invalid or expired")
		case errors.Is(err, appauth.ErrBlockedUser):
			writeProblem(response, request, http.StatusForbidden, "USER_BLOCKED", "User is blocked")
		default:
			writeProblem(response, request, http.StatusBadGateway, "GITHUB_AUTHENTICATION_FAILED", "GitHub authentication failed")
		}
		return
	}
	session, err := handler.sessions.Create(request.Context(), user.ID, handler.sessionTTL)
	if err != nil {
		writeProblem(response, request, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "Could not create session")
		return
	}
	if previous, cookieErr := request.Cookie(sessionCookie); cookieErr == nil {
		if old, _, findErr := handler.sessions.FindActive(request.Context(), previous.Value); findErr == nil {
			_ = handler.sessions.Revoke(request.Context(), old.ID)
		}
	}
	if err := handler.audit.Record(request.Context(), audit.Event{ActorID: &user.ID, ActorKind: "user", Action: "auth.github.login", TargetType: "user", TargetID: &user.ID, RequestID: RequestID(request.Context()), Metadata: map[string]any{"status": user.Status}}); err != nil {
		_ = handler.sessions.Revoke(request.Context(), session.ID)
		writeProblem(response, request, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "Could not record login")
		return
	}
	handler.setCookies(response, session)
	if returnPath == "/" {
		returnPath = "/app"
	}
	http.Redirect(response, request, returnPath, http.StatusFound)
}

func (handler *authHTTP) me(response http.ResponseWriter, request *http.Request) {
	session, user, _, ok := handler.authenticate(response, request)
	if !ok {
		return
	}
	value := presentUser(user)
	if session.ViewAsRole != nil {
		value.ViewAs = presentViewAs(*session.ViewAsRole, session.ViewAsTargetID)
	}
	writeJSON(response, http.StatusOK, value)
}

func (handler *authHTTP) logout(response http.ResponseWriter, request *http.Request) {
	session, user, csrfHash, ok := handler.authenticate(response, request)
	if !ok {
		return
	}
	if err := security.VerifyCSRFToken(handler.csrfKey, session.ID, request.Header.Get("X-CSRF-Token"), csrfHash); err != nil {
		writeProblem(response, request, http.StatusForbidden, "CSRF_TOKEN_INVALID", "CSRF token is invalid")
		return
	}
	if err := handler.sessions.Revoke(request.Context(), session.ID); err != nil {
		writeProblem(response, request, http.StatusInternalServerError, "SESSION_REVOKE_FAILED", "Could not revoke session")
		return
	}
	if err := handler.audit.Record(request.Context(), audit.Event{ActorID: &user.ID, ActorKind: "user", Action: "auth.logout", TargetType: "session", TargetID: &session.ID, RequestID: RequestID(request.Context())}); err != nil {
		writeProblem(response, request, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "Could not record logout")
		return
	}
	handler.clearCookies(response)
	response.WriteHeader(http.StatusNoContent)
}

func (handler *authHTTP) approve(response http.ResponseWriter, request *http.Request) {
	session, actor, csrfHash, ok := handler.authenticate(response, request)
	if !ok || !handler.authorizeMutation(response, request, session, csrfHash) {
		return
	}
	var input struct {
		StudentRecordID string `json:"student_record_id"`
		Reason          string `json:"reason"`
	}
	if !decodeJSON(response, request, &input) || !uuidPattern.MatchString(input.StudentRecordID) || !validIdempotencyKey(request) {
		writeProblem(response, request, http.StatusBadRequest, "INVALID_REQUEST", "Request is invalid")
		return
	}
	userID := request.PathValue("user_id")
	if !uuidPattern.MatchString(userID) {
		writeProblem(response, request, http.StatusBadRequest, "INVALID_USER_ID", "User ID is invalid")
		return
	}
	user, err := handler.service.Approve(request.Context(), actor.ID, userID, input.StudentRecordID, request.Header.Get("Idempotency-Key"))
	if err != nil {
		handler.writeAuthorizationError(response, request, err)
		return
	}
	if err := handler.audit.Record(request.Context(), audit.Event{ActorID: &actor.ID, ActorKind: "user", Action: "student_link.approve", TargetType: "user", TargetID: &user.ID, RequestID: RequestID(request.Context()), Reason: optionalString(input.Reason), Metadata: map[string]any{"student_record_id": input.StudentRecordID}}); err != nil {
		writeProblem(response, request, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "Could not record approval")
		return
	}
	writeJSON(response, http.StatusOK, presentUser(user))
}

func (handler *authHTTP) reject(response http.ResponseWriter, request *http.Request) {
	session, actor, csrfHash, ok := handler.authenticate(response, request)
	if !ok || !handler.authorizeMutation(response, request, session, csrfHash) {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(response, request, &input) || !validIdempotencyKey(request) {
		writeProblem(response, request, http.StatusBadRequest, "INVALID_REQUEST", "Request is invalid")
		return
	}
	userID := request.PathValue("user_id")
	if !uuidPattern.MatchString(userID) {
		writeProblem(response, request, http.StatusBadRequest, "INVALID_USER_ID", "User ID is invalid")
		return
	}
	user, err := handler.service.Reject(request.Context(), actor.ID, userID, input.Reason, request.Header.Get("Idempotency-Key"))
	if err != nil {
		handler.writeAuthorizationError(response, request, err)
		return
	}
	if err := handler.audit.Record(request.Context(), audit.Event{ActorID: &actor.ID, ActorKind: "user", Action: "student_link.reject", TargetType: "user", TargetID: &user.ID, RequestID: RequestID(request.Context()), Reason: optionalString(input.Reason)}); err != nil {
		writeProblem(response, request, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "Could not record rejection")
		return
	}
	writeJSON(response, http.StatusOK, presentUser(user))
}

func (handler *authHTTP) authorizeMutation(response http.ResponseWriter, request *http.Request, session postgres.Session, csrfHash []byte) bool {
	if !handler.authorizeCSRF(response, request, session, csrfHash) {
		return false
	}
	if session.ViewAsRole != nil {
		writeProblem(response, request, http.StatusForbidden, "VIEW_AS_READ_ONLY", "Role preview is read-only")
		return false
	}
	return true
}

func (handler *authHTTP) authorizeCSRF(response http.ResponseWriter, request *http.Request, session postgres.Session, csrfHash []byte) bool {
	if err := security.VerifyCSRFToken(handler.csrfKey, session.ID, request.Header.Get("X-CSRF-Token"), csrfHash); err != nil {
		writeProblem(response, request, http.StatusForbidden, "CSRF_TOKEN_INVALID", "CSRF token is invalid")
		return false
	}
	return true
}

func (handler *authHTTP) writeAuthorizationError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, appauth.ErrForbidden):
		writeProblem(response, request, http.StatusForbidden, "FORBIDDEN", "Operation is not allowed")
	case errors.Is(err, appauth.ErrConflict):
		writeProblem(response, request, http.StatusConflict, "STATE_CONFLICT", "Resource state has changed")
	default:
		writeProblem(response, request, http.StatusUnprocessableEntity, "INVALID_REQUEST", "Request cannot be applied")
	}
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

func validIdempotencyKey(request *http.Request) bool {
	return uuidPattern.MatchString(request.Header.Get("Idempotency-Key"))
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (handler *authHTTP) authenticate(response http.ResponseWriter, request *http.Request) (postgres.Session, appauth.User, []byte, bool) {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeProblem(response, request, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication required")
		return postgres.Session{}, appauth.User{}, nil, false
	}
	session, csrfHash, err := handler.sessions.FindActive(request.Context(), cookie.Value)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			writeProblem(response, request, http.StatusInternalServerError, "SESSION_LOOKUP_FAILED", "Could not read session")
		} else {
			writeProblem(response, request, http.StatusUnauthorized, "SESSION_INVALID", "Session is invalid or expired")
		}
		return postgres.Session{}, appauth.User{}, nil, false
	}
	user, err := handler.service.User(request.Context(), session.UserID)
	if err != nil || user.Status == "blocked" {
		writeProblem(response, request, http.StatusUnauthorized, "SESSION_INVALID", "Session is invalid or expired")
		return postgres.Session{}, appauth.User{}, nil, false
	}
	return session, user, csrfHash, true
}

func (handler *authHTTP) setCookies(response http.ResponseWriter, session postgres.NewSession) {
	http.SetCookie(response, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()), HttpOnly: true, Secure: handler.secure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(response, &http.Cookie{Name: csrfCookie, Value: session.CSRFToken, Path: "/", Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()), Secure: handler.secure, SameSite: http.SameSiteLaxMode})
}

func (handler *authHTTP) clearCookies(response http.ResponseWriter) {
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(response, &http.Cookie{Name: name, Path: "/", MaxAge: -1, HttpOnly: name == sessionCookie, Secure: handler.secure, SameSite: http.SameSiteLaxMode})
	}
}

func presentUser(user appauth.User) userResponse {
	return userResponse{ID: user.ID, Status: user.Status, Role: user.Role, DisplayName: user.DisplayName, CreatedAt: user.CreatedAt,
		Identities: []identityResponse{{Provider: "github", Subject: user.Identity.Subject, Login: user.Identity.Login, VerifiedAt: user.Identity.VerifiedAt}}}
}

func presentViewAs(role string, target *string) map[string]any {
	value := map[string]any{"role": role, "read_only": true}
	if role == "teacher" {
		value["group_id"] = target
	}
	if role == "student" {
		value["student_record_id"] = target
	}
	return value
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func isHTTPS(origin string) bool { return strings.HasPrefix(strings.ToLower(origin), "https://") }
