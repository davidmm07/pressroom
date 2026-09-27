package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// RunRepo stores runs, their transcripts and their audit trail.
type RunRepo struct{ db *DB }

// JSON shapes for the transcript and pending calls. They belong to the
// storage format, not the domain, so renaming a domain field never breaks
// rows written by an older release.
type messageRow struct {
	Role          string          `json:"role"`
	Text          string          `json:"text,omitempty"`
	ToolCalls     []toolCallRow   `json:"toolCalls,omitempty"`
	ToolResults   []toolResultRow `json:"toolResults,omitempty"`
	ProviderState json.RawMessage `json:"providerState,omitempty"`
	StateProvider string          `json:"stateProvider,omitempty"`
}

type toolCallRow struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolResultRow struct {
	CallID  string `json:"callId"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"isError,omitempty"`
}

type pendingRow struct {
	Calls          []toolCallRow   `json:"calls"`
	Results        []toolResultRow `json:"results"`
	AwaitingCallID string          `json:"awaitingCallId,omitempty"`
	ApprovedCallID string          `json:"approvedCallId,omitempty"`
}

func toCallRows(calls []domain.ToolCall) []toolCallRow {
	out := make([]toolCallRow, len(calls))
	for i, c := range calls {
		out[i] = toolCallRow{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
	}
	return out
}

func fromCallRows(rows []toolCallRow) []domain.ToolCall {
	out := make([]domain.ToolCall, len(rows))
	for i, c := range rows {
		out[i] = domain.ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
	}
	return out
}

func toResultRows(results []domain.ToolResult) []toolResultRow {
	out := make([]toolResultRow, len(results))
	for i, r := range results {
		out[i] = toolResultRow(r)
	}
	return out
}

func fromResultRows(rows []toolResultRow) []domain.ToolResult {
	out := make([]domain.ToolResult, len(rows))
	for i, r := range rows {
		out[i] = domain.ToolResult(r)
	}
	return out
}

func encodeTranscript(msgs []domain.Message) ([]byte, error) {
	rows := make([]messageRow, len(msgs))
	for i, m := range msgs {
		rows[i] = messageRow{
			Role: string(m.Role), Text: m.Text, ToolCalls: toCallRows(m.ToolCalls), ToolResults: toResultRows(m.ToolResults),
			ProviderState: m.ProviderState, StateProvider: string(m.StateProvider),
		}
	}
	return json.Marshal(rows)
}

func decodeTranscript(b []byte) ([]domain.Message, error) {
	var rows []messageRow
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("decode transcript: %w", err)
	}
	out := make([]domain.Message, len(rows))
	for i, r := range rows {
		out[i] = domain.Message{
			Role: domain.Role(r.Role), Text: r.Text, ToolCalls: fromCallRows(r.ToolCalls), ToolResults: fromResultRows(r.ToolResults),
			ProviderState: r.ProviderState, StateProvider: domain.Provider(r.StateProvider),
		}
	}
	return out, nil
}

func encodePending(p *domain.PendingCalls) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(pendingRow{
		Calls: toCallRows(p.Calls), Results: toResultRows(p.Results),
		AwaitingCallID: p.AwaitingCallID, ApprovedCallID: p.ApprovedCallID,
	})
}

func decodePending(b []byte) (*domain.PendingCalls, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var p pendingRow
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("decode pending calls: %w", err)
	}
	return &domain.PendingCalls{
		Calls: fromCallRows(p.Calls), Results: fromResultRows(p.Results),
		AwaitingCallID: p.AwaitingCallID, ApprovedCallID: p.ApprovedCallID,
	}, nil
}

// runSummaryColumns are enough to render lists; runColumns add the heavy
// transcript used by the executor.
const runSummaryColumns = `id, agent_id, model_provider, model_name, experiment_id, variant, trigger, input,
	coalesce(idempotency_key, ''), status, pending, output, failure_reason, turns,
	input_tokens, cached_input_tokens, output_tokens, cost_micros,
	review_verdict, coalesce(reviewer, ''), coalesce(review_note, ''), reviewed_at,
	version, created_at, started_at, finished_at`

const runColumns = runSummaryColumns + `, transcript`

func scanRun(row pgx.Row, withTranscript bool) (*domain.Run, error) {
	var (
		r                                  domain.Run
		provider, variant, trigger, status string
		experimentID, verdict              *string
		pending, transcript                []byte
		inTok, cachedTok, outTok, cost     int64
		reviewer, note                     string
		reviewedAt                         *time.Time
	)
	dest := []any{&r.ID, &r.AgentID, &provider, &r.Model.Name, &experimentID, &variant, &trigger, &r.Input,
		&r.IdempotencyKey, &status, &pending, &r.Output, &r.FailureReason, &r.Turns,
		&inTok, &cachedTok, &outTok, &cost,
		&verdict, &reviewer, &note, &reviewedAt,
		&r.Version, &r.CreatedAt, &r.StartedAt, &r.FinishedAt}
	if withTranscript {
		dest = append(dest, &transcript)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, mapErr(err)
	}
	r.Model.Provider = domain.Provider(provider)
	r.ExperimentID = idOrEmpty(experimentID)
	r.Variant, r.Trigger, r.Status = domain.Variant(variant), domain.Trigger(trigger), domain.RunStatus(status)
	r.Usage = domain.Usage{InputTokens: int(inTok), CachedInputTokens: int(cachedTok), OutputTokens: int(outTok)}
	r.Cost = domain.Micros(cost)
	if verdict != nil && reviewedAt != nil {
		r.Review = &domain.Review{Verdict: domain.Verdict(*verdict), Reviewer: reviewer, Note: note, At: *reviewedAt}
	}
	var err error
	if r.Pending, err = decodePending(pending); err != nil {
		return nil, err
	}
	if withTranscript {
		if r.Transcript, err = decodeTranscript(transcript); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

func nullableIdempotencyKey(k string) *string {
	if k == "" {
		return nil
	}
	return &k
}

func (r *RunRepo) Create(ctx context.Context, run *domain.Run) error {
	return r.db.WithinTx(ctx, func(ctx context.Context) error {
		transcript, err := encodeTranscript(run.Transcript)
		if err != nil {
			return err
		}
		pending, err := encodePending(run.Pending)
		if err != nil {
			return err
		}
		verdict, reviewer, note, reviewedAt := reviewColumns(run)
		_, err = r.db.q(ctx).Exec(ctx, `INSERT INTO runs (
			id, agent_id, model_provider, model_name, experiment_id, variant, trigger, input, idempotency_key,
			status, transcript, pending, output, failure_reason, turns, input_tokens, cached_input_tokens, output_tokens,
			cost_micros, review_verdict, reviewer, review_note, reviewed_at, version, created_at, started_at, finished_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)`,
			run.ID, run.AgentID, run.Model.Provider, run.Model.Name, nullID(run.ExperimentID), run.Variant, run.Trigger,
			run.Input, nullableIdempotencyKey(run.IdempotencyKey), run.Status, transcript, pending, run.Output,
			run.FailureReason, run.Turns, run.Usage.InputTokens, run.Usage.CachedInputTokens, run.Usage.OutputTokens,
			int64(run.Cost), verdict, reviewer, note, reviewedAt, run.Version, run.CreatedAt, run.StartedAt, run.FinishedAt)
		if err != nil {
			return mapErr(err)
		}
		return r.insertSteps(ctx, run)
	})
}

// Save updates the run with an optimistic version check and appends any
// steps not stored yet, in one transaction.
func (r *RunRepo) Save(ctx context.Context, run *domain.Run) error {
	transcript, err := encodeTranscript(run.Transcript)
	if err != nil {
		return err
	}
	pending, err := encodePending(run.Pending)
	if err != nil {
		return err
	}
	verdict, reviewer, note, reviewedAt := reviewColumns(run)
	err = r.db.WithinTx(ctx, func(ctx context.Context) error {
		tag, err := r.db.q(ctx).Exec(ctx, `UPDATE runs SET
			status=$3, transcript=$4, pending=$5, output=$6, failure_reason=$7, turns=$8,
			input_tokens=$9, cached_input_tokens=$10, output_tokens=$11, cost_micros=$12,
			review_verdict=$13, reviewer=$14, review_note=$15, reviewed_at=$16,
			started_at=$17, finished_at=$18, version = version + 1
			WHERE id = $1 AND version = $2`,
			run.ID, run.Version, run.Status, transcript, pending, run.Output, run.FailureReason, run.Turns,
			run.Usage.InputTokens, run.Usage.CachedInputTokens, run.Usage.OutputTokens, int64(run.Cost),
			verdict, reviewer, note, reviewedAt, run.StartedAt, run.FinishedAt)
		if err != nil {
			return mapErr(err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("run %s version %d: %w", run.ID, run.Version, domain.ErrConflict)
		}
		return r.insertSteps(ctx, run)
	})
	if err != nil {
		return err
	}
	run.Version++
	return nil
}

func reviewColumns(run *domain.Run) (verdict, reviewer, note *string, at *time.Time) {
	if run.Review == nil {
		return nil, nil, nil, nil
	}
	v := string(run.Review.Verdict)
	return &v, &run.Review.Reviewer, &run.Review.Note, &run.Review.At
}

// insertSteps appends the audit trail. Steps are immutable, so existing
// indexes are skipped rather than rewritten.
func (r *RunRepo) insertSteps(ctx context.Context, run *domain.Run) error {
	if len(run.Steps) == 0 {
		return nil
	}
	n := len(run.Steps)
	idx, latency, cost := make([]int, n), make([]int64, n), make([]int64, n)
	kinds, tools, summaries, details := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	ats := make([]time.Time, n)
	for i, s := range run.Steps {
		detail, err := json.Marshal(s.Detail)
		if err != nil {
			return err
		}
		idx[i], kinds[i], tools[i], summaries[i], details[i] = s.Index, string(s.Kind), s.ToolName, s.Summary, string(detail)
		latency[i], cost[i], ats[i] = s.Latency.Milliseconds(), int64(s.Cost), s.At
	}
	_, err := r.db.q(ctx).Exec(ctx, `INSERT INTO run_steps (run_id, idx, kind, tool_name, summary, detail, latency_ms, cost_micros, at)
		SELECT $1::uuid, * FROM unnest($2::int[], $3::text[], $4::text[], $5::text[], $6::jsonb[], $7::bigint[], $8::bigint[], $9::timestamptz[])
		ON CONFLICT (run_id, idx) DO NOTHING`,
		run.ID, idx, kinds, tools, summaries, details, latency, cost, ats)
	return mapErr(err)
}

func (r *RunRepo) loadSteps(ctx context.Context, run *domain.Run) error {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT idx, kind, tool_name, summary, detail, latency_ms, cost_micros, at
		FROM run_steps WHERE run_id = $1 ORDER BY idx`, run.ID)
	if err != nil {
		return mapErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s domain.Step
		var kind string
		var detail []byte
		var latencyMs, cost int64
		if err := rows.Scan(&s.Index, &kind, &s.ToolName, &s.Summary, &detail, &latencyMs, &cost, &s.At); err != nil {
			return mapErr(err)
		}
		s.Kind, s.Latency, s.Cost = domain.StepKind(kind), time.Duration(latencyMs)*time.Millisecond, domain.Micros(cost)
		if err := json.Unmarshal(detail, &s.Detail); err != nil {
			return fmt.Errorf("decode step detail: %w", err)
		}
		run.Steps = append(run.Steps, s)
	}
	return mapErr(rows.Err())
}

// Get loads a complete run: transcript and steps included.
func (r *RunRepo) Get(ctx context.Context, id domain.ID) (*domain.Run, error) {
	run, err := scanRun(r.db.q(ctx).QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id = $1`, id), true)
	if err != nil {
		return nil, err
	}
	return run, r.loadSteps(ctx, run)
}

func (r *RunRepo) FindByIdempotencyKey(ctx context.Context, agentID domain.ID, key string) (*domain.Run, error) {
	var id domain.ID
	err := r.db.q(ctx).QueryRow(ctx, `SELECT id FROM runs WHERE agent_id = $1 AND idempotency_key = $2`, agentID, key).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	return r.Get(ctx, id)
}

// List returns run summaries, newest first: no transcript and no steps.
// Load a run with Get before changing it.
func (r *RunRepo) List(ctx context.Context, f port.RunFilter, p port.Page) ([]*domain.Run, error) {
	statuses := make([]string, len(f.Status))
	for i, s := range f.Status {
		statuses[i] = string(s)
	}
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+runSummaryColumns+` FROM runs
		WHERE ($1::uuid IS NULL OR agent_id = $1)
		  AND (cardinality($2::text[]) = 0 OR status = ANY($2))
		  AND ($3::uuid IS NULL OR id < $3)
		ORDER BY id DESC
		LIMIT $4`, nullID(f.AgentID), statuses, nullID(p.After), p.First+1)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.Run
	for rows.Next() {
		run, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, mapErr(rows.Err())
}

// Stats aggregates finished runs per agent. The counting rules mirror
// domain.RunStats: cancelled and in-flight runs are excluded.
func (r *RunRepo) Stats(ctx context.Context, q port.StatsQuery) (map[domain.ID]domain.RunStats, error) {
	ids := make([]string, len(q.AgentIDs))
	for i, id := range q.AgentIDs {
		ids[i] = string(id)
	}
	rows, err := r.db.q(ctx).Query(ctx, `
		SELECT agent_id,
		       count(*) FILTER (WHERE status IN ('SUCCEEDED', 'FAILED')),
		       count(*) FILTER (WHERE status = 'SUCCEEDED'),
		       count(*) FILTER (WHERE status = 'FAILED'),
		       count(*) FILTER (WHERE review_verdict = 'ACCEPTED'),
		       count(*) FILTER (WHERE review_verdict = 'EDITED'),
		       count(*) FILTER (WHERE review_verdict = 'REJECTED'),
		       coalesce(sum(cost_micros) FILTER (WHERE status IN ('SUCCEEDED', 'FAILED')), 0)::bigint,
		       coalesce(avg(extract(epoch FROM finished_at - started_at) * 1000)
		                FILTER (WHERE status IN ('SUCCEEDED', 'FAILED') AND started_at IS NOT NULL), 0)::bigint
		FROM runs
		WHERE agent_id = ANY($1::uuid[]) AND created_at >= $2
		  AND ($3::uuid IS NULL OR experiment_id = $3)
		  AND ($4 = '' OR variant = $4)
		GROUP BY agent_id`, ids, q.Since, nullID(q.ExperimentID), string(q.Variant))
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make(map[domain.ID]domain.RunStats, len(q.AgentIDs))
	for rows.Next() {
		var id domain.ID
		var s domain.RunStats
		var cost, avgMs int64
		if err := rows.Scan(&id, &s.Finished, &s.Succeeded, &s.Failed, &s.Accepted, &s.Edited, &s.Rejected, &cost, &avgMs); err != nil {
			return nil, mapErr(err)
		}
		s.Cost, s.AvgDuration = domain.Micros(cost), time.Duration(avgMs)*time.Millisecond
		out[id] = s
	}
	return out, mapErr(rows.Err())
}
