// Package port declares the interfaces the use cases need from the outside
// world. The application layer owns these definitions and adapters implement
// them, which is what makes the source dependency point inward (Dependency
// Inversion Principle). Each interface is small and focused on one client
// need (Interface Segregation Principle).
package port

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
)

// ---- Storage (Repository pattern) ----

// AgentFilter narrows an agent listing. Zero values mean "any".
type AgentFilter struct {
	Status     []domain.AgentStatus
	Department domain.Department
	Trigger    string
}

// AgentRepository persists the crew.
type AgentRepository interface {
	Create(ctx context.Context, a *domain.Agent) error
	// Update fails with domain.ErrConflict when a.Version is stale
	// (optimistic concurrency); on success it bumps a.Version.
	Update(ctx context.Context, a *domain.Agent) error
	Get(ctx context.Context, id domain.ID) (*domain.Agent, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Agent, error)
	List(ctx context.Context, f AgentFilter) ([]*domain.Agent, error)
}

// RunFilter narrows a run listing.
type RunFilter struct {
	AgentID domain.ID
	Status  []domain.RunStatus
}

// Page is a keyset pagination request. After is an exclusive cursor on the
// time-ordered run ID; results are newest first.
type Page struct {
	First int
	After domain.ID
}

// StatsQuery selects the runs a scorecard summarises.
type StatsQuery struct {
	AgentIDs     []domain.ID
	Since        time.Time
	ExperimentID domain.ID      // optional: only runs of this experiment
	Variant      domain.Variant // optional: only this arm
}

// RunRepository persists runs and their audit trail.
type RunRepository interface {
	Create(ctx context.Context, r *domain.Run) error
	// Save writes the run and appends new steps. It fails with
	// domain.ErrConflict when r.Version is stale; on success it bumps it.
	Save(ctx context.Context, r *domain.Run) error
	Get(ctx context.Context, id domain.ID) (*domain.Run, error)
	FindByIdempotencyKey(ctx context.Context, agentID domain.ID, key string) (*domain.Run, error)
	// List returns up to p.First+1 runs so callers can tell if a next page exists.
	List(ctx context.Context, f RunFilter, p Page) ([]*domain.Run, error)
	// Stats aggregates finished runs per agent.
	Stats(ctx context.Context, q StatsQuery) (map[domain.ID]domain.RunStats, error)
}

// ExperimentRepository persists model trials.
type ExperimentRepository interface {
	Create(ctx context.Context, e *domain.Experiment) error
	Update(ctx context.Context, e *domain.Experiment) error
	Get(ctx context.Context, id domain.ID) (*domain.Experiment, error)
	// Running returns the agent's running experiment or domain.ErrNotFound.
	Running(ctx context.Context, agentID domain.ID) (*domain.Experiment, error)
	List(ctx context.Context, agentID domain.ID, status domain.ExperimentStatus) ([]*domain.Experiment, error)
}

// EvaluationRepository stores the history of policy decisions.
type EvaluationRepository interface {
	Create(ctx context.Context, e *domain.Evaluation) error
	ListByAgent(ctx context.Context, agentID domain.ID, limit int) ([]*domain.Evaluation, error)
}

// OpportunityRepository stores intake requests from department leads.
type OpportunityRepository interface {
	Create(ctx context.Context, o *domain.Opportunity) error
	Update(ctx context.Context, o *domain.Opportunity) error
	Get(ctx context.Context, id domain.ID) (*domain.Opportunity, error)
	List(ctx context.Context, status domain.OpportunityStatus, dept domain.Department) ([]*domain.Opportunity, error)
}

// TxManager runs a function inside one database transaction (Unit of Work).
// Repositories called with the ctx passed to fn join that transaction.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ---- Work queue ----

// Job is a leased unit of work for the executor.
type Job struct {
	RunID   domain.ID
	Attempt int
}

// JobQueue hands runs to workers with at-least-once delivery. Leases expire,
// so a crashed worker's job is picked up by another one.
type JobQueue interface {
	Enqueue(ctx context.Context, runID domain.ID, at time.Time) error
	// Claim leases the next due job, or returns nil when the queue is empty.
	Claim(ctx context.Context, lease time.Duration) (*Job, error)
	Extend(ctx context.Context, runID domain.ID, lease time.Duration) error
	Complete(ctx context.Context, runID domain.ID) error
	Retry(ctx context.Context, runID domain.ID, at time.Time, reason string) error
}

// ---- Language models (Strategy: one interface, one adapter per vendor) ----

// StopReason says why the model stopped generating.
type StopReason string

const (
	StopEndTurn   StopReason = "END_TURN"
	StopToolUse   StopReason = "TOOL_USE"
	StopMaxTokens StopReason = "MAX_TOKENS"
	StopRefusal   StopReason = "REFUSAL"
)

// CompletionRequest is one provider-neutral model turn.
type CompletionRequest struct {
	Model           domain.ModelRef
	System          string
	Messages        []domain.Message
	Tools           []domain.ToolSpec
	MaxOutputTokens int
}

// Completion is the model's reply.
type Completion struct {
	Message    domain.Message
	Usage      domain.Usage
	StopReason StopReason
	// Detail explains unusual stops, e.g. the refusal category.
	Detail string
	// ServedBy names the model that actually answered when a provider-side
	// fallback kicked in. Empty when the requested model answered.
	ServedBy string
}

// LanguageModel produces the next assistant turn.
type LanguageModel interface {
	Complete(ctx context.Context, req CompletionRequest) (*Completion, error)
}

// PriceBook prices model usage.
type PriceBook interface {
	Price(m domain.ModelRef) domain.Price
}

// ModelCatalog lists the models operators can pick in the dashboard.
type ModelCatalog interface {
	Models() []ModelInfo
}

// ModelInfo describes a selectable model.
type ModelInfo struct {
	Ref       domain.ModelRef
	Label     string
	Price     domain.Price
	Available bool // credentials configured, or served by the sandbox
}

// ---- Tools ----

// ToolRegistry resolves and executes the tools agents may call.
type ToolRegistry interface {
	// Catalog lists every registered tool.
	Catalog() []domain.ToolSpec
	// Lookup returns the spec of a registered tool.
	Lookup(name string) (domain.ToolSpec, bool)
	// Invoke validates args against the tool's schema and runs it. Invalid
	// arguments return a *ToolInputError so the caller can show the model
	// what to fix.
	Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
}

// ToolCallInfo identifies the call a tool is serving. Execution is
// at-least-once (a worker can crash after a tool ran but before the run was
// saved), so tools with side effects use IdempotencyKey to act only once.
type ToolCallInfo struct {
	RunID     domain.ID
	CallID    string
	AgentSlug string
}

// IdempotencyKey is stable across retries of the same call.
func (i ToolCallInfo) IdempotencyKey() string { return string(i.RunID) + ":" + i.CallID }

type toolCallKey struct{}

// WithToolCall attaches call metadata to the context handed to a tool.
func WithToolCall(ctx context.Context, info ToolCallInfo) context.Context {
	return context.WithValue(ctx, toolCallKey{}, info)
}

// ToolCallFrom reads the metadata set by WithToolCall.
func ToolCallFrom(ctx context.Context) (ToolCallInfo, bool) {
	info, ok := ctx.Value(toolCallKey{}).(ToolCallInfo)
	return info, ok
}

// ToolInputError reports arguments that failed schema validation.
type ToolInputError struct {
	Tool   string
	Detail string
}

func (e *ToolInputError) Error() string {
	return fmt.Sprintf("invalid arguments for %s: %s", e.Tool, e.Detail)
}

// ---- Events, time ----

// EventPublisher fans domain events out to subscribers (Observer pattern).
type EventPublisher interface {
	Publish(ctx context.Context, events ...domain.Event)
}

// Clock abstracts time so tests control it.
type Clock interface {
	Now() time.Time
}

// ---- Error classification ----

// TransientError marks failures worth retrying later: rate limits, overloaded
// providers, timeouts. Anything else is treated as permanent.
type TransientError struct {
	Err        error
	RetryAfter time.Duration
}

func (e *TransientError) Error() string { return "transient: " + e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

// Transient wraps err as retryable.
func Transient(err error, retryAfter time.Duration) error {
	return &TransientError{Err: err, RetryAfter: retryAfter}
}

// IsTransient reports whether err, or anything it wraps, is retryable.
func IsTransient(err error) (*TransientError, bool) {
	var te *TransientError
	ok := errors.As(err, &te)
	return te, ok
}
