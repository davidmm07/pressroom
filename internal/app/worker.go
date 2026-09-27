package app

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// WorkerConfig tunes the queue consumer.
type WorkerConfig struct {
	Concurrency  int
	PollInterval time.Duration
	Lease        time.Duration
	MaxAttempts  int
	// ShutdownGrace is how long in-flight runs may finish after a SIGTERM.
	// Cloud Run allows ten seconds.
	ShutdownGrace time.Duration
}

// runExecutor is what the worker needs from the Executor.
type runExecutor interface {
	Execute(ctx context.Context, runID domain.ID) error
	Abandon(ctx context.Context, runID domain.ID, reason string) error
}

// Worker pulls jobs off the queue and drives the executor. It owns retries:
// transient failures go back to the queue with backoff, and a run that keeps
// failing is abandoned instead of retried forever.
type Worker struct {
	queue port.JobQueue
	exec  runExecutor
	clock port.Clock
	log   *slog.Logger
	cfg   WorkerConfig
}

func NewWorker(queue port.JobQueue, exec runExecutor, clock port.Clock, log *slog.Logger, cfg WorkerConfig) *Worker {
	return &Worker{queue: queue, exec: exec, clock: clock, log: log, cfg: cfg}
}

// Run blocks until ctx is cancelled, then waits up to ShutdownGrace for
// in-flight runs. Runs cut off by the deadline stay leased and are picked up
// again when the lease expires.
func (w *Worker) Run(ctx context.Context) {
	// Executions get their own context so a shutdown signal stops claiming
	// new work immediately but lets current runs reach a checkpoint.
	execCtx, cancelExec := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelExec()

	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx, execCtx)
		}()
	}
	<-ctx.Done()
	w.log.Info("worker draining", "grace", w.cfg.ShutdownGrace)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(w.cfg.ShutdownGrace):
		cancelExec()
		<-done
	}
}

func (w *Worker) loop(ctx, execCtx context.Context) {
	for ctx.Err() == nil {
		job, err := w.queue.Claim(ctx, w.cfg.Lease)
		if err != nil {
			if ctx.Err() == nil {
				w.log.Error("claim failed", "error", err)
			}
			w.sleep(ctx, w.cfg.PollInterval*5)
			continue
		}
		if job == nil {
			w.sleep(ctx, w.cfg.PollInterval)
			continue
		}
		w.Process(execCtx, *job)
	}
}

// Process handles one job end to end. Exported for tests and for running a
// single job from the CLI.
func (w *Worker) Process(ctx context.Context, job port.Job) {
	log := w.log.With("run_id", job.RunID, "attempt", job.Attempt)
	stop := w.heartbeat(ctx, job.RunID)
	err := w.exec.Execute(ctx, job.RunID)
	stop()

	// Bookkeeping must happen even if ctx was cancelled mid-run.
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	te, transient := port.IsTransient(err)
	switch {
	case err == nil:
		w.must(bg, log, w.queue.Complete(bg, job.RunID))
	case errors.Is(err, domain.ErrNotFound):
		log.Warn("job points at a missing run; dropping it")
		w.must(bg, log, w.queue.Complete(bg, job.RunID))
	case transient && job.Attempt < w.cfg.MaxAttempts:
		wait := backoff(job.Attempt, te.RetryAfter)
		log.Warn("run will be retried", "error", err, "in", wait)
		w.must(bg, log, w.queue.Retry(bg, job.RunID, w.clock.Now().Add(wait), err.Error()))
	default:
		reason := "gave up after repeated errors: " + err.Error()
		if !transient {
			reason = "internal error: " + err.Error()
		}
		log.Error("abandoning run", "error", err)
		w.must(bg, log, w.exec.Abandon(bg, job.RunID, reason))
		w.must(bg, log, w.queue.Complete(bg, job.RunID))
	}
}

// heartbeat extends the lease while a long run is still making progress.
func (w *Worker) heartbeat(ctx context.Context, runID domain.ID) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(w.cfg.Lease / 3)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := w.queue.Extend(ctx, runID, w.cfg.Lease); err != nil && ctx.Err() == nil {
					w.log.Warn("lease extension failed", "run_id", runID, "error", err)
				}
			}
		}
	}()
	return cancel
}

func (w *Worker) must(_ context.Context, log *slog.Logger, err error) {
	if err != nil {
		log.Error("queue bookkeeping failed", "error", err)
	}
}

func (w *Worker) sleep(ctx context.Context, d time.Duration) {
	jitter := time.Duration(rand.Int64N(int64(d)/5 + 1))
	t := time.NewTimer(d + jitter)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// backoff doubles from 15 seconds up to 30 minutes, honouring a provider's
// Retry-After when it asks for longer.
func backoff(attempt int, retryAfter time.Duration) time.Duration {
	d := 15 * time.Second << min(attempt-1, 7)
	if d > 30*time.Minute {
		d = 30 * time.Minute
	}
	return max(d, retryAfter)
}
