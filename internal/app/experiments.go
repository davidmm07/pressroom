package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// ExperimentService runs champion/challenger trials so a new model is
// adopted because it did the job better or cheaper, not because it shipped.
type ExperimentService struct {
	agents      port.AgentRepository
	runs        port.RunRepository
	experiments port.ExperimentRepository
	tx          port.TxManager
	events      port.EventPublisher
	clock       port.Clock
	hourlyRate  domain.Micros
	policy      domain.ComparisonPolicy
}

func NewExperimentService(
	agents port.AgentRepository, runs port.RunRepository, experiments port.ExperimentRepository,
	tx port.TxManager, events port.EventPublisher, clock port.Clock,
	hourlyRate domain.Micros, policy domain.ComparisonPolicy,
) *ExperimentService {
	return &ExperimentService{agents: agents, runs: runs, experiments: experiments, tx: tx, events: events,
		clock: clock, hourlyRate: hourlyRate, policy: policy}
}

// StartExperimentInput describes a trial.
type StartExperimentInput struct {
	AgentID        domain.ID
	Challenger     domain.ModelRef
	TrafficPercent int
	Hypothesis     string
}

// Start opens a trial. An agent runs at most one experiment at a time so
// results are attributable.
func (s *ExperimentService) Start(ctx context.Context, in StartExperimentInput) (*domain.Experiment, error) {
	agent, err := s.agents.Get(ctx, in.AgentID)
	if err != nil {
		return nil, err
	}
	if running, err := s.experiments.Running(ctx, agent.ID); err == nil {
		return nil, fmt.Errorf("%s is already trialling %s: %w", agent.Slug, running.Challenger, domain.ErrConflict)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	exp, err := domain.NewExperiment(agent, in.Challenger, in.TrafficPercent, in.Hypothesis, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if err := s.experiments.Create(ctx, exp); err != nil {
		return nil, fmt.Errorf("create experiment: %w", err)
	}
	return exp, nil
}

// ArmScorecards returns the champion and challenger cards of a trial.
func (s *ExperimentService) ArmScorecards(ctx context.Context, exp *domain.Experiment) (champion, challenger domain.Scorecard, err error) {
	agent, err := s.agents.Get(ctx, exp.AgentID)
	if err != nil {
		return champion, challenger, err
	}
	arm := func(v domain.Variant) (domain.Scorecard, error) {
		stats, err := s.runs.Stats(ctx, port.StatsQuery{
			AgentIDs: []domain.ID{agent.ID}, Since: exp.CreatedAt, ExperimentID: exp.ID, Variant: v,
		})
		if err != nil {
			return domain.Scorecard{}, err
		}
		days := int(s.clock.Now().Sub(exp.CreatedAt).Hours()/24) + 1
		return domain.Scorecard{RunStats: stats[agent.ID], WindowDays: days,
			MinutesSavedPerRun: agent.MinutesSavedPerRun, HourlyRate: s.hourlyRate}, nil
	}
	if champion, err = arm(domain.VariantChampion); err != nil {
		return champion, challenger, err
	}
	challenger, err = arm(domain.VariantChallenger)
	return champion, challenger, err
}

// Conclude compares the arms and, when the challenger wins, makes it the
// agent's model. force closes an inconclusive trial and keeps the champion.
func (s *ExperimentService) Conclude(ctx context.Context, id domain.ID, force bool) (*domain.Experiment, error) {
	exp, err := s.experiments.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if exp.Status != domain.ExperimentRunning {
		return nil, &domain.TransitionError{Entity: "experiment", From: string(exp.Status), To: "concluded"}
	}
	champion, challenger, err := s.ArmScorecards(ctx, exp)
	if err != nil {
		return nil, err
	}
	promote, conclusive, summary := s.policy.Compare(champion, challenger)
	if !conclusive && !force {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidTransition, summary)
	}
	if !conclusive {
		summary = "closed early, champion kept: " + summary
	}

	var agent *domain.Agent
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		if err := exp.Conclude(promote, summary, now); err != nil {
			return err
		}
		if err := s.experiments.Update(ctx, exp); err != nil {
			return err
		}
		if agent, err = s.agents.Get(ctx, exp.AgentID); err != nil {
			return err
		}
		if !promote {
			return nil
		}
		if err := agent.PromoteModel(exp.Challenger, now); err != nil {
			return err
		}
		return s.agents.Update(ctx, agent)
	})
	if err != nil {
		return nil, err
	}
	winner := exp.Champion
	if promote {
		winner = exp.Challenger
	}
	s.events.Publish(ctx, domain.ExperimentConcluded{
		ExperimentID: exp.ID, AgentSlug: agent.Slug, Promoted: promote, Winner: winner, Summary: summary,
	})
	return exp, nil
}

// Get returns one experiment.
func (s *ExperimentService) Get(ctx context.Context, id domain.ID) (*domain.Experiment, error) {
	return s.experiments.Get(ctx, id)
}

// Running returns the agent's running experiment, or nil when there is none.
func (s *ExperimentService) Running(ctx context.Context, agentID domain.ID) (*domain.Experiment, error) {
	exp, err := s.experiments.Running(ctx, agentID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return exp, err
}

// List returns experiments, optionally for one agent or status.
func (s *ExperimentService) List(ctx context.Context, agentID domain.ID, status domain.ExperimentStatus) ([]*domain.Experiment, error) {
	return s.experiments.List(ctx, agentID, status)
}
