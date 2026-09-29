package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/app"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Test doubles. Repositories store deep copies so a use case that forgets to
// save its changes fails the test, exactly as it would against Postgres.

func clone[T any](t *T) *T {
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	out := new(T)
	if err := json.Unmarshal(b, out); err != nil {
		panic(err)
	}
	return out
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time                                             { return c.now }
func (c *fakeClock) Advance(d time.Duration)                                    { c.now = c.now.Add(d) }
func newClock() *fakeClock                                                      { return &fakeClock{now: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)} }
func discardLogger() *slog.Logger                                               { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
func (noTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type noTx struct{}

// ---- agents ----

type memAgents struct {
	mu   sync.Mutex
	byID map[domain.ID]*domain.Agent
}

func newMemAgents() *memAgents { return &memAgents{byID: map[domain.ID]*domain.Agent{}} }

func (m *memAgents) Create(_ context.Context, a *domain.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[a.ID] = clone(a)
	return nil
}

func (m *memAgents) Update(_ context.Context, a *domain.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.byID[a.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if cur.Version != a.Version {
		return domain.ErrConflict
	}
	a.Version++
	m.byID[a.ID] = clone(a)
	return nil
}

func (m *memAgents) Get(_ context.Context, id domain.ID) (*domain.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.byID[id]; ok {
		return clone(a), nil
	}
	return nil, domain.ErrNotFound
}

func (m *memAgents) GetBySlug(_ context.Context, slug string) (*domain.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.byID {
		if a.Slug == slug {
			return clone(a), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *memAgents) List(_ context.Context, f port.AgentFilter) ([]*domain.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Agent
	for _, a := range m.byID {
		if len(f.Status) > 0 && !slices.Contains(f.Status, a.Status) {
			continue
		}
		if f.Trigger != "" && !a.ListensTo(f.Trigger) {
			continue
		}
		out = append(out, clone(a))
	}
	slices.SortFunc(out, func(a, b *domain.Agent) int { return compare(a.Slug, b.Slug) })
	return out, nil
}

// ---- runs ----

type memRuns struct {
	mu   sync.Mutex
	byID map[domain.ID]*domain.Run
	// onSave lets a test interleave another writer with the executor.
	onSave func(*domain.Run)
}

func newMemRuns() *memRuns { return &memRuns{byID: map[domain.ID]*domain.Run{}} }

func (m *memRuns) Create(_ context.Context, r *domain.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.byID {
		if r.IdempotencyKey != "" && o.AgentID == r.AgentID && o.IdempotencyKey == r.IdempotencyKey {
			return domain.ErrConflict
		}
	}
	m.byID[r.ID] = clone(r)
	return nil
}

func (m *memRuns) Save(_ context.Context, r *domain.Run) error {
	m.mu.Lock()
	hook := m.onSave
	m.mu.Unlock()
	if hook != nil {
		hook(r)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.byID[r.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if cur.Version != r.Version {
		return domain.ErrConflict
	}
	r.Version++
	m.byID[r.ID] = clone(r)
	return nil
}

func (m *memRuns) Get(_ context.Context, id domain.ID) (*domain.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byID[id]; ok {
		return clone(r), nil
	}
	return nil, domain.ErrNotFound
}

func (m *memRuns) FindByIdempotencyKey(_ context.Context, agentID domain.ID, key string) (*domain.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.byID {
		if r.AgentID == agentID && r.IdempotencyKey == key {
			return clone(r), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *memRuns) List(_ context.Context, f port.RunFilter, p port.Page) ([]*domain.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Run
	for _, r := range m.byID {
		if (f.AgentID == "" || r.AgentID == f.AgentID) && (len(f.Status) == 0 || slices.Contains(f.Status, r.Status)) &&
			(p.After == "" || r.ID < p.After) {
			out = append(out, clone(r))
		}
	}
	slices.SortFunc(out, func(a, b *domain.Run) int { return compare(b.ID, a.ID) })
	if len(out) > p.First+1 {
		out = out[:p.First+1]
	}
	return out, nil
}

func (m *memRuns) Stats(_ context.Context, q port.StatsQuery) (map[domain.ID]domain.RunStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[domain.ID]domain.RunStats{}
	for _, r := range m.byID {
		if !slices.Contains(q.AgentIDs, r.AgentID) || r.CreatedAt.Before(q.Since) ||
			(q.ExperimentID != "" && r.ExperimentID != q.ExperimentID) || (q.Variant != "" && r.Variant != q.Variant) {
			continue
		}
		s := out[r.AgentID]
		switch r.Status {
		case domain.RunSucceeded:
			s.Finished++
			s.Succeeded++
		case domain.RunFailed:
			s.Finished++
			s.Failed++
		default:
			continue
		}
		if r.Review != nil {
			switch r.Review.Verdict {
			case domain.VerdictAccepted:
				s.Accepted++
			case domain.VerdictEdited:
				s.Edited++
			case domain.VerdictRejected:
				s.Rejected++
			}
		}
		s.Cost += r.Cost
		out[r.AgentID] = s
	}
	return out, nil
}

// ---- experiments, evaluations ----

type memExperiments struct {
	mu   sync.Mutex
	byID map[domain.ID]*domain.Experiment
}

func newMemExperiments() *memExperiments {
	return &memExperiments{byID: map[domain.ID]*domain.Experiment{}}
}

func (m *memExperiments) Create(_ context.Context, e *domain.Experiment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[e.ID] = clone(e)
	return nil
}

func (m *memExperiments) Update(ctx context.Context, e *domain.Experiment) error {
	return m.Create(ctx, e)
}

func (m *memExperiments) Get(_ context.Context, id domain.ID) (*domain.Experiment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.byID[id]; ok {
		return clone(e), nil
	}
	return nil, domain.ErrNotFound
}

func (m *memExperiments) Running(_ context.Context, agentID domain.ID) (*domain.Experiment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.byID {
		if e.AgentID == agentID && e.Status == domain.ExperimentRunning {
			return clone(e), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *memExperiments) List(_ context.Context, agentID domain.ID, status domain.ExperimentStatus) ([]*domain.Experiment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Experiment
	for _, e := range m.byID {
		if (agentID == "" || e.AgentID == agentID) && (status == "" || e.Status == status) {
			out = append(out, clone(e))
		}
	}
	return out, nil
}

type memEvaluations struct {
	mu   sync.Mutex
	list []*domain.Evaluation
}

func (m *memEvaluations) Create(_ context.Context, e *domain.Evaluation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.list = append(m.list, clone(e))
	return nil
}

func (m *memEvaluations) ListByAgent(_ context.Context, agentID domain.ID, limit int) ([]*domain.Evaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Evaluation
	for i := len(m.list) - 1; i >= 0 && len(out) < limit; i-- {
		if m.list[i].AgentID == agentID {
			out = append(out, clone(m.list[i]))
		}
	}
	return out, nil
}

// ---- queue, events ----

type memQueue struct {
	mu        sync.Mutex
	enqueued  []domain.ID
	completed []domain.ID
}

func (q *memQueue) Enqueue(_ context.Context, id domain.ID, _ time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueued = append(q.enqueued, id)
	return nil
}
func (q *memQueue) Claim(context.Context, time.Duration) (*port.Job, error)   { return nil, nil }
func (q *memQueue) Extend(context.Context, domain.ID, time.Duration) error    { return nil }
func (q *memQueue) Retry(context.Context, domain.ID, time.Time, string) error { return nil }
func (q *memQueue) Complete(_ context.Context, id domain.ID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, id)
	return nil
}

type recorder struct {
	mu     sync.Mutex
	events []domain.Event
}

func (r *recorder) Publish(_ context.Context, events ...domain.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = e.EventName()
	}
	return out
}

// ---- model, tools, prices ----

// scriptedModel replays canned completions and records every request.
type scriptedModel struct {
	mu       sync.Mutex
	script   []func(port.CompletionRequest) (*port.Completion, error)
	requests []port.CompletionRequest
}

func (m *scriptedModel) then(f func(port.CompletionRequest) (*port.Completion, error)) *scriptedModel {
	m.script = append(m.script, f)
	return m
}

func (m *scriptedModel) callTool(id, name, args string) *scriptedModel {
	return m.then(func(port.CompletionRequest) (*port.Completion, error) {
		return &port.Completion{
			Message:    domain.Message{ToolCalls: []domain.ToolCall{{ID: id, Name: name, Arguments: json.RawMessage(args)}}},
			Usage:      domain.Usage{InputTokens: 1000, OutputTokens: 100},
			StopReason: port.StopToolUse,
		}, nil
	})
}

func (m *scriptedModel) answer(text string) *scriptedModel {
	return m.then(func(port.CompletionRequest) (*port.Completion, error) {
		return &port.Completion{Message: domain.Message{Text: text}, Usage: domain.Usage{InputTokens: 1200, OutputTokens: 80}, StopReason: port.StopEndTurn}, nil
	})
}

func (m *scriptedModel) fail(err error) *scriptedModel {
	return m.then(func(port.CompletionRequest) (*port.Completion, error) { return nil, err })
}

func (m *scriptedModel) Complete(_ context.Context, req port.CompletionRequest) (*port.Completion, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	if len(m.script) == 0 {
		m.mu.Unlock()
		return nil, errors.New("script exhausted")
	}
	next := m.script[0]
	m.script = m.script[1:]
	m.mu.Unlock()
	return next(req)
}

func (m *scriptedModel) lastRequest() port.CompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[len(m.requests)-1]
}

type fakeTools struct {
	mu    sync.Mutex
	specs map[string]domain.ToolSpec
	calls map[string]int
	infos []port.ToolCallInfo
	fn    map[string]func(json.RawMessage) (json.RawMessage, error)
}

func newFakeTools() *fakeTools {
	t := &fakeTools{specs: map[string]domain.ToolSpec{}, calls: map[string]int{}, fn: map[string]func(json.RawMessage) (json.RawMessage, error){}}
	t.add(domain.ToolSpec{Name: "lookup_order"}, func(json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"orderId":"ORD-1042","status":"SHIPPED","trackingNumber":"1ZPRESS"}`), nil
	})
	t.add(domain.ToolSpec{Name: "issue_refund", RequiresApproval: true}, func(json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"refundId":"re_1"}`), nil
	})
	t.add(domain.ToolSpec{Name: "track_shipment"}, func(args json.RawMessage) (json.RawMessage, error) {
		return nil, &port.ToolInputError{Tool: "track_shipment", Detail: "missing property 'trackingNumber'"}
	})
	return t
}

func (t *fakeTools) add(spec domain.ToolSpec, fn func(json.RawMessage) (json.RawMessage, error)) {
	t.specs[spec.Name] = spec
	t.fn[spec.Name] = fn
}

func (t *fakeTools) Catalog() []domain.ToolSpec {
	var out []domain.ToolSpec
	for _, s := range t.specs {
		out = append(out, s)
	}
	return out
}

func (t *fakeTools) Lookup(name string) (domain.ToolSpec, bool) {
	s, ok := t.specs[name]
	return s, ok
}

func (t *fakeTools) Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	t.calls[name]++
	if info, ok := port.ToolCallFrom(ctx); ok {
		t.infos = append(t.infos, info)
	}
	fn := t.fn[name]
	t.mu.Unlock()
	return fn(args)
}

func (t *fakeTools) count(name string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls[name]
}

type flatPrices struct{}

// $1 per million input tokens and $5 per million output tokens.
func (flatPrices) Price(domain.ModelRef) domain.Price {
	return domain.Price{InputPerMTok: 1, OutputPerMTok: 5}
}

// ---- harness ----

type harness struct {
	t           *testing.T
	clock       *fakeClock
	agents      *memAgents
	runs        *memRuns
	experiments *memExperiments
	evals       *memEvaluations
	queue       *memQueue
	events      *recorder
	model       *scriptedModel
	tools       *fakeTools

	agentSvc *app.AgentService
	runSvc   *app.RunService
	executor *app.Executor
	evalSvc  *app.EvaluationService
	expSvc   *app.ExperimentService
}

func newHarness(t *testing.T) *harness {
	h := &harness{
		t: t, clock: newClock(), agents: newMemAgents(), runs: newMemRuns(), experiments: newMemExperiments(),
		evals: &memEvaluations{}, queue: &memQueue{}, events: &recorder{}, model: &scriptedModel{}, tools: newFakeTools(),
	}
	h.agentSvc = app.NewAgentService(h.agents, h.tools, h.events, h.clock)
	h.runSvc = app.NewRunService(h.agents, h.runs, h.experiments, h.queue, noTx{}, h.events, h.clock)
	h.executor = app.NewExecutor(h.agents, h.runs, h.model, h.tools, flatPrices{}, h.events, h.clock,
		discardLogger(), app.DefaultExecutorConfig())
	h.evalSvc = app.NewEvaluationService(h.agents, h.runs, h.evals, noTx{}, h.events, h.clock, app.EvaluationConfig{
		Window: 30 * 24 * time.Hour, HourlyRate: domain.MicrosFromUSD(40), Policy: domain.DefaultRetirementPolicy(),
	})
	h.expSvc = app.NewExperimentService(h.agents, h.runs, h.experiments, noTx{}, h.events, h.clock,
		domain.MicrosFromUSD(40), domain.DefaultComparisonPolicy())
	return h
}

// hireAgent creates and activates an agent with the given slug and tools.
func (h *harness) hireAgent(slug string, tools ...string) *domain.Agent {
	h.t.Helper()
	ctx := context.Background()
	a, err := h.agentSvc.Create(ctx, domain.AgentSpec{
		Slug: slug, Name: slug, Department: domain.DepartmentCustomerExperience, Owner: "cx-lead@example.com",
		Instructions: "Resolve customer tickets.", Model: domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"},
		Tools: tools, Triggers: []string{"ticket.created"},
		Budget:             domain.Budget{MaxSteps: 6, MaxCost: domain.MicrosFromUSD(1)},
		MinutesSavedPerRun: 10,
	})
	if err != nil {
		h.t.Fatalf("create agent: %v", err)
	}
	if a, err = h.agentSvc.Activate(ctx, a.ID); err != nil {
		h.t.Fatalf("activate agent: %v", err)
	}
	return a
}

func (h *harness) start(a *domain.Agent, input string) *domain.Run {
	h.t.Helper()
	r, err := h.runSvc.Start(context.Background(), app.StartRunInput{AgentID: a.ID, Input: json.RawMessage(input), Trigger: domain.TriggerManual})
	if err != nil {
		h.t.Fatalf("start run: %v", err)
	}
	return r
}

func (h *harness) reload(id domain.ID) *domain.Run {
	h.t.Helper()
	r, err := h.runs.Get(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

// seedFinishedRuns writes historical runs straight into the repository.
func (h *harness) seedFinishedRuns(a *domain.Agent, n, succeeded, accepted int, costEach float64, exp *domain.Experiment, v domain.Variant) {
	for i := 0; i < n; i++ {
		r := &domain.Run{
			ID: domain.NewID(), AgentID: a.ID, Model: a.Model, Variant: v, Trigger: domain.TriggerManual,
			Input: json.RawMessage(`{}`), Status: domain.RunFailed, Cost: domain.MicrosFromUSD(costEach),
			Version: 1, CreatedAt: h.clock.Now().Add(-time.Hour),
		}
		if exp != nil {
			r.ExperimentID = exp.ID
			r.Model = exp.ModelFor(v)
		}
		if i < succeeded {
			r.Status = domain.RunSucceeded
			verdict := domain.VerdictRejected
			if i < accepted {
				verdict = domain.VerdictAccepted
			}
			r.Review = &domain.Review{Verdict: verdict, Reviewer: "lead@example.com"}
		}
		if err := h.runs.Create(context.Background(), r); err != nil {
			h.t.Fatal(err)
		}
	}
}

func compare[T ~string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
