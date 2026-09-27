# GraphQL API

Endpoint: `POST /graphql`. The schema is
[schema.graphqls](../internal/adapter/graphql/schema.graphqls); in development
GraphiQL is served at `/` and introspection is on.

## Authentication

| Deployment | How the caller is identified |
| --- | --- |
| Local development | No token. `X-Pressroom-Actor: you@example.com` names the operator; it defaults to `operator@pressroom.local`. |
| Token | `PRESSROOM_API_TOKEN` set: `Authorization: Bearer <token>` is required, and the dashboard names the operator with `X-Pressroom-Actor`. |
| Identity-Aware Proxy | `PRESSROOM_TRUST_IAP=true`: the operator comes from IAP's verified `X-Goog-Authenticated-User-Email`, and `X-Pressroom-Actor` is ignored. |

The operator's email is what the audit trail records for approvals, denials
and reviews. It is never taken from mutation arguments.

## Conventions

### Mutations return payloads

Every mutation returns a payload with the changed object and `userErrors`:

```graphql
type AgentPayload {
  agent: Agent
  userErrors: [UserError!]!
}

type UserError {
  field: [String!]!   # path into the input, e.g. ["input", "tools", "1"]
  code: UserErrorCode!
  message: String!
}
```

`userErrors` carries everything the client can fix, all at once:

| Code | Meaning |
| --- | --- |
| `REQUIRED`, `TOO_LONG`, `OUT_OF_RANGE`, `INVALID_FORMAT`, `INVALID_VALUE` | A field failed validation. |
| `NOT_FOUND` | The referenced agent, run or experiment does not exist. |
| `CONFLICT` | Stale `expectedVersion`, a duplicate slug, a second review, a second experiment. |
| `INVALID_TRANSITION` | The object's state forbids the change, e.g. approving a run that is not waiting. |

### Top-level errors are for everything else

| `extensions.code` | When |
| --- | --- |
| `BAD_USER_INPUT` | A query argument or custom scalar is invalid (a malformed cursor, a `USD` value with seven decimals). |
| `NOT_FOUND`, `CONFLICT`, `INVALID_TRANSITION` | The same domain errors, raised from a query field. |
| `TIMEOUT` | The request took longer than 30 seconds. |
| `UNAUTHENTICATED` | Missing or wrong bearer token (HTTP 401). |
| `INTERNAL` | Anything unexpected. The message is always "internal error" and `extensions.requestId` matches the server logs. |

Looking up something that does not exist by ID or slug returns `null`, not an
error.

### Validation happens in layers

1. **Dashboard**: zod schemas give instant feedback.
2. **GraphQL types and scalars**: enums, non-null fields, `USD` precision.
3. **Domain**: every rule, every field, one pass. This is the source of truth.
4. **PostgreSQL**: check constraints and unique indexes for anything that bypasses the application.
5. **Tools**: arguments a model produces are validated against the tool's
   JSON Schema before the tool runs. Violations go back to the model as tool
   errors so it can correct itself; they never reach a real system.

### Money

Amounts use the `USD` scalar: a decimal string with up to six places
(`"0.012500"`). Inputs accept a string or a number but reject more than six
decimals instead of rounding. Storage is integer micro-dollars.

### Pagination

Runs are a Relay-style connection, newest first:

```graphql
{ runs(agentId: "...", first: 20, after: "<endCursor>") {
    edges { cursor node { id status } }
    pageInfo { hasNextPage endCursor }
} }
```

Cursors are opaque. Today they wrap a time-ordered UUIDv7, so each page is an
index range scan.

### Idempotency and concurrency

- `startRun(input: { idempotencyKey })`: retrying with the same key returns
  the first run. Pub/Sub message IDs are used the same way for events.
- `updateAgent(input: { expectedVersion })`: fails with `CONFLICT` if someone
  else saved first. Every agent query returns `version`.

### Limits

Queries above a complexity of 600 are rejected before execution, request
bodies are capped at 1 MiB, and requests time out after 30 seconds. Lists
that can grow without bound are paginated.

## Examples

Crew overview:

```graphql
query Crew {
  crew { activeAgents awaitingApproval hoursSavedLast30Days spendLast30Days netValueLast30Days }
  agents(status: [ACTIVE, PROBATION]) {
    slug status model { id }
    scorecard(windowDays: 30) { finishedRuns successRate acceptanceRate costPerRun netValue }
  }
}
```

Start a run and read its trace:

```graphql
mutation {
  startRun(input: {
    agentSlug: "support-triage",
    input: { ticketId: "T-9001", orderId: "SM-1046", message: "Where are my buttons?" },
    idempotencyKey: "ticket-T-9001"
  }) {
    run { id status }
    userErrors { field code message }
  }
}

query Run($id: ID!) {
  run(id: $id) {
    status output cost usage { inputTokens cachedInputTokens outputTokens }
    pendingToolCall { tool { name } arguments }
    steps { kind summary latencyMs cost }
  }
}
```

Approve the pending refund, then review the result:

```graphql
mutation { approveToolCall(runId: "...") { run { status } userErrors { code message } } }
mutation { reviewRun(input: { runId: "...", verdict: ACCEPTED }) { run { review { verdict reviewer } } userErrors { code message } } }
```

Trial a cheaper model and conclude when both arms have data:

```graphql
mutation {
  startExperiment(input: {
    agentId: "...", challenger: "xai/grok-4-fast", trafficPercent: 20,
    hypothesis: "Same quality for a fraction of the cost"
  }) { experiment { id } userErrors { field code message } }
}

mutation { concludeExperiment(id: "...") { experiment { status outcome } userErrors { code message } } }
```

Validation errors come back together, with paths into the input:

```json
{
  "data": {
    "createAgent": {
      "agent": null,
      "userErrors": [
        { "field": ["input", "slug"], "code": "INVALID_FORMAT", "message": "must be 3-40 lowercase letters, digits or dashes, starting with a letter" },
        { "field": ["input", "maxSteps"], "code": "OUT_OF_RANGE", "message": "must be between 1 and 50" },
        { "field": ["input", "tools", "1"], "code": "INVALID_VALUE", "message": "unknown tool \"launch_rockets\"" }
      ]
    }
  }
}
```

With curl:

```bash
curl -s localhost:8080/graphql -H 'Content-Type: application/json' \
  -H 'X-Pressroom-Actor: cx-lead@example.com' \
  -d '{"query":"{ crew { activeAgents awaitingApproval } }"}'
```

## Events and jobs (worker)

The worker is private on Cloud Run. Pub/Sub and Cloud Scheduler call it with
OIDC tokens that Cloud Run verifies before the request reaches the code.

| Endpoint | Caller | Behaviour |
| --- | --- | --- |
| `POST /events/pubsub` | Pub/Sub push | Starts a run for every agent on duty whose `triggers` include the message's `eventType` attribute. Returns 2xx to ack, 5xx to have Pub/Sub retry; malformed messages are acked and logged because redelivery cannot fix them. |
| `POST /jobs/evaluate` | Cloud Scheduler, daily | Applies the retirement policy to the crew. |
| `GET /healthz`, `GET /readyz` | Cloud Run probes | Process up; database reachable. |

Event types in use: `artwork.uploaded`, `ticket.created`, `shipment.stalled`,
`customer.reorder_due`.
