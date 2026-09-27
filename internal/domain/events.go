package domain

// Event is something that happened in the domain that other parts of the
// system may react to (Observer pattern). Use cases publish events after a
// successful commit; subscribers such as the Slack notifier never block or
// fail the use case that raised them.
type Event interface {
	EventName() string
}

// RunNeedsApproval is raised when a run pauses on a sensitive tool call.
type RunNeedsApproval struct {
	RunID     ID
	AgentSlug string
	Tool      string
	Arguments string
}

func (RunNeedsApproval) EventName() string { return "run.needs_approval" }

// RunFinished is raised when a run reaches a terminal status.
type RunFinished struct {
	RunID     ID
	AgentSlug string
	Status    RunStatus
	Cost      Micros
	Reason    string
}

func (RunFinished) EventName() string { return "run.finished" }

// AgentStatusChanged is raised when an evaluation moves an agent.
type AgentStatusChanged struct {
	AgentID   ID
	AgentSlug string
	Owner     string
	From      AgentStatus
	To        AgentStatus
	Reason    string
}

func (AgentStatusChanged) EventName() string { return "agent.status_changed" }

// ExperimentConcluded is raised when a model trial ends.
type ExperimentConcluded struct {
	ExperimentID ID
	AgentSlug    string
	Promoted     bool
	Winner       ModelRef
	Summary      string
}

func (ExperimentConcluded) EventName() string { return "experiment.concluded" }
