package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestLeaseNegotiatesLegacyBackendOnce(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		status := http.StatusNoContent
		if _, ok := request["wait_ms"]; ok {
			status = http.StatusBadRequest
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	a := agent{client: client, config: config{Backend: "http://backend", Poll: time.Millisecond}}
	if _, ok, err := a.lease(context.Background()); err != nil || ok {
		t.Fatalf("fallback failed: %v", err)
	}
	if _, ok, err := a.lease(context.Background()); err != nil || ok {
		t.Fatalf("legacy repeat failed: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatal("legacy negotiation repeated")
	}
}

func TestHeartbeatLoopStops(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	a := &agent{config: config{Backend: "http://backend", Token: "token"}, client: client}
	done, finished := make(chan struct{}), make(chan struct{})
	close(done)
	go func() {
		a.heartbeatLoop(context.Background(), done, postgres.Lease{JobID: "job", LeaseToken: "lease"}, func() {})
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop")
	}
}
