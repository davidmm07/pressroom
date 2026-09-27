package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/app"
	"github.com/davidmm07/pressroom/internal/domain"
)

func TestCreateAgentReportsUnknownToolsWithOtherFieldErrors(t *testing.T) {
	h := newHarness(t)
	_, err := h.agentSvc.Create(context.Background(), domain.AgentSpec{
		Slug: "reorder-nudger", Department: domain.DepartmentMarketing, Owner: "growth@example.com",
		Instructions: "Nudge customers.", Model: domain.ModelRef{Provider: domain.ProviderOpenAI, Name: "gpt-5"},
		Tools:  []string{"lookup_order", "launch_rockets"},
		Budget: domain.Budget{MaxSteps: 4, MaxCost: 100},
	})
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	fields := map[string]bool{}
	for _, f := range ve.Fields {
		fields[f.Field] = true
	}
	if !fields["name"] || !fields["tools[1]"] {
		t.Fatalf("want name and tools[1] errors together, got %+v", ve.Fields)
	}
}

func TestUpdateAgentDetectsConcurrentEdits(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("support-triage", "lookup_order")
	name := "Support Triage"
	if _, err := h.agentSvc.Update(ctx, agent.ID, agent.Version, app.AgentPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.agentSvc.Update(ctx, agent.ID, agent.Version, app.AgentPatch{Name: &name}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale version must conflict, got %v", err)
	}
}

func TestStartRunIsIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("support-triage", "lookup_order")
	in := app.StartRunInput{AgentSlug: agent.Slug, Input: json.RawMessage(`{"ticketId":"T-1"}`), Trigger: domain.TriggerManual, IdempotencyKey: "ticket-T-1"}

	first, err := h.runSvc.Start(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.runSvc.Start(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(h.queue.enqueued) != 1 {
		t.Fatalf("want one run and one job, got runs %s/%s and %d jobs", first.ID, second.ID, len(h.queue.enqueued))
	}
}

func TestDispatchStartsRunsForListeningAgentsOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.hireAgent("support-triage", "lookup_order")
	h.hireAgent("refund-desk", "issue_refund")
	draft, _ := h.agentSvc.Create(ctx, domain.AgentSpec{
		Slug: "draft-agent", Name: "Draft", Department: domain.DepartmentCustomerExperience, Owner: "cx@example.com",
		Instructions: "x", Model: domain.ModelRef{Provider: domain.ProviderSandbox, Name: "planner"},
		Tools: []string{"lookup_order"}, Triggers: []string{"ticket.created"}, Budget: domain.Budget{MaxSteps: 1, MaxCost: 1},
	})

	runs, err := h.runSvc.Dispatch(ctx, "pubsub-123", "ticket.created", json.RawMessage(`{"ticketId":"T-2"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(runs))
	}
	for _, r := range runs {
		if r.AgentID == draft.ID || r.Trigger != domain.TriggerEvent {
			t.Fatalf("unexpected run %+v", r)
		}
	}
	again, _ := h.runSvc.Dispatch(ctx, "pubsub-123", "ticket.created", json.RawMessage(`{"ticketId":"T-2"}`))
	if again[0].ID != runs[0].ID && again[0].ID != runs[1].ID {
		t.Fatal("a redelivered event must not start new runs")
	}
}

func TestExperimentRoutesTrafficAndPromotesAWinner(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("support-triage", "lookup_order")
	grok := domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"}

	exp, err := h.expSvc.Start(ctx, app.StartExperimentInput{AgentID: agent.ID, Challenger: grok, TrafficPercent: 50, Hypothesis: "Grok triages as well for less"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.expSvc.Start(ctx, app.StartExperimentInput{AgentID: agent.ID, Challenger: grok, TrafficPercent: 10, Hypothesis: "again"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("one experiment per agent, got %v", err)
	}

	variants := map[domain.Variant]int{}
	for i := 0; i < 40; i++ {
		r := h.start(agent, `{}`)
		variants[r.Variant]++
		if r.ExperimentID != exp.ID || r.Model != exp.ModelFor(r.Variant) {
			t.Fatalf("run %s not routed through the experiment: %+v", r.ID, r)
		}
	}
	if variants[domain.VariantChampion] == 0 || variants[domain.VariantChallenger] == 0 {
		t.Fatalf("both arms should get traffic: %v", variants)
	}

	if _, err := h.expSvc.Conclude(ctx, exp.ID, false); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("an inconclusive trial must not close without force, got %v", err)
	}

	h.clock.Advance(48 * time.Hour)
	h.seedFinishedRuns(agent, 20, 19, 17, 0.30, exp, domain.VariantChampion)
	h.seedFinishedRuns(agent, 20, 19, 17, 0.04, exp, domain.VariantChallenger)
	done, err := h.expSvc.Conclude(ctx, exp.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := h.agents.Get(ctx, agent.ID)
	if done.Status != domain.ExperimentPromoted || updated.Model != grok {
		t.Fatalf("status=%s model=%s outcome=%q", done.Status, updated.Model, done.Outcome)
	}
}

func TestEvaluateCrewRemovesAgentsThatDoNotDeliver(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	good := h.hireAgent("proof-checker", "lookup_order")
	bad := h.hireAgent("reorder-nudger", "lookup_order")
	h.seedFinishedRuns(good, 30, 29, 26, 0.05, nil, domain.VariantChampion)
	h.seedFinishedRuns(bad, 30, 29, 8, 0.05, nil, domain.VariantChampion)

	decisions := func() map[domain.ID]domain.Decision {
		evals, err := h.evalSvc.EvaluateCrew(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[domain.ID]domain.Decision{}
		for _, e := range evals {
			out[e.AgentID] = e.Decision
		}
		return out
	}

	first := decisions()
	if first[good.ID] != domain.DecisionKeep || first[bad.ID] != domain.DecisionProbation {
		t.Fatalf("first evaluation: %v", first)
	}
	second := decisions()
	if second[bad.ID] != domain.DecisionRetire {
		t.Fatalf("second strike should retire, got %v", second[bad.ID])
	}
	retired, _ := h.agents.Get(ctx, bad.ID)
	if retired.Status != domain.AgentRetired {
		t.Fatalf("status = %s", retired.Status)
	}
	if third := decisions(); len(third) != 1 {
		t.Fatalf("retired agents are no longer evaluated: %v", third)
	}
	history, _ := h.evalSvc.History(ctx, bad.ID, 5)
	if len(history) != 2 || history[0].StatusAfter != domain.AgentRetired {
		t.Fatalf("history = %+v", history)
	}
}
