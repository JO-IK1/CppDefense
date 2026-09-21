package runner

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	s3storage "github.com/JO-IK1/CppDefense/backend/internal/infrastructure/objectstore/s3"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type agent struct {
	config  config
	client  *http.Client
	storage appstorage.FileStorage
}

// Run starts the runner agent and blocks until it receives a shutdown signal.
func Run() error {
	cfg, err := load()
	if err != nil {
		return err
	}
	store, err := s3storage.New(cfg.Storage)
	if err != nil {
		return err
	}
	a := &agent{config: cfg, client: &http.Client{Timeout: cfg.Timeout + 30*time.Second}, storage: store}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return a.loop(ctx)
}

func (a *agent) loop(ctx context.Context) error {
	var workers sync.WaitGroup
	workers.Add(a.config.Slots)
	for slot := 0; slot < a.config.Slots; slot++ {
		go func() { defer workers.Done(); a.workerLoop(ctx) }()
	}
	workers.Wait()
	return nil
}

func (a *agent) workerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		lease, ok, err := a.lease(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "lease:", err)
			if !wait(ctx, a.config.Poll) {
				return
			}
			continue
		}
		if !ok {
			if !wait(ctx, a.config.Poll) {
				return
			}
			continue
		}
		if err := a.execute(ctx, lease); err != nil {
			fmt.Fprintln(os.Stderr, "job", lease.JobID, err)
		}
	}
}

func (a *agent) execute(parent context.Context, lease postgres.Lease) error {
	ctx, cancel := context.WithTimeout(parent, a.config.Timeout)
	defer cancel()
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go a.heartbeatLoop(ctx, heartbeatDone, lease, cancel)
	workspace, err := os.MkdirTemp("", "cppdefense-job-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	session := filepath.Join(workspace, lease.SessionID)
	project := filepath.Join(session, "project")
	if err = os.MkdirAll(project, 0o700); err != nil {
		return err
	}
	key, err := appstorage.ParseKey(lease.SubmissionObjectKey)
	if err != nil {
		return err
	}
	reader, _, err := a.storage.Open(ctx, key)
	if err != nil {
		return err
	}
	err = extract(reader, project)
	reader.Close()
	if err != nil {
		return err
	}
	if lease.Kind == "prepare_defense" {
		return a.prepare(parent, ctx, workspace, lease)
	}
	return a.check(parent, ctx, filepath.Join(session, "attempt"), workspace, lease)
}

func (a *agent) prepare(parent, ctx context.Context, workspace string, lease postgres.Lease) error {
	workerPayload := map[string]any{"project_root": "project", "top_n": lease.TopN, "seed": lease.Seed}
	if lease.SelectedIndex != nil {
		workerPayload["selected_index"] = *lease.SelectedIndex
	}
	payload := map[string]any{"protocol_version": "1.0", "request_id": lease.JobID, "session_id": lease.SessionID, "command": "prepare_defense", "payload": workerPayload}
	var response prepareResponse
	if err := runWorker(ctx, a.config.Worker, workspace, payload, &response); err != nil || response.Status != "ok" {
		return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
	}
	return a.complete(parent, lease, postgres.Completion{Candidates: response.Result.Candidates, SelectedIndex: response.Result.SelectedIndex, MaskedSource: response.Result.MaskedSource})
}

func (a *agent) check(parent, ctx context.Context, attempt, workspace string, lease postgres.Lease) error {
	if lease.SelectedFunction == nil || lease.Answer == nil {
		return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
	}
	payload := map[string]any{"protocol_version": "1.0", "request_id": lease.JobID, "session_id": lease.SessionID, "command": "materialize_attempt", "payload": map[string]any{"project_root": "project", "output_root": "attempt", "selected_function": lease.SelectedFunction, "answer": *lease.Answer}}
	var response struct {
		Status string `json:"status"`
	}
	if err := runWorker(ctx, a.config.Worker, workspace, payload, &response); err != nil || response.Status != "ok" {
		return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
	}
	result := runSandbox(ctx, a.config.Runtime, a.config.Image, attempt)
	outcome := "failed"
	if result.OK {
		outcome = "passed"
	}
	return a.complete(parent, lease, postgres.Completion{Outcome: outcome, ConfigureResult: result, BuildResult: result, CTestResult: result})
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
