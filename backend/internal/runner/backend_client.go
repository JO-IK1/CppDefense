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
	body, _ := json.Marshal(map[string]any{"runner_id": a.config.RunnerID, "capacity": a.config.Slots})
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Backend+"/api/v1/runner/jobs/lease", bytes.NewReader(body))
	a.headers(request)
	response, err := a.client.Do(request)
	if err != nil {
		return postgres.Lease{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return postgres.Lease{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return postgres.Lease{}, false, fmt.Errorf("status %d", response.StatusCode)
	}
	var lease postgres.Lease
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&lease)
	return lease, true, err
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
		return fmt.Errorf("complete status %d", response.StatusCode)
	}
	return nil
}

func (a *agent) headers(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+a.config.Token)
	request.Header.Set("Content-Type", "application/json")
}
