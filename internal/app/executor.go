package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// ExecutorConfig tunes the agent loop.
type ExecutorConfig struct {
	MaxOutputTokens int
	ModelTimeout    time.Duration
	ToolTimeout     time.Duration
	// MaxToolResultBytes truncates tool output before it reaches the model,
	// so one oversized API response cannot blow the context window.
	MaxToolResultBytes int
}

// DefaultExecutorConfig suits the print shop's tools, which answer in
// kilobytes and well under a minute.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{MaxOutputTokens: 16000, ModelTimeout: 3 * time.Minute, ToolTimeout: 30 * time.Second, MaxToolResultBytes: 16 << 10}
}

// errRunChanged signals that another writer (usually a cancel) saved the run
// while this worker held it. The other writer wins.
var errRunChanged = errors.New("run changed by another writer")

// Executor is the autonomous agent loop: ask the model, run the tools it
// requests, feed back the results, repeat until it answers, fails or needs a
// human. It depends only on ports (Dependency Inversion), so the same loop
// drives Claude, GPT, Grok, an open-weights model or the test double.
//
// The run is checkpointed after every model turn and every tool call, so a
// worker that dies mid-run is replaced by one that resumes from the last
// saved step instead of starting over.
type Executor struct {
	agents port.AgentRepository
	runs   port.RunRepository
	model  port.LanguageModel
	tools  port.ToolRegistry
	prices port.PriceBook
	events port.EventPublisher
	clock  port.Clock
	log    *slog.Logger
	cfg    ExecutorConfig
}

func NewExecutor(
	agents port.AgentRepository, runs port.RunRepository, model port.LanguageModel, tools port.ToolRegistry,
	prices port.PriceBook, events port.EventPublisher, clock port.Clock, log *slog.Logger, cfg ExecutorConfig,
) *Executor {
	return &Executor{agents: agents, runs: runs, model: model, tools: tools, prices: prices, events: events, clock: clock, log: log, cfg: cfg}
}

// Execute advances a run as far as it can go. A nil error means the job is
// done: the run finished, failed or paused for approval. A *port.TransientError
// means "try again later"; the run stays RUNNING and resumes from its last
// checkpoint. Any other error is permanent.
func (e *Executor) Execute(ctx context.Context, runID domain.ID) error {
	err := e.execute(ctx, runID)
	if errors.Is(err, errRunChanged) {
		e.log.InfoContext(ctx, "run changed underneath the worker; yielding", "run_id", runID)
		return nil
	}
	return err
}

func (e *Executor) execute(ctx context.Context, runID domain.ID) error {
	run, err := e.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	switch run.Status {
	case domain.RunQueued:
		if err := run.Start(e.clock.Now()); err != nil {
			return err
		}
	case domain.RunRunning:
		e.log.InfoContext(ctx, "resuming run from checkpoint", "run_id", run.ID, "turns", run.Turns)
	default:
		return nil // finished, cancelled or waiting on a human: a stale job
	}

	agent, err := e.agents.Get(ctx, run.AgentID)
	if err != nil {
		return err
	}
	if !agent.CanRun() {
		return e.fail(ctx, agent, run, "agent was taken off duty before the run finished")
	}
	specs := e.toolSpecs(agent)

	for {
		if call, ok := run.Pending.Next(); ok {
			paused, err := e.resolveCall(ctx, agent, run, call)
			if err != nil || paused {
				return err
			}
			continue
		}

		if reason, ok := run.CheckBudget(agent.Budget); !ok {
			return e.fail(ctx, agent, run, reason)
		}
		if err := e.save(ctx, run); err != nil {
			return err
		}

		started := time.Now()
		completion, err := e.complete(ctx, agent, run, specs)
		if err != nil {
			if _, transient := port.IsTransient(err); transient {
				return err
			}
			return e.fail(ctx, agent, run, "model error: "+err.Error())
		}
		cost := e.prices.Price(run.Model).Cost(completion.Usage)
		if err := run.RecordTurn(completion.Message, completion.Usage, cost, time.Since(started), e.clock.Now()); err != nil {
			return err
		}
		if completion.ServedBy != "" {
			e.log.WarnContext(ctx, "provider fallback served the turn", "run_id", run.ID, "requested", run.Model.String(), "served_by", completion.ServedBy)
		}

		switch {
		case completion.StopReason == port.StopRefusal:
			return e.fail(ctx, agent, run, "model declined the task: "+orUnknown(completion.Detail))
		case len(completion.Message.ToolCalls) > 0:
			continue
		case completion.StopReason == port.StopMaxTokens:
			return e.fail(ctx, agent, run, "model hit the output token limit before finishing")
		default:
			return e.succeed(ctx, agent, run, completion.Message.Text)
		}
	}
}

// resolveCall enforces the guardrails for one tool call, then runs it.
// paused is true when the run now waits for a human.
func (e *Executor) resolveCall(ctx context.Context, agent *domain.Agent, run *domain.Run, call domain.ToolCall) (paused bool, err error) {
	now := e.clock.Now()
	spec, known := e.tools.Lookup(call.Name)
	switch {
	case !known || !agent.Allows(call.Name):
		// Guardrail: the allow-list is enforced here, not only in the prompt.
		res := domain.ToolResult{CallID: call.ID, IsError: true,
			Content: fmt.Sprintf("Tool %q is not available to you. Use one of: %s.", call.Name, strings.Join(agent.Tools, ", "))}
		if err := run.ResolveCall(res, call.Arguments, 0, now); err != nil {
			return false, err
		}
		return false, e.save(ctx, run)

	case spec.RequiresApproval && !run.IsApproved(call.ID):
		// Guardrail: money and customer-visible actions wait for a person.
		if err := run.AwaitApproval(call, now); err != nil {
			return false, err
		}
		if err := e.save(ctx, run); err != nil {
			return false, err
		}
		e.events.Publish(ctx, domain.RunNeedsApproval{RunID: run.ID, AgentSlug: agent.Slug, Tool: call.Name, Arguments: string(call.Arguments)})
		return true, nil
	}

	toolCtx, cancel := context.WithTimeout(port.WithToolCall(ctx, port.ToolCallInfo{
		RunID: run.ID, CallID: call.ID, AgentSlug: agent.Slug,
	}), e.cfg.ToolTimeout)
	defer cancel()

	started := time.Now()
	out, err := e.tools.Invoke(toolCtx, call.Name, call.Arguments)
	latency := time.Since(started)
	if err != nil && ctx.Err() != nil {
		// The worker is shutting down; leave the call pending for the retry.
		return false, port.Transient(ctx.Err(), 0)
	}

	res := domain.ToolResult{CallID: call.ID}
	var inputErr *port.ToolInputError
	switch {
	case errors.As(err, &inputErr):
		// Tell the model exactly what to fix; it usually corrects itself.
		res.IsError, res.Content = true, inputErr.Error()
	case err != nil:
		e.log.WarnContext(ctx, "tool failed", "run_id", run.ID, "tool", call.Name, "error", err)
		res.IsError, res.Content = true, "The tool failed: "+err.Error()
	default:
		res.Content = truncate(string(out), e.cfg.MaxToolResultBytes)
	}
	if err := run.ResolveCall(res, call.Arguments, latency, e.clock.Now()); err != nil {
		return false, err
	}
	return false, e.save(ctx, run)
}

func (e *Executor) complete(ctx context.Context, agent *domain.Agent, run *domain.Run, specs []domain.ToolSpec) (*port.Completion, error) {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.ModelTimeout)
	defer cancel()
	c, err := e.model.Complete(ctx, port.CompletionRequest{
		Model:           run.Model,
		System:          systemPrompt(agent),
		Messages:        run.Transcript,
		Tools:           specs,
		MaxOutputTokens: e.cfg.MaxOutputTokens,
	})
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, port.Transient(fmt.Errorf("model call timed out after %s: %w", e.cfg.ModelTimeout, err), 0)
	}
	return c, err
}

// Abandon fails a run whose job kept failing transiently until the worker
// gave up on it.
func (e *Executor) Abandon(ctx context.Context, runID domain.ID, reason string) error {
	run, err := e.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != domain.RunRunning && run.Status != domain.RunQueued {
		return nil
	}
	if run.Status == domain.RunQueued {
		if err := run.Start(e.clock.Now()); err != nil {
			return err
		}
	}
	agent, err := e.agents.Get(ctx, run.AgentID)
	if err != nil {
		return err
	}
	return e.fail(ctx, agent, run, reason)
}

func (e *Executor) succeed(ctx context.Context, agent *domain.Agent, run *domain.Run, output string) error {
	if err := run.Succeed(output, e.clock.Now()); err != nil {
		return err
	}
	return e.finish(ctx, agent, run, "")
}

func (e *Executor) fail(ctx context.Context, agent *domain.Agent, run *domain.Run, reason string) error {
	if err := run.Fail(reason, e.clock.Now()); err != nil {
		return err
	}
	return e.finish(ctx, agent, run, reason)
}

func (e *Executor) finish(ctx context.Context, agent *domain.Agent, run *domain.Run, reason string) error {
	if err := e.save(ctx, run); err != nil {
		return err
	}
	e.log.InfoContext(ctx, "run finished", "run_id", run.ID, "agent", agent.Slug, "status", run.Status,
		"turns", run.Turns, "cost_usd", run.Cost.USD(), "reason", reason)
	e.events.Publish(ctx, domain.RunFinished{RunID: run.ID, AgentSlug: agent.Slug, Status: run.Status, Cost: run.Cost, Reason: reason})
	return nil
}

func (e *Executor) save(ctx context.Context, run *domain.Run) error {
	err := e.runs.Save(ctx, run)
	if errors.Is(err, domain.ErrConflict) {
		return errRunChanged
	}
	return err
}

// toolSpecs returns the specs of the agent's allow-listed tools that still
// exist in the registry, in allow-list order (stable order keeps the
// provider's prompt cache warm).
func (e *Executor) toolSpecs(agent *domain.Agent) []domain.ToolSpec {
	specs := make([]domain.ToolSpec, 0, len(agent.Tools))
	for _, name := range agent.Tools {
		if spec, ok := e.tools.Lookup(name); ok {
			specs = append(specs, spec)
		}
	}
	return specs
}

// systemPrompt wraps the lead-written instructions with the operating rules
// every agent on the crew shares.
func systemPrompt(a *domain.Agent) string {
	return fmt.Sprintf(`You are %s, an AI agent on the %s team of a custom printing company that makes stickers, labels, magnets, buttons, packaging and t-shirts, with free online proofs and free worldwide shipping.

%s

Operating rules:
- Use the tools for facts about orders, artwork and shipments. Never invent order numbers, prices, tracking numbers or dates.
- Some tools need a human's approval. If an approval is denied, do not retry that action; find another way or escalate.
- If a tool returns an error, read it and fix your input, or explain what blocked you.
- Finish with a short summary a teammate can act on.`,
		a.Name, strings.ToLower(strings.ReplaceAll(string(a.Department), "_", " ")), a.Instructions)
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n[truncated %d bytes]", len(s)-max)
}

func orUnknown(s string) string {
	if s == "" {
		return "no category given"
	}
	return s
}
