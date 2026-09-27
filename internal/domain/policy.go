package domain

import (
	"fmt"
	"time"
)

// Spec is a business rule over a scorecard (Specification pattern). Rules
// are small, individually testable values that compose with AllOf, so the
// retirement policy is assembled from parts instead of one growing if-chain
// (Single Responsibility and Open/Closed Principles).
type Spec interface {
	// Check reports whether the scorecard satisfies the rule and, when it
	// does not, a sentence explaining why.
	Check(Scorecard) (ok bool, reason string)
}

// SpecFunc adapts a function to the Spec interface.
type SpecFunc func(Scorecard) (bool, string)

func (f SpecFunc) Check(s Scorecard) (bool, string) { return f(s) }

// MinSuccessRate requires the share of runs that finish without error.
func MinSuccessRate(min float64) Spec {
	return SpecFunc(func(s Scorecard) (bool, string) {
		if rate := s.SuccessRate(); rate < min {
			return false, fmt.Sprintf("success rate %.0f%% is below the %.0f%% target", rate*100, min*100)
		}
		return true, ""
	})
}

// MinAcceptanceRate requires reviewers to keep enough of the output. It only
// judges once minReviews verdicts exist; before that the rule abstains.
func MinAcceptanceRate(min float64, minReviews int) Spec {
	return SpecFunc(func(s Scorecard) (bool, string) {
		rate, ok := s.AcceptanceRate()
		if !ok || s.Reviewed() < minReviews {
			return true, ""
		}
		if rate < min {
			return false, fmt.Sprintf("reviewers accepted %.0f%% of output, below the %.0f%% target", rate*100, min*100)
		}
		return true, ""
	})
}

// PositiveNetValue requires the agent to be worth more than it costs.
func PositiveNetValue() Spec {
	return SpecFunc(func(s Scorecard) (bool, string) {
		if net := s.NetValue(); net <= 0 {
			return false, fmt.Sprintf("net value is %s USD: model spend exceeds the value of the time saved", net)
		}
		return true, ""
	})
}

// MaxCostPerRun caps average spend per run.
func MaxCostPerRun(max Micros) Spec {
	return SpecFunc(func(s Scorecard) (bool, string) {
		if c := s.CostPerRun(); c > max {
			return false, fmt.Sprintf("average run costs %s USD, above the %s USD ceiling", c, max)
		}
		return true, ""
	})
}

// AllOf combines rules (Composite). Every failing rule contributes a reason,
// so the evaluation tells the lead everything that is wrong at once.
func AllOf(specs ...Spec) Spec {
	return SpecFunc(func(s Scorecard) (bool, string) {
		ok, reasons := true, ""
		for _, spec := range specs {
			if pass, why := spec.Check(s); !pass {
				ok = false
				if reasons != "" {
					reasons += "; "
				}
				reasons += why
			}
		}
		return ok, reasons
	})
}

// Decision is the outcome of evaluating one agent.
type Decision string

const (
	DecisionKeep             Decision = "KEEP"
	DecisionProbation        Decision = "PROBATION"
	DecisionRetire           Decision = "RETIRE"
	DecisionReinstate        Decision = "REINSTATE"
	DecisionInsufficientData Decision = "INSUFFICIENT_DATA"
)

// RetirementPolicy decides which agents keep their place on the crew. Agents
// get one strike: missing targets once means probation, missing them again
// while on probation means retirement. Recovering on probation reinstates.
type RetirementPolicy struct {
	MinRuns int  // evaluations with fewer finished runs abstain
	Keep    Spec // what "delivering" means
}

// DefaultRetirementPolicy mirrors the targets in docs/agents.md.
func DefaultRetirementPolicy() RetirementPolicy {
	return RetirementPolicy{
		MinRuns: 20,
		Keep: AllOf(
			MinSuccessRate(0.85),
			MinAcceptanceRate(0.70, 10),
			PositiveNetValue(),
		),
	}
}

// Decide applies the policy to an agent's current status and scorecard.
func (p RetirementPolicy) Decide(status AgentStatus, s Scorecard) (Decision, string) {
	if status != AgentActive && status != AgentProbation {
		return DecisionKeep, "agent is not on duty"
	}
	if s.Finished < p.MinRuns {
		return DecisionInsufficientData, fmt.Sprintf("%d finished runs, %d needed for a verdict", s.Finished, p.MinRuns)
	}
	ok, why := p.Keep.Check(s)
	switch {
	case ok && status == AgentProbation:
		return DecisionReinstate, "back on target"
	case ok:
		return DecisionKeep, "meeting targets"
	case status == AgentProbation:
		return DecisionRetire, why
	default:
		return DecisionProbation, why
	}
}

// Apply performs the lifecycle change a decision implies.
func (d Decision) Apply(a *Agent, now time.Time) error {
	switch d {
	case DecisionProbation:
		return a.PutOnProbation(now)
	case DecisionRetire:
		return a.Retire(now)
	case DecisionReinstate:
		return a.Reinstate(now)
	default:
		return nil
	}
}

// Evaluation records one policy decision so the crew's history explains why
// each agent was kept, warned or let go.
type Evaluation struct {
	ID           ID
	AgentID      ID
	Decision     Decision
	Reason       string
	Scorecard    Scorecard
	StatusBefore AgentStatus
	StatusAfter  AgentStatus
	CreatedAt    time.Time
}
