package graphql

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/davidmm07/pressroom/internal/adapter/graphql/model"
	"github.com/davidmm07/pressroom/internal/domain"
)

// Domain to transport mapping. Enums share their string values on both
// sides, so a conversion is a cast; everything else is copied field by field
// so the API can evolve without touching the domain (and the other way round).

func toModelRef(m domain.ModelRef) *model.Model {
	return &model.Model{ID: m.String(), Provider: model.ModelProvider(m.Provider), Name: m.Name}
}

func (r *Resolver) toTool(s domain.ToolSpec) *model.Tool {
	schema, _ := json.Marshal(s.InputSchema)
	return &model.Tool{Name: s.Name, Description: s.Description, RequiresApproval: s.RequiresApproval, InputSchema: schema}
}

func (r *Resolver) toAgent(a *domain.Agent) *model.Agent {
	tools := make([]*model.Tool, 0, len(a.Tools))
	for _, name := range a.Tools {
		if spec, ok := r.Tools.Lookup(name); ok {
			tools = append(tools, r.toTool(spec))
		}
	}
	return &model.Agent{
		ID: string(a.ID), Slug: a.Slug, Name: a.Name, Description: a.Description,
		Department: model.Department(a.Department), Owner: a.Owner, Instructions: a.Instructions,
		Model: toModelRef(a.Model), Tools: tools, Triggers: nonNil(a.Triggers),
		Budget:             &model.Budget{MaxSteps: a.Budget.MaxSteps, MaxCost: a.Budget.MaxCost},
		MinutesSavedPerRun: a.MinutesSavedPerRun, Status: model.AgentStatus(a.Status), Version: a.Version,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toScorecard(s domain.Scorecard) *model.Scorecard {
	out := &model.Scorecard{
		WindowDays: s.WindowDays, FinishedRuns: s.Finished, Succeeded: s.Succeeded, Failed: s.Failed,
		SuccessRate: s.SuccessRate(), Reviewed: s.Reviewed(), TotalCost: s.Cost, CostPerRun: s.CostPerRun(),
		HoursSaved: s.HoursSaved(), ValueDelivered: s.ValueDelivered(), NetValue: s.NetValue(),
		AvgDurationSeconds: s.AvgDuration.Seconds(),
	}
	if rate, ok := s.AcceptanceRate(); ok {
		out.AcceptanceRate = &rate
	}
	return out
}

func (r *Resolver) toRun(run *domain.Run) *model.Run {
	out := &model.Run{
		ID: string(run.ID), AgentID: string(run.AgentID), Model: toModelRef(run.Model),
		Variant: model.Variant(run.Variant), ExperimentID: optionalID(run.ExperimentID),
		Trigger: model.RunTrigger(run.Trigger), Input: run.Input, Status: model.RunStatus(run.Status),
		Output: run.Output, FailureReason: run.FailureReason, Turns: run.Turns,
		Usage: &model.Usage{InputTokens: run.Usage.InputTokens, CachedInputTokens: run.Usage.CachedInputTokens, OutputTokens: run.Usage.OutputTokens},
		Cost:  run.Cost, CreatedAt: run.CreatedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
	if d := run.Duration(); d > 0 {
		secs := d.Seconds()
		out.DurationSeconds = &secs
	}
	if run.Review != nil {
		out.Review = &model.Review{Verdict: model.Verdict(run.Review.Verdict), Reviewer: run.Review.Reviewer, Note: run.Review.Note, At: run.Review.At}
	}
	if call, ok := run.AwaitingCall(); ok {
		spec, _ := r.Tools.Lookup(call.Name)
		if spec.Name == "" {
			spec = domain.ToolSpec{Name: call.Name}
		}
		out.PendingToolCall = &model.PendingToolCall{CallID: call.ID, Tool: r.toTool(spec), Arguments: call.Arguments}
	}
	return out
}

func toStep(s domain.Step) *model.RunStep {
	detail, _ := json.Marshal(s.Detail)
	var tool *string
	if s.ToolName != "" {
		tool = &s.ToolName
	}
	return &model.RunStep{
		Index: s.Index, Kind: model.StepKind(s.Kind), ToolName: tool, Summary: s.Summary, Detail: detail,
		LatencyMs: int(s.Latency.Milliseconds()), Cost: s.Cost, At: s.At,
	}
}

func toExperiment(e *domain.Experiment) *model.Experiment {
	return &model.Experiment{
		ID: string(e.ID), AgentID: string(e.AgentID), Champion: toModelRef(e.Champion), Challenger: toModelRef(e.Challenger),
		TrafficPercent: e.TrafficPercent, Hypothesis: e.Hypothesis, Status: model.ExperimentStatus(e.Status),
		Outcome: e.Outcome, CreatedAt: e.CreatedAt, ConcludedAt: e.ConcludedAt,
	}
}

func toEvaluation(e *domain.Evaluation) *model.Evaluation {
	return &model.Evaluation{
		ID: string(e.ID), Decision: model.Decision(e.Decision), Reason: e.Reason,
		StatusBefore: model.AgentStatus(e.StatusBefore), StatusAfter: model.AgentStatus(e.StatusAfter),
		Scorecard: toScorecard(e.Scorecard), CreatedAt: e.CreatedAt,
	}
}

func toOpportunity(o *domain.Opportunity) *model.Opportunity {
	return &model.Opportunity{
		ID: string(o.ID), Title: o.Title, Problem: o.Problem, Department: model.Department(o.Department),
		SubmittedBy: o.SubmittedBy, WeeklyVolume: o.WeeklyVolume, MinutesPerTask: o.MinutesPerTask,
		DataSensitivity: model.Level(o.DataSensitivity), ErrorCost: model.Level(o.ErrorCost),
		Status: model.OpportunityStatus(o.Status), HoursPerWeek: o.HoursPerWeek(), Feasibility: o.Feasibility(),
		Score: o.Score(), AgentID: optionalID(o.AgentID), CreatedAt: o.CreatedAt,
	}
}

func (r *Resolver) toConnection(runs []*domain.Run, hasNext bool) *model.RunConnection {
	conn := &model.RunConnection{Edges: make([]*model.RunEdge, len(runs)), PageInfo: &model.PageInfo{HasNextPage: hasNext}}
	for i, run := range runs {
		conn.Edges[i] = &model.RunEdge{Cursor: encodeCursor(run.ID), Node: r.toRun(run)}
	}
	if n := len(runs); n > 0 {
		end := conn.Edges[n-1].Cursor
		conn.PageInfo.EndCursor = &end
	}
	return conn
}

// Cursors are opaque to clients; today they wrap a time-ordered run ID.
func encodeCursor(id domain.ID) string {
	return base64.RawURLEncoding.EncodeToString([]byte("run:" + string(id)))
}

func decodeCursor(c *string) (domain.ID, error) {
	if c == nil || *c == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*c)
	if err == nil {
		if id, ok := strings.CutPrefix(string(raw), "run:"); ok {
			return domain.ParseID(id)
		}
	}
	return "", domain.NewValidationError("after", domain.CodeInvalidFmt, "is not a cursor returned by this API")
}

// parseID validates a client-supplied ID; malformed IDs cannot exist, so
// they are reported as not found rather than as a server error.
func parseID(s string) (domain.ID, error) {
	id, err := domain.ParseID(s)
	if err != nil {
		return "", domain.ErrNotFound
	}
	return id, nil
}

func optionalID(id domain.ID) *string {
	if id == "" {
		return nil
	}
	s := string(id)
	return &s
}

func deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
