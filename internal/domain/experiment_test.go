package domain_test

import (
	"errors"
	"testing"

	"github.com/davidmm07/pressroom/internal/domain"
)

var grok = domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"}

func TestNewExperimentValidation(t *testing.T) {
	a := activeAgent(t)
	_, err := domain.NewExperiment(a, a.Model, 80, "", now)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 3 {
		t.Fatalf("want 3 field errors (same model, traffic, hypothesis), got %v", err)
	}
}

func TestExperimentAssignmentIsStickyAndProportional(t *testing.T) {
	a := activeAgent(t)
	e, err := domain.NewExperiment(a, grok, 20, "Grok is cheaper for triage", now)
	if err != nil {
		t.Fatal(err)
	}
	challengers := 0
	for i := 0; i < 5000; i++ {
		id := domain.NewID()
		v := e.Assign(id)
		if v != e.Assign(id) {
			t.Fatal("assignment must be deterministic per run")
		}
		if v == domain.VariantChallenger {
			challengers++
		}
	}
	if share := float64(challengers) / 5000; share < 0.17 || share > 0.23 {
		t.Fatalf("challenger share %.3f is far from 20%%", share)
	}
}

func TestComparisonPolicy(t *testing.T) {
	p := domain.DefaultComparisonPolicy()
	champion := scorecard(40, 0.95, 0.9, 0.30)

	tests := []struct {
		name       string
		challenger domain.Scorecard
		promote    bool
		conclusive bool
	}{
		{"cheaper at equal quality wins", scorecard(20, 0.95, 0.9, 0.05), true, true},
		{"cheaper but sloppier loses", scorecard(20, 0.95, 0.6, 0.05), false, true},
		{"less reliable loses", scorecard(20, 0.8, 0.9, 0.05), false, true},
		{"too few runs is inconclusive", scorecard(4, 1, 1, 0.01), false, false},
		{"pricier at equal quality loses", scorecard(20, 0.95, 0.9, 0.60), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promote, conclusive, summary := p.Compare(champion, tt.challenger)
			if promote != tt.promote || conclusive != tt.conclusive {
				t.Fatalf("promote=%v conclusive=%v (%s)", promote, conclusive, summary)
			}
		})
	}
}

func TestOpportunityScoring(t *testing.T) {
	o, err := domain.NewOpportunity(domain.OpportunityInput{
		Title: "Answer where-is-my-order tickets", Problem: "Half of CX tickets ask for tracking.",
		Department: domain.DepartmentCustomerExperience, SubmittedBy: "cx-lead@example.com",
		WeeklyVolume: 600, MinutesPerTask: 4, DataSensitivity: domain.LevelMedium, ErrorCost: domain.LevelLow,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if o.HoursPerWeek() != 40 || o.Score() != 34 { // 40h * (1 - 0.15)
		t.Fatalf("hours=%v score=%v", o.HoursPerWeek(), o.Score())
	}
	if err := o.MoveTo(domain.OpportunityShipped, "", now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("cannot ship before approval, got %v", err)
	}
	_ = o.MoveTo(domain.OpportunityApproved, "", now)
	var ve *domain.ValidationError
	if err := o.MoveTo(domain.OpportunityShipped, "", now); !errors.As(err, &ve) {
		t.Fatalf("shipping needs an agent, got %v", err)
	}
}
