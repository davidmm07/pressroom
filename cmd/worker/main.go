// Command worker executes agent runs from the queue and receives the events
// and scheduled jobs Google Cloud pushes to it. It is deployed as a private
// Cloud Run service: only Pub/Sub and Cloud Scheduler may invoke it.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/httpserver"
	"github.com/davidmm07/pressroom/internal/bootstrap"
	"github.com/davidmm07/pressroom/internal/platform/config"
	"github.com/davidmm07/pressroom/internal/platform/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, logging.ParseLevel(cfg.LogLevel), cfg.LogFormat).With("service", "worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := bootstrap.Build(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()

	mux := http.NewServeMux()
	httpserver.Health(mux, a.Store.Ping)
	mux.Handle("POST /events/pubsub", httpserver.PubSubPush(a.Runs.Dispatch, log))
	mux.Handle("POST /jobs/evaluate", httpserver.Job("evaluate", func(ctx context.Context) (any, error) {
		evals, err := a.Evaluation.EvaluateCrew(ctx)
		return map[string]int{"evaluated": len(evals)}, err
	}, log))

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpserver.Chain(mux,
			httpserver.RequestContext(cfg.GCPProject),
			httpserver.Recover(log),
			httpserver.AccessLog(log),
			httpserver.MaxBytes(1<<20),
		),
		ReadHeaderTimeout: 10 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info("worker started", "concurrency", cfg.WorkerConcurrency)
		a.Worker.Run(ctx)
	}()

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err = <-errc:
		stop()
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	wg.Wait()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("worker stopped")
	return nil
}
