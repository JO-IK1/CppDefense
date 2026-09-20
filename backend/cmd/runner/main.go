package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	s3storage "github.com/JO-IK1/CppDefense/backend/internal/infrastructure/objectstore/s3"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type config struct {
	Backend, Token, RunnerID, Worker, Image, Runtime string
	Poll, Timeout                                    time.Duration
	Slots                                            int
	Storage                                          s3storage.Config
}
type agent struct {
	config  config
	client  *http.Client
	storage appstorage.FileStorage
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := load()
	if err != nil {
		return err
	}
	store, err := s3storage.New(cfg.Storage)
	if err != nil {
		return err
	}
	agent := &agent{config: cfg, client: &http.Client{Timeout: cfg.Timeout + 30*time.Second}, storage: store}
	ctx, stop := signalContext()
	defer stop()
	return agent.loop(ctx)
}
func load() (config, error) {
	cfg := config{Backend: strings.TrimRight(os.Getenv("CPPDEFENSE_BACKEND_URL"), "/"), Token: os.Getenv("CPPDEFENSE_RUNNER_TOKEN"), RunnerID: os.Getenv("CPPDEFENSE_RUNNER_ID"), Worker: env("CPPDEFENSE_WORKER_PATH", "cpp-defense-worker"), Image: env("CPPDEFENSE_SANDBOX_IMAGE", "localhost/cppdefense-sandbox:2.0"), Runtime: env("CPPDEFENSE_CONTAINER_RUNTIME", "podman"), Poll: duration("CPPDEFENSE_RUNNER_POLL_INTERVAL", 2*time.Second), Timeout: duration("CPPDEFENSE_JOB_TIMEOUT", 5*time.Minute), Slots: integer("CPPDEFENSE_RUNNER_SLOTS", 4)}
	cfg.Storage = s3storage.Config{Endpoint: os.Getenv("CPPDEFENSE_STORAGE_S3_ENDPOINT"), Region: env("CPPDEFENSE_STORAGE_S3_REGION", "us-east-1"), Bucket: os.Getenv("CPPDEFENSE_STORAGE_S3_BUCKET"), AccessKey: os.Getenv("CPPDEFENSE_STORAGE_S3_ACCESS_KEY"), SecretKey: os.Getenv("CPPDEFENSE_STORAGE_S3_SECRET_KEY"), Secure: env("CPPDEFENSE_STORAGE_S3_SECURE", "false") == "true", SpoolDir: os.Getenv("CPPDEFENSE_STORAGE_SPOOL_DIR")}
	if cfg.Backend == "" || len(cfg.Token) < 32 || cfg.RunnerID == "" {
		return cfg, errors.New("backend URL, runner ID and token are required")
	}
	if cfg.Slots < 1 || cfg.Slots > 6 {
		return cfg, errors.New("runner slots must be between 1 and 6")
	}
	return cfg, nil
}
func (a *agent) loop(ctx context.Context) error {
	var workers sync.WaitGroup
	workers.Add(a.config.Slots)
	for slot := 0; slot < a.config.Slots; slot++ {
		go func() {
			defer workers.Done()
			a.workerLoop(ctx)
		}()
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
func (a *agent) lease(ctx context.Context) (postgres.Lease, bool, error) {
	body, _ := json.Marshal(map[string]any{"runner_id": a.config.RunnerID, "capacity": a.config.Slots})
	request, _ := http.NewRequestWithContext(ctx, "POST", a.config.Backend+"/api/v1/runner/jobs/lease", bytes.NewReader(body))
	a.headers(request)
	response, err := a.client.Do(request)
	if err != nil {
		return postgres.Lease{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == 204 {
		return postgres.Lease{}, false, nil
	}
	if response.StatusCode != 200 {
		return postgres.Lease{}, false, fmt.Errorf("status %d", response.StatusCode)
	}
	var lease postgres.Lease
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&lease)
	return lease, true, err
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
	requestID := lease.JobID
	if lease.Kind == "prepare_defense" {
		payload := map[string]any{"protocol_version": "1.0", "request_id": requestID, "session_id": lease.SessionID, "command": "prepare_defense", "payload": map[string]any{"project_root": "project", "top_n": lease.TopN, "seed": lease.Seed}}
		var response struct {
			Status string `json:"status"`
			Result struct {
				Candidates    []postgres.Candidate `json:"candidates"`
				SelectedIndex int                  `json:"selected_index"`
				MaskedSource  string               `json:"masked_source"`
			} `json:"result"`
			Error any `json:"error"`
		}
		if err = runWorker(ctx, a.config.Worker, workspace, payload, &response); err != nil {
			return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
		}
		if response.Status != "ok" {
			return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
		}
		return a.complete(parent, lease, postgres.Completion{Candidates: response.Result.Candidates, SelectedIndex: response.Result.SelectedIndex, MaskedSource: response.Result.MaskedSource})
	}
	if lease.SelectedFunction == nil || lease.Answer == nil {
		return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
	}
	payload := map[string]any{"protocol_version": "1.0", "request_id": requestID, "session_id": lease.SessionID, "command": "materialize_attempt", "payload": map[string]any{"project_root": "project", "output_root": "attempt", "selected_function": lease.SelectedFunction, "answer": *lease.Answer}}
	var response struct {
		Status string `json:"status"`
	}
	if err = runWorker(ctx, a.config.Worker, workspace, payload, &response); err != nil || response.Status != "ok" {
		return a.complete(parent, lease, postgres.Completion{Outcome: "error"})
	}
	result := runSandbox(ctx, a.config.Runtime, a.config.Image, filepath.Join(session, "attempt"))
	outcome := "failed"
	if result.OK {
		outcome = "passed"
	}
	return a.complete(parent, lease, postgres.Completion{Outcome: outcome, ConfigureResult: result, BuildResult: result, CTestResult: result})
}
func (a *agent) heartbeatLoop(ctx context.Context, done <-chan struct{}, lease postgres.Lease, cancel context.CancelFunc) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			request, _ := http.NewRequestWithContext(ctx, "POST", a.config.Backend+"/api/v1/runner/jobs/"+lease.JobID+"/heartbeat", nil)
			a.headers(request)
			request.Header.Set("X-Lease-Token", lease.LeaseToken)
			response, err := a.client.Do(request)
			if err == nil {
				response.Body.Close()
				if response.StatusCode != http.StatusNoContent {
					cancel()
					return
				}
			}
		}
	}
}
func runWorker(ctx context.Context, binary, dir string, input any, output any) error {
	data, _ := json.Marshal(input)
	command := exec.CommandContext(ctx, binary)
	command.Dir = dir
	command.Stdin = bytes.NewReader(data)
	var stdout, stderr limitedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("worker: %w: %s", err, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), output); err != nil {
		return fmt.Errorf("worker JSON: %w", err)
	}
	return nil
}

type commandResult struct {
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exit_code"`
	Log      string `json:"log"`
	TimedOut bool   `json:"timed_out"`
}

func runSandbox(ctx context.Context, runtime, image, project string) commandResult {
	absolute, _ := filepath.Abs(project)
	script := "cmake -S /workspace -B /tmp/build -DCMAKE_BUILD_TYPE=Release && cmake --build /tmp/build --parallel 2 && ctest --test-dir /tmp/build --output-on-failure"
	args := []string{"run", "--rm", "--network", "none", "--cpus", "1.0", "--memory", "512m", "--memory-swap", "512m", "--pids-limit", "128", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--tmpfs", "/tmp:rw,noexec,nosuid,size=1g", "--user", "65532:65532", "-v", absolute + ":/workspace:ro", image, "sh", "-lc", script}
	command := exec.CommandContext(ctx, runtime, args...)
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	result := commandResult{OK: err == nil, Log: output.String(), TimedOut: ctx.Err() != nil}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
	} else if err != nil {
		result.ExitCode = -1
	}
	return result
}
func (a *agent) complete(ctx context.Context, lease postgres.Lease, value postgres.Completion) error {
	body, _ := json.Marshal(value)
	request, _ := http.NewRequestWithContext(ctx, "POST", a.config.Backend+"/api/v1/runner/jobs/"+lease.JobID+"/complete", bytes.NewReader(body))
	a.headers(request)
	request.Header.Set("X-Lease-Token", lease.LeaseToken)
	request.Header.Set("Idempotency-Key", lease.JobID)
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("complete status %d", response.StatusCode)
	}
	return nil
}
func (a *agent) headers(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+a.config.Token)
	request.Header.Set("Content-Type", "application/json")
}
func extract(source io.ReadCloser, destination string) error {
	data, err := io.ReadAll(io.LimitReader(source, 500<<20))
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, entry := range archive.File {
		name := filepath.Clean(entry.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || strings.Contains(entry.Name, "\\") || entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe normalized archive")
		}
		target := filepath.Join(destination, name)
		if !strings.HasPrefix(target, destination+string(filepath.Separator)) {
			return errors.New("archive escape")
		}
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, e := entry.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if e == nil {
			_, e = io.Copy(out, io.LimitReader(in, 100<<20))
			out.Close()
		}
		in.Close()
		if e != nil {
			return e
		}
	}
	return nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := (1 << 20) - b.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func duration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(env(name, fallback.String()))
	if err != nil {
		return fallback
	}
	return value
}
func integer(name string, fallback int) int {
	value, err := strconv.Atoi(env(name, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
