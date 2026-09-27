package llm

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/davidmm07/pressroom/internal/port"
)

// RetryPolicy bounds in-process retries. Anything that needs a longer wait
// than MaxWait is handed back to the job queue, which retries later without
// holding a worker.
type RetryPolicy struct {
	Attempts int
	BaseWait time.Duration
	MaxWait  time.Duration
}

// DefaultRetryPolicy retries twice with jittered exponential backoff.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Attempts: 3, BaseWait: 500 * time.Millisecond, MaxWait: 20 * time.Second}
}

// Retrying adds retries to any LanguageModel (Decorator pattern). One policy
// covers every provider, instead of each SDK's own defaults.
type Retrying struct {
	next   port.LanguageModel
	policy RetryPolicy
	sleep  func(context.Context, time.Duration) error
}

// WithRetry decorates next.
func WithRetry(next port.LanguageModel, p RetryPolicy) *Retrying {
	return &Retrying{next: next, policy: p, sleep: sleepCtx}
}

func (r *Retrying) Complete(ctx context.Context, req port.CompletionRequest) (*port.Completion, error) {
	for attempt := 1; ; attempt++ {
		c, err := r.next.Complete(ctx, req)
		te, transient := port.IsTransient(err)
		if err == nil || !transient || attempt >= r.policy.Attempts {
			return c, err
		}
		wait := r.backoff(attempt)
		if te.RetryAfter > wait {
			wait = te.RetryAfter
		}
		if wait > r.policy.MaxWait {
			return nil, err // let the queue wait instead of a worker
		}
		if err := r.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// backoff is "full jitter" exponential backoff: random in [0, base*2^n).
func (r *Retrying) backoff(attempt int) time.Duration {
	ceiling := r.policy.BaseWait << (attempt - 1)
	return time.Duration(rand.Int64N(int64(ceiling)) + 1)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Logging records latency and token usage of every model call (Decorator).
type Logging struct {
	next port.LanguageModel
	log  *slog.Logger
}

// WithLogging decorates next.
func WithLogging(next port.LanguageModel, log *slog.Logger) *Logging {
	return &Logging{next: next, log: log}
}

func (l *Logging) Complete(ctx context.Context, req port.CompletionRequest) (*port.Completion, error) {
	start := time.Now()
	c, err := l.next.Complete(ctx, req)
	attrs := []any{"model", req.Model.String(), "latency_ms", time.Since(start).Milliseconds(), "messages", len(req.Messages)}
	if err != nil {
		_, transient := port.IsTransient(err)
		l.log.WarnContext(ctx, "model call failed", append(attrs, "transient", transient, "error", err)...)
		return nil, err
	}
	l.log.InfoContext(ctx, "model call", append(attrs,
		"input_tokens", c.Usage.InputTokens, "cached_tokens", c.Usage.CachedInputTokens,
		"output_tokens", c.Usage.OutputTokens, "stop", c.StopReason, "tool_calls", len(c.Message.ToolCalls))...)
	return c, nil
}
