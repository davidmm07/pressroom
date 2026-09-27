package domain

import (
	"fmt"
	"regexp"
	"slices"
	"time"
)

// AgentStatus is a stage in an agent's employment lifecycle.
type AgentStatus string

const (
	AgentDraft     AgentStatus = "DRAFT"     // configured, not yet allowed to run
	AgentActive    AgentStatus = "ACTIVE"    // on the crew
	AgentProbation AgentStatus = "PROBATION" // failed one evaluation, still running
	AgentRetired   AgentStatus = "RETIRED"   // removed from the crew; terminal
)

// agentTransitions is a table-driven state machine (State pattern without a
// class per state): adding a status means adding a row, not editing every
// method that changes status (Open/Closed Principle).
var agentTransitions = map[AgentStatus][]AgentStatus{
	AgentDraft:     {AgentActive, AgentRetired},
	AgentActive:    {AgentProbation, AgentRetired},
	AgentProbation: {AgentActive, AgentRetired},
	AgentRetired:   {},
}

func (s AgentStatus) Valid() bool {
	_, ok := agentTransitions[s]
	return ok
}

// CanTransitionTo reports whether the lifecycle allows moving to next.
func (s AgentStatus) CanTransitionTo(next AgentStatus) bool {
	return slices.Contains(agentTransitions[s], next)
}

// Budget caps what a single run may spend. Autonomy without a budget is how
// an agent loops on a tool error all night.
type Budget struct {
	MaxSteps int
	MaxCost  Micros
}

// Validation limits. Exported so adapters can publish them (GraphQL
// descriptions, client-side form hints) from a single source of truth.
const (
	MaxAgentSteps         = 50
	MaxAgentCostUSD       = 25.0
	MaxInstructionsLength = 8000
	MaxMinutesSavedPerRun = 480.0
)

var (
	slugPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{2,39}$`)
	triggerPattern = regexp.MustCompile(`^[a-z]+(\.[a-z_]+)+$`)
)

// Validate checks a budget in isolation.
func (b Budget) Validate() error {
	var v Validator
	v.IntRange("maxSteps", b.MaxSteps, 1, MaxAgentSteps)
	v.Check(b.MaxCost > 0 && b.MaxCost <= MicrosFromUSD(MaxAgentCostUSD),
		"maxCost", CodeOutOfRange, fmt.Sprintf("must be greater than 0 and at most %.2f USD", MaxAgentCostUSD))
	return v.Err()
}

// Agent is an autonomous worker on the crew. It is the aggregate root for its
// configuration and lifecycle; runs reference it by ID only.
type Agent struct {
	ID          ID
	Slug        string
	Name        string
	Description string
	Department  Department
	Owner       string // department lead accountable for the agent's results
	// Instructions is the system prompt: the agent's job description.
	Instructions string
	Model        ModelRef
	Tools        []string // allow-list; the agent can never call anything else
	Triggers     []string // inbound event types that start a run, e.g. artwork.uploaded
	Budget       Budget
	// MinutesSavedPerRun is the lead's estimate of human effort one accepted
	// run replaces. It turns run counts into hours and dollars on the scorecard.
	MinutesSavedPerRun float64
	Status             AgentStatus
	Version            int // optimistic concurrency token
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AgentSpec is the editable part of an agent, shared by create and update.
type AgentSpec struct {
	Slug               string
	Name               string
	Description        string
	Department         Department
	Owner              string
	Instructions       string
	Model              ModelRef
	Tools              []string
	Triggers           []string
	Budget             Budget
	MinutesSavedPerRun float64
}

// Validate checks every field and reports all problems at once.
func (s AgentSpec) Validate() error {
	var v Validator
	v.Pattern("slug", s.Slug, slugPattern, "must be 3-40 lowercase letters, digits or dashes, starting with a letter")
	v.Text("name", s.Name, 80)
	v.OptionalText("description", s.Description, 500)
	v.Enum("department", s.Department.Valid(), string(s.Department))
	v.Email("owner", s.Owner)
	v.Text("instructions", s.Instructions, MaxInstructionsLength)
	v.Merge("model", s.Model.Validate())
	v.Merge("budget", s.Budget.Validate())
	v.FloatRange("minutesSavedPerRun", s.MinutesSavedPerRun, 0, MaxMinutesSavedPerRun)

	v.Check(len(s.Tools) > 0, "tools", CodeRequired, "an agent needs at least one tool")
	v.Check(len(s.Tools) <= 20, "tools", CodeOutOfRange, "an agent may use at most 20 tools")
	seen := map[string]bool{}
	for i, name := range s.Tools {
		field := fmt.Sprintf("tools[%d]", i)
		v.Check(ValidToolName(name), field, CodeInvalidFmt, "must be a snake_case tool name")
		v.Check(!seen[name], field, CodeInvalidValue, fmt.Sprintf("%q is listed twice", name))
		seen[name] = true
	}
	for i, t := range s.Triggers {
		v.Check(triggerPattern.MatchString(t), fmt.Sprintf("triggers[%d]", i), CodeInvalidFmt,
			"must be a dotted event type such as ticket.created")
	}
	return v.Err()
}

// NewAgent creates a draft agent. It must be activated before it can run,
// which gives the owning lead a chance to review the configuration.
func NewAgent(spec AgentSpec, now time.Time) (*Agent, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	a := &Agent{ID: NewID(), Status: AgentDraft, Version: 1, CreatedAt: now}
	a.apply(spec, now)
	return a, nil
}

func (a *Agent) apply(s AgentSpec, now time.Time) {
	a.Slug = s.Slug
	a.Name = s.Name
	a.Description = s.Description
	a.Department = s.Department
	a.Owner = s.Owner
	a.Instructions = s.Instructions
	a.Model = s.Model
	a.Tools = slices.Clone(s.Tools)
	a.Triggers = slices.Clone(s.Triggers)
	a.Budget = s.Budget
	a.MinutesSavedPerRun = s.MinutesSavedPerRun
	a.UpdatedAt = now
}

// Spec returns the agent's editable fields, handy for partial updates.
func (a *Agent) Spec() AgentSpec {
	return AgentSpec{
		Slug: a.Slug, Name: a.Name, Description: a.Description, Department: a.Department,
		Owner: a.Owner, Instructions: a.Instructions, Model: a.Model,
		Tools: slices.Clone(a.Tools), Triggers: slices.Clone(a.Triggers),
		Budget: a.Budget, MinutesSavedPerRun: a.MinutesSavedPerRun,
	}
}

// Reconfigure replaces the editable fields. Retired agents are read-only so
// their history keeps describing what actually ran.
func (a *Agent) Reconfigure(spec AgentSpec, now time.Time) error {
	if a.Status == AgentRetired {
		return &TransitionError{Entity: "agent", From: string(a.Status), To: "reconfigured"}
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	a.apply(spec, now)
	return nil
}

// PromoteModel replaces the champion model, used when an experiment's
// challenger wins.
func (a *Agent) PromoteModel(m ModelRef, now time.Time) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if a.Status == AgentRetired {
		return &TransitionError{Entity: "agent", From: string(a.Status), To: "model promotion"}
	}
	a.Model = m
	a.UpdatedAt = now
	return nil
}

func (a *Agent) transition(to AgentStatus, now time.Time) error {
	if !a.Status.CanTransitionTo(to) {
		return &TransitionError{Entity: "agent", From: string(a.Status), To: string(to)}
	}
	a.Status = to
	a.UpdatedAt = now
	return nil
}

// Activate puts a draft agent to work.
func (a *Agent) Activate(now time.Time) error { return a.transition(AgentActive, now) }

// PutOnProbation flags an active agent that missed its targets once.
func (a *Agent) PutOnProbation(now time.Time) error { return a.transition(AgentProbation, now) }

// Reinstate returns an agent on probation to full duty after it recovers.
func (a *Agent) Reinstate(now time.Time) error {
	if a.Status != AgentProbation {
		return &TransitionError{Entity: "agent", From: string(a.Status), To: string(AgentActive)}
	}
	return a.transition(AgentActive, now)
}

// Retire removes the agent from the crew for good.
func (a *Agent) Retire(now time.Time) error { return a.transition(AgentRetired, now) }

// CanRun reports whether new runs may start. Agents on probation keep
// working so the next evaluation has fresh evidence.
func (a *Agent) CanRun() bool { return a.Status == AgentActive || a.Status == AgentProbation }

// Allows reports whether the tool is on the agent's allow-list.
func (a *Agent) Allows(tool string) bool { return slices.Contains(a.Tools, tool) }

// ListensTo reports whether an inbound event type should start a run.
func (a *Agent) ListensTo(eventType string) bool { return slices.Contains(a.Triggers, eventType) }
