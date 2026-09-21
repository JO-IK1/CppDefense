package runner

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

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
