package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// OpportunityService is the intake desk where department leads describe work
// an agent could take over, and where those requests become a ranked backlog.
type OpportunityService struct {
	opportunities port.OpportunityRepository
	agents        port.AgentRepository
	clock         port.Clock
}

func NewOpportunityService(opportunities port.OpportunityRepository, agents port.AgentRepository, clock port.Clock) *OpportunityService {
	return &OpportunityService{opportunities: opportunities, agents: agents, clock: clock}
}

// Submit records a new request.
func (s *OpportunityService) Submit(ctx context.Context, in domain.OpportunityInput) (*domain.Opportunity, error) {
	o, err := domain.NewOpportunity(in, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if err := s.opportunities.Create(ctx, o); err != nil {
		return nil, fmt.Errorf("create opportunity: %w", err)
	}
	return o, nil
}

// Backlog lists requests, highest score first.
func (s *OpportunityService) Backlog(ctx context.Context, status domain.OpportunityStatus, dept domain.Department) ([]*domain.Opportunity, error) {
	list, err := s.opportunities.List(ctx, status, dept)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Score() > list[j].Score() })
	return list, nil
}

// Move advances a request. Shipping links the agent that now does the work,
// which closes the loop between "what leads asked for" and "what runs".
func (s *OpportunityService) Move(ctx context.Context, id domain.ID, status domain.OpportunityStatus, agentID domain.ID) (*domain.Opportunity, error) {
	if !status.Valid() {
		return nil, domain.NewValidationError("status", domain.CodeInvalidValue, fmt.Sprintf("%q is not a status", status))
	}
	o, err := s.opportunities.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if agentID != "" {
		if _, err := s.agents.Get(ctx, agentID); err != nil {
			return nil, fmt.Errorf("link agent: %w", err)
		}
	}
	if err := o.MoveTo(status, agentID, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.opportunities.Update(ctx, o); err != nil {
		return nil, fmt.Errorf("update opportunity: %w", err)
	}
	return o, nil
}
