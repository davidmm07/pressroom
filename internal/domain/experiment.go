package domain

import (
	"fmt"
	"hash/fnv"
	"time"
)

// ExperimentStatus tracks a champion/challenger trial.
type ExperimentStatus string

const (
	ExperimentRunning  ExperimentStatus = "RUNNING"
	ExperimentPromoted ExperimentStatus = "PROMOTED" // challenger became the champion
	ExperimentRejected ExperimentStatus = "REJECTED" // champion kept its seat
)

func (s ExperimentStatus) Valid() bool {
	return s == ExperimentRunning || s == ExperimentPromoted || s == ExperimentRejected
}

// Experiment sends a share of an agent's runs to a challenger model so a new
// release (a Claude, GPT, Grok or open-weights update) is adopted on evidence
// from real work instead of on launch-day benchmarks.
type Experiment struct {
	ID             ID
	AgentID        ID
	Champion       ModelRef
	Challenger     ModelRef
	TrafficPercent int
	Hypothesis     string
	Status         ExperimentStatus
	Outcome        string
	CreatedAt      time.Time
	ConcludedAt    *time.Time
}

// MaxChallengerTraffic keeps most customers on the proven model during a trial.
const MaxChallengerTraffic = 50

// NewExperiment starts a trial for an agent on duty.
func NewExperiment(agent *Agent, challenger ModelRef, trafficPercent int, hypothesis string, now time.Time) (*Experiment, error) {
	var v Validator
	v.Merge("challenger", challenger.Validate())
	v.Check(challenger != agent.Model, "challenger", CodeInvalidValue, "must differ from the agent's current model")
	v.IntRange("trafficPercent", trafficPercent, 1, MaxChallengerTraffic)
	v.Text("hypothesis", hypothesis, 500)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if !agent.CanRun() {
		return nil, &TransitionError{Entity: "agent " + agent.Slug, From: string(agent.Status), To: "experiment"}
	}
	return &Experiment{
		ID: NewID(), AgentID: agent.ID, Champion: agent.Model, Challenger: challenger,
		TrafficPercent: trafficPercent, Hypothesis: hypothesis, Status: ExperimentRunning, CreatedAt: now,
	}, nil
}

// Assign picks the arm for a run. Hashing the run ID makes the split
// deterministic, so a retried run never switches models halfway through.
func (e *Experiment) Assign(runID ID) Variant {
	h := fnv.New32a()
	_, _ = h.Write([]byte(runID))
	if int(h.Sum32()%100) < e.TrafficPercent {
		return VariantChallenger
	}
	return VariantChampion
}

// ModelFor returns the model serving an arm.
func (e *Experiment) ModelFor(v Variant) ModelRef {
	if v == VariantChallenger {
		return e.Challenger
	}
	return e.Champion
}

// ComparisonPolicy decides whether a challenger earned promotion.
type ComparisonPolicy struct {
	MinRunsPerArm    int
	QualityTolerance float64 // how much success/acceptance the challenger may lose
}

// DefaultComparisonPolicy requires a meaningful sample and allows a two point
// quality dip when the challenger is cheaper enough to deliver more net value.
func DefaultComparisonPolicy() ComparisonPolicy {
	return ComparisonPolicy{MinRunsPerArm: 10, QualityTolerance: 0.02}
}

// Compare returns promote=true when the challenger is at least as good and
// delivers more net value per run. conclusive is false while either arm has
// too few runs to judge.
func (p ComparisonPolicy) Compare(champion, challenger Scorecard) (promote, conclusive bool, summary string) {
	if champion.Finished < p.MinRunsPerArm || challenger.Finished < p.MinRunsPerArm {
		return false, false, fmt.Sprintf("need %d finished runs per arm (champion %d, challenger %d)",
			p.MinRunsPerArm, champion.Finished, challenger.Finished)
	}
	if challenger.SuccessRate() < champion.SuccessRate()-p.QualityTolerance {
		return false, true, fmt.Sprintf("challenger success rate %.0f%% trails champion %.0f%%",
			challenger.SuccessRate()*100, champion.SuccessRate()*100)
	}
	if ca, okA := challenger.AcceptanceRate(); okA {
		if cb, okB := champion.AcceptanceRate(); okB && ca < cb-p.QualityTolerance {
			return false, true, fmt.Sprintf("challenger acceptance %.0f%% trails champion %.0f%%", ca*100, cb*100)
		}
	}
	if challenger.NetValuePerRun() <= champion.NetValuePerRun() {
		return false, true, fmt.Sprintf("challenger nets %s USD per run versus %s USD for the champion",
			challenger.NetValuePerRun(), champion.NetValuePerRun())
	}
	return true, true, fmt.Sprintf("challenger matches quality and nets %s USD per run versus %s USD",
		challenger.NetValuePerRun(), champion.NetValuePerRun())
}

// Conclude closes the experiment with the given outcome.
func (e *Experiment) Conclude(promote bool, summary string, now time.Time) error {
	if e.Status != ExperimentRunning {
		return &TransitionError{Entity: "experiment", From: string(e.Status), To: "concluded"}
	}
	e.Status = ExperimentRejected
	if promote {
		e.Status = ExperimentPromoted
	}
	e.Outcome, e.ConcludedAt = summary, &now
	return nil
}
