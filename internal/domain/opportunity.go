package domain

import (
	"math"
	"slices"
	"time"
)

// Level is a coarse low/medium/high rating used during intake.
type Level string

const (
	LevelLow    Level = "LOW"
	LevelMedium Level = "MEDIUM"
	LevelHigh   Level = "HIGH"
)

func (l Level) Valid() bool { return l == LevelLow || l == LevelMedium || l == LevelHigh }

func (l Level) penalty() float64 {
	switch l {
	case LevelMedium:
		return 0.15
	case LevelHigh:
		return 0.35
	default:
		return 0
	}
}

// OpportunityStatus tracks a request from intake to a shipped agent.
type OpportunityStatus string

const (
	OpportunitySubmitted OpportunityStatus = "SUBMITTED"
	OpportunityApproved  OpportunityStatus = "APPROVED"
	OpportunityShipped   OpportunityStatus = "SHIPPED"
	OpportunityDeclined  OpportunityStatus = "DECLINED"
)

var opportunityTransitions = map[OpportunityStatus][]OpportunityStatus{
	OpportunitySubmitted: {OpportunityApproved, OpportunityDeclined},
	OpportunityApproved:  {OpportunityShipped, OpportunityDeclined},
	OpportunityShipped:   {},
	OpportunityDeclined:  {},
}

func (s OpportunityStatus) Valid() bool {
	_, ok := opportunityTransitions[s]
	return ok
}

// Opportunity is a department lead's description of work an agent might
// take over. Scoring them the same way turns "where could AI help?" into a
// ranked backlog instead of whoever asks loudest.
type Opportunity struct {
	ID              ID
	Title           string
	Problem         string
	Department      Department
	SubmittedBy     string
	WeeklyVolume    int     // how often the task happens per week
	MinutesPerTask  float64 // human minutes per occurrence
	DataSensitivity Level   // customer PII, payment data, artwork rights
	ErrorCost       Level   // what a wrong answer costs: a typo or a reprint
	Status          OpportunityStatus
	AgentID         ID // the agent that shipped for it, if any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// OpportunityInput is what a lead submits.
type OpportunityInput struct {
	Title           string
	Problem         string
	Department      Department
	SubmittedBy     string
	WeeklyVolume    int
	MinutesPerTask  float64
	DataSensitivity Level
	ErrorCost       Level
}

// NewOpportunity validates and records an intake request.
func NewOpportunity(in OpportunityInput, now time.Time) (*Opportunity, error) {
	var v Validator
	v.Text("title", in.Title, 120)
	v.Text("problem", in.Problem, 2000)
	v.Enum("department", in.Department.Valid(), string(in.Department))
	v.Email("submittedBy", in.SubmittedBy)
	v.IntRange("weeklyVolume", in.WeeklyVolume, 1, 100_000)
	v.FloatRange("minutesPerTask", in.MinutesPerTask, 0.5, 480)
	v.Enum("dataSensitivity", in.DataSensitivity.Valid(), string(in.DataSensitivity))
	v.Enum("errorCost", in.ErrorCost.Valid(), string(in.ErrorCost))
	if err := v.Err(); err != nil {
		return nil, err
	}
	return &Opportunity{
		ID: NewID(), Title: in.Title, Problem: in.Problem, Department: in.Department,
		SubmittedBy: in.SubmittedBy, WeeklyVolume: in.WeeklyVolume, MinutesPerTask: in.MinutesPerTask,
		DataSensitivity: in.DataSensitivity, ErrorCost: in.ErrorCost,
		Status: OpportunitySubmitted, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// HoursPerWeek is the human time the task consumes today.
func (o *Opportunity) HoursPerWeek() float64 {
	return float64(o.WeeklyVolume) * o.MinutesPerTask / 60
}

// Feasibility discounts risky work: sensitive data and expensive mistakes
// both need more guardrails and human review, so they return less per hour.
func (o *Opportunity) Feasibility() float64 {
	return math.Max(0.3, 1-o.DataSensitivity.penalty()-o.ErrorCost.penalty())
}

// Score ranks the backlog: weekly hours an agent could win back, adjusted
// for feasibility, to one decimal.
func (o *Opportunity) Score() float64 {
	return math.Round(o.HoursPerWeek()*o.Feasibility()*10) / 10
}

// MoveTo changes status. Shipping requires the agent that now does the work.
func (o *Opportunity) MoveTo(status OpportunityStatus, agentID ID, now time.Time) error {
	if !slices.Contains(opportunityTransitions[o.Status], status) {
		return &TransitionError{Entity: "opportunity", From: string(o.Status), To: string(status)}
	}
	if status == OpportunityShipped && agentID == "" {
		return NewValidationError("agentId", CodeRequired, "link the agent that shipped for this opportunity")
	}
	o.Status, o.UpdatedAt = status, now
	if status == OpportunityShipped {
		o.AgentID = agentID
	}
	return nil
}
