package domain

import "time"

// RunStats is the raw tally of an agent's finished runs in a time window.
// Storage adapters compute it with an aggregate query; the business meaning
// of the numbers lives in Scorecard.
type RunStats struct {
	Finished    int // succeeded + failed; cancelled runs do not count
	Succeeded   int
	Failed      int
	Accepted    int
	Edited      int
	Rejected    int
	Cost        Micros
	AvgDuration time.Duration
}

// UnreviewedAcceptancePrior is the acceptance rate assumed before anyone has
// reviewed an agent's work. Deliberately pessimistic: an agent earns credit
// for saving time only once people confirm its output is usable.
const UnreviewedAcceptancePrior = 0.5

// Scorecard turns run statistics into the numbers a department lead cares
// about: does the agent work, do people keep its output, and is it worth
// what it costs.
type Scorecard struct {
	RunStats
	WindowDays         int
	MinutesSavedPerRun float64
	HourlyRate         Micros // loaded labor cost of the work the agent replaces
}

// Reviewed counts runs that received a human verdict.
func (s Scorecard) Reviewed() int { return s.Accepted + s.Edited + s.Rejected }

// SuccessRate is succeeded / finished, or 0 without data.
func (s Scorecard) SuccessRate() float64 {
	if s.Finished == 0 {
		return 0
	}
	return float64(s.Succeeded) / float64(s.Finished)
}

// AcceptanceRate weights edited output at half credit. ok is false until at
// least one run was reviewed.
func (s Scorecard) AcceptanceRate() (rate float64, ok bool) {
	n := s.Reviewed()
	if n == 0 {
		return 0, false
	}
	credit := float64(s.Accepted) + 0.5*float64(s.Edited)
	return credit / float64(n), true
}

func (s Scorecard) acceptanceOrPrior() float64 {
	if rate, ok := s.AcceptanceRate(); ok {
		return rate
	}
	return UnreviewedAcceptancePrior
}

// CostPerRun averages model spend over finished runs.
func (s Scorecard) CostPerRun() Micros {
	if s.Finished == 0 {
		return 0
	}
	return s.Cost / Micros(s.Finished)
}

// HoursSaved estimates human hours replaced by useful output.
func (s Scorecard) HoursSaved() float64 {
	return float64(s.Succeeded) * s.acceptanceOrPrior() * s.MinutesSavedPerRun / 60
}

// ValueDelivered prices the hours saved at the configured labor rate.
func (s Scorecard) ValueDelivered() Micros {
	return Micros(s.HoursSaved() * float64(s.HourlyRate))
}

// NetValue is value delivered minus model spend. Negative means the agent
// costs more than the work it does.
func (s Scorecard) NetValue() Micros { return s.ValueDelivered() - s.Cost }

// NetValuePerRun normalises NetValue so arms of an experiment with different
// traffic shares can be compared.
func (s Scorecard) NetValuePerRun() Micros {
	if s.Finished == 0 {
		return 0
	}
	return s.NetValue() / Micros(s.Finished)
}
