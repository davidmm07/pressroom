package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
)

// ---- experiments ----

// ExperimentRepo stores model trials.
type ExperimentRepo struct{ db *DB }

const experimentColumns = `id, agent_id, champion_provider, champion_name, challenger_provider, challenger_name,
	traffic_percent, hypothesis, status, outcome, created_at, concluded_at`

func scanExperiment(row pgx.Row) (*domain.Experiment, error) {
	var e domain.Experiment
	var champion, challenger, status string
	err := row.Scan(&e.ID, &e.AgentID, &champion, &e.Champion.Name, &challenger, &e.Challenger.Name,
		&e.TrafficPercent, &e.Hypothesis, &status, &e.Outcome, &e.CreatedAt, &e.ConcludedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	e.Champion.Provider, e.Challenger.Provider = domain.Provider(champion), domain.Provider(challenger)
	e.Status = domain.ExperimentStatus(status)
	return &e, nil
}

func (r *ExperimentRepo) Create(ctx context.Context, e *domain.Experiment) error {
	_, err := r.db.q(ctx).Exec(ctx, `INSERT INTO experiments (`+experimentColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.ID, e.AgentID, e.Champion.Provider, e.Champion.Name, e.Challenger.Provider, e.Challenger.Name,
		e.TrafficPercent, e.Hypothesis, e.Status, e.Outcome, e.CreatedAt, e.ConcludedAt)
	return mapErr(err)
}

func (r *ExperimentRepo) Update(ctx context.Context, e *domain.Experiment) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE experiments SET status=$2, outcome=$3, concluded_at=$4 WHERE id=$1`,
		e.ID, e.Status, e.Outcome, e.ConcludedAt)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ExperimentRepo) Get(ctx context.Context, id domain.ID) (*domain.Experiment, error) {
	return scanExperiment(r.db.q(ctx).QueryRow(ctx, `SELECT `+experimentColumns+` FROM experiments WHERE id=$1`, id))
}

func (r *ExperimentRepo) Running(ctx context.Context, agentID domain.ID) (*domain.Experiment, error) {
	return scanExperiment(r.db.q(ctx).QueryRow(ctx,
		`SELECT `+experimentColumns+` FROM experiments WHERE agent_id=$1 AND status='RUNNING'`, agentID))
}

func (r *ExperimentRepo) List(ctx context.Context, agentID domain.ID, status domain.ExperimentStatus) ([]*domain.Experiment, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+experimentColumns+` FROM experiments
		WHERE ($1::uuid IS NULL OR agent_id = $1) AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC`, nullID(agentID), string(status))
	if err != nil {
		return nil, mapErr(err)
	}
	return collect(rows, scanExperiment)
}

// ---- evaluations ----

// EvaluationRepo stores policy decisions.
type EvaluationRepo struct{ db *DB }

type scorecardRow struct {
	WindowDays         int     `json:"windowDays"`
	Finished           int     `json:"finished"`
	Succeeded          int     `json:"succeeded"`
	Failed             int     `json:"failed"`
	Accepted           int     `json:"accepted"`
	Edited             int     `json:"edited"`
	Rejected           int     `json:"rejected"`
	CostMicros         int64   `json:"costMicros"`
	AvgDurationMs      int64   `json:"avgDurationMs"`
	MinutesSavedPerRun float64 `json:"minutesSavedPerRun"`
	HourlyRateMicros   int64   `json:"hourlyRateMicros"`
}

func (r *EvaluationRepo) Create(ctx context.Context, e *domain.Evaluation) error {
	s := e.Scorecard
	card, err := json.Marshal(scorecardRow{
		WindowDays: s.WindowDays, Finished: s.Finished, Succeeded: s.Succeeded, Failed: s.Failed,
		Accepted: s.Accepted, Edited: s.Edited, Rejected: s.Rejected, CostMicros: int64(s.Cost),
		AvgDurationMs: s.AvgDuration.Milliseconds(), MinutesSavedPerRun: s.MinutesSavedPerRun, HourlyRateMicros: int64(s.HourlyRate),
	})
	if err != nil {
		return err
	}
	_, err = r.db.q(ctx).Exec(ctx, `INSERT INTO evaluations
		(id, agent_id, decision, reason, status_before, status_after, scorecard, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ID, e.AgentID, e.Decision, e.Reason, e.StatusBefore, e.StatusAfter, card, e.CreatedAt)
	return mapErr(err)
}

func (r *EvaluationRepo) ListByAgent(ctx context.Context, agentID domain.ID, limit int) ([]*domain.Evaluation, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT id, agent_id, decision, reason, status_before, status_after, scorecard, created_at
		FROM evaluations WHERE agent_id = $1 ORDER BY created_at DESC LIMIT $2`, agentID, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	return collect(rows, func(row pgx.Row) (*domain.Evaluation, error) {
		var e domain.Evaluation
		var decision, before, after string
		var card []byte
		if err := row.Scan(&e.ID, &e.AgentID, &decision, &e.Reason, &before, &after, &card, &e.CreatedAt); err != nil {
			return nil, mapErr(err)
		}
		var s scorecardRow
		if err := json.Unmarshal(card, &s); err != nil {
			return nil, fmt.Errorf("decode scorecard: %w", err)
		}
		e.Decision, e.StatusBefore, e.StatusAfter = domain.Decision(decision), domain.AgentStatus(before), domain.AgentStatus(after)
		e.Scorecard = domain.Scorecard{
			RunStats: domain.RunStats{
				Finished: s.Finished, Succeeded: s.Succeeded, Failed: s.Failed, Accepted: s.Accepted, Edited: s.Edited,
				Rejected: s.Rejected, Cost: domain.Micros(s.CostMicros), AvgDuration: time.Duration(s.AvgDurationMs) * time.Millisecond,
			},
			WindowDays: s.WindowDays, MinutesSavedPerRun: s.MinutesSavedPerRun, HourlyRate: domain.Micros(s.HourlyRateMicros),
		}
		return &e, nil
	})
}

// ---- opportunities ----

// OpportunityRepo stores intake requests.
type OpportunityRepo struct{ db *DB }

const opportunityColumns = `id, title, problem, department, submitted_by, weekly_volume, minutes_per_task,
	data_sensitivity, error_cost, status, agent_id, created_at, updated_at`

func scanOpportunity(row pgx.Row) (*domain.Opportunity, error) {
	var o domain.Opportunity
	var dept, sensitivity, errorCost, status string
	var agentID *string
	err := row.Scan(&o.ID, &o.Title, &o.Problem, &dept, &o.SubmittedBy, &o.WeeklyVolume, &o.MinutesPerTask,
		&sensitivity, &errorCost, &status, &agentID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	o.Department, o.DataSensitivity, o.ErrorCost = domain.Department(dept), domain.Level(sensitivity), domain.Level(errorCost)
	o.Status, o.AgentID = domain.OpportunityStatus(status), idOrEmpty(agentID)
	return &o, nil
}

func (r *OpportunityRepo) Create(ctx context.Context, o *domain.Opportunity) error {
	_, err := r.db.q(ctx).Exec(ctx, `INSERT INTO opportunities (`+opportunityColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		o.ID, o.Title, o.Problem, o.Department, o.SubmittedBy, o.WeeklyVolume, o.MinutesPerTask,
		o.DataSensitivity, o.ErrorCost, o.Status, nullID(o.AgentID), o.CreatedAt, o.UpdatedAt)
	return mapErr(err)
}

func (r *OpportunityRepo) Update(ctx context.Context, o *domain.Opportunity) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE opportunities SET status=$2, agent_id=$3, updated_at=$4 WHERE id=$1`,
		o.ID, o.Status, nullID(o.AgentID), o.UpdatedAt)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *OpportunityRepo) Get(ctx context.Context, id domain.ID) (*domain.Opportunity, error) {
	return scanOpportunity(r.db.q(ctx).QueryRow(ctx, `SELECT `+opportunityColumns+` FROM opportunities WHERE id=$1`, id))
}

func (r *OpportunityRepo) List(ctx context.Context, status domain.OpportunityStatus, dept domain.Department) ([]*domain.Opportunity, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+opportunityColumns+` FROM opportunities
		WHERE ($1 = '' OR status = $1) AND ($2 = '' OR department = $2)
		ORDER BY created_at DESC`, string(status), string(dept))
	if err != nil {
		return nil, mapErr(err)
	}
	return collect(rows, scanOpportunity)
}

// collect drains rows through a scan function.
func collect[T any](rows pgx.Rows, scan func(pgx.Row) (*T, error)) ([]*T, error) {
	defer rows.Close()
	var out []*T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, mapErr(rows.Err())
}
