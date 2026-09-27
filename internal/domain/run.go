package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// RunStatus is where a run sits in its lifecycle.
type RunStatus string

const (
	RunQueued           RunStatus = "QUEUED"
	RunRunning          RunStatus = "RUNNING"
	RunAwaitingApproval RunStatus = "AWAITING_APPROVAL"
	RunSucceeded        RunStatus = "SUCCEEDED"
	RunFailed           RunStatus = "FAILED"
	RunCancelled        RunStatus = "CANCELLED"
)

var runTransitions = map[RunStatus][]RunStatus{
	RunQueued:           {RunRunning, RunCancelled},
	RunRunning:          {RunAwaitingApproval, RunSucceeded, RunFailed, RunCancelled},
	RunAwaitingApproval: {RunQueued, RunCancelled},
	RunSucceeded:        {},
	RunFailed:           {},
	RunCancelled:        {},
}

func (s RunStatus) Valid() bool {
	_, ok := runTransitions[s]
	return ok
}

// Terminal reports whether the run can no longer change.
func (s RunStatus) Terminal() bool { return s.Valid() && len(runTransitions[s]) == 0 }

// Trigger records what started a run.
type Trigger string

const (
	TriggerManual   Trigger = "MANUAL"   // someone pressed "run" in the dashboard or API
	TriggerEvent    Trigger = "EVENT"    // a Pub/Sub event such as ticket.created
	TriggerSchedule Trigger = "SCHEDULE" // Cloud Scheduler
)

func (t Trigger) Valid() bool {
	return t == TriggerManual || t == TriggerEvent || t == TriggerSchedule
}

// Variant tells which arm of a model experiment served the run.
type Variant string

const (
	VariantChampion   Variant = "CHAMPION"
	VariantChallenger Variant = "CHALLENGER"
)

// StepKind classifies entries in a run's audit trail.
type StepKind string

const (
	StepModelTurn         StepKind = "MODEL_TURN"
	StepToolCall          StepKind = "TOOL_CALL"
	StepApprovalRequested StepKind = "APPROVAL_REQUESTED"
	StepApprovalGranted   StepKind = "APPROVAL_GRANTED"
	StepApprovalDenied    StepKind = "APPROVAL_DENIED"
	StepCompleted         StepKind = "COMPLETED"
	StepFailed            StepKind = "FAILED"
)

// Step is one entry in the run's trace, what the dashboard shows as a
// timeline and what an auditor reads when an agent does something odd.
type Step struct {
	Index    int
	Kind     StepKind
	ToolName string
	Summary  string
	Detail   map[string]any
	Latency  time.Duration
	Cost     Micros
	At       time.Time
}

// PendingCalls holds the tool calls of the latest model turn while they are
// being resolved. Providers expect every result of a turn in one reply, so
// results wait here until the last call (possibly one needing approval) ends.
type PendingCalls struct {
	Calls          []ToolCall
	Results        []ToolResult
	AwaitingCallID string // set while a human decides
	ApprovedCallID string // set once a human said yes
}

// Next returns the first call without a result.
func (p *PendingCalls) Next() (ToolCall, bool) {
	if p == nil || len(p.Results) >= len(p.Calls) {
		return ToolCall{}, false
	}
	return p.Calls[len(p.Results)], true
}

// Review is a human's verdict on a finished run. Reviews feed the acceptance
// rate, the metric that tells a busy agent apart from a useful one.
type Review struct {
	Verdict  Verdict
	Reviewer string
	Note     string
	At       time.Time
}

// Verdict grades a run's output.
type Verdict string

const (
	VerdictAccepted Verdict = "ACCEPTED" // used as is
	VerdictEdited   Verdict = "EDITED"   // useful after a human fixed it
	VerdictRejected Verdict = "REJECTED" // thrown away
)

func (v Verdict) Valid() bool {
	return v == VerdictAccepted || v == VerdictEdited || v == VerdictRejected
}

// Run is one piece of work an agent performs, from input to reviewed output.
// It is an aggregate root: the transcript, pending calls and steps change only
// through its methods, which keep them consistent with Status.
type Run struct {
	ID             ID
	AgentID        ID
	Model          ModelRef
	ExperimentID   ID // empty when no experiment was running
	Variant        Variant
	Trigger        Trigger
	Input          json.RawMessage
	IdempotencyKey string

	Status        RunStatus
	Transcript    []Message
	Pending       *PendingCalls
	Output        string
	FailureReason string
	Turns         int
	Usage         Usage
	Cost          Micros
	Steps         []Step
	Review        *Review

	Version    int
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// MaxRunInputBytes bounds the task payload stored with each run.
const MaxRunInputBytes = 64 << 10

// NewRunParams groups what the caller decides when starting a run.
type NewRunParams struct {
	// ID may be chosen up front, e.g. to assign an experiment arm before the
	// run exists. A fresh ID is generated when empty.
	ID             ID
	Input          json.RawMessage
	Trigger        Trigger
	IdempotencyKey string
	Model          ModelRef
	ExperimentID   ID
	Variant        Variant
}

// NewRun queues a run for an agent that is allowed to work.
func NewRun(agent *Agent, p NewRunParams, now time.Time) (*Run, error) {
	if !agent.CanRun() {
		return nil, &TransitionError{Entity: "agent " + agent.Slug, From: string(agent.Status), To: "running"}
	}
	var v Validator
	trimmed := bytes.TrimSpace(p.Input)
	v.Check(len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed), "input", CodeInvalidFmt, "must be a JSON object")
	v.Check(len(trimmed) <= MaxRunInputBytes, "input", CodeTooLong, fmt.Sprintf("must be at most %d bytes", MaxRunInputBytes))
	v.Enum("trigger", p.Trigger.Valid(), string(p.Trigger))
	v.OptionalText("idempotencyKey", p.IdempotencyKey, 128)
	v.Merge("model", p.Model.Validate())
	if err := v.Err(); err != nil {
		return nil, err
	}
	variant := p.Variant
	if variant == "" {
		variant = VariantChampion
	}
	id := p.ID
	if id == "" {
		id = NewID()
	}
	return &Run{
		ID:             id,
		AgentID:        agent.ID,
		Model:          p.Model,
		ExperimentID:   p.ExperimentID,
		Variant:        variant,
		Trigger:        p.Trigger,
		Input:          json.RawMessage(trimmed),
		IdempotencyKey: p.IdempotencyKey,
		Status:         RunQueued,
		Transcript:     []Message{{Role: RoleUser, Text: taskPrompt(trimmed)}},
		Version:        1,
		CreatedAt:      now,
	}, nil
}

// taskPrompt frames the run input as the first user turn. The agent's
// instructions travel separately as the system prompt.
func taskPrompt(input []byte) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, input, "", "  "); err != nil {
		pretty.Write(input)
	}
	return "Task input (JSON):\n" + pretty.String() + "\n\n" +
		"Complete the task described in your instructions using only the tools provided. " +
		"When you are done, reply with a short summary of what you did and anything a human should double-check."
}

func (r *Run) transition(to RunStatus) error {
	if !slices.Contains(runTransitions[r.Status], to) {
		return &TransitionError{Entity: "run", From: string(r.Status), To: string(to)}
	}
	r.Status = to
	return nil
}

func (r *Run) addStep(s Step) {
	s.Index = len(r.Steps)
	r.Steps = append(r.Steps, s)
}

// Start moves a queued run to RUNNING.
func (r *Run) Start(now time.Time) error {
	if err := r.transition(RunRunning); err != nil {
		return err
	}
	if r.StartedAt == nil {
		r.StartedAt = &now
	}
	return nil
}

// CheckBudget returns a reason when the run may not take another model turn.
func (r *Run) CheckBudget(b Budget) (string, bool) {
	switch {
	case r.Turns >= b.MaxSteps:
		return fmt.Sprintf("step budget exhausted after %d model turns", r.Turns), false
	case r.Cost >= b.MaxCost:
		return fmt.Sprintf("cost budget exhausted: spent %s of %s USD", r.Cost, b.MaxCost), false
	}
	return "", true
}

// RecordTurn appends a model response and charges its cost to the run.
func (r *Run) RecordTurn(msg Message, usage Usage, cost Micros, latency time.Duration, now time.Time) error {
	if r.Status != RunRunning {
		return &TransitionError{Entity: "run", From: string(r.Status), To: "model turn"}
	}
	if r.Pending != nil {
		return fmt.Errorf("run %s: model turn recorded while %d tool calls are unresolved: %w",
			r.ID, len(r.Pending.Calls)-len(r.Pending.Results), ErrInvalidTransition)
	}
	msg.Role = RoleAssistant
	r.Transcript = append(r.Transcript, msg)
	r.Turns++
	r.Usage = r.Usage.Add(usage)
	r.Cost += cost

	names := make([]string, 0, len(msg.ToolCalls))
	for _, c := range msg.ToolCalls {
		names = append(names, c.Name)
	}
	summary := "Model answered"
	if len(names) > 0 {
		summary = "Model requested " + strings.Join(names, ", ")
		r.Pending = &PendingCalls{Calls: slices.Clone(msg.ToolCalls)}
	}
	r.addStep(Step{
		Kind:    StepModelTurn,
		Summary: summary,
		Detail: map[string]any{
			"text": msg.Text, "tools": names,
			"inputTokens": usage.InputTokens, "outputTokens": usage.OutputTokens,
		},
		Latency: latency, Cost: cost, At: now,
	})
	return nil
}

// ResolveCall records the result of the next pending call. When it was the
// last one, all results are appended to the transcript as a single user turn.
func (r *Run) ResolveCall(res ToolResult, args json.RawMessage, latency time.Duration, now time.Time) error {
	next, ok := r.Pending.Next()
	if !ok || next.ID != res.CallID {
		return fmt.Errorf("run %s: result for unexpected tool call %q: %w", r.ID, res.CallID, ErrInvalidTransition)
	}
	res.Name = next.Name
	r.Pending.Results = append(r.Pending.Results, res)
	summary := "Called " + next.Name
	if res.IsError {
		summary = next.Name + " returned an error"
	}
	r.addStep(Step{
		Kind: StepToolCall, ToolName: next.Name, Summary: summary,
		Detail:  map[string]any{"arguments": string(args), "result": res.Content, "isError": res.IsError},
		Latency: latency, At: now,
	})
	if _, more := r.Pending.Next(); !more {
		r.Transcript = append(r.Transcript, Message{Role: RoleUser, ToolResults: r.Pending.Results})
		r.Pending = nil
	}
	return nil
}

// IsApproved reports whether a human already approved this call.
func (r *Run) IsApproved(callID string) bool {
	return r.Pending != nil && r.Pending.ApprovedCallID == callID
}

// AwaitApproval pauses the run on a call that needs a human decision.
func (r *Run) AwaitApproval(call ToolCall, now time.Time) error {
	if next, ok := r.Pending.Next(); !ok || next.ID != call.ID {
		return fmt.Errorf("run %s: approval requested for call %q out of order: %w", r.ID, call.ID, ErrInvalidTransition)
	}
	if err := r.transition(RunAwaitingApproval); err != nil {
		return err
	}
	r.Pending.AwaitingCallID = call.ID
	r.addStep(Step{
		Kind: StepApprovalRequested, ToolName: call.Name,
		Summary: "Waiting for a human to approve " + call.Name,
		Detail:  map[string]any{"arguments": string(call.Arguments)}, At: now,
	})
	return nil
}

// AwaitingCall returns the call a human must decide on, if any.
func (r *Run) AwaitingCall() (ToolCall, bool) {
	if r.Status != RunAwaitingApproval || r.Pending == nil {
		return ToolCall{}, false
	}
	next, ok := r.Pending.Next()
	return next, ok && next.ID == r.Pending.AwaitingCallID
}

// Approve lets the paused call proceed; the run goes back to the queue.
func (r *Run) Approve(approver string, now time.Time) error {
	call, ok := r.AwaitingCall()
	if !ok {
		return &TransitionError{Entity: "run", From: string(r.Status), To: "approved"}
	}
	if err := r.transition(RunQueued); err != nil {
		return err
	}
	r.Pending.ApprovedCallID, r.Pending.AwaitingCallID = call.ID, ""
	r.addStep(Step{Kind: StepApprovalGranted, ToolName: call.Name, Summary: approver + " approved " + call.Name,
		Detail: map[string]any{"approver": approver}, At: now})
	return nil
}

// Deny refuses the paused call. The agent receives the refusal as a tool
// error and decides what to do next (usually escalate to a person).
func (r *Run) Deny(approver, reason string, now time.Time) error {
	call, ok := r.AwaitingCall()
	if !ok {
		return &TransitionError{Entity: "run", From: string(r.Status), To: "denied"}
	}
	if strings.TrimSpace(reason) == "" {
		return NewValidationError("reason", CodeRequired, "explain why the action was denied")
	}
	if err := r.transition(RunQueued); err != nil {
		return err
	}
	r.Pending.AwaitingCallID = ""
	r.addStep(Step{Kind: StepApprovalDenied, ToolName: call.Name, Summary: approver + " denied " + call.Name,
		Detail: map[string]any{"approver": approver, "reason": reason}, At: now})
	return r.ResolveCall(ToolResult{
		CallID:  call.ID,
		Content: fmt.Sprintf("A human reviewer denied this action. Reason: %s. Do not retry it; choose another way forward.", reason),
		IsError: true,
	}, call.Arguments, 0, now)
}

// Succeed completes the run with the model's final answer.
func (r *Run) Succeed(output string, now time.Time) error {
	if err := r.transition(RunSucceeded); err != nil {
		return err
	}
	r.Output, r.FinishedAt = output, &now
	r.addStep(Step{Kind: StepCompleted, Summary: "Run completed", At: now})
	return nil
}

// Fail ends the run with a reason a human can act on.
func (r *Run) Fail(reason string, now time.Time) error {
	if err := r.transition(RunFailed); err != nil {
		return err
	}
	r.FailureReason, r.FinishedAt = reason, &now
	r.addStep(Step{Kind: StepFailed, Summary: reason, At: now})
	return nil
}

// Cancel stops a run that has not finished.
func (r *Run) Cancel(now time.Time) error {
	if err := r.transition(RunCancelled); err != nil {
		return err
	}
	r.FinishedAt = &now
	return nil
}

// SubmitReview grades a successful run. Each run is reviewed once so the
// acceptance rate cannot be gamed by re-reviewing.
func (r *Run) SubmitReview(verdict Verdict, reviewer, note string, now time.Time) error {
	var v Validator
	v.Enum("verdict", verdict.Valid(), string(verdict))
	v.Email("reviewer", reviewer)
	v.OptionalText("note", note, 1000)
	if err := v.Err(); err != nil {
		return err
	}
	if r.Status != RunSucceeded {
		return &TransitionError{Entity: "run", From: string(r.Status), To: "reviewed"}
	}
	if r.Review != nil {
		return fmt.Errorf("run %s was already reviewed by %s: %w", r.ID, r.Review.Reviewer, ErrConflict)
	}
	r.Review = &Review{Verdict: verdict, Reviewer: reviewer, Note: note, At: now}
	return nil
}

// Duration is the wall time from start to finish, or zero while in flight.
func (r *Run) Duration() time.Duration {
	if r.StartedAt == nil || r.FinishedAt == nil {
		return 0
	}
	return r.FinishedAt.Sub(*r.StartedAt)
}
