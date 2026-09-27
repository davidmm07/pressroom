// Package postgres implements Pressroom's repositories, unit of work and job
// queue on PostgreSQL with pgx. Rows are mapped to domain types by hand so a
// column change never leaks into the business rules.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Connect opens a pool and verifies the connection.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies pending migrations. A session advisory lock lets several
// instances start at once (rolling deploys) without racing.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	results, err := provider.Up(ctx)
	for _, r := range results {
		log.InfoContext(ctx, "migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
	}
	return err
}

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// DB hands repositories either the pool or the transaction stored in the
// context by WithinTx.
type DB struct {
	pool *pgxpool.Pool
}

func (db *DB) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return db.pool
}

// WithinTx implements port.TxManager (Unit of Work). Nested calls join the
// outer transaction instead of opening a new one.
func (db *DB) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// Store groups every repository over one pool.
type Store struct {
	*DB
	Agents        *AgentRepo
	Runs          *RunRepo
	Experiments   *ExperimentRepo
	Evaluations   *EvaluationRepo
	Opportunities *OpportunityRepo
	Queue         *Queue
}

var (
	_ port.TxManager             = (*DB)(nil)
	_ port.AgentRepository       = (*AgentRepo)(nil)
	_ port.RunRepository         = (*RunRepo)(nil)
	_ port.ExperimentRepository  = (*ExperimentRepo)(nil)
	_ port.EvaluationRepository  = (*EvaluationRepo)(nil)
	_ port.OpportunityRepository = (*OpportunityRepo)(nil)
	_ port.JobQueue              = (*Queue)(nil)
)

// NewStore wires the repositories.
func NewStore(pool *pgxpool.Pool) *Store {
	db := &DB{pool: pool}
	return &Store{
		DB: db, Agents: &AgentRepo{db}, Runs: &RunRepo{db}, Experiments: &ExperimentRepo{db},
		Evaluations: &EvaluationRepo{db}, Opportunities: &OpportunityRepo{db}, Queue: &Queue{db},
	}
}

// Ping reports whether the database is reachable, for readiness probes.
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// mapErr translates driver errors into domain errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%s: %w", pgErr.ConstraintName, domain.ErrConflict)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%s: %w", pgErr.ConstraintName, domain.ErrNotFound)
		}
	}
	return err
}

// nullID maps the domain's empty ID to SQL NULL.
func nullID(id domain.ID) *string {
	if id == "" {
		return nil
	}
	s := string(id)
	return &s
}

func idOrEmpty(s *string) domain.ID {
	if s == nil {
		return ""
	}
	return domain.ID(*s)
}

// nonNil keeps NOT NULL array columns from receiving NULL for a nil slice.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
