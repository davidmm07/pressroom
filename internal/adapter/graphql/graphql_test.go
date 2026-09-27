package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/events"
	"github.com/davidmm07/pressroom/internal/adapter/graphql"
	"github.com/davidmm07/pressroom/internal/adapter/llm"
	"github.com/davidmm07/pressroom/internal/adapter/postgres"
	"github.com/davidmm07/pressroom/internal/adapter/tools"
	"github.com/davidmm07/pressroom/internal/app"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/platform/auth"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

type stack struct {
	srv      *httptest.Server
	executor *app.Executor
}

// newStack wires the real API over Postgres with the sandbox model, the same
// way cmd/api does, and serves it over HTTP.
func newStack(t *testing.T) *stack {
	t.Helper()
	url := os.Getenv("PRESSROOM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PRESSROOM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, log); err != nil {
		t.Fatal(err)
	}
	store := postgres.NewStore(pool)
	registry, err := tools.Standard(tools.Dependencies{
		Commerce: tools.NewPGCommerce(pool), Studio: tools.SandboxImageStudio{}, Carrier: tools.SandboxCarrier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModeSandbox, log)
	catalog, _ := llm.NewCatalog("", router.Available)
	bus := events.NewBus(log)
	hourly := domain.MicrosFromUSD(32)

	r := &graphql.Resolver{
		Agents:        app.NewAgentService(store.Agents, registry, bus, clock{}),
		Runs:          app.NewRunService(store.Agents, store.Runs, store.Experiments, store.Queue, store, bus, clock{}),
		Evaluation:    app.NewEvaluationService(store.Agents, store.Runs, store.Evaluations, store, bus, clock{}, app.EvaluationConfig{Window: 720 * time.Hour, HourlyRate: hourly, Policy: domain.DefaultRetirementPolicy()}),
		Experiments:   app.NewExperimentService(store.Agents, store.Runs, store.Experiments, store, bus, clock{}, hourly, domain.DefaultComparisonPolicy()),
		Opportunities: app.NewOpportunityService(store.Opportunities, store.Agents, clock{}),
		Tools:         registry, Models: catalog,
	}
	handler := auth.Middleware(auth.Config{DefaultActor: "lead@example.com"})(graphql.NewHandler(r, graphql.ServerConfig{Introspection: true, ComplexityLimit: 600}, log))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	exec := app.NewExecutor(store.Agents, store.Runs, router, registry, catalog, bus, clock{}, log, app.DefaultExecutorConfig())
	return &stack{srv: srv, executor: exec}
}

type response struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func (s *stack) do(t *testing.T, query string, vars map[string]any, out any) response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	resp, err := http.Post(s.srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if out != nil {
		for _, v := range r.Data {
			if err := json.Unmarshal(v, out); err != nil {
				t.Fatalf("decode %s: %v", v, err)
			}
		}
	}
	return r
}

const createAgent = `mutation($input: CreateAgentInput!) {
  createAgent(input: $input) { agent { id slug status version } userErrors { field code message } }
}`

func agentInput(slug string) map[string]any {
	return map[string]any{
		"slug": slug, "name": "Shipping Watch", "department": "OPERATIONS", "owner": "ops@example.com",
		"instructions": "Chase stalled parcels.", "model": "sandbox/planner",
		"tools": []string{"lookup_order", "notify_team"}, "maxSteps": 5, "maxCost": "0.50", "minutesSavedPerRun": 5,
	}
}

type userError struct {
	Field   []string `json:"field"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
}

func TestCreateAgentReportsFieldPathsAndCodes(t *testing.T) {
	s := newStack(t)
	in := agentInput("Bad Slug")
	in["tools"] = []string{"lookup_order", "launch_rockets"}
	in["maxSteps"] = 99
	in["model"] = "acme/model-x"

	var payload struct {
		UserErrors []userError `json:"userErrors"`
	}
	s.do(t, createAgent, map[string]any{"input": in}, &payload)
	got := map[string]string{}
	for _, e := range payload.UserErrors {
		got[strings.Join(e.Field, ".")] = e.Code
	}
	// The model is parsed first; fix it and the domain reports the rest together.
	if got["input.model"] != "INVALID_VALUE" {
		t.Fatalf("model error = %v", payload.UserErrors)
	}
	in["model"] = "sandbox/planner"
	s.do(t, createAgent, map[string]any{"input": in}, &payload)
	got = map[string]string{}
	for _, e := range payload.UserErrors {
		got[strings.Join(e.Field, ".")] = e.Code
	}
	want := map[string]string{"input.slug": "INVALID_FORMAT", "input.maxSteps": "OUT_OF_RANGE", "input.tools.1": "INVALID_VALUE"}
	for field, code := range want {
		if got[field] != code {
			t.Errorf("%s: want %s, got %q (all %v)", field, code, got[field], got)
		}
	}
}

func TestInvalidMoneyIsABadUserInput(t *testing.T) {
	s := newStack(t)
	in := agentInput("money-" + suffix())
	in["maxCost"] = "0.1234567"
	r := s.do(t, createAgent, map[string]any{"input": in}, nil)
	if len(r.Errors) != 1 || r.Errors[0].Extensions["code"] != "BAD_USER_INPUT" {
		t.Fatalf("errors = %+v", r.Errors)
	}
}

func TestRunLifecycleOverGraphQL(t *testing.T) {
	s := newStack(t)
	slug := "watch-" + suffix()

	var created struct {
		Agent      struct{ ID string } `json:"agent"`
		UserErrors []userError         `json:"userErrors"`
	}
	s.do(t, createAgent, map[string]any{"input": agentInput(slug)}, &created)
	if len(created.UserErrors) > 0 {
		t.Fatalf("create: %+v", created.UserErrors)
	}
	s.do(t, `mutation($id: ID!) { activateAgent(id: $id) { agent { status } } }`, map[string]any{"id": created.Agent.ID}, nil)

	start := `mutation($slug: String!) {
	  startRun(input: {agentSlug: $slug, input: {message: "Parcel stuck for a week, please help"}, idempotencyKey: "ticket-9"}) {
	    run { id status } userErrors { code message }
	  }
	}`
	var first, second struct {
		Run struct{ ID, Status string } `json:"run"`
	}
	s.do(t, start, map[string]any{"slug": slug}, &first)
	s.do(t, start, map[string]any{"slug": slug}, &second)
	if first.Run.ID == "" || first.Run.ID != second.Run.ID || first.Run.Status != "QUEUED" {
		t.Fatalf("idempotent start failed: %+v %+v", first, second)
	}

	if err := s.executor.Execute(context.Background(), domain.ID(first.Run.ID)); err != nil {
		t.Fatal(err)
	}

	var run struct {
		Status string `json:"status"`
		Agent  struct{ Slug string }
		Steps  []struct{ Kind, Summary string }
		Cost   string `json:"cost"`
	}
	s.do(t, `query($id: ID!) { run(id: $id) { status agent { slug } steps { kind summary } cost } }`, map[string]any{"id": first.Run.ID}, &run)
	if run.Status != "SUCCEEDED" || run.Agent.Slug != slug || len(run.Steps) < 3 || run.Steps[len(run.Steps)-1].Kind != "COMPLETED" {
		t.Fatalf("run = %+v", run)
	}

	var reviewed struct {
		Run struct {
			Review struct{ Verdict, Reviewer string }
		} `json:"run"`
		UserErrors []userError `json:"userErrors"`
	}
	review := `mutation($id: ID!) { reviewRun(input: {runId: $id, verdict: ACCEPTED}) { run { review { verdict reviewer } } userErrors { code field message } } }`
	s.do(t, review, map[string]any{"id": first.Run.ID}, &reviewed)
	if reviewed.Run.Review.Verdict != "ACCEPTED" || reviewed.Run.Review.Reviewer != "lead@example.com" {
		t.Fatalf("review = %+v", reviewed)
	}
	s.do(t, review, map[string]any{"id": first.Run.ID}, &reviewed)
	if len(reviewed.UserErrors) != 1 || reviewed.UserErrors[0].Code != "CONFLICT" {
		t.Fatalf("second review must conflict: %+v", reviewed.UserErrors)
	}

	var agent struct {
		Scorecard struct {
			FinishedRuns   int      `json:"finishedRuns"`
			AcceptanceRate *float64 `json:"acceptanceRate"`
		} `json:"scorecard"`
		Runs struct {
			Edges    []struct{ Cursor string }  `json:"edges"`
			PageInfo struct{ HasNextPage bool } `json:"pageInfo"`
		} `json:"runs"`
	}
	s.do(t, `query($slug: String!) { agent(slug: $slug) { scorecard { finishedRuns acceptanceRate } runs(first: 1) { edges { cursor } pageInfo { hasNextPage } } } }`,
		map[string]any{"slug": slug}, &agent)
	if agent.Scorecard.FinishedRuns != 1 || agent.Scorecard.AcceptanceRate == nil || *agent.Scorecard.AcceptanceRate != 1 {
		t.Fatalf("scorecard = %+v", agent.Scorecard)
	}
	if len(agent.Runs.Edges) != 1 || agent.Runs.PageInfo.HasNextPage {
		t.Fatalf("runs = %+v", agent.Runs)
	}
}

func TestUnknownObjectsAreNullAndBadCursorsAreRejected(t *testing.T) {
	s := newStack(t)
	r := s.do(t, `{ run(id: "nope") { id } agent(slug: "no-such-agent") { id } }`, nil, nil)
	if len(r.Errors) != 0 || string(r.Data["run"]) != "null" || string(r.Data["agent"]) != "null" {
		t.Fatalf("got %+v %s", r.Errors, r.Data)
	}
	r = s.do(t, `{ runs(after: "garbage") { edges { cursor } } }`, nil, nil)
	if len(r.Errors) != 1 || r.Errors[0].Extensions["code"] != "BAD_USER_INPUT" {
		t.Fatalf("errors = %+v", r.Errors)
	}
}

func suffix() string { return fmt.Sprint(time.Now().UnixNano() % 1_000_000_000) }
