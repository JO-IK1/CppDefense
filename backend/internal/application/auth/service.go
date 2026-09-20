package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/security"
)

var (
	ErrInvalidFlow = errors.New("invalid or expired OAuth flow")
	ErrBlockedUser = errors.New("user is blocked")
	ErrForbidden   = errors.New("operation is forbidden")
	ErrConflict    = errors.New("state conflict")
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthorizeURL string
	FlowKey      []byte
	FlowTTL      time.Duration
}

type Flow struct {
	StateHash         []byte
	EncryptedVerifier []byte
	ReturnPath        string
	ExpiresAt         time.Time
}

type Profile struct {
	ID          int64
	Login       string
	DisplayName string
	AvatarURL   string
}

type Identity struct {
	Subject    string    `json:"subject"`
	Login      string    `json:"login"`
	VerifiedAt time.Time `json:"verified_at"`
}

type User struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Role        *string   `json:"role"`
	DisplayName *string   `json:"display_name"`
	Identity    Identity  `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

type Repository interface {
	CreateFlow(context.Context, Flow) error
	ConsumeFlow(context.Context, []byte) (Flow, error)
	LoginGitHub(context.Context, Profile) (User, error)
	UserByID(context.Context, string) (User, error)
	ApproveStudent(context.Context, string, string, string, string) (User, error)
	RejectUser(context.Context, string, string, string, string) (User, error)
}

type GitHub interface {
	Authenticate(context.Context, string, string, string, string) (Profile, error)
}

type Service struct {
	config     Config
	repository Repository
	github     GitHub
	aead       cipher.AEAD
}

func New(config Config, repository Repository, github GitHub) (*Service, error) {
	if repository == nil || github == nil {
		return nil, errors.New("auth dependencies are required")
	}
	block, err := aes.NewCipher(config.FlowKey)
	if err != nil {
		return nil, fmt.Errorf("OAuth flow key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Service{config: config, repository: repository, github: github, aead: aead}, nil
}

func (service *Service) Start(ctx context.Context, returnPath string) (string, error) {
	if returnPath == "" {
		returnPath = "/"
	}
	if !strings.HasPrefix(returnPath, "/") || strings.HasPrefix(returnPath, "//") || strings.Contains(returnPath, "\\") || len(returnPath) > 2048 {
		return "", errors.New("invalid return path")
	}
	state, err := security.NewToken()
	if err != nil {
		return "", err
	}
	verifier, err := security.NewToken()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, service.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	encrypted := service.aead.Seal(nonce, nonce, []byte(verifier), nil)
	stateDigest := sha256.Sum256([]byte(state))
	if err := service.repository.CreateFlow(ctx, Flow{
		StateHash: stateDigest[:], EncryptedVerifier: encrypted, ReturnPath: returnPath,
		ExpiresAt: time.Now().UTC().Add(service.config.FlowTTL),
	}); err != nil {
		return "", err
	}
	challengeDigest := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"client_id":             {service.config.ClientID},
		"redirect_uri":          {service.config.RedirectURL},
		"scope":                 {"read:user"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challengeDigest[:])},
		"code_challenge_method": {"S256"},
	}
	return service.config.AuthorizeURL + "?" + query.Encode(), nil
}

func (service *Service) Complete(ctx context.Context, code, state string) (User, string, error) {
	if code == "" || state == "" || len(code) > 512 || len(state) > 512 {
		return User{}, "", ErrInvalidFlow
	}
	digest := sha256.Sum256([]byte(state))
	flow, err := service.repository.ConsumeFlow(ctx, digest[:])
	if err != nil {
		return User{}, "", ErrInvalidFlow
	}
	if len(flow.EncryptedVerifier) < service.aead.NonceSize() {
		return User{}, "", ErrInvalidFlow
	}
	nonce := flow.EncryptedVerifier[:service.aead.NonceSize()]
	verifier, err := service.aead.Open(nil, nonce, flow.EncryptedVerifier[service.aead.NonceSize():], nil)
	if err != nil {
		return User{}, "", ErrInvalidFlow
	}
	profile, err := service.github.Authenticate(ctx, service.config.ClientID, service.config.ClientSecret, code, string(verifier))
	if err != nil {
		return User{}, "", fmt.Errorf("GitHub authentication: %w", err)
	}
	if profile.ID <= 0 || strings.TrimSpace(profile.Login) == "" {
		return User{}, "", errors.New("GitHub returned an invalid profile")
	}
	user, err := service.repository.LoginGitHub(ctx, profile)
	if err != nil {
		return User{}, "", err
	}
	if user.Status == "blocked" {
		return User{}, "", ErrBlockedUser
	}
	return user, flow.ReturnPath, nil
}

func (service *Service) User(ctx context.Context, id string) (User, error) {
	return service.repository.UserByID(ctx, id)
}

func (service *Service) Approve(ctx context.Context, actorID, userID, studentRecordID, idempotencyKey string) (User, error) {
	return service.repository.ApproveStudent(ctx, actorID, userID, studentRecordID, idempotencyKey)
}

func (service *Service) Reject(ctx context.Context, actorID, userID, reason, idempotencyKey string) (User, error) {
	if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return User{}, errors.New("invalid rejection reason")
	}
	return service.repository.RejectUser(ctx, actorID, userID, strings.TrimSpace(reason), idempotencyKey)
}

func NewFlowID() (string, error) { return domain.NewUUIDv7() }
