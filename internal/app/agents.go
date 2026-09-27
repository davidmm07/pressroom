// Package app holds Pressroom's use cases. Each service orchestrates domain
// objects through the interfaces in package port and knows nothing about
// GraphQL, Postgres or any model vendor.
//
// Services receive their collaborators through constructors (Dependency
// Injection) and each one covers a single area of the business: the crew,
// runs, evaluation, experiments or intake (Single Responsibility Principle).
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// AgentService manages the crew's configuration and manual lifecycle moves.
type AgentService struct {
	agents port.AgentRepository
	tools  port.ToolRegistry
	events port.EventPublisher
	clock  port.Clock
}

func NewAgentService(agents port.AgentRepository, tools port.ToolRegistry, events port.EventPublisher, clock port.Clock) *AgentService {
	return &AgentService{agents: agents, tools: tools, events: events, clock: clock}
}

// AgentPatch lists optional changes; nil fields are left as they are. The
// slug is immutable because dashboards and event routes link to it.
type AgentPatch struct {
	Name               *string
	Description        *string
	Department         *domain.Department
	Owner              *string
	Instructions       *string
	Model              *domain.ModelRef
	Tools              []string
	Triggers           []string
	Budget             *domain.Budget
	MinutesSavedPerRun *float64
}

func (p AgentPatch) applyTo(s *domain.AgentSpec) {
	set(&s.Name, p.Name)
	set(&s.Description, p.Description)
	set(&s.Department, p.Department)
	set(&s.Owner, p.Owner)
	set(&s.Instructions, p.Instructions)
	set(&s.Model, p.Model)
	set(&s.Budget, p.Budget)
	set(&s.MinutesSavedPerRun, p.MinutesSavedPerRun)
	if p.Tools != nil {
		s.Tools = p.Tools
	}
	if p.Triggers != nil {
		s.Triggers = p.Triggers
	}
}

func set[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

// Create validates the spec, checks every tool exists and stores a draft.
func (s *AgentService) Create(ctx context.Context, spec domain.AgentSpec) (*domain.Agent, error) {
	if err := s.validate(spec); err != nil {
		return nil, err
	}
	if _, err := s.agents.GetBySlug(ctx, spec.Slug); err == nil {
		return nil, domain.NewValidationError("slug", domain.CodeInvalidValue, fmt.Sprintf("%q is already taken", spec.Slug))
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	agent, err := domain.NewAgent(spec, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if err := s.agents.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	return agent, nil
}

// Update applies a patch. expectedVersion guards against two leads editing
// the same agent at once: the second save fails with domain.ErrConflict.
func (s *AgentService) Update(ctx context.Context, id domain.ID, expectedVersion int, patch AgentPatch) (*domain.Agent, error) {
	agent, err := s.agents.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if agent.Version != expectedVersion {
		return nil, fmt.Errorf("agent %s is at version %d, not %d: %w", agent.Slug, agent.Version, expectedVersion, domain.ErrConflict)
	}
	spec := agent.Spec()
	patch.applyTo(&spec)
	if err := s.validate(spec); err != nil {
		return nil, err
	}
	if err := agent.Reconfigure(spec, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.agents.Update(ctx, agent); err != nil {
		return nil, fmt.Errorf("update agent: %w", err)
	}
	return agent, nil
}

// Activate puts a draft agent to work.
func (s *AgentService) Activate(ctx context.Context, id domain.ID) (*domain.Agent, error) {
	return s.move(ctx, id, "activated by an operator", (*domain.Agent).Activate)
}

// Retire removes an agent by hand, e.g. when a lead no longer needs it.
func (s *AgentService) Retire(ctx context.Context, id domain.ID, reason string) (*domain.Agent, error) {
	var v domain.Validator
	v.Text("reason", reason, 500)
	if err := v.Err(); err != nil {
		return nil, err
	}
	return s.move(ctx, id, reason, (*domain.Agent).Retire)
}

func (s *AgentService) move(ctx context.Context, id domain.ID, reason string, change func(*domain.Agent, time.Time) error) (*domain.Agent, error) {
	agent, err := s.agents.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	from := agent.Status
	if err := change(agent, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.agents.Update(ctx, agent); err != nil {
		return nil, fmt.Errorf("update agent: %w", err)
	}
	s.events.Publish(ctx, domain.AgentStatusChanged{
		AgentID: agent.ID, AgentSlug: agent.Slug, Owner: agent.Owner, From: from, To: agent.Status, Reason: reason,
	})
	return agent, nil
}

// Get returns one agent.
func (s *AgentService) Get(ctx context.Context, id domain.ID) (*domain.Agent, error) {
	return s.agents.Get(ctx, id)
}

// GetBySlug returns one agent by its stable slug.
func (s *AgentService) GetBySlug(ctx context.Context, slug string) (*domain.Agent, error) {
	return s.agents.GetBySlug(ctx, slug)
}

// List returns the crew, optionally filtered.
func (s *AgentService) List(ctx context.Context, f port.AgentFilter) ([]*domain.Agent, error) {
	return s.agents.List(ctx, f)
}

// validate merges the domain's field checks with the registry lookup, so a
// form gets "unknown tool" in the same response as "name is required".
func (s *AgentService) validate(spec domain.AgentSpec) error {
	var fields []domain.FieldError
	var ve *domain.ValidationError
	if err := spec.Validate(); errors.As(err, &ve) {
		fields = append(fields, ve.Fields...)
	}
	for i, name := range spec.Tools {
		if _, ok := s.tools.Lookup(name); !ok && domain.ValidToolName(name) {
			fields = append(fields, domain.FieldError{
				Field: fmt.Sprintf("tools[%d]", i), Code: domain.CodeInvalidValue,
				Message: fmt.Sprintf("unknown tool %q", name),
			})
		}
	}
	if len(fields) > 0 {
		return &domain.ValidationError{Fields: fields}
	}
	return nil
}
