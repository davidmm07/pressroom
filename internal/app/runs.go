package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// RunService starts runs and handles the human side of them: approvals,
// denials, reviews and cancellations. Executing a run is the Executor's job.
type RunService struct {
	agents      port.AgentRepository
	runs        port.RunRepository
	experiments port.ExperimentRepository
	queue       port.JobQueue
	tx          port.TxManager
	events      port.EventPublisher
	clock       port.Clock
}

func NewRunService(
	agents port.AgentRepository, runs port.RunRepository, experiments port.ExperimentRepository,
	queue port.JobQueue, tx port.TxManager, events port.EventPublisher, clock port.Clock,
) *RunService {
	return &RunService{agents: agents, runs: runs, experiments: experiments, queue: queue, tx: tx, events: events, clock: clock}
}

// StartRunInput identifies the agent by ID or slug.
type StartRunInput struct {
	AgentID        domain.ID
	AgentSlug      string
	Input          json.RawMessage
	Trigger        domain.Trigger
	IdempotencyKey string
}

// Start queues a run. Calls with the same idempotency key return the run the
// first call created, so a retried HTTP request or a redelivered Pub/Sub
// message never makes an agent do the same work twice.
func (s *RunService) Start(ctx context.Context, in StartRunInput) (*domain.Run, error) {
	agent, err := s.resolveAgent(ctx, in.AgentID, in.AgentSlug)
	if err != nil {
		return nil, err
	}
	if in.IdempotencyKey != "" {
		existing, err := s.runs.FindByIdempotencyKey(ctx, agent.ID, in.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}

	params := domain.NewRunParams{
		ID: domain.NewID(), Input: in.Input, Trigger: in.Trigger,
		IdempotencyKey: in.IdempotencyKey, Model: agent.Model, Variant: domain.VariantChampion,
	}
	if exp, err := s.experiments.Running(ctx, agent.ID); err == nil {
		// Route a share of traffic to the challenger (see Experiment.Assign).
		params.ExperimentID = exp.ID
		params.Variant = exp.Assign(params.ID)
		params.Model = exp.ModelFor(params.Variant)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	now := s.clock.Now()
	run, err := domain.NewRun(agent, params, now)
	if err != nil {
		return nil, err
	}
	// Creating the run and queueing its job commit together (Unit of Work):
	// no run is stranded without a job, and no job points at a missing run.
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.runs.Create(ctx, run); err != nil {
			return err
		}
		return s.queue.Enqueue(ctx, run.ID, now)
	})
	if errors.Is(err, domain.ErrConflict) && in.IdempotencyKey != "" {
		// Lost a race with a concurrent call using the same key.
		return s.runs.FindByIdempotencyKey(ctx, agent.ID, in.IdempotencyKey)
	}
	if err != nil {
		return nil, fmt.Errorf("start run: %w", err)
	}
	return run, nil
}

var eventTypePattern = regexp.MustCompile(`^[a-z]+(\.[a-z_]+)+$`)

// Dispatch starts a run for every agent on duty that listens to the event
// type. The event ID doubles as the idempotency key, which makes Pub/Sub's
// at-least-once delivery safe.
func (s *RunService) Dispatch(ctx context.Context, eventID, eventType string, data json.RawMessage) ([]*domain.Run, error) {
	var v domain.Validator
	v.Text("eventId", eventID, 100)
	v.Pattern("eventType", eventType, eventTypePattern, "must be a dotted event type such as ticket.created")
	if err := v.Err(); err != nil {
		return nil, err
	}
	agents, err := s.agents.List(ctx, port.AgentFilter{
		Status: []domain.AgentStatus{domain.AgentActive, domain.AgentProbation}, Trigger: eventType,
	})
	if err != nil {
		return nil, err
	}
	runs := make([]*domain.Run, 0, len(agents))
	for _, a := range agents {
		run, err := s.Start(ctx, StartRunInput{
			AgentID: a.ID, Input: data, Trigger: domain.TriggerEvent, IdempotencyKey: "event:" + eventID,
		})
		if err != nil {
			return runs, fmt.Errorf("dispatch %s to %s: %w", eventType, a.Slug, err)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// Approve lets a paused run perform its sensitive tool call.
func (s *RunService) Approve(ctx context.Context, runID domain.ID, approver string) (*domain.Run, error) {
	return s.resume(ctx, runID, func(r *domain.Run) error { return r.Approve(approver, s.clock.Now()) })
}

// Deny refuses the paused tool call; the agent is told why and carries on.
func (s *RunService) Deny(ctx context.Context, runID domain.ID, approver, reason string) (*domain.Run, error) {
	return s.resume(ctx, runID, func(r *domain.Run) error { return r.Deny(approver, strings.TrimSpace(reason), s.clock.Now()) })
}

func (s *RunService) resume(ctx context.Context, runID domain.ID, decide func(*domain.Run) error) (*domain.Run, error) {
	var run *domain.Run
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if run, err = s.runs.Get(ctx, runID); err != nil {
			return err
		}
		if err := decide(run); err != nil {
			return err
		}
		if err := s.runs.Save(ctx, run); err != nil {
			return err
		}
		return s.queue.Enqueue(ctx, run.ID, s.clock.Now())
	})
	return run, err
}

// Review records a human verdict on a finished run.
func (s *RunService) Review(ctx context.Context, runID domain.ID, verdict domain.Verdict, reviewer, note string) (*domain.Run, error) {
	run, err := s.runs.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	if err := run.SubmitReview(verdict, reviewer, strings.TrimSpace(note), s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.runs.Save(ctx, run); err != nil {
		return nil, fmt.Errorf("save review: %w", err)
	}
	return run, nil
}

// Cancel stops a run. A worker holding it notices on its next checkpoint,
// when its save fails the version check.
func (s *RunService) Cancel(ctx context.Context, runID domain.ID) (*domain.Run, error) {
	var run *domain.Run
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if run, err = s.runs.Get(ctx, runID); err != nil {
			return err
		}
		if err := run.Cancel(s.clock.Now()); err != nil {
			return err
		}
		if err := s.runs.Save(ctx, run); err != nil {
			return err
		}
		return s.queue.Complete(ctx, run.ID)
	})
	if err != nil {
		return nil, err
	}
	event := domain.RunFinished{RunID: run.ID, Status: run.Status, Cost: run.Cost, Reason: "cancelled"}
	if agent, err := s.agents.Get(ctx, run.AgentID); err == nil {
		event.AgentSlug = agent.Slug
	}
	s.events.Publish(ctx, event)
	return run, nil
}

// Get returns one run with its steps.
func (s *RunService) Get(ctx context.Context, id domain.ID) (*domain.Run, error) {
	return s.runs.Get(ctx, id)
}

// List pages through runs, newest first. hasNext reports whether another
// page exists after the returned slice.
func (s *RunService) List(ctx context.Context, f port.RunFilter, p port.Page) (runs []*domain.Run, hasNext bool, err error) {
	if p.First <= 0 || p.First > 100 {
		return nil, false, domain.NewValidationError("first", domain.CodeOutOfRange, "must be between 1 and 100")
	}
	runs, err = s.runs.List(ctx, f, p)
	if err != nil {
		return nil, false, err
	}
	if len(runs) > p.First {
		return runs[:p.First], true, nil
	}
	return runs, false, nil
}

func (s *RunService) resolveAgent(ctx context.Context, id domain.ID, slug string) (*domain.Agent, error) {
	switch {
	case id != "":
		return s.agents.Get(ctx, id)
	case slug != "":
		return s.agents.GetBySlug(ctx, slug)
	default:
		return nil, domain.NewValidationError("agent", domain.CodeRequired, "give an agent id or slug")
	}
}
