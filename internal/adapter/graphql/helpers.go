package graphql

import (
	"context"
	"errors"
	"sync"

	"github.com/davidmm07/pressroom/internal/adapter/graphql/model"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Shared resolver plumbing, kept out of schema.resolvers.go so gqlgen's
// regeneration never has to move it around.

func (r *Resolver) runConnection(ctx context.Context, agentID domain.ID, status []model.RunStatus, first *int, after *string) (*model.RunConnection, error) {
	cursor, err := decodeCursor(after)
	if err != nil {
		return nil, err
	}
	f := port.RunFilter{AgentID: agentID}
	for _, s := range status {
		f.Status = append(f.Status, domain.RunStatus(s))
	}
	runs, hasNext, err := r.Runs.List(ctx, f, port.Page{First: deref(first, 20), After: cursor})
	if err != nil {
		return nil, err
	}
	return r.toConnection(runs, hasNext), nil
}

// changeRun runs a run mutation and packs the outcome into a payload.
func (r *Resolver) changeRun(ctx context.Context, rawID string, change func(domain.ID) (*domain.Run, error)) (*model.RunPayload, error) {
	id, err := parseID(rawID)
	if err != nil {
		return runFailure(err)
	}
	run, err := change(id)
	if err != nil {
		return runFailure(err)
	}
	return &model.RunPayload{Run: r.toRun(run), UserErrors: []*model.UserError{}}, nil
}

// armScorecards computes both arms once per request even though the champion
// and challenger fields resolve separately.
func (r *Resolver) armScorecards(ctx context.Context, rawID string) (*model.Scorecard, *model.Scorecard, error) {
	cache := r.loadersFrom(ctx)
	cache.armsMu.Lock()
	defer cache.armsMu.Unlock()
	if cache.arms == nil {
		cache.arms = map[string][2]*model.Scorecard{}
	}
	if cards, ok := cache.arms[rawID]; ok {
		return cards[0], cards[1], nil
	}
	id, err := parseID(rawID)
	if err != nil {
		return nil, nil, err
	}
	exp, err := r.Experiments.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	champion, challenger, err := r.Experiments.ArmScorecards(ctx, exp)
	if err != nil {
		return nil, nil, err
	}
	cards := [2]*model.Scorecard{toScorecard(champion), toScorecard(challenger)}
	cache.arms[rawID] = cards
	return cards[0], cards[1], nil
}

func agentFailure(err error) (*model.AgentPayload, error) {
	errs, err := userErrors(err, "", agentInputFields)
	return &model.AgentPayload{UserErrors: errs}, err
}

func runFailure(err error) (*model.RunPayload, error) {
	errs, err := userErrors(err, "input", nil)
	return &model.RunPayload{UserErrors: errs}, err
}

func experimentFailure(err error) (*model.ExperimentPayload, error) {
	errs, err := userErrors(err, "", nil)
	return &model.ExperimentPayload{UserErrors: errs}, err
}

// modelFieldError reports an unparsable "provider/name" on the model field.
func modelFieldError(err error) *model.UserError {
	errs, _ := userErrors(err, "input", fieldNames{"model": "model", "provider": "model", "name": "model"})
	if len(errs) > 0 {
		return errs[0]
	}
	return &model.UserError{Field: []string{"input", "model"}, Code: model.UserErrorCodeInvalidFormat, Message: err.Error()}
}

// nilIfNotFound follows the GraphQL convention that a missing object is null,
// not an error.
func nilIfNotFound(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

// armCache lives on the per-request loaders.
type armCache struct {
	armsMu sync.Mutex
	arms   map[string][2]*model.Scorecard
}
