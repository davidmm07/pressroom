package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/app"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

type stubExecutor struct {
	err       error
	abandoned string
}

func (s *stubExecutor) Execute(context.Context, domain.ID) error { return s.err }
func (s *stubExecutor) Abandon(_ context.Context, _ domain.ID, reason string) error {
	s.abandoned = reason
	return nil
}

type recordingQueue struct {
	memQueue
	retriedAt time.Time
}

func (q *recordingQueue) Retry(_ context.Context, _ domain.ID, at time.Time, _ string) error {
	q.retriedAt = at
	return nil
}

func TestWorkerDecidesWhatHappensToAJob(t *testing.T) {
	clock := newClock()
	cfg := app.WorkerConfig{Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute, MaxAttempts: 3}
	transient := port.Transient(errors.New("overloaded"), 0)

	tests := []struct {
		name          string
		err           error
		attempt       int
		wantComplete  bool
		wantRetryIn   time.Duration
		wantAbandoned string
	}{
		{"success completes", nil, 1, true, 0, ""},
		{"transient error retries with backoff", transient, 2, false, 30 * time.Second, ""},
		{"provider retry-after wins", port.Transient(errors.New("429"), 10*time.Minute), 1, false, 10 * time.Minute, ""},
		{"too many attempts abandons", transient, 3, true, 0, "gave up"},
		{"permanent error abandons", errors.New("bad request"), 1, true, 0, "internal error"},
		{"missing run is dropped", domain.ErrNotFound, 1, true, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, exec := &recordingQueue{}, &stubExecutor{err: tt.err}
			w := app.NewWorker(q, exec, clock, discardLogger(), cfg)
			w.Process(context.Background(), port.Job{RunID: domain.NewID(), Attempt: tt.attempt})

			if completed := len(q.completed) == 1; completed != tt.wantComplete {
				t.Fatalf("completed = %v, want %v", completed, tt.wantComplete)
			}
			if tt.wantRetryIn > 0 && q.retriedAt.Sub(clock.Now()) != tt.wantRetryIn {
				t.Fatalf("retry in %s, want %s", q.retriedAt.Sub(clock.Now()), tt.wantRetryIn)
			}
			if !strings.Contains(exec.abandoned, tt.wantAbandoned) || (tt.wantAbandoned == "" && exec.abandoned != "") {
				t.Fatalf("abandoned = %q, want %q", exec.abandoned, tt.wantAbandoned)
			}
		})
	}
}
