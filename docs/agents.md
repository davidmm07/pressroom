# The crew and the rules they work under

## Agents in the demo

| Agent | Department | Model | Triggered by | Tools | Replaces |
| --- | --- | --- | --- | --- | --- |
| Proof Checker | Prepress | Claude Opus 5 | `artwork.uploaded` | inspect_artwork, upscale_image, vectorize_artwork, remove_background, send_proof | 6 min of a prepress technician per upload |
| Support Triage | Customer experience | Claude Opus 5, trialling Grok 4 Fast | `ticket.created` | lookup_order, track_shipment, draft_reply, issue_refund*, notify_team | 7 min per ticket |
| Shipping Watch | Operations | GPT-5 mini | `shipment.stalled` | lookup_order, track_shipment, draft_reply, notify_team | 5 min per stalled parcel |
| Reorder Nudger | Marketing | Qwen3 32B, self-hosted | `customer.reorder_due` | lookup_customer, create_promo_code, queue_email* | 4 min per customer |
| Payout Reconciler | Finance | Grok 4 | none yet (draft) | lookup_order, notify_team | 20 min per payout |

\* needs human approval before it runs.

Each agent's instructions are its job description, written by the owning
lead. Pressroom adds shared operating rules to every system prompt: use tools
for facts, never invent order data, do not retry a denied action, finish with
a summary a teammate can act on.

## Tools

| Tool | System | Guardrail |
| --- | --- | --- |
| `inspect_artwork` | Prepress rules (pure logic) | Minimum DPI per product (300 for stickers, labels, magnets, buttons, packaging; 150 for t-shirts), aspect ratio, transparent background for die-cut products. |
| `upscale_image`, `vectorize_artwork`, `remove_background` | Image studio (the same tools customers use) | Upscale factor limited to 2 to 4. |
| `send_proof` | Storefront | Order number format; idempotent per call. |
| `lookup_order`, `lookup_customer` | Storefront read replica | Read only. |
| `track_shipment` | Carrier API | Read only. |
| `draft_reply` | Helpdesk | Drafts only; a support agent sends. |
| `issue_refund` | Payments | **Approval required**; at most $500 by schema; never more than the order total. |
| `create_promo_code` | Storefront | At most 20% off, enforced by the schema so no prompt can raise it. |
| `queue_email` | Email service | **Approval required**. |
| `notify_team` | Slack incoming webhook | Degrades to "not delivered" when no webhook is configured. |

Every side-effecting tool derives an idempotency key from the run and call ID,
so a retried run never refunds twice.

## Budgets

Each agent has a step budget (model turns) and a cost budget per run. The
executor checks both before every model call and fails the run with a clear
reason when either is spent. Unknown models are priced pessimistically so a
budget still bites before anyone updates the price book.

## Scorecards

Computed over a rolling window (30 days by default) from finished runs:

| Metric | Definition |
| --- | --- |
| Success rate | Succeeded / (succeeded + failed). Cancelled and in-flight runs do not count. |
| Acceptance rate | Of reviewed runs: accepted = 1, edited = 0.5, rejected = 0. Null until something is reviewed. |
| Hours saved | Succeeded runs x acceptance (0.5 before any review) x minutes saved per run / 60. |
| Value delivered | Hours saved x the loaded hourly rate (`PRESSROOM_HOURLY_RATE_USD`). |
| Net value | Value delivered minus model spend. |

The pessimistic 0.5 prior means an agent earns credit for saving time only as
people confirm its output is usable.

## Retirement policy

An agent is **delivering** when, over at least 20 finished runs:

- success rate is at least 85%,
- acceptance is at least 70% (judged once 10 runs are reviewed),
- net value is positive.

Evaluation runs daily (Cloud Scheduler) or on demand:

| Status | Delivering | Not delivering |
| --- | --- | --- |
| Active | keep | **probation** |
| Probation | **reinstate** | **retire** |

Agents on probation keep working so the next evaluation has fresh evidence.
Every decision is stored with the scorecard and the reasons, and status
changes are posted to Slack with the owning lead's name.

## Model experiments

To adopt a new model on evidence rather than launch-day benchmarks, an
experiment sends 1 to 50% of an agent's runs to a challenger. The split hashes
the run ID, so a retried run never changes models midway. Concluding compares
the arms since the experiment started:

- each arm needs 10 finished runs, otherwise the result is inconclusive
  (it can be closed early, keeping the champion);
- the challenger may trail by at most 2 points on success and on acceptance;
- it must deliver more net value per run.

If it passes, it becomes the agent's model in the same transaction that closes
the experiment.

## Intake scoring

Department leads describe work an agent could take over. Each request is
scored:

```
hours per week = weekly volume x minutes per task / 60
feasibility    = max(0.3, 1 - sensitivity penalty - error-cost penalty)
                 penalties: low 0, medium 0.15, high 0.35
score          = hours per week x feasibility
```

The backlog is sorted by score. Shipping a request links it to the agent that
now does the work, which closes the loop between what leads asked for and what
runs.
