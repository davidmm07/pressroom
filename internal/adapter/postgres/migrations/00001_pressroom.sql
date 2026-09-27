-- +goose Up

-- The crew. Check constraints repeat the domain's limits so data written by
-- anything other than the application (a backfill, a console) stays valid.
CREATE TABLE agents (
    id                    uuid PRIMARY KEY,
    slug                  text NOT NULL UNIQUE,
    name                  text NOT NULL,
    description           text NOT NULL DEFAULT '',
    department            text NOT NULL,
    owner_email           text NOT NULL,
    instructions          text NOT NULL,
    model_provider        text NOT NULL,
    model_name            text NOT NULL,
    tools                 text[] NOT NULL,
    triggers              text[] NOT NULL DEFAULT '{}',
    max_steps             integer NOT NULL CHECK (max_steps BETWEEN 1 AND 50),
    max_cost_micros       bigint NOT NULL CHECK (max_cost_micros > 0),
    minutes_saved_per_run double precision NOT NULL DEFAULT 0,
    status                text NOT NULL CHECK (status IN ('DRAFT', 'ACTIVE', 'PROBATION', 'RETIRED')),
    version               integer NOT NULL DEFAULT 1,
    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL
);

-- Event dispatch asks "which agents listen to ticket.created?"
CREATE INDEX agents_triggers_idx ON agents USING gin (triggers);

CREATE TABLE experiments (
    id                  uuid PRIMARY KEY,
    agent_id            uuid NOT NULL REFERENCES agents (id),
    champion_provider   text NOT NULL,
    champion_name       text NOT NULL,
    challenger_provider text NOT NULL,
    challenger_name     text NOT NULL,
    traffic_percent     integer NOT NULL CHECK (traffic_percent BETWEEN 1 AND 50),
    hypothesis          text NOT NULL,
    status              text NOT NULL CHECK (status IN ('RUNNING', 'PROMOTED', 'REJECTED')),
    outcome             text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL,
    concluded_at        timestamptz
);

-- One running experiment per agent, guaranteed even under concurrent requests.
CREATE UNIQUE INDEX experiments_one_running_idx ON experiments (agent_id) WHERE status = 'RUNNING';

CREATE TABLE runs (
    id                  uuid PRIMARY KEY,
    agent_id            uuid NOT NULL REFERENCES agents (id),
    model_provider      text NOT NULL,
    model_name          text NOT NULL,
    experiment_id       uuid REFERENCES experiments (id),
    variant             text NOT NULL CHECK (variant IN ('CHAMPION', 'CHALLENGER')),
    trigger             text NOT NULL,
    input               jsonb NOT NULL,
    idempotency_key     text,
    status              text NOT NULL,
    transcript          jsonb NOT NULL,
    pending             jsonb,
    output              text NOT NULL DEFAULT '',
    failure_reason      text NOT NULL DEFAULT '',
    turns               integer NOT NULL DEFAULT 0,
    input_tokens        bigint NOT NULL DEFAULT 0,
    cached_input_tokens bigint NOT NULL DEFAULT 0,
    output_tokens       bigint NOT NULL DEFAULT 0,
    cost_micros         bigint NOT NULL DEFAULT 0,
    review_verdict      text CHECK (review_verdict IN ('ACCEPTED', 'EDITED', 'REJECTED')),
    reviewer            text,
    review_note         text,
    reviewed_at         timestamptz,
    version             integer NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL,
    started_at          timestamptz,
    finished_at         timestamptz,
    CONSTRAINT runs_idempotency_key_unique UNIQUE (agent_id, idempotency_key)
);

-- UUIDv7 ids sort by time, so "newest first" pages are index scans on id.
CREATE INDEX runs_agent_idx ON runs (agent_id, id DESC);
CREATE INDEX runs_status_idx ON runs (status, id DESC);
-- Covering index for scorecards: the aggregate never touches the heap.
CREATE INDEX runs_stats_idx ON runs (agent_id, created_at)
    INCLUDE (status, review_verdict, cost_micros, experiment_id, variant, started_at, finished_at);

-- Append-only audit trail of every model turn, tool call and approval.
CREATE TABLE run_steps (
    run_id      uuid NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    idx         integer NOT NULL,
    kind        text NOT NULL,
    tool_name   text NOT NULL DEFAULT '',
    summary     text NOT NULL,
    detail      jsonb NOT NULL DEFAULT '{}',
    latency_ms  bigint NOT NULL DEFAULT 0,
    cost_micros bigint NOT NULL DEFAULT 0,
    at          timestamptz NOT NULL,
    PRIMARY KEY (run_id, idx)
);

-- Work queue consumed with SELECT ... FOR UPDATE SKIP LOCKED.
CREATE TABLE run_jobs (
    run_id       uuid PRIMARY KEY REFERENCES runs (id) ON DELETE CASCADE,
    available_at timestamptz NOT NULL,
    attempts     integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    last_error   text NOT NULL DEFAULT ''
);
CREATE INDEX run_jobs_due_idx ON run_jobs (available_at);

CREATE TABLE evaluations (
    id            uuid PRIMARY KEY,
    agent_id      uuid NOT NULL REFERENCES agents (id),
    decision      text NOT NULL,
    reason        text NOT NULL,
    status_before text NOT NULL,
    status_after  text NOT NULL,
    scorecard     jsonb NOT NULL,
    created_at    timestamptz NOT NULL
);
CREATE INDEX evaluations_agent_idx ON evaluations (agent_id, created_at DESC);

CREATE TABLE opportunities (
    id               uuid PRIMARY KEY,
    title            text NOT NULL,
    problem          text NOT NULL,
    department       text NOT NULL,
    submitted_by     text NOT NULL,
    weekly_volume    integer NOT NULL CHECK (weekly_volume > 0),
    minutes_per_task double precision NOT NULL CHECK (minutes_per_task > 0),
    data_sensitivity text NOT NULL,
    error_cost       text NOT NULL,
    status           text NOT NULL,
    agent_id         uuid REFERENCES agents (id),
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);

-- +goose Down
DROP TABLE opportunities;
DROP TABLE evaluations;
DROP TABLE run_jobs;
DROP TABLE run_steps;
DROP TABLE runs;
DROP TABLE experiments;
DROP TABLE agents;
