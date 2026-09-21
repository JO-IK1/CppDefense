package runner

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	s3storage "github.com/JO-IK1/CppDefense/backend/internal/infrastructure/objectstore/s3"
)

type config struct {
	Backend, Token, RunnerID, Worker, Image, Runtime string
	Poll, Timeout                                    time.Duration
	Slots                                            int
	Storage                                          s3storage.Config
}

func load() (config, error) {
	cfg := config{Backend: strings.TrimRight(os.Getenv("CPPDEFENSE_BACKEND_URL"), "/"), Token: os.Getenv("CPPDEFENSE_RUNNER_TOKEN"), RunnerID: os.Getenv("CPPDEFENSE_RUNNER_ID"), Worker: env("CPPDEFENSE_WORKER_PATH", "cpp-defense-worker"), Image: env("CPPDEFENSE_SANDBOX_IMAGE", "localhost/cppdefense-sandbox:2.1.0"), Runtime: env("CPPDEFENSE_CONTAINER_RUNTIME", "podman"), Poll: duration("CPPDEFENSE_RUNNER_POLL_INTERVAL", 2*time.Second), Timeout: duration("CPPDEFENSE_JOB_TIMEOUT", 5*time.Minute), Slots: integer("CPPDEFENSE_RUNNER_SLOTS", 4)}
	cfg.Storage = s3storage.Config{Endpoint: os.Getenv("CPPDEFENSE_STORAGE_S3_ENDPOINT"), Region: env("CPPDEFENSE_STORAGE_S3_REGION", "us-east-1"), Bucket: os.Getenv("CPPDEFENSE_STORAGE_S3_BUCKET"), AccessKey: os.Getenv("CPPDEFENSE_STORAGE_S3_ACCESS_KEY"), SecretKey: os.Getenv("CPPDEFENSE_STORAGE_S3_SECRET_KEY"), Secure: env("CPPDEFENSE_STORAGE_S3_SECURE", "false") == "true", SpoolDir: os.Getenv("CPPDEFENSE_STORAGE_SPOOL_DIR")}
	if cfg.Backend == "" || len(cfg.Token) < 32 || cfg.RunnerID == "" {
		return cfg, errors.New("backend URL, runner ID and token are required")
	}
	if cfg.Slots < 1 || cfg.Slots > 6 {
		return cfg, errors.New("runner slots must be between 1 and 6")
	}
	return cfg, nil
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
