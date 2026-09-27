package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// AgentRepo stores agents.
type AgentRepo struct{ db *DB }

const agentColumns = `id, slug, name, description, department, owner_email, instructions,
	model_provider, model_name, tools, triggers, max_steps, max_cost_micros,
	minutes_saved_per_run, status, version, created_at, updated_at`

func scanAgent(row pgx.Row) (*domain.Agent, error) {
	var a domain.Agent
	var provider string
	var maxCost int64
	err := row.Scan(&a.ID, &a.Slug, &a.Name, &a.Description, &a.Department, &a.Owner, &a.Instructions,
		&provider, &a.Model.Name, &a.Tools, &a.Triggers, &a.Budget.MaxSteps, &maxCost,
		&a.MinutesSavedPerRun, &a.Status, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	a.Model.Provider = domain.Provider(provider)
	a.Budget.MaxCost = domain.Micros(maxCost)
	return &a, nil
}

func (r *AgentRepo) Create(ctx context.Context, a *domain.Agent) error {
	_, err := r.db.q(ctx).Exec(ctx, `INSERT INTO agents (`+agentColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		a.ID, a.Slug, a.Name, a.Description, a.Department, a.Owner, a.Instructions,
		a.Model.Provider, a.Model.Name, nonNil(a.Tools), nonNil(a.Triggers), a.Budget.MaxSteps, int64(a.Budget.MaxCost),
		a.MinutesSavedPerRun, a.Status, a.Version, a.CreatedAt, a.UpdatedAt)
	return mapErr(err)
}

// Update writes the agent if nobody changed it since it was read.
func (r *AgentRepo) Update(ctx context.Context, a *domain.Agent) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE agents SET
		name=$3, description=$4, department=$5, owner_email=$6, instructions=$7,
		model_provider=$8, model_name=$9, tools=$10, triggers=$11, max_steps=$12, max_cost_micros=$13,
		minutes_saved_per_run=$14, status=$15, updated_at=$16, version = version + 1
		WHERE id = $1 AND version = $2`,
		a.ID, a.Version, a.Name, a.Description, a.Department, a.Owner, a.Instructions,
		a.Model.Provider, a.Model.Name, nonNil(a.Tools), nonNil(a.Triggers), a.Budget.MaxSteps, int64(a.Budget.MaxCost),
		a.MinutesSavedPerRun, a.Status, a.UpdatedAt)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("agent %s version %d: %w", a.Slug, a.Version, domain.ErrConflict)
	}
	a.Version++
	return nil
}

func (r *AgentRepo) Get(ctx context.Context, id domain.ID) (*domain.Agent, error) {
	return scanAgent(r.db.q(ctx).QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, id))
}

func (r *AgentRepo) GetBySlug(ctx context.Context, slug string) (*domain.Agent, error) {
	return scanAgent(r.db.q(ctx).QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE slug = $1`, slug))
}

func (r *AgentRepo) List(ctx context.Context, f port.AgentFilter) ([]*domain.Agent, error) {
	statuses := make([]string, len(f.Status))
	for i, s := range f.Status {
		statuses[i] = string(s)
	}
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+agentColumns+` FROM agents
		WHERE (cardinality($1::text[]) = 0 OR status = ANY($1))
		  AND ($2 = '' OR department = $2)
		  AND ($3 = '' OR triggers @> ARRAY[$3])
		ORDER BY department, slug`, statuses, string(f.Department), f.Trigger)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, mapErr(rows.Err())
}
