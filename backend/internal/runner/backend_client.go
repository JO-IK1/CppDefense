package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func (a *agent) lease(ctx context.Context) (postgres.Lease, bool, error) {
	for attempt := 0; attempt < 2; attempt++ {
		payload := map[string]any{"runner_id": a.config.RunnerID, "capacity": a.config.Slots}
		if !a.legacyLease.Load() {
			payload["wait_ms"] = 10000
		}
		body, _ := json.Marshal(payload)
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Backend+"/api/v1/runner/jobs/lease", bytes.NewReader(body))
		a.headers(request)
		response, err := a.client.Do(request)
		if err != nil {
			return postgres.Lease{}, false, err
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusNoContent {
			if a.legacyLease.Load() {
				wait(ctx, a.config.Poll)
			}
			return postgres.Lease{}, false, nil
		}
		if response.StatusCode == http.StatusBadRequest && !a.legacyLease.Load() {
			response.Body.Close()
			a.legacyLease.Store(true)
			continue
		}
		if response.StatusCode != http.StatusOK {
			return postgres.Lease{}, false, fmt.Errorf("status %d", response.StatusCode)
		}
		var lease postgres.Lease
		err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&lease)
		return lease, true, err
	}
	return postgres.Lease{}, false, fmt.Errorf("lease protocol negotiation failed")
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
			request, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Backend+"/api/v1/runner/jobs/"+lease.JobID+"/heartbeat", nil)
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

func (a *agent) complete(ctx context.Context, lease postgres.Lease, value postgres.Completion) error {
	if p := profile(ctx); p != nil {
		p.Lock()
		timing := p.value
		timing.TotalMS = elapsedMS(p.started) + timing.QueueMS
		p.Unlock()
		value.Timings = &timing
	}
	body, _ := json.Marshal(value)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Backend+"/api/v1/runner/jobs/"+lease.JobID+"/complete", bytes.NewReader(body))
	a.headers(request)
	request.Header.Set("X-Lease-Token", lease.LeaseToken)
	request.Header.Set("Idempotency-Key", lease.JobID)
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		if trimmed := bytes.TrimSpace(detail); len(trimmed) != 0 {
			return fmt.Errorf("complete status %d: %s", response.StatusCode, trimmed)
		}
		return fmt.Errorf("complete status %d", response.StatusCode)
	}
	return nil
}

func (a *agent) headers(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+a.config.Token)
	request.Header.Set("Content-Type", "application/json")
}
