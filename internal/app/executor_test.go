package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

func stepKinds(r *domain.Run) []domain.StepKind {
	out := make([]domain.StepKind, len(r.Steps))
	for i, s := range r.Steps {
		out[i] = s.Kind
	}
	return out
}

func TestExecutorCompletesAToolLoop(t *testing.T) {
	h := newHarness(t)
	agent := h.hireAgent("support-triage", "lookup_order")
	h.model.callTool("c1", "lookup_order", `{"orderId":"ORD-1042"}`).answer("Order ORD-1042 shipped; tracking sent.")

	run := h.start(agent, `{"ticketId":"T-9","orderId":"ORD-1042"}`)
	if err := h.executor.Execute(context.Background(), run.ID); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := h.reload(run.ID)
	if got.Status != domain.RunSucceeded || got.Output != "Order ORD-1042 shipped; tracking sent." {
		t.Fatalf("status=%s output=%q reason=%q", got.Status, got.Output, got.FailureReason)
	}
	want := []domain.StepKind{domain.StepModelTurn, domain.StepToolCall, domain.StepModelTurn, domain.StepCompleted}
	if !equal(stepKinds(got), want) {
		t.Fatalf("steps = %v, want %v", stepKinds(got), want)
	}
	// 2200 input tokens at $1/M + 180 output tokens at $5/M = $0.0031
	if got.Cost != domain.MicrosFromUSD(0.0031) || got.Turns != 2 {
		t.Fatalf("cost=%s turns=%d", got.Cost, got.Turns)
	}
	if len(h.tools.infos) != 1 || h.tools.infos[0].IdempotencyKey() != string(run.ID)+":c1" {
		t.Fatalf("tool did not receive call info: %+v", h.tools.infos)
	}
	req := h.model.lastRequest()
	if !strings.Contains(req.System, "Resolve customer tickets.") || len(req.Tools) != 1 {
		t.Fatalf("request lacks instructions or tools: %+v", req)
	}
	if names := h.events.names(); !equal(names, []string{"agent.status_changed", "run.finished"}) {
		t.Fatalf("events = %v", names)
	}
}

func TestExecutorPausesForApprovalAndResumes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("refund-desk", "lookup_order", "issue_refund")
	h.model.callTool("c1", "issue_refund", `{"orderId":"ORD-1042","amountCents":1500}`).answer("Refunded $15.")

	run := h.start(agent, `{"orderId":"ORD-1042"}`)
	if err := h.executor.Execute(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	paused := h.reload(run.ID)
	if paused.Status != domain.RunAwaitingApproval || h.tools.count("issue_refund") != 0 {
		t.Fatalf("status=%s refunds=%d; the refund must wait for a human", paused.Status, h.tools.count("issue_refund"))
	}
	if !contains(h.events.names(), "run.needs_approval") {
		t.Fatalf("no approval event: %v", h.events.names())
	}

	if _, err := h.runSvc.Approve(ctx, run.ID, "cx-lead@example.com"); err != nil {
		t.Fatal(err)
	}
	if last := h.queue.enqueued[len(h.queue.enqueued)-1]; last != run.ID {
		t.Fatal("approval must re-queue the run")
	}
	if err := h.executor.Execute(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	done := h.reload(run.ID)
	if done.Status != domain.RunSucceeded || h.tools.count("issue_refund") != 1 {
		t.Fatalf("status=%s refunds=%d", done.Status, h.tools.count("issue_refund"))
	}
}

func TestExecutorRelaysADenialToTheModel(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("refund-desk", "issue_refund")
	h.model.callTool("c1", "issue_refund", `{"orderId":"ORD-1042","amountCents":99900}`).answer("Escalated to a human.")

	run := h.start(agent, `{"orderId":"ORD-1042"}`)
	_ = h.executor.Execute(ctx, run.ID)
	if _, err := h.runSvc.Deny(ctx, run.ID, "cx-lead@example.com", "refund exceeds order total"); err != nil {
		t.Fatal(err)
	}
	if err := h.executor.Execute(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if h.tools.count("issue_refund") != 0 {
		t.Fatal("a denied tool must never run")
	}
	msgs := h.model.lastRequest().Messages
	results := msgs[len(msgs)-1].ToolResults
	if len(results) != 1 || !results[0].IsError || !strings.Contains(results[0].Content, "refund exceeds order total") {
		t.Fatalf("model did not see the denial: %+v", msgs[len(msgs)-1])
	}
	if got := h.reload(run.ID); got.Status != domain.RunSucceeded {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestExecutorGuardrails(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		args     string
		contains string
	}{
		{"tool outside the allow-list", "issue_refund", `{}`, "not available to you"},
		{"unknown tool", "delete_database", `{}`, "not available to you"},
		{"arguments that fail the schema", "track_shipment", `{}`, "missing property 'trackingNumber'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			agent := h.hireAgent("support-triage", "lookup_order", "track_shipment")
			h.model.callTool("c1", tt.tool, tt.args).answer("Could not complete the task.")

			run := h.start(agent, `{}`)
			if err := h.executor.Execute(context.Background(), run.ID); err != nil {
				t.Fatal(err)
			}
			if h.tools.count("issue_refund") != 0 {
				t.Fatal("guardrail bypassed")
			}
			msgs := h.model.lastRequest().Messages
			res := msgs[len(msgs)-1].ToolResults
			if len(res) != 1 || !res[0].IsError || !strings.Contains(res[0].Content, tt.contains) {
				t.Fatalf("tool result = %+v", res)
			}
		})
	}
}

func TestExecutorRetriesTransientModelErrorsFromCheckpoint(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("support-triage", "lookup_order")
	h.model.
		callTool("c1", "lookup_order", `{"orderId":"ORD-1042"}`).
		fail(port.Transient(errors.New("529 overloaded"), 30*time.Second)).
		answer("Done.")

	run := h.start(agent, `{}`)
	err := h.executor.Execute(ctx, run.ID)
	if te, ok := port.IsTransient(err); !ok || te.RetryAfter != 30*time.Second {
		t.Fatalf("want a transient error with retry-after, got %v", err)
	}
	mid := h.reload(run.ID)
	if mid.Status != domain.RunRunning || mid.Turns != 1 || h.tools.count("lookup_order") != 1 {
		t.Fatalf("checkpoint lost: status=%s turns=%d lookups=%d", mid.Status, mid.Turns, h.tools.count("lookup_order"))
	}

	if err := h.executor.Execute(ctx, run.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := h.reload(run.ID); got.Status != domain.RunSucceeded || h.tools.count("lookup_order") != 1 {
		t.Fatalf("status=%s lookups=%d; the resumed run must not redo finished tool calls", got.Status, h.tools.count("lookup_order"))
	}
}

func TestExecutorStopsAtTheStepBudget(t *testing.T) {
	h := newHarness(t)
	agent := h.hireAgent("support-triage", "lookup_order")
	for i := 0; i < agent.Budget.MaxSteps+2; i++ {
		h.model.callTool("c", "lookup_order", `{"orderId":"ORD-1042"}`)
	}
	run := h.start(agent, `{}`)
	if err := h.executor.Execute(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	got := h.reload(run.ID)
	if got.Status != domain.RunFailed || !strings.Contains(got.FailureReason, "step budget") || got.Turns != agent.Budget.MaxSteps {
		t.Fatalf("status=%s reason=%q turns=%d", got.Status, got.FailureReason, got.Turns)
	}
}

func TestExecutorFailsOnRefusal(t *testing.T) {
	h := newHarness(t)
	agent := h.hireAgent("support-triage", "lookup_order")
	h.model.then(func(port.CompletionRequest) (*port.Completion, error) {
		return &port.Completion{StopReason: port.StopRefusal, Detail: "cyber"}, nil
	})
	run := h.start(agent, `{}`)
	_ = h.executor.Execute(context.Background(), run.ID)
	if got := h.reload(run.ID); got.Status != domain.RunFailed || !strings.Contains(got.FailureReason, "declined") {
		t.Fatalf("status=%s reason=%q", got.Status, got.FailureReason)
	}
}

func TestCancelWinsOverARunningWorker(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	agent := h.hireAgent("support-triage", "lookup_order")
	run := h.start(agent, `{}`)

	h.model.then(func(port.CompletionRequest) (*port.Completion, error) {
		// An operator cancels while the model is thinking.
		if _, err := h.runSvc.Cancel(ctx, run.ID); err != nil {
			t.Errorf("cancel: %v", err)
		}
		return &port.Completion{Message: domain.Message{Text: "done"}, StopReason: port.StopEndTurn}, nil
	})
	if err := h.executor.Execute(ctx, run.ID); err != nil {
		t.Fatalf("the worker should yield quietly, got %v", err)
	}
	if got := h.reload(run.ID); got.Status != domain.RunCancelled {
		t.Fatalf("status = %s, want CANCELLED", got.Status)
	}
}

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains[T comparable](list []T, v T) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
