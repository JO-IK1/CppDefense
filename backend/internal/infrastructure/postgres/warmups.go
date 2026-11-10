package postgres

import (
	"context"

	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func enqueueSubmissionWarmup(ctx context.Context, tx pgx.Tx, submission string) error {
	id, err := domain.NewUUIDv7()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `insert into runner_jobs(id,kind,submission_version_id,priority,timeout_seconds,max_retries) values($1,'warm_submission',$2,-100,300,1) on conflict do nothing`, id, submission)
	return err
}
