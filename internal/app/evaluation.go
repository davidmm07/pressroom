package app

import (
	"context"
	"fmt"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// EvaluationConfig sets how the crew is measured.
type EvaluationConfig struct {
	Window     time.Duration
	HourlyRate domain.Micros // loaded cost of an hour of the work agents replace
	Policy     domain.RetirementPolicy
}

// EvaluationService measures agents and removes the ones that do not
// deliver. It runs daily from Cloud Scheduler and on demand from the API.
type EvaluationService struct {
	agents port.AgentRepository
	runs   port.RunRepository
	evals  port.EvaluationRepository
	tx     port.TxManager
	events port.EventPublisher
	clock  port.Clock
	cfg    EvaluationConfig
}

func NewEvaluationService(
	agents port.AgentRepository, runs port.RunRepository, evals port.EvaluationRepository,
	tx port.TxManager, events port.EventPublisher, clock port.Clock, cfg EvaluationConfig,
) *EvaluationService {
	return &EvaluationService{agents: agents, runs: runs, evals: evals, tx: tx, events: events, clock: clock, cfg: cfg}
}

// Scorecards builds cards for several agents with a single aggregate query.
// The GraphQL layer calls it through a DataLoader to avoid N+1 queries.
func (s *EvaluationService) Scorecards(ctx context.Context, agents []*domain.Agent, windowDays int) (map[domain.ID]domain.Scorecard, error) {
	if windowDays < 1 || windowDays > 365 {
		return nil, domain.NewValidationError("windowDays", domain.CodeOutOfRange, "must be between 1 and 365")
	}
	ids := make([]domain.ID, len(agents))
	for i, a := range agents {
		ids[i] = a.ID
	}
	since := s.clock.Now().AddDate(0, 0, -windowDays)
	stats, err := s.runs.Stats(ctx, port.StatsQuery{AgentIDs: ids, Since: since})
	if err != nil {
		return nil, fmt.Errorf("load run stats: %w", err)
	}
	cards := make(map[domain.ID]domain.Scorecard, len(agents))
	for _, a := range agents {
		cards[a.ID] = s.card(a, stats[a.ID], windowDays)
	}
	return cards, nil
}

func (s *EvaluationService) card(a *domain.Agent, stats domain.RunStats, windowDays int) domain.Scorecard {
	return domain.Scorecard{
		RunStats: stats, WindowDays: windowDays,
		MinutesSavedPerRun: a.MinutesSavedPerRun, HourlyRate: s.cfg.HourlyRate,
	}
}

// EvaluateCrew applies the retirement policy to every agent on duty and
// records each decision, including the ones that change nothing.
func (s *EvaluationService) EvaluateCrew(ctx context.Context) ([]*domain.Evaluation, error) {
	agents, err := s.agents.List(ctx, port.AgentFilter{Status: []domain.AgentStatus{domain.AgentActive, domain.AgentProbation}})
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return nil, nil
	}
	windowDays := int(s.cfg.Window.Hours() / 24)
	cards, err := s.Scorecards(ctx, agents, windowDays)
	if err != nil {
		return nil, err
	}

	evaluations := make([]*domain.Evaluation, 0, len(agents))
	for _, agent := range agents {
		now := s.clock.Now()
		card := cards[agent.ID]
		decision, reason := s.cfg.Policy.Decide(agent.Status, card)
		before := agent.Status
		if err := decision.Apply(agent, now); err != nil {
			return evaluations, fmt.Errorf("apply %s to %s: %w", decision, agent.Slug, err)
		}
		eval := &domain.Evaluation{
			ID: domain.NewID(), AgentID: agent.ID, Decision: decision, Reason: reason, Scorecard: card,
			StatusBefore: before, StatusAfter: agent.Status, CreatedAt: now,
		}
		err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
			if before != agent.Status {
				if err := s.agents.Update(ctx, agent); err != nil {
					return err
				}
			}
			return s.evals.Create(ctx, eval)
		})
		if err != nil {
			return evaluations, fmt.Errorf("record evaluation of %s: %w", agent.Slug, err)
		}
		evaluations = append(evaluations, eval)
		if before != agent.Status {
			s.events.Publish(ctx, domain.AgentStatusChanged{
				AgentID: agent.ID, AgentSlug: agent.Slug, Owner: agent.Owner, From: before, To: agent.Status, Reason: reason,
			})
		}
	}
	return evaluations, nil
}

// History lists an agent's most recent evaluations.
func (s *EvaluationService) History(ctx context.Context, agentID domain.ID, limit int) ([]*domain.Evaluation, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	return s.evals.ListByAgent(ctx, agentID, limit)
}
