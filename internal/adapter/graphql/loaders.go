package graphql

import (
	"context"
	"net/http"
	"time"

	"github.com/vikstrous/dataloadgen"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Per-request DataLoaders batch and cache lookups that would otherwise run
// once per list item: a crew page with 30 agents asks for 30 scorecards, and
// the loader turns that into one aggregate query (the N+1 problem).
type loaders struct {
	agent     *dataloadgen.Loader[domain.ID, *domain.Agent]
	run       *dataloadgen.Loader[domain.ID, *domain.Run]
	scorecard *dataloadgen.Loader[scorecardKey, domain.Scorecard]
	armCache
}

type scorecardKey struct {
	AgentID    domain.ID
	WindowDays int
}

type loadersKey struct{}

func (r *Resolver) newLoaders() *loaders {
	l := &loaders{}
	wait := dataloadgen.WithWait(2 * time.Millisecond)

	// The crew is small, so one listing answers any batch of agent IDs.
	l.agent = dataloadgen.NewLoader(func(ctx context.Context, ids []domain.ID) ([]*domain.Agent, []error) {
		all, err := r.Agents.List(ctx, port.AgentFilter{})
		out, errs := make([]*domain.Agent, len(ids)), make([]error, len(ids))
		byID := make(map[domain.ID]*domain.Agent, len(all))
		for _, a := range all {
			byID[a.ID] = a
		}
		for i, id := range ids {
			switch {
			case err != nil:
				errs[i] = err
			case byID[id] == nil:
				errs[i] = domain.ErrNotFound
			default:
				out[i] = byID[id]
			}
		}
		return out, errs
	}, wait)

	l.run = dataloadgen.NewLoader(func(ctx context.Context, ids []domain.ID) ([]*domain.Run, []error) {
		out, errs := make([]*domain.Run, len(ids)), make([]error, len(ids))
		for i, id := range ids {
			out[i], errs[i] = r.Runs.Get(ctx, id)
		}
		return out, errs
	}, wait)

	l.scorecard = dataloadgen.NewLoader(func(ctx context.Context, keys []scorecardKey) ([]domain.Scorecard, []error) {
		out, errs := make([]domain.Scorecard, len(keys)), make([]error, len(keys))
		byWindow := map[int][]int{} // window -> key positions
		for i, k := range keys {
			byWindow[k.WindowDays] = append(byWindow[k.WindowDays], i)
		}
		for window, positions := range byWindow {
			agents := make([]*domain.Agent, 0, len(positions))
			for _, i := range positions {
				a, err := l.agent.Load(ctx, keys[i].AgentID)
				if err != nil {
					errs[i] = err
					continue
				}
				agents = append(agents, a)
			}
			cards, err := r.Evaluation.Scorecards(ctx, agents, window)
			for _, i := range positions {
				if errs[i] == nil {
					out[i], errs[i] = cards[keys[i].AgentID], err
				}
			}
		}
		return out, errs
	}, wait)
	return l
}

// LoaderMiddleware gives every request fresh loaders, so cached values never
// leak between requests or users.
func (r *Resolver) LoaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := context.WithValue(req.Context(), loadersKey{}, r.newLoaders())
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

func (r *Resolver) loadersFrom(ctx context.Context) *loaders {
	if l, ok := ctx.Value(loadersKey{}).(*loaders); ok {
		return l
	}
	return r.newLoaders() // e.g. in tests that call resolvers directly
}
