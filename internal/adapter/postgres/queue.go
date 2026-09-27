package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Queue is a work queue on a plain table. FOR UPDATE SKIP LOCKED lets many
// workers claim jobs concurrently without blocking each other, and leases
// (locked_until) hand a crashed worker's job to someone else.
//
// Enqueueing in the same transaction that creates the run is what makes
// Postgres the right queue here: no dual write, no outbox to relay.
type Queue struct{ db *DB }

func (q *Queue) Enqueue(ctx context.Context, runID domain.ID, at time.Time) error {
	_, err := q.db.q(ctx).Exec(ctx, `INSERT INTO run_jobs (run_id, available_at) VALUES ($1, $2)
		ON CONFLICT (run_id) DO UPDATE SET available_at = EXCLUDED.available_at, locked_until = NULL, attempts = 0`,
		runID, at)
	return mapErr(err)
}

func (q *Queue) Claim(ctx context.Context, lease time.Duration) (*port.Job, error) {
	var job port.Job
	err := q.db.q(ctx).QueryRow(ctx, `
		UPDATE run_jobs SET attempts = attempts + 1, locked_until = now() + make_interval(secs => $1)
		WHERE run_id = (
			SELECT run_id FROM run_jobs
			WHERE available_at <= now() AND (locked_until IS NULL OR locked_until < now())
			ORDER BY available_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING run_id, attempts`, lease.Seconds()).Scan(&job.RunID, &job.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return &job, nil
}

// Extend renews a lease while a long run is still making progress.
func (q *Queue) Extend(ctx context.Context, runID domain.ID, lease time.Duration) error {
	_, err := q.db.q(ctx).Exec(ctx, `UPDATE run_jobs SET locked_until = now() + make_interval(secs => $2) WHERE run_id = $1`,
		runID, lease.Seconds())
	return mapErr(err)
}

func (q *Queue) Complete(ctx context.Context, runID domain.ID) error {
	_, err := q.db.q(ctx).Exec(ctx, `DELETE FROM run_jobs WHERE run_id = $1`, runID)
	return mapErr(err)
}

func (q *Queue) Retry(ctx context.Context, runID domain.ID, at time.Time, reason string) error {
	_, err := q.db.q(ctx).Exec(ctx, `UPDATE run_jobs SET available_at = $2, locked_until = NULL, last_error = $3 WHERE run_id = $1`,
		runID, at, reason)
	return mapErr(err)
}
