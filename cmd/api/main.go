// Command api serves the GraphQL API used by the dashboard and integrations.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/graphql"
	"github.com/davidmm07/pressroom/internal/adapter/httpserver"
	"github.com/davidmm07/pressroom/internal/bootstrap"
	"github.com/davidmm07/pressroom/internal/platform/auth"
	"github.com/davidmm07/pressroom/internal/platform/config"
	"github.com/davidmm07/pressroom/internal/platform/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, logging.ParseLevel(cfg.LogLevel), cfg.LogFormat).With("service", "api")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := bootstrap.Build(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()

	resolver := &graphql.Resolver{
		Agents: a.Agents, Runs: a.Runs, Evaluation: a.Evaluation, Experiments: a.Experiments,
		Opportunities: a.Opportunities, Tools: a.Tools, Models: a.Catalog,
	}
	gql := graphql.NewHandler(resolver, graphql.ServerConfig{
		Introspection: cfg.Introspection, ComplexityLimit: cfg.ComplexityLimit,
	}, log)

	mux := http.NewServeMux()
	httpserver.Health(mux, a.Store.Ping)
	mux.Handle("/graphql", httpserver.Chain(gql,
		httpserver.MaxBytes(1<<20),
		auth.Middleware(auth.Config{TrustIAP: cfg.TrustIAP, APIToken: cfg.APIToken, DefaultActor: "operator@pressroom.local"}),
	))
	if !cfg.Production() {
		mux.Handle("GET /{$}", graphql.Playground("/graphql"))
	}

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpserver.Chain(mux,
			httpserver.RequestContext(cfg.GCPProject),
			httpserver.Recover(log),
			httpserver.AccessLog(log),
			httpserver.SecurityHeaders,
			httpserver.CORS(cfg.CORSOrigins),
		),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return serve(ctx, srv, log)
}

// serve runs the server until ctx ends, then drains in-flight requests.
func serve(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	log.Info("shutting down")
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
