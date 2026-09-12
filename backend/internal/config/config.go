package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTP        HTTP
	Database    Database
	Session     Session
	Storage     Storage
	LogLevel    slog.Level
	AutoMigrate bool
}

type HTTP struct {
	Address           string
	PublicOrigin      string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	ReadyTimeout      time.Duration
	MaxHeaderBytes    int
}

type Database struct {
	URL               string
	MaxConnections    int32
	MinConnections    int32
	MaxConnectionLife time.Duration
	MaxConnectionIdle time.Duration
	HealthTimeout     time.Duration
}

type Session struct {
	HashKey []byte
	CSRFKey []byte
	TTL     time.Duration
}

type Storage struct {
	Mode      string
	LocalRoot string
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Secure    bool
	SpoolDir  string
}

func Load() (Config, error) {
	var cfg Config
	var err error

	cfg.HTTP.Address = env("CPPDEFENSE_HTTP_ADDRESS", "127.0.0.1:8080")
	if _, _, splitErr := net.SplitHostPort(cfg.HTTP.Address); splitErr != nil {
		return cfg, fmt.Errorf("CPPDEFENSE_HTTP_ADDRESS: %w", splitErr)
	}
	cfg.HTTP.PublicOrigin = strings.TrimRight(env("CPPDEFENSE_PUBLIC_ORIGIN", "http://localhost:8080"), "/")
	cfg.HTTP.ReadHeaderTimeout = duration("CPPDEFENSE_HTTP_READ_HEADER_TIMEOUT", 5*time.Second, &err)
	cfg.HTTP.ReadTimeout = duration("CPPDEFENSE_HTTP_READ_TIMEOUT", 15*time.Second, &err)
	cfg.HTTP.WriteTimeout = duration("CPPDEFENSE_HTTP_WRITE_TIMEOUT", 30*time.Second, &err)
	cfg.HTTP.IdleTimeout = duration("CPPDEFENSE_HTTP_IDLE_TIMEOUT", 60*time.Second, &err)
	cfg.HTTP.ShutdownTimeout = duration("CPPDEFENSE_HTTP_SHUTDOWN_TIMEOUT", 15*time.Second, &err)
	cfg.HTTP.ReadyTimeout = duration("CPPDEFENSE_READY_TIMEOUT", 2*time.Second, &err)
	cfg.HTTP.MaxHeaderBytes = integer("CPPDEFENSE_HTTP_MAX_HEADER_BYTES", 1<<20, &err)

	cfg.Database.URL = strings.TrimSpace(os.Getenv("CPPDEFENSE_DATABASE_URL"))
	if cfg.Database.URL == "" {
		return cfg, errors.New("CPPDEFENSE_DATABASE_URL is required")
	}
	cfg.Database.MaxConnections = int32(integer("CPPDEFENSE_DATABASE_MAX_CONNECTIONS", 10, &err))
	cfg.Database.MinConnections = int32(integer("CPPDEFENSE_DATABASE_MIN_CONNECTIONS", 1, &err))
	cfg.Database.MaxConnectionLife = duration("CPPDEFENSE_DATABASE_MAX_CONNECTION_LIFETIME", 30*time.Minute, &err)
	cfg.Database.MaxConnectionIdle = duration("CPPDEFENSE_DATABASE_MAX_CONNECTION_IDLE", 5*time.Minute, &err)
	cfg.Database.HealthTimeout = duration("CPPDEFENSE_DATABASE_HEALTH_TIMEOUT", 3*time.Second, &err)

	cfg.Session.HashKey = secret("CPPDEFENSE_SESSION_HASH_KEY", &err)
	cfg.Session.CSRFKey = secret("CPPDEFENSE_CSRF_HASH_KEY", &err)
	cfg.Session.TTL = duration("CPPDEFENSE_SESSION_TTL", 24*time.Hour, &err)
	cfg.Storage.Mode = strings.ToLower(env("CPPDEFENSE_STORAGE_MODE", "local"))
	cfg.Storage.LocalRoot = env("CPPDEFENSE_STORAGE_LOCAL_ROOT", "./var/objects")
	cfg.Storage.Endpoint = strings.TrimSpace(os.Getenv("CPPDEFENSE_STORAGE_S3_ENDPOINT"))
	cfg.Storage.Region = env("CPPDEFENSE_STORAGE_S3_REGION", "us-east-1")
	cfg.Storage.Bucket = env("CPPDEFENSE_STORAGE_S3_BUCKET", "cppdefense")
	cfg.Storage.AccessKey = strings.TrimSpace(os.Getenv("CPPDEFENSE_STORAGE_S3_ACCESS_KEY"))
	cfg.Storage.SecretKey = strings.TrimSpace(os.Getenv("CPPDEFENSE_STORAGE_S3_SECRET_KEY"))
	cfg.Storage.Secure = boolean("CPPDEFENSE_STORAGE_S3_SECURE", true, &err)
	cfg.Storage.SpoolDir = strings.TrimSpace(os.Getenv("CPPDEFENSE_STORAGE_SPOOL_DIR"))
	cfg.AutoMigrate = boolean("CPPDEFENSE_AUTO_MIGRATE", true, &err)
	cfg.LogLevel = logLevel(env("CPPDEFENSE_LOG_LEVEL", "info"), &err)

	if err != nil {
		return cfg, err
	}
	if cfg.Database.MinConnections > cfg.Database.MaxConnections {
		return cfg, errors.New("database minimum connections exceeds maximum")
	}
	if cfg.Session.TTL <= 0 {
		return cfg, errors.New("session TTL must be positive")
	}
	if cfg.Storage.Mode != "local" && cfg.Storage.Mode != "s3" {
		return cfg, errors.New("CPPDEFENSE_STORAGE_MODE must be local or s3")
	}
	if cfg.Storage.Mode == "local" && strings.TrimSpace(cfg.Storage.LocalRoot) == "" {
		return cfg, errors.New("CPPDEFENSE_STORAGE_LOCAL_ROOT is required in local mode")
	}
	if cfg.Storage.Mode == "s3" && (cfg.Storage.Endpoint == "" || cfg.Storage.Bucket == "" || cfg.Storage.AccessKey == "" || cfg.Storage.SecretKey == "") {
		return cfg, errors.New("S3 endpoint, bucket, access key and secret key are required in s3 mode")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func duration(name string, fallback time.Duration, target *error) time.Duration {
	value, parseErr := time.ParseDuration(env(name, fallback.String()))
	if parseErr != nil && *target == nil {
		*target = fmt.Errorf("%s: %w", name, parseErr)
	}
	return value
}

func integer(name string, fallback int, target *error) int {
	value, parseErr := strconv.Atoi(env(name, strconv.Itoa(fallback)))
	if (parseErr != nil || value <= 0) && *target == nil {
		*target = fmt.Errorf("%s must be a positive integer", name)
	}
	return value
}

func boolean(name string, fallback bool, target *error) bool {
	value, parseErr := strconv.ParseBool(env(name, strconv.FormatBool(fallback)))
	if parseErr != nil && *target == nil {
		*target = fmt.Errorf("%s: %w", name, parseErr)
	}
	return value
}

func secret(name string, target *error) []byte {
	encoded := strings.TrimSpace(os.Getenv(name))
	decoded, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
	if decodeErr != nil || len(decoded) < 32 {
		if *target == nil {
			*target = fmt.Errorf("%s must be base64url without padding and decode to at least 32 bytes", name)
		}
		return nil
	}
	return decoded
}

func logLevel(value string, target *error) slog.Level {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		if *target == nil {
			*target = fmt.Errorf("CPPDEFENSE_LOG_LEVEL has unsupported value %q", value)
		}
		return slog.LevelInfo
	}
}
