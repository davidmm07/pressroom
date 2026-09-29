package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/davidmm07/pressroom/internal/domain"
)

func activeAgent(t *testing.T) *domain.Agent {
	t.Helper()
	a, err := domain.NewAgent(validSpec(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Activate(now); err != nil {
		t.Fatal(err)
	}
	return a
}

func newRun(t *testing.T) *domain.Run {
	t.Helper()
	a := activeAgent(t)
	r, err := domain.NewRun(a, domain.NewRunParams{
		Input: json.RawMessage(`{"orderId":"ORD-1042"}`), Trigger: domain.TriggerManual, Model: a.Model,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewRunValidatesInput(t *testing.T) {
	a := activeAgent(t)
	for _, input := range []string{``, `[]`, `"text"`, `{"broken":`} {
		_, err := domain.NewRun(a, domain.NewRunParams{Input: json.RawMessage(input), Trigger: domain.TriggerManual, Model: a.Model}, now)
		var ve *domain.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "input" {
			t.Errorf("input %q: want input validation error, got %v", input, err)
		}
	}
}

func TestNewRunRequiresAnAgentOnDuty(t *testing.T) {
	a, _ := domain.NewAgent(validSpec(), now) // still a draft
	_, err := domain.NewRun(a, domain.NewRunParams{Input: json.RawMessage(`{}`), Trigger: domain.TriggerManual, Model: a.Model}, now)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
}

func TestRunToolLoopBatchesResultsIntoOneTurn(t *testing.T) {
	r := newRun(t)
	if err := r.Start(now); err != nil {
		t.Fatal(err)
	}
	calls := []domain.ToolCall{
		{ID: "c1", Name: "lookup_order", Arguments: json.RawMessage(`{"orderId":"ORD-1042"}`)},
		{ID: "c2", Name: "track_shipment", Arguments: json.RawMessage(`{"trackingNumber":"1Z"}`)},
	}
	usage := domain.Usage{InputTokens: 1000, OutputTokens: 200}
	if err := r.RecordTurn(domain.Message{ToolCalls: calls}, usage, 1500, 0, now); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordTurn(domain.Message{Text: "too early"}, usage, 0, 0, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("a model turn with unresolved calls must be refused, got %v", err)
	}

	_ = r.ResolveCall(domain.ToolResult{CallID: "c1", Content: `{"status":"SHIPPED"}`}, calls[0].Arguments, 0, now)
	if got := len(r.Transcript); got != 2 {
		t.Fatalf("results must wait for the whole turn, transcript has %d messages", got)
	}
	_ = r.ResolveCall(domain.ToolResult{CallID: "c2", Content: `{"eta":"tomorrow"}`}, calls[1].Arguments, 0, now)

	last := r.Transcript[len(r.Transcript)-1]
	if last.Role != domain.RoleUser || len(last.ToolResults) != 2 || r.Pending != nil {
		t.Fatalf("want one user turn with 2 results and nothing pending, got %+v pending=%v", last, r.Pending)
	}
	if r.Cost != 1500 || r.Usage.InputTokens != 1000 || r.Turns != 1 {
		t.Fatalf("usage not accumulated: %+v cost=%d turns=%d", r.Usage, r.Cost, r.Turns)
	}
}

func TestRunApprovalFlow(t *testing.T) {
	refund := domain.ToolCall{ID: "c1", Name: "issue_refund", Arguments: json.RawMessage(`{"amountCents":1200}`)}

	t.Run("approve", func(t *testing.T) {
		r := newRun(t)
		_ = r.Start(now)
		_ = r.RecordTurn(domain.Message{ToolCalls: []domain.ToolCall{refund}}, domain.Usage{}, 0, 0, now)
		if err := r.AwaitApproval(refund, now); err != nil {
			t.Fatal(err)
		}
		if call, ok := r.AwaitingCall(); !ok || call.ID != "c1" {
			t.Fatalf("AwaitingCall = %v, %v", call, ok)
		}
		if err := r.Approve("cx-lead@example.com", now); err != nil {
			t.Fatal(err)
		}
		if r.Status != domain.RunQueued || !r.IsApproved("c1") {
			t.Fatalf("status=%s approved=%v", r.Status, r.IsApproved("c1"))
		}
	})

	t.Run("deny sends the refusal to the model", func(t *testing.T) {
		r := newRun(t)
		_ = r.Start(now)
		_ = r.RecordTurn(domain.Message{ToolCalls: []domain.ToolCall{refund}}, domain.Usage{}, 0, 0, now)
		_ = r.AwaitApproval(refund, now)

		var ve *domain.ValidationError
		if err := r.Deny("cx-lead@example.com", " ", now); !errors.As(err, &ve) {
			t.Fatalf("a denial needs a reason, got %v", err)
		}
		if err := r.Deny("cx-lead@example.com", "order already credited", now); err != nil {
			t.Fatal(err)
		}
		last := r.Transcript[len(r.Transcript)-1]
		if len(last.ToolResults) != 1 || !last.ToolResults[0].IsError ||
			!strings.Contains(last.ToolResults[0].Content, "order already credited") {
			t.Fatalf("denial not relayed to the model: %+v", last)
		}
		if r.Status != domain.RunQueued {
			t.Fatalf("status = %s, want QUEUED", r.Status)
		}
	})
}

func TestRunBudget(t *testing.T) {
	r := newRun(t)
	b := domain.Budget{MaxSteps: 2, MaxCost: 1000}
	if _, ok := r.CheckBudget(b); !ok {
		t.Fatal("a fresh run is within budget")
	}
	r.Cost = 1000
	if reason, ok := r.CheckBudget(b); ok || !strings.Contains(reason, "cost budget") {
		t.Fatalf("want cost budget exhausted, got %q %v", reason, ok)
	}
	r.Cost, r.Turns = 0, 2
	if reason, ok := r.CheckBudget(b); ok || !strings.Contains(reason, "step budget") {
		t.Fatalf("want step budget exhausted, got %q %v", reason, ok)
	}
}

func TestReviewRules(t *testing.T) {
	r := newRun(t)
	if err := r.SubmitReview(domain.VerdictAccepted, "lead@example.com", "", now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("only succeeded runs can be reviewed, got %v", err)
	}
	_ = r.Start(now)
	_ = r.RecordTurn(domain.Message{Text: "done"}, domain.Usage{}, 0, 0, now)
	_ = r.Succeed("done", now)
	if err := r.SubmitReview(domain.VerdictAccepted, "lead@example.com", "", now); err != nil {
		t.Fatal(err)
	}
	if err := r.SubmitReview(domain.VerdictRejected, "lead@example.com", "", now); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second review must conflict, got %v", err)
	}
}
