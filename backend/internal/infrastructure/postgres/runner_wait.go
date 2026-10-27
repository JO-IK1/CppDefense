package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Listen uses a dedicated connection, never an idle pool connection: long polls
// must not exhaust the normal API pool. LISTEN precedes the first lease query,
// so a committed insertion cannot be lost between querying and subscribing.
func (r *DefenseRepository) LeaseWaiting(ctx context.Context, id string, slots int, hash []byte, wait time.Duration) (Lease, error) {
	if wait <= 0 {
		return r.Lease(ctx, id, slots, hash)
	}
	if wait > 20*time.Second {
		wait = 20 * time.Second
	}
	waiting, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	cfg := r.db.pool.Config().ConnConfig.Copy()
	cfg.RuntimeParams["application_name"] = "cppdefense-runner-wait"
	conn, err := pgx.ConnectConfig(waiting, cfg)
	if err != nil {
		return r.Lease(ctx, id, slots, hash)
	}
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn.Close(closing)
	}()
	if _, err = conn.Exec(waiting, "listen cppdefense_runner_jobs"); err != nil {
		return r.Lease(ctx, id, slots, hash)
	}
	for {
		lease, err := r.Lease(ctx, id, slots, hash)
		if !errors.Is(err, pgx.ErrNoRows) {
			return lease, err
		}
		// Periodic reconciliation also discovers expired leases and delayed retries
		// when no fresh insert notification is generated.
		tick, cancel := context.WithTimeout(waiting, time.Second)
		_, notifyErr := conn.WaitForNotification(tick)
		cancel()
		if waiting.Err() != nil {
			if ctx.Err() != nil {
				return Lease{}, ctx.Err()
			}
			return Lease{}, pgx.ErrNoRows
		}
		if notifyErr != nil && !errors.Is(notifyErr, context.DeadlineExceeded) {
			return Lease{}, notifyErr
		}
	}
}
