// Package events delivers domain events to in-process subscribers.
package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Subscriber reacts to events (Observer pattern).
type Subscriber interface {
	Handle(ctx context.Context, e domain.Event) error
}

// SubscriberFunc adapts a function to Subscriber.
type SubscriberFunc func(ctx context.Context, e domain.Event) error

func (f SubscriberFunc) Handle(ctx context.Context, e domain.Event) error { return f(ctx, e) }

// Bus fans events out synchronously with a timeout per subscriber. A failing
// subscriber is logged and never fails the use case that published: a Slack
// outage must not block a refund approval.
type Bus struct {
	subs []Subscriber
	log  *slog.Logger
}

var _ port.EventPublisher = (*Bus)(nil)

func NewBus(log *slog.Logger, subs ...Subscriber) *Bus { return &Bus{subs: subs, log: log} }

func (b *Bus) Publish(ctx context.Context, events ...domain.Event) {
	for _, e := range events {
		for _, s := range b.subs {
			sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			if err := s.Handle(sctx, e); err != nil {
				b.log.WarnContext(ctx, "event subscriber failed", "event", e.EventName(), "error", err)
			}
			cancel()
		}
	}
}

// LogSubscriber writes every event to the structured log, which doubles as
// an audit stream in Cloud Logging.
func LogSubscriber(log *slog.Logger) Subscriber {
	return SubscriberFunc(func(ctx context.Context, e domain.Event) error {
		log.InfoContext(ctx, "domain event", "event", e.EventName(), "payload", fmt.Sprintf("%+v", e))
		return nil
	})
}

// Poster sends a text message somewhere humans read (Slack in production).
type Poster interface {
	Post(ctx context.Context, text string) (bool, error)
}

// NotifySubscriber tells people about the events that need them: approvals
// waiting, agents put on probation or retired, experiments concluded.
func NotifySubscriber(p Poster) Subscriber {
	return SubscriberFunc(func(ctx context.Context, e domain.Event) error {
		var text string
		switch ev := e.(type) {
		case domain.RunNeedsApproval:
			text = fmt.Sprintf(":hand: %s wants to call %s with %s. Approve or deny run %s in Pressroom.", ev.AgentSlug, ev.Tool, ev.Arguments, ev.RunID)
		case domain.AgentStatusChanged:
			text = fmt.Sprintf(":clipboard: %s moved from %s to %s (%s). Owner: %s", ev.AgentSlug, ev.From, ev.To, ev.Reason, ev.Owner)
		case domain.ExperimentConcluded:
			verb := "kept its champion"
			if ev.Promoted {
				verb = "promoted " + ev.Winner.String()
			}
			text = fmt.Sprintf(":test_tube: %s %s. %s", ev.AgentSlug, verb, ev.Summary)
		default:
			return nil
		}
		_, err := p.Post(ctx, text)
		return err
	})
}
