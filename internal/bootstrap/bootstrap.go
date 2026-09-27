// Package bootstrap is the composition root shared by the binaries in cmd/.
// It is the only package that knows every concrete type: it picks adapters,
// wires them into the use cases and hands back ready services (Dependency
// Injection by hand; no container, no reflection).
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/davidmm07/pressroom/internal/adapter/events"
	"github.com/davidmm07/pressroom/internal/adapter/llm"
	"github.com/davidmm07/pressroom/internal/adapter/postgres"
	"github.com/davidmm07/pressroom/internal/adapter/tools"
	"github.com/davidmm07/pressroom/internal/app"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/platform/config"
)

// SystemClock is the production port.Clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// App holds the wired services.
type App struct {
	Pool          *pgxpool.Pool
	Store         *postgres.Store
	Tools         *tools.Registry
	Catalog       *llm.Catalog
	Agents        *app.AgentService
	Runs          *app.RunService
	Executor      *app.Executor
	Evaluation    *app.EvaluationService
	Experiments   *app.ExperimentService
	Opportunities *app.OpportunityService
	Worker        *app.Worker
}

// Close releases the database pool.
func (a *App) Close() { a.Pool.Close() }

// Build connects to the database and wires everything.
func Build(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	store := postgres.NewStore(pool)
	clock := SystemClock{}

	models, err := buildModels(cfg, log)
	if err != nil {
		pool.Close()
		return nil, err
	}
	catalog, err := llm.NewCatalog(cfg.PricebookJSON, models.Available)
	if err != nil {
		pool.Close()
		return nil, err
	}

	var studio tools.ImageStudio = tools.SandboxImageStudio{}
	if cfg.ImageStudioURL != "" {
		studio = tools.HTTPImageStudio{BaseURL: cfg.ImageStudioURL, Token: cfg.ImageStudioToken}
	}
	slack := tools.Slack{WebhookURL: cfg.SlackWebhookURL}
	registry, err := tools.Standard(tools.Dependencies{
		Commerce: tools.NewPGCommerce(pool), Studio: studio, Carrier: tools.SandboxCarrier{}, Slack: slack,
	})
	if err != nil {
		pool.Close()
		return nil, err
	}

	bus := events.NewBus(log, events.LogSubscriber(log), events.NotifySubscriber(slack))
	hourly := domain.MicrosFromUSD(cfg.HourlyRateUSD)

	// Decorators around the router: Retrying (outermost) repeats transient
	// failures, Logging records every attempt, the Router picks the adapter.
	model := llm.WithRetry(llm.WithLogging(models, log), llm.DefaultRetryPolicy())

	a := &App{Pool: pool, Store: store, Tools: registry, Catalog: catalog}
	a.Agents = app.NewAgentService(store.Agents, registry, bus, clock)
	a.Runs = app.NewRunService(store.Agents, store.Runs, store.Experiments, store.Queue, store, bus, clock)
	a.Executor = app.NewExecutor(store.Agents, store.Runs, model, registry, catalog, bus, clock, log, app.DefaultExecutorConfig())
	a.Evaluation = app.NewEvaluationService(store.Agents, store.Runs, store.Evaluations, store, bus, clock, app.EvaluationConfig{
		Window: time.Duration(cfg.EvalWindowDays) * 24 * time.Hour, HourlyRate: hourly, Policy: domain.DefaultRetirementPolicy(),
	})
	a.Experiments = app.NewExperimentService(store.Agents, store.Runs, store.Experiments, store, bus, clock, hourly, domain.DefaultComparisonPolicy())
	a.Opportunities = app.NewOpportunityService(store.Opportunities, store.Agents, clock)
	a.Worker = app.NewWorker(store.Queue, a.Executor, clock, log, app.WorkerConfig{
		Concurrency: cfg.WorkerConcurrency, PollInterval: cfg.PollInterval, Lease: cfg.Lease,
		MaxAttempts: cfg.MaxAttempts, ShutdownGrace: 8 * time.Second,
	})
	return a, nil
}

// buildModels registers an adapter for every provider with credentials
// (Factory). Providers without credentials fall back to the sandbox in
// "auto" mode, which keeps local development free and offline.
func buildModels(cfg config.Config, log *slog.Logger) (*llm.Router, error) {
	mode, err := llm.ParseMode(cfg.ModelMode)
	if err != nil {
		return nil, err
	}
	router := llm.NewRouter(mode, log)
	if cfg.AnthropicAPIKey != "" {
		router.Register(domain.ProviderAnthropic, llm.NewAnthropic(llm.AnthropicConfig{APIKey: cfg.AnthropicAPIKey}))
	}
	if cfg.OpenAIAPIKey != "" {
		router.Register(domain.ProviderOpenAI, llm.NewOpenAICompat(llm.OpenAICompatConfig{
			Provider: domain.ProviderOpenAI, BaseURL: cfg.OpenAIBaseURL, APIKey: cfg.OpenAIAPIKey,
		}))
	}
	if cfg.XAIAPIKey != "" {
		router.Register(domain.ProviderXAI, llm.NewOpenAICompat(llm.OpenAICompatConfig{
			Provider: domain.ProviderXAI, BaseURL: cfg.XAIBaseURL, APIKey: cfg.XAIAPIKey, MaxTokensField: "max_tokens",
		}))
	}
	if cfg.OpenSourceBaseURL != "" {
		router.Register(domain.ProviderOpenSource, llm.NewOpenAICompat(llm.OpenAICompatConfig{
			Provider: domain.ProviderOpenSource, BaseURL: cfg.OpenSourceBaseURL, APIKey: cfg.OpenSourceAPIKey, MaxTokensField: "max_tokens",
		}))
	}
	if mode == llm.ModeLive && cfg.AnthropicAPIKey == "" && cfg.OpenAIAPIKey == "" && cfg.XAIAPIKey == "" && cfg.OpenSourceBaseURL == "" {
		return nil, fmt.Errorf("PRESSROOM_MODEL_MODE=live needs at least one provider configured")
	}
	log.Info("model providers ready", "mode", mode,
		"anthropic", cfg.AnthropicAPIKey != "", "openai", cfg.OpenAIAPIKey != "",
		"xai", cfg.XAIAPIKey != "", "open_source", cfg.OpenSourceBaseURL != "")
	return router, nil
}
