package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/postgres"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Integration tests run against a real database when
// PRESSROOM_TEST_DATABASE_URL is set (CI starts a Postgres service; locally,
// `make test-integration`). Each test creates its own agents, so tests do not need
// to clean up after one another.
func store(t *testing.T) *postgres.Store {
	t.Helper()
	url := os.Getenv("PRESSROOM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PRESSROOM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return postgres.NewStore(pool)
}

var now = time.Now().UTC().Truncate(time.Microsecond)

func newAgent(t *testing.T, s *postgres.Store) *domain.Agent {
	t.Helper()
	a, err := domain.NewAgent(domain.AgentSpec{
		Slug: "agent-" + string(domain.NewID())[24:], Name: "Test agent", Department: domain.DepartmentOperations,
		Owner: "ops@example.com", Instructions: "Watch shipments.",
		Model: domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"},
		Tools: []string{"track_shipment"}, Triggers: []string{"shipment.stalled"},
		Budget: domain.Budget{MaxSteps: 5, MaxCost: domain.MicrosFromUSD(1)}, MinutesSavedPerRun: 12,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Activate(now)
	if err := s.Agents.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAgentRepository(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	a := newAgent(t, s)

	got, err := s.Agents.GetBySlug(ctx, a.Slug)
	if err != nil || got.ID != a.ID || got.Budget != a.Budget || got.Model != a.Model || got.Triggers[0] != "shipment.stalled" {
		t.Fatalf("round trip: %+v, %v", got, err)
	}

	stale := *got
	got.Name = "Renamed"
	if err := s.Agents.Update(ctx, got); err != nil || got.Version != 2 {
		t.Fatalf("update: version=%d err=%v", got.Version, err)
	}
	if err := s.Agents.Update(ctx, &stale); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale update must conflict, got %v", err)
	}
	if err := s.Agents.Create(ctx, a); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate slug must conflict, got %v", err)
	}

	listening, err := s.Agents.List(ctx, port.AgentFilter{Status: []domain.AgentStatus{domain.AgentActive}, Trigger: "shipment.stalled"})
	if err != nil || !containsAgent(listening, a.ID) {
		t.Fatalf("trigger filter missed the agent: %v", err)
	}
	if _, err := s.Agents.Get(ctx, domain.NewID()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRunRepositoryRoundTripsTheTranscript(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	a := newAgent(t, s)

	run, err := domain.NewRun(a, domain.NewRunParams{
		Input: json.RawMessage(`{"trackingNumber":"1ZPRESS"}`), Trigger: domain.TriggerEvent, IdempotencyKey: "event:1", Model: a.Model,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	_ = run.Start(now)
	call := domain.ToolCall{ID: "c1", Name: "track_shipment", Arguments: json.RawMessage(`{"trackingNumber":"1ZPRESS"}`)}
	_ = run.RecordTurn(domain.Message{Text: "checking", ToolCalls: []domain.ToolCall{call},
		ProviderState: json.RawMessage(`{"role":"assistant"}`), StateProvider: domain.ProviderAnthropic},
		domain.Usage{InputTokens: 100, CachedInputTokens: 50, OutputTokens: 10}, 1234, time.Second, now)
	if err := s.Runs.Save(ctx, run); err != nil {
		t.Fatal(err)
	}
	_ = run.ResolveCall(domain.ToolResult{CallID: "c1", Content: `{"shipmentStatus":"STALLED"}`}, call.Arguments, 0, now)
	_ = run.RecordTurn(domain.Message{Text: "Stalled at hub."}, domain.Usage{}, 0, 0, now)
	_ = run.Succeed("Stalled at hub.", now)
	_ = run.SubmitReview(domain.VerdictEdited, "ops@example.com", "tone", now)
	if err := s.Runs.Save(ctx, run); err != nil {
		t.Fatal(err)
	}

	got, err := s.Runs.Get(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RunSucceeded || got.Version != 3 || len(got.Transcript) != 4 || len(got.Steps) != len(run.Steps) {
		t.Fatalf("status=%s version=%d transcript=%d steps=%d/%d", got.Status, got.Version, len(got.Transcript), len(got.Steps), len(run.Steps))
	}
	if m := got.Transcript[1]; m.StateProvider != domain.ProviderAnthropic || string(m.ProviderState) != `{"role": "assistant"}` && string(m.ProviderState) != `{"role":"assistant"}` {
		t.Fatalf("provider state lost: %+v", m)
	}
	if got.Review == nil || got.Review.Verdict != domain.VerdictEdited || got.Usage.CachedInputTokens != 50 || got.Cost != 1234 {
		t.Fatalf("review/usage lost: %+v %+v", got.Review, got.Usage)
	}

	dup, _ := domain.NewRun(a, domain.NewRunParams{Input: json.RawMessage(`{}`), Trigger: domain.TriggerEvent, IdempotencyKey: "event:1", Model: a.Model}, now)
	if err := s.Runs.Create(ctx, dup); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate idempotency key must conflict, got %v", err)
	}
	byKey, err := s.Runs.FindByIdempotencyKey(ctx, a.ID, "event:1")
	if err != nil || byKey.ID != run.ID {
		t.Fatalf("FindByIdempotencyKey = %v, %v", byKey, err)
	}

	stats, err := s.Runs.Stats(ctx, port.StatsQuery{AgentIDs: []domain.ID{a.ID}, Since: now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if st := stats[a.ID]; st.Finished != 1 || st.Succeeded != 1 || st.Edited != 1 || st.Cost != 1234 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestQueueHandsEachJobToOneWorker(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	a := newAgent(t, s)

	const jobs = 20
	for i := 0; i < jobs; i++ {
		r, _ := domain.NewRun(a, domain.NewRunParams{Input: json.RawMessage(`{}`), Trigger: domain.TriggerManual, Model: a.Model}, now)
		err := s.WithinTx(ctx, func(ctx context.Context) error {
			if err := s.Runs.Create(ctx, r); err != nil {
				return err
			}
			return s.Queue.Enqueue(ctx, r.ID, now.Add(-time.Second))
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	seen := map[domain.ID]int{}
	var wg sync.WaitGroup
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				job, err := s.Queue.Claim(ctx, time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				if job == nil {
					return
				}
				mu.Lock()
				seen[job.RunID]++
				mu.Unlock()
				_ = s.Queue.Complete(ctx, job.RunID)
			}
		}()
	}
	wg.Wait()

	mine := 0
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("job %s claimed %d times", id, n)
		}
		if r, err := s.Runs.Get(ctx, id); err == nil && r.AgentID == a.ID {
			mine++
		}
	}
	if mine != jobs {
		t.Fatalf("claimed %d of this test's %d jobs", mine, jobs)
	}
}

func TestOnlyOneRunningExperimentPerAgent(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	a := newAgent(t, s)
	grok := domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"}
	first, _ := domain.NewExperiment(a, grok, 10, "cheaper", now)
	second, _ := domain.NewExperiment(a, grok, 20, "cheaper still", now)
	if err := s.Experiments.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.Experiments.Create(ctx, second); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("the partial unique index must reject a second running experiment, got %v", err)
	}
	running, err := s.Experiments.Running(ctx, a.ID)
	if err != nil || running.ID != first.ID || running.Challenger != grok {
		t.Fatalf("Running = %+v, %v", running, err)
	}
}

func containsAgent(list []*domain.Agent, id domain.ID) bool {
	for _, a := range list {
		if a.ID == id {
			return true
		}
	}
	return false
}
