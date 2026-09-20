package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"testing"
	"time"
)

type memoryRepository struct {
	flow appFlow
	user User
}

type appFlow struct {
	value Flow
	used  bool
}

func (repository *memoryRepository) CreateFlow(_ context.Context, flow Flow) error {
	repository.flow.value = flow
	return nil
}
func (repository *memoryRepository) ConsumeFlow(_ context.Context, hash []byte) (Flow, error) {
	if repository.flow.used || string(hash) != string(repository.flow.value.StateHash) {
		return Flow{}, errors.New("missing flow")
	}
	repository.flow.used = true
	return repository.flow.value, nil
}
func (repository *memoryRepository) LoginGitHub(context.Context, Profile) (User, error) {
	return repository.user, nil
}
func (repository *memoryRepository) UserByID(context.Context, string) (User, error) {
	return repository.user, nil
}
func (repository *memoryRepository) ApproveStudent(context.Context, string, string, string, string) (User, error) {
	return repository.user, nil
}
func (repository *memoryRepository) RejectUser(context.Context, string, string, string, string) (User, error) {
	return repository.user, nil
}

type githubStub struct {
	profile  Profile
	verifier string
}

func (github *githubStub) Authenticate(_ context.Context, _, _, _, verifier string) (Profile, error) {
	github.verifier = verifier
	return github.profile, nil
}

func TestStartAndCompleteUsesPKCEAndOneTimeState(t *testing.T) {
	repository := &memoryRepository{user: User{ID: "user", Status: "pending"}}
	github := &githubStub{profile: Profile{ID: 42, Login: "Student"}}
	service, err := New(Config{ClientID: "client", ClientSecret: "secret", RedirectURL: "https://app/callback", AuthorizeURL: "https://github/authorize", FlowKey: make([]byte, 32), FlowTTL: time.Minute}, repository, github)
	if err != nil {
		t.Fatal(err)
	}
	location, err := service.Start(context.Background(), "/labs")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(location)
	state := parsed.Query().Get("state")
	if state == "" || parsed.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("invalid authorization URL: %s", location)
	}
	user, returnPath, err := service.Complete(context.Background(), "code", state)
	if err != nil || user.ID != "user" || returnPath != "/labs" {
		t.Fatalf("complete = %#v, %q, %v", user, returnPath, err)
	}
	digest := sha256.Sum256([]byte(github.verifier))
	if parsed.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(digest[:]) {
		t.Fatal("PKCE challenge does not match verifier")
	}
	if _, _, err := service.Complete(context.Background(), "code", state); !errors.Is(err, ErrInvalidFlow) {
		t.Fatalf("reused flow error = %v", err)
	}
}

func TestStartRejectsExternalReturnPath(t *testing.T) {
	service, err := New(Config{FlowKey: make([]byte, 32)}, &memoryRepository{}, &githubStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(context.Background(), "//attacker.example"); err == nil {
		t.Fatal("external return path accepted")
	}
	if _, err := service.Start(context.Background(), `/\\attacker.example`); err == nil {
		t.Fatal("backslash return path accepted")
	}
}
