// Command pressroomctl runs operational tasks: migrations, demo data, crew
// evaluations and manual event dispatch.
//
//	pressroomctl migrate
//	pressroomctl seed
//	pressroomctl evaluate
//	pressroomctl dispatch ticket.created '{"ticketId":"T-1","orderId":"SM-1042"}'
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/postgres"
	"github.com/davidmm07/pressroom/internal/bootstrap"
	"github.com/davidmm07/pressroom/internal/platform/config"
	"github.com/davidmm07/pressroom/internal/platform/logging"
)

const usage = `usage: pressroomctl <command>

commands:
  migrate                      apply database migrations
  seed                         load the demo crew, orders and history (idempotent)
  evaluate                     apply the retirement policy to the crew now
  dispatch <type> <json>       start runs for an event, as Pub/Sub would`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pressroomctl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(os.Stderr, logging.ParseLevel(cfg.LogLevel), "text")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if args[0] == "migrate" {
		pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		return postgres.Migrate(ctx, pool, log)
	}

	a, err := bootstrap.Build(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()

	switch args[0] {
	case "seed":
		return seed(ctx, a, log)
	case "evaluate":
		evals, err := a.Evaluation.EvaluateCrew(ctx)
		if err != nil {
			return err
		}
		for _, e := range evals {
			agent, _ := a.Agents.Get(ctx, e.AgentID)
			fmt.Printf("%-20s %-18s %s -> %s  %s\n", agent.Slug, e.Decision, e.StatusBefore, e.StatusAfter, e.Reason)
		}
		return nil
	case "dispatch":
		if len(args) != 3 {
			return errors.New(usage)
		}
		id := fmt.Sprintf("cli-%d", time.Now().UnixNano())
		runs, err := a.Runs.Dispatch(ctx, id, args[1], json.RawMessage(args[2]))
		if err != nil {
			return err
		}
		for _, r := range runs {
			fmt.Printf("queued run %s (%s)\n", r.ID, r.Model)
		}
		if len(runs) == 0 {
			fmt.Println("no agent on duty listens to", args[1])
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}
