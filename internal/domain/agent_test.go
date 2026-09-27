package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
)

var now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func validSpec() domain.AgentSpec {
	return domain.AgentSpec{
		Slug:               "proof-checker",
		Name:               "Proof Checker",
		Department:         domain.DepartmentPrepress,
		Owner:              "prepress-lead@example.com",
		Instructions:       "Check uploaded artwork for print readiness.",
		Model:              domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"},
		Tools:              []string{"inspect_artwork", "upscale_image"},
		Triggers:           []string{"artwork.uploaded"},
		Budget:             domain.Budget{MaxSteps: 8, MaxCost: domain.MicrosFromUSD(0.5)},
		MinutesSavedPerRun: 6,
	}
}

func TestNewAgentStartsAsDraft(t *testing.T) {
	a, err := domain.NewAgent(validSpec(), now)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if a.Status != domain.AgentDraft || a.Version != 1 || a.ID == "" {
		t.Fatalf("unexpected agent: status=%s version=%d id=%q", a.Status, a.Version, a.ID)
	}
	if a.CanRun() {
		t.Fatal("a draft agent must not run")
	}
}

func TestAgentSpecValidationReportsEveryField(t *testing.T) {
	spec := validSpec()
	spec.Slug = "Bad Slug"
	spec.Owner = "not-an-email"
	spec.Tools = []string{"inspect_artwork", "inspect_artwork", "Has.Dots"}
	spec.Budget = domain.Budget{MaxSteps: 0, MaxCost: 0}
	spec.Model = domain.ModelRef{Provider: "ACME", Name: "x"}

	_, err := domain.NewAgent(spec, now)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	want := map[string]string{
		"slug":            domain.CodeInvalidFmt,
		"owner":           domain.CodeInvalidFmt,
		"tools[1]":        domain.CodeInvalidValue,
		"tools[2]":        domain.CodeInvalidFmt,
		"budget.maxSteps": domain.CodeOutOfRange,
		"budget.maxCost":  domain.CodeOutOfRange,
		"model.provider":  domain.CodeInvalidValue,
	}
	got := map[string]string{}
	for _, f := range ve.Fields {
		got[f.Field] = f.Code
	}
	for field, code := range want {
		if got[field] != code {
			t.Errorf("field %s: want code %s, got %q (all: %v)", field, code, got[field], got)
		}
	}
}

func TestAgentLifecycle(t *testing.T) {
	tests := []struct {
		name    string
		steps   []func(*domain.Agent) error
		want    domain.AgentStatus
		wantErr bool
	}{
		{"activate draft", []func(*domain.Agent) error{activate}, domain.AgentActive, false},
		{"probation then reinstate", []func(*domain.Agent) error{activate, probation, reinstate}, domain.AgentActive, false},
		{"probation then retire", []func(*domain.Agent) error{activate, probation, retire}, domain.AgentRetired, false},
		{"cannot put a draft on probation", []func(*domain.Agent) error{probation}, domain.AgentDraft, true},
		{"cannot reinstate an active agent", []func(*domain.Agent) error{activate, reinstate}, domain.AgentActive, true},
		{"retired is terminal", []func(*domain.Agent) error{retire, activate}, domain.AgentRetired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := domain.NewAgent(validSpec(), now)
			var err error
			for _, step := range tt.steps {
				if err = step(a); err != nil {
					break
				}
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
				t.Fatalf("want ErrInvalidTransition, got %v", err)
			}
			if a.Status != tt.want {
				t.Fatalf("status = %s, want %s", a.Status, tt.want)
			}
		})
	}
}

func TestRetiredAgentIsReadOnly(t *testing.T) {
	a, _ := domain.NewAgent(validSpec(), now)
	_ = a.Retire(now)
	if err := a.Reconfigure(validSpec(), now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		in      string
		want    domain.ModelRef
		wantErr bool
	}{
		{"anthropic/claude-opus-5", domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"}, false},
		{"open_source/qwen/qwen3-32b", domain.ModelRef{Provider: domain.ProviderOpenSource, Name: "qwen/qwen3-32b"}, false},
		{"xai/grok-4", domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"}, false},
		{"claude-opus-5", domain.ModelRef{}, true},
		{"acme/model", domain.ModelRef{Provider: "ACME", Name: "model"}, true},
	}
	for _, tt := range tests {
		got, err := domain.ParseModelRef(tt.in)
		if (err != nil) != tt.wantErr || (!tt.wantErr && got != tt.want) {
			t.Errorf("ParseModelRef(%q) = %+v, %v", tt.in, got, err)
		}
		if !tt.wantErr && got.String() != tt.in {
			t.Errorf("String() = %q, want %q", got.String(), tt.in)
		}
	}
}

func TestNewIDIsTimeOrderedUUIDv7(t *testing.T) {
	a, b := domain.NewID(), domain.NewID()
	if _, err := domain.ParseID(string(a)); err != nil {
		t.Fatalf("NewID produced an invalid UUID %q: %v", a, err)
	}
	if string(a)[14] != '7' {
		t.Fatalf("want version 7, got %q", a)
	}
	if a[:8] > b[:8] {
		t.Fatalf("IDs are not time ordered: %s then %s", a, b)
	}
}

func activate(a *domain.Agent) error  { return a.Activate(now) }
func probation(a *domain.Agent) error { return a.PutOnProbation(now) }
func reinstate(a *domain.Agent) error { return a.Reinstate(now) }
func retire(a *domain.Agent) error    { return a.Retire(now) }
