# Pressroom: knowledge transfer

Welcome. This guide takes you from a fresh clone to running Pressroom, knowing
where things live, and making your first change. Plan on about 15 minutes for
the first run and an hour for the code tour.

Keep these open alongside it:

- [README.md](README.md): what Pressroom is and why it exists
- [docs/adr/0001-clean-architecture.md](docs/adr/0001-clean-architecture.md): why the code is shaped the way it is
- [docs/architecture.md](docs/architecture.md): diagrams, SOLID and design patterns mapped to files
- [docs/api.md](docs/api.md): GraphQL conventions and examples
- [docs/agents.md](docs/agents.md): the crew, the tools, and the rules that keep or retire agents

---

## 1. Pressroom in five minutes

Pressroom runs a crew of **AI agents** for a company that prints custom
stickers, labels, magnets, buttons, packaging and t-shirts. Each agent has a
job (check uploaded artwork, answer support tickets, settle damage claims from
a photo, screen marketplace designs, move production jobs that would ship
late, turn bad reviews into defect reports), a model (Claude, GPT, Grok or an
open-weights model), a set of **tools** it may call, and a budget.

Work arrives as **events** (for example `ticket.created`). Each event starts a
**run**: the agent's model decides which tools to call, Pressroom executes
them, and the loop repeats until the model answers. Anything that moves money
or reaches a customer's inbox **pauses for a human approval**. People then
**review** the output (accepted, edited, rejected).

From those runs Pressroom builds **scorecards** (success rate, acceptance,
cost, hours saved, net value). A daily **evaluation** puts agents that miss
their targets on **probation** and **retires** them if they miss again.
**Experiments** send part of an agent's traffic to a new model and promote it
only if it does the job as well for less. Department leads suggest new work
for agents on the **intake** page.

### Vocabulary

| Term | Meaning | Code |
| --- | --- | --- |
| Agent | A configured worker: instructions, model, tools, triggers, budget, status | `internal/domain/agent.go` |
| Run | One piece of work by one agent, from input to reviewed output | `internal/domain/run.go` |
| Step | One entry in a run's audit trail (model turn, tool call, approval) | `internal/domain/run.go` |
| Tool | A capability with a JSON Schema, e.g. `lookup_order`, `issue_refund` | `internal/adapter/tools` |
| Trigger | An event type that starts runs for an agent, e.g. `artwork.uploaded` | `Agent.Triggers` |
| Approval | A human yes or no on a tool call flagged `RequiresApproval` | `Run.AwaitApproval` |
| Review | A human verdict on a finished run: `ACCEPTED`, `EDITED`, `REJECTED` | `Run.SubmitReview` |
| Scorecard | Metrics over a time window, turned into hours and dollars | `internal/domain/scorecard.go` |
| Evaluation | The retirement policy applied to one agent, stored with its reasons | `internal/domain/policy.go` |
| Experiment | Champion vs challenger model on a share of an agent's runs | `internal/domain/experiment.go` |
| Opportunity | An intake request from a department lead, scored for the backlog | `internal/domain/opportunity.go` |
| Sandbox model | A deterministic fake model so everything runs without API keys | `internal/adapter/llm/sandbox.go` |

---

## 2. How the pieces fit

```mermaid
flowchart LR
  UI[Dashboard :5173 dev / :3300 docker] -- GraphQL --> API[API :8080]
  API <--> DB[(PostgreSQL :5433)]
  EV[Events: Pub/Sub or pressroomctl dispatch] --> W[Worker :8081]
  W <--> DB
  W --> M{{Model providers}}
  W --> T[Tools]
```

| Process | Command | Port | What it does |
| --- | --- | --- | --- |
| API | `cmd/api` | 8080 | GraphQL for the dashboard; GraphiQL playground at `/` in development |
| Worker | `cmd/worker` | 8081 | Claims runs from the queue and executes them; receives Pub/Sub events and the daily evaluation job |
| CLI | `cmd/pressroomctl` | none | `migrate`, `seed`, `evaluate`, `dispatch` |
| Dashboard | `web/` | 5173 (dev), 3300 (Docker) | React + TypeScript UI |
| PostgreSQL | Docker | 5433 on the host | System of record **and** the job queue |

The API and the worker are separate processes sharing one database. If runs
stay `QUEUED`, the worker is not running.

Host ports deliberately avoid 5432 and 3000, which other projects on the same
machine often use for their own Postgres and apps. Change them with
`PRESSROOM_DB_PORT` and `WEB_PORT`; the Makefile and compose read both.

---

## 3. Prerequisites

| Tool | Version | Check | Needed for |
| --- | --- | --- | --- |
| Docker with Compose v2 | recent | `docker compose version` | Everything (Postgres at minimum) |
| Go | 1.26 (older Go downloads it automatically) | `go version` | Local development and tests |
| Node.js | 20 or newer | `node -v` | Dashboard development |
| pnpm | 9 (via Corepack) | `corepack enable && pnpm -v` | Dashboard development |
| make | any | `make -v` | Shortcuts below |
| Terraform, gcloud | optional | | Only for deploying to Google Cloud |

On Windows, work inside WSL 2 and keep the repository on the Linux filesystem
(for example `~/projects/pressroom`), not under `/mnt/c`. Go builds and pnpm
installs are far slower across that boundary.

---

## 4. Run it: pick a path

Run one path at a time. Both serve the API on 8080 and the worker on 8081, so
the containers from Path A cannot start while the Path B servers are running
(and the other way round).

### Path A: everything in Docker (fastest, no Go or Node needed)

1. **Start the stack** from the repository root. The first build downloads Go
   modules and npm packages, so give it a few minutes.

   ```bash
   make up
   ```

   Compose starts Postgres, runs the `migrations` and `seed` one-shot
   containers, then starts `api`, `worker` and `web`. If port 3300 is taken:

   ```bash
   WEB_PORT=3400 make up
   ```

2. **Open the dashboard** at <http://localhost:3300> (or your `WEB_PORT`).
   The API answers at <http://localhost:8080/graphql>.

3. **Check the worker processed the seeded events.** The Approvals link in the
   sidebar should show **2**.

4. **Stop, or reset everything.**

   ```bash
   docker compose down        # stop, keep data
   make down                  # stop and delete the database
   ```

### Path B: local development (hot reload)

Use this when you are changing code. Open four terminals in the repository root.

1. **Terminal 1: database, migrations and demo data.**

   ```bash
   make db      # Postgres in Docker on :5433
   make seed    # migrations, then 13 agents, a month of history, 14 queued events
   ```

   Running `make seed` again prints `demo data already loaded; nothing to do`.
   It never duplicates data.

2. **Terminal 2: the API.**

   ```bash
   make dev-api      # http://localhost:8080 (GraphiQL at /)
   ```

3. **Terminal 3: the worker.** It picks up the 14 queued events within seconds.

   ```bash
   make dev-worker   # :8081
   ```

   Six runs end in `AWAITING_APPROVAL`; the others succeed. Logs are readable
   text locally (`LOG_FORMAT=text`) and JSON in production.

4. **Terminal 4: the dashboard.**

   ```bash
   make dev-web      # http://localhost:5173, proxies /graphql to :8080
   ```

5. **Start over whenever you like.**

   ```bash
   make down && make db seed
   ```

`DATABASE_URL` defaults to the Docker database
(`postgres://pressroom:pressroom@localhost:5433/pressroom?sslmode=disable`).
To load your own settings, copy the example file and export it before the
`make` commands:

```bash
cp .env.example .env
set -a && source .env && set +a
```

### Path C: real models instead of the sandbox

The default `PRESSROOM_MODEL_MODE=auto` sends each agent to its real provider
when that provider has credentials, and to the sandbox otherwise. Set any
of these before starting the API and worker (or in `.env` for Docker):

| Variable | Provider | Agents in the demo that use it |
| --- | --- | --- |
| `ANTHROPIC_API_KEY` | Claude (official Go SDK) | Proof Checker, Support Triage, Damage Claim Assessor, Proof Revision Assistant, Production Watch |
| `OPENAI_API_KEY` | OpenAI | Shipping Watch, Store Launch Coach |
| `XAI_API_KEY` | Grok | Support Triage challenger, Payout Reconciler, Listing Screener, Deal Planner |
| `OPEN_SOURCE_BASE_URL` | Any OpenAI-compatible server (Ollama, vLLM) | Reorder Nudger, Review Defect Analyst, Giveaway Screener |

For Ollama on your machine: `OPEN_SOURCE_BASE_URL=http://localhost:11434/v1`
locally, or `http://host.docker.internal:11434/v1` from Docker. Leave it
empty if no such server is running, or those runs will fail after retries.

`PRESSROOM_MODEL_MODE=sandbox` forces the fake model everywhere (useful for
demos). `live` refuses to run an agent whose provider is not configured.

---

## 5. Guided tour of the demo

After Path A or B, walk through these in the dashboard. Each one shows a
feature you will meet in the code.

| # | Where | Do this | What you are seeing |
| --- | --- | --- | --- |
| 1 | **Crew** | Read the table | Scorecards per agent. Support Triage is trialling `xai/grok-4-fast`; Reorder Nudger's work is only 40% accepted; Payout Reconciler is a draft. |
| 2 | **Approvals** | Approve the Support Triage `issue_refund` | Human-in-the-loop. The run goes back to the queue and the worker resumes it where it paused. |
| 3 | **Agents > Support Triage > Recent runs** | Open the newest run | The trace: every model turn, tool call, approval, latency and cost. |
| 4 | Same run page | Click **Accept** | Reviews feed the acceptance rate. A second review is refused with `CONFLICT`. |
| 5 | **Approvals** | Deny the Reorder Nudger email with a reason | The agent receives your reason as a tool error and finishes without sending. |
| 6 | **Support Triage** page | **Conclude experiment** | The challenger matched quality at a fraction of the cost, so it becomes the agent's model. |
| 7 | **Crew** | **Evaluate crew** | Reorder Nudger goes on probation, with the reasons shown. Evaluate again and it retires. |
| 8 | **Intake** | Submit a request, approve it, ship it with an agent | The scored backlog department leads use. |
| 9 | **Hire an agent** | Create one, activate it on its page, then **Run it now** | Form validation (client and server), the draft/active lifecycle, a manual run. |
| 10 | **Approvals** | Look at the Production Watch `reroute_job` | Job J-7004 would finish two days after its ship-by date on PRESS-1; PRESS-2 finishes it tomorrow. The tool did the arithmetic, the lead decides. |
| 11 | **Agents > Listing Screener** | Open both runs | One design went live with no human involved; the one naming a protected character was held and the team told. |

The same stories, automated: the Postman scenarios run each one through the
API with assertions and print its business outcome. Start here if you want to
understand what Pressroom is for before reading code.

```bash
make scenarios                                   # all nine, against the running stack
make scenarios S="04 A cheaper model earns the job"
```

They live in `postman/` in Postman's Native Git format (Collection v3 YAML).
To use them in the Postman desktop app: sidebar **Files** icon, **Open
folder**, pick the repository root, switch to **Local View**, select the
**Pressroom Local** environment and run a scenario folder with the Collection
Runner. [docs/scenarios.md](docs/scenarios.md) has a diagram of each scenario
and how to turn one into a Postman Flow.

You can also send events the way the storefront would:

```bash
go run ./cmd/pressroomctl dispatch ticket.created \
  '{"ticketId":"T-9001","orderId":"ORD-1046","message":"Where are my buttons?"}'
```

With Docker, run the CLI container instead:

```bash
docker compose run --rm seed dispatch ticket.created '{"ticketId":"T-9001","orderId":"ORD-1046"}'
```

Useful seeded data: orders `ORD-1042` to `ORD-1050` (ORD-1042 has a stalled
parcel, ORD-1050 is the misprinted one), customers `ana.lopez@example.com` and
`priya.nair@example.com`, stores `S-201` (no sales) and `S-202` (selling),
listings `L-3501` (flagged) and `L-3502` (clean), production job `J-7004`
(late on PRESS-1) and giveaway `G-12`. Event types in use:
`artwork.uploaded`, `ticket.created`, `shipment.stalled`,
`customer.reorder_due`, `claim.submitted`, `proof.changes_requested`,
`listing.submitted`, `store.no_sales`, `production.job_at_risk`,
`reviews.weekly_digest`, `capacity.weekly_forecast`, `giveaway.closed`.

---

## 6. Talk to the API directly

GraphiQL is at <http://localhost:8080/> when the API runs in development. Or
with curl:

```bash
curl -s localhost:8080/graphql -H 'Content-Type: application/json' \
  -H 'X-Pressroom-Actor: you@example.com' \
  -d '{"query":"{ crew { activeAgents awaitingApproval } agents { slug status model { id } } }"}'
```

`X-Pressroom-Actor` names who is approving and reviewing. Mutations return
`userErrors` for problems you can fix and top-level errors only for the rest;
[docs/api.md](docs/api.md) has the full contract and more examples.

---

## 7. Tests and checks

| Command | What it runs | Needs |
| --- | --- | --- |
| `make test` | All Go unit tests with the race detector; database tests are skipped | Go |
| `make test-integration` | Everything above plus repositories, the queue and GraphQL end to end on Postgres (creates `pressroom_test` in the Docker database) | Go, Docker |
| `make test-web` | Dashboard typecheck and Vitest | Node, pnpm |
| `make scenarios` | The nine Postman business scenarios against the running stack, failing if any step is skipped | Node, a running stack (`make up`) |
| `make lint` | `go vet` and a `gofmt` check | Go |
| `make check-generated` | Regenerates GraphQL code and fails if anything changed | Go, pnpm |
| `make ci` | lint, integration tests and web checks, like CI | all of the above |

To run one package or one test:

```bash
go test ./internal/app/ -run TestExecutorPausesForApprovalAndResumes -v
PRESSROOM_TEST_DATABASE_URL=postgres://pressroom:pressroom@localhost:5433/pressroom_test?sslmode=disable \
  go test ./internal/adapter/postgres/ -v
```

Where the important tests live:

- `internal/domain/*_test.go`: lifecycle rules, validation, scorecard math, retirement and experiment policies.
- `internal/app/executor_test.go`: the agent loop (approvals, denials, guardrails, budgets, crash recovery, cancellation). `fakes_test.go` holds the in-memory repositories and the scripted model used there.
- `internal/adapter/llm/llm_test.go`: provider wire formats, retries, routing, prices.
- `internal/adapter/postgres/postgres_test.go`: round trips, optimistic locking, concurrent queue claims.
- `internal/adapter/graphql/graphql_test.go`: the API end to end, including the error contract.

CI (`.github/workflows/ci.yml`) runs Go with a Postgres service, the web checks,
the image builds and `terraform validate`.

---

## 8. Code tour

```
cmd/                     api, worker, pressroomctl (thin mains)
internal/domain          business rules; imports only the standard library
internal/port            interfaces the use cases need (repositories, model, tools, queue, clock, events)
internal/app             use cases: agents, runs, executor, worker, evaluation, experiments, intake
internal/adapter/llm     Claude, Chat Completions (OpenAI, Grok, open weights), sandbox, router, retry/logging decorators, price book
internal/adapter/tools   tool registry and the 13 print-shop tools
internal/adapter/postgres repositories, transactions, job queue, migrations
internal/adapter/graphql schema, resolvers, error mapping, DataLoaders (gqlgen)
internal/adapter/httpserver middleware, health checks, Pub/Sub push and scheduled job endpoints
internal/adapter/events  in-process event bus (log and Slack subscribers)
internal/bootstrap       composition root: builds and wires everything
internal/platform        config from env, logging, auth
web/                     dashboard (React, urql, GraphQL Codegen, zod)
deploy/terraform         Google Cloud infrastructure
```

### Read in this order

1. `internal/domain/agent.go` and `internal/domain/run.go`: the two aggregates and their state machines.
2. `internal/port/port.go`: every seam between business logic and the outside world.
3. `internal/app/executor.go`: the agent loop. This is the heart of the product.
4. `internal/app/runs.go`: starting runs (idempotency, experiment routing), approvals, reviews.
5. `internal/adapter/llm/router.go`, then `anthropic.go` and `openaicompat.go`: how a provider-neutral request becomes a vendor call.
6. `internal/adapter/tools/registry.go`, then `support.go`: schema validation and a tool that needs approval.
7. `internal/adapter/postgres/queue.go`: how workers claim runs safely.
8. `internal/adapter/graphql/schema.graphqls`, `errors.go` and `schema.resolvers.go`: the API.
9. `internal/bootstrap/bootstrap.go`: how it all connects.

### Rules the architecture depends on

These are what keep the code easy to change. Code review checks them.

- `internal/domain` imports nothing outside the standard library.
- `internal/app` never imports `internal/adapter/...`; it talks to the world through `internal/port`.
- Adapters never import each other. Only `internal/bootstrap` and `cmd/` know concrete types.
- Business rules go in the domain, not in resolvers, repositories or React components.
- Expected failures are domain errors (`ValidationError`, `ErrNotFound`, `ErrConflict`, `ErrInvalidTransition`); adapters translate them. Retryable failures are wrapped with `port.Transient`.

Comments in the code name the SOLID principle or design pattern a piece
applies; [docs/architecture.md](docs/architecture.md) collects them in one table.

---

## 9. Everyday tasks

### Add a tool

1. Write it in `internal/adapter/tools/` with `Func(domain.ToolSpec{...}, handler)`. Give it a snake_case name, a clear description (the model reads it), and a strict JSON Schema via `Schema(...)`.
2. Depend on the narrowest system it needs (`Commerce`, `Marketplace`, `Factory`, `Reviews`, `Giveaways`...). A new system gets its own small interface, a field in `Dependencies`, and a line in `bootstrap.Build`.
3. Set `RequiresApproval: true` if it moves money, reaches a customer, changes public prices or reshuffles production.
4. Rules that must hold whatever the model does go in the handler, not the agent's instructions (see `publish_listing` and `propose_deal`). Keep the rule set a pure, exported function so it is easy to test, as `Inspect`, `AssessClaim` and `PlanJob` are.
5. For side effects, record them with `idempotencyKey(ctx, "<action>")` so a retried run never acts twice.
6. Register it in `Standard` (`internal/adapter/tools/standard.go`) and update the catalog count and approval list in `tools_test.go`.
7. Give it to an agent in the dashboard or with `updateAgent`.

### Add a department

1. Add the constant and its place in `Departments` (`internal/domain/department.go`).
2. Add it to the `Department` enum in `schema.graphqls` and to `departments` in `web/src/lib/validation.ts`.
3. Run `make generate`.

### Add a model provider

1. Implement `port.LanguageModel` in `internal/adapter/llm/`. Return `port.Transient(err, retryAfter)` for rate limits, overload, 5xx and network errors.
2. Add the provider to `domain.Provider` and `domain.Providers` (`internal/domain/model.go`) and to the `ModelProvider` enum in `schema.graphqls`.
3. Register it in `buildModels` (`internal/bootstrap/bootstrap.go`) behind its config, and add prices to `internal/adapter/llm/catalog.go`.
4. Run `make generate`.

If it speaks the OpenAI Chat Completions protocol, skip step 1 and reuse `NewOpenAICompat`.

### Change the GraphQL API

1. Edit `internal/adapter/graphql/schema.graphqls`.
2. Fields that need a lookup per object: mark `resolver: true` in `gqlgen.yml` and batch through the loaders in `loaders.go`.
3. `make generate` regenerates the Go server and the TypeScript types.
4. Implement new resolver stubs in `schema.resolvers.go` and mapping in `mapping.go`. Mutations return a payload with `userErrors` (use `userErrors(...)`).
5. Use the new fields from the dashboard through `graphql(...)`; `pnpm typecheck` catches mismatches.

### Add a database migration

1. Create `internal/adapter/postgres/migrations/00004_<what>.sql` (the next free number) with `-- +goose Up` and `-- +goose Down` sections.
2. `make migrate` applies it locally. Docker runs it in the `migrations` service; production runs it as a Cloud Run job before rollout.
3. Migrations run while the previous release still serves traffic, so make them backward compatible: add first, remove in a later release.

### Add an agent that reacts to a new event

1. Create the agent (dashboard **Hire an agent**) with the event type in **Triggers**, then activate it.
2. Locally, send the event with `pressroomctl dispatch <type> '<json>'`. In production the storefront publishes to the `storefront-events` topic with an `eventType` attribute.

### Tune the policies

The retirement targets are in `DefaultRetirementPolicy` (`internal/domain/policy.go`)
and the experiment rules in `DefaultComparisonPolicy` (`internal/domain/experiment.go`).
Both have table-driven tests; update them and [docs/agents.md](docs/agents.md) together.

---

## 10. Configuration reference

All settings are environment variables, read and validated at startup in
`internal/platform/config/config.go`. A missing or malformed value stops the
process with a list of every problem.

| Variable | Default | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | none (required) | PostgreSQL connection string |
| `PORT` | `8080` | HTTP port for the API or worker |
| `PRESSROOM_ENV` | `development` | `production` disables the playground and requires auth |
| `LOG_LEVEL`, `LOG_FORMAT` | `info`, `json` | `LOG_FORMAT=text` for local reading |
| `PRESSROOM_MODEL_MODE` | `auto` | `auto`, `sandbox` or `live` |
| `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `XAI_API_KEY` | empty | Provider credentials |
| `OPENAI_BASE_URL`, `XAI_BASE_URL` | public endpoints | Override for proxies |
| `OPEN_SOURCE_BASE_URL`, `OPEN_SOURCE_API_KEY` | empty | OpenAI-compatible server for open-weights models |
| `PRESSROOM_PRICEBOOK_JSON` | empty | Price overrides, e.g. `{"openai/gpt-5":{"input":1.25,"output":10}}` |
| `PRESSROOM_HOURLY_RATE_USD` | `32` | Labor cost used to value hours saved |
| `PRESSROOM_EVAL_WINDOW_DAYS` | `30` | Scorecard window for evaluations |
| `WORKER_CONCURRENCY` | `4` | Runs executed in parallel per worker |
| `WORKER_POLL_INTERVAL`, `WORKER_LEASE`, `WORKER_MAX_ATTEMPTS` | `1s`, `5m`, `6` | Queue behaviour |
| `PRESSROOM_API_TOKEN` | empty | Bearer token required by the API when set |
| `PRESSROOM_TRUST_IAP` | `false` | Take the operator from Google IAP headers |
| `PRESSROOM_CORS_ORIGINS` | `http://localhost:5173` | Browser origins allowed to call the API |
| `GRAPHQL_INTROSPECTION`, `GRAPHQL_COMPLEXITY_LIMIT` | `true`, `600` | API hardening |
| `SLACK_WEBHOOK_URL` | empty | Notifications and the `notify_team` tool |
| `IMAGE_STUDIO_URL`, `IMAGE_STUDIO_TOKEN` | empty | Real image service; a sandbox is used when empty |
| `GOOGLE_CLOUD_PROJECT` | empty | Links log lines to Cloud Trace |

Dashboard build variables: `VITE_GRAPHQL_URL` (default `/graphql`),
`VITE_API_TOKEN`, and `PRESSROOM_API` for the dev proxy target.

---

## 11. Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| `bind: address already in use` for port 3300 | Something else uses it. Run `WEB_PORT=3400 make up`. |
| `bind: address already in use` for port 8080 or 8081 | The Path B servers (`make dev-api`, `make dev-worker`) are running. Stop them before `make up`, or stay on Path B. |
| Port 5433 already in use | Pick another: `PRESSROOM_DB_PORT=5434 make db seed`. The Makefile derives `DATABASE_URL` from it. |
| Pressroom's Postgres logs `password authentication failed for user "<other>"` | Another project's app is connecting to Pressroom's database port. Pressroom uses 5433 so it never takes the usual 5432; if you moved it to 5432, move it back. Pressroom itself is unaffected. |
| Runs stay `QUEUED` | The worker is not running, or points at another database. Start `make dev-worker` and compare `DATABASE_URL`. |
| Runs fail with `provider ... is not configured` | `PRESSROOM_MODEL_MODE=live` without that provider's key. Use `auto`, or set the key. |
| Reorder Nudger runs retry and then fail | `OPEN_SOURCE_BASE_URL` points at a model server that is not running. Clear it or start the server. |
| `make seed` says the data is already loaded | Seeding is idempotent. Reset with `make down && make db seed`. |
| Dashboard shows a network or CORS error | The API is not on :8080, or the dashboard's origin is missing from `PRESSROOM_CORS_ORIGINS`. |
| CI fails on "generated code is current" | Run `make generate` and commit the result. |
| `compile: version "go1.26.0" does not match go tool version` | Your Go is older than go.mod and switched toolchains mid-build (seen with `go test -cover`). Install Go 1.26 or run without `-cover`. |
| Slow `pnpm install` or builds on Windows | The repository is under `/mnt/c`. Move it into the WSL filesystem. |

---

## 12. Conventions

- **Formatting and types:** `gofmt` for Go (`make fmt`); `pnpm typecheck` in strict mode for the dashboard.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org), for example
  `feat(tools): add carrier claim tool`. Subject in the imperative, lowercase,
  no period, at most 50 characters. Body optional, at most 2 lines.
- **Tests with every change:** domain rules get table-driven tests; adapters get
  tests against their real protocol or a real database.
- **Every commit builds** and keeps generated code in sync with the schema.

---

## 13. Deploying

Deployment to Google Cloud (Cloud Run, Cloud SQL, Pub/Sub, Cloud Scheduler,
Secret Manager) is Terraform plus Cloud Build. The three commands are in the
[README](README.md#deploying-to-google-cloud); `deploy/terraform/variables.tf`
documents every setting. Model API keys are created as Secret Manager secrets
by hand, so they never enter Terraform state.

---

## 14. Your first week

1. Run Path A and do the guided tour.
2. Run Path B and `make test-integration`.
3. Read the code in the order of section 8, with the ADR open.
4. Make a small change end to end: add a field to the GraphQL schema, show it
   in the dashboard, and cover it with a test.
5. Add a tool (section 9) and give it to an agent in the dashboard.
