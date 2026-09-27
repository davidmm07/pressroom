package domain_test

import (
	"strings"
	"testing"

	"github.com/davidmm07/pressroom/internal/domain"
)

// scorecard builds a card for n finished runs at the given success and
// acceptance rates, each run saving 10 minutes of $40/h work.
func scorecard(n int, success, accepted float64, costPerRun float64) domain.Scorecard {
	succeeded := int(float64(n) * success)
	reviewed := succeeded
	acc := int(float64(reviewed) * accepted)
	return domain.Scorecard{
		RunStats: domain.RunStats{
			Finished: n, Succeeded: succeeded, Failed: n - succeeded,
			Accepted: acc, Rejected: reviewed - acc,
			Cost: domain.MicrosFromUSD(costPerRun * float64(n)),
		},
		WindowDays: 30, MinutesSavedPerRun: 10, HourlyRate: domain.MicrosFromUSD(40),
	}
}

func TestScorecardMath(t *testing.T) {
	s := domain.Scorecard{
		RunStats: domain.RunStats{
			Finished: 10, Succeeded: 8, Failed: 2, Accepted: 6, Edited: 2,
			Cost: domain.MicrosFromUSD(2),
		},
		MinutesSavedPerRun: 15, HourlyRate: domain.MicrosFromUSD(40),
	}
	rate, ok := s.AcceptanceRate()
	if !ok || rate != 0.875 { // (6 + 0.5*2) / 8
		t.Fatalf("acceptance = %v, %v", rate, ok)
	}
	if got := s.HoursSaved(); got != 1.75 { // 8 * 0.875 * 15 / 60
		t.Fatalf("hours saved = %v", got)
	}
	if got := s.NetValue(); got != domain.MicrosFromUSD(68) { // 1.75h * $40 - $2
		t.Fatalf("net value = %s", got)
	}
	if got := s.CostPerRun(); got != domain.MicrosFromUSD(0.2) {
		t.Fatalf("cost per run = %s", got)
	}
}

func TestUnreviewedWorkEarnsHalfCredit(t *testing.T) {
	s := domain.Scorecard{RunStats: domain.RunStats{Finished: 4, Succeeded: 4}, MinutesSavedPerRun: 30}
	if _, ok := s.AcceptanceRate(); ok {
		t.Fatal("no reviews means no acceptance rate")
	}
	if got := s.HoursSaved(); got != 1 { // 4 * 0.5 * 30 / 60
		t.Fatalf("hours saved = %v", got)
	}
}

func TestRetirementPolicy(t *testing.T) {
	p := domain.DefaultRetirementPolicy()
	healthy := scorecard(40, 0.95, 0.9, 0.05)
	sloppy := scorecard(40, 0.95, 0.4, 0.05)
	pricey := scorecard(40, 0.95, 0.9, 20)

	tests := []struct {
		name   string
		status domain.AgentStatus
		card   domain.Scorecard
		want   domain.Decision
		reason string
	}{
		{"healthy agent stays", domain.AgentActive, healthy, domain.DecisionKeep, ""},
		{"too few runs abstains", domain.AgentActive, scorecard(5, 0, 0, 1), domain.DecisionInsufficientData, "needed"},
		{"low acceptance means probation", domain.AgentActive, sloppy, domain.DecisionProbation, "reviewers accepted"},
		{"second strike retires", domain.AgentProbation, sloppy, domain.DecisionRetire, "reviewers accepted"},
		{"recovery reinstates", domain.AgentProbation, healthy, domain.DecisionReinstate, ""},
		{"costing more than it saves", domain.AgentActive, pricey, domain.DecisionProbation, "net value"},
		{"flaky agent", domain.AgentActive, scorecard(40, 0.6, 0.9, 0.05), domain.DecisionProbation, "success rate"},
		{"drafts are ignored", domain.AgentDraft, sloppy, domain.DecisionKeep, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := p.Decide(tt.status, tt.card)
			if got != tt.want {
				t.Fatalf("decision = %s (%s), want %s", got, reason, tt.want)
			}
			if tt.reason != "" && !strings.Contains(reason, tt.reason) {
				t.Fatalf("reason %q does not mention %q", reason, tt.reason)
			}
		})
	}
}

func TestAllOfCollectsEveryReason(t *testing.T) {
	spec := domain.AllOf(domain.MinSuccessRate(0.9), domain.MaxCostPerRun(domain.MicrosFromUSD(0.1)))
	ok, reason := spec.Check(scorecard(20, 0.5, 1, 1))
	if ok || !strings.Contains(reason, "success rate") || !strings.Contains(reason, "average run costs") {
		t.Fatalf("got ok=%v reason=%q", ok, reason)
	}
}
