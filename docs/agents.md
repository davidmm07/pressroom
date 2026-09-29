# The crew and the rules they work under

## Agents in the demo

| Agent | Department | Model | Triggered by | Tools | Replaces |
| --- | --- | --- | --- | --- | --- |
| Proof Checker | Prepress | Claude Opus 5 | `artwork.uploaded` | inspect_artwork, upscale_image, vectorize_artwork, remove_background, send_proof | 6 min of a prepress technician per upload |
| Support Triage | Customer experience | Claude Opus 5, trialling Grok 4 Fast | `ticket.created` | lookup_order, track_shipment, draft_reply, issue_refund*, notify_team | 7 min per ticket |
| Shipping Watch | Operations | GPT-5 mini | `shipment.stalled` | lookup_order, track_shipment, draft_reply, notify_team | 5 min per stalled parcel |
| Reorder Nudger | Marketing | Qwen3 32B, self-hosted | `customer.reorder_due` | lookup_customer, create_promo_code, queue_email* | 4 min per customer |
| Payout Reconciler | Finance | Grok 4 | none yet (draft) | lookup_order, notify_team | 20 min per payout |
| Damage Claim Assessor | Customer experience | Claude Opus 5 | `claim.submitted` | assess_damage_photo, order_reprint*, issue_refund*, check_reply_style, draft_reply, notify_team | 8 min per claim photo |
| Proof Revision Assistant | Prepress | Claude Sonnet 5 | `proof.changes_requested` | apply_proof_revision, send_proof | 10 min per revised proof |
| Listing Screener | Marketplace | Grok 4 Fast | `listing.submitted` | screen_listing, publish_listing, notify_team | 3 min per new design |
| Store Launch Coach | Marketplace | GPT-5 mini | `store.no_sales` | audit_store, queue_email* | 15 min per struggling store |
| Production Watch | Manufacturing | Claude Haiku 4.5 | `production.job_at_risk` | production_status, reroute_job*, notify_team | 12 min per late job |
| Review Defect Analyst | Manufacturing | Llama 3.3 70B, self-hosted | `reviews.weekly_digest` | fetch_reviews, file_defect_report, notify_team | an hour of reading reviews a week |
| Deal Planner | Marketing | Grok 4 | `capacity.weekly_forecast` | capacity_forecast, propose_deal* | 45 min of planning a week |
| Giveaway Screener | Marketing | Qwen3 32B, self-hosted | `giveaway.closed` | screen_giveaway_entries, notify_team | 30 min per giveaway |

\* needs human approval before it runs.

The first five are the founding crew with a month of history. The other
eight are new hires, each with one demo event waiting for it: four finish on
their own and four stop at an approval. Models are matched to the job: the
strongest model where a photo decides money, small fast models for high-volume
screening, and self-hosted open weights where the input is customers' personal
data (reviews, giveaway entries).

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
| `assess_damage_photo` | Vision model and claims policy | Proposes a remedy for at most the ordered quantity; unclear photos, and refunds above the agent limit, go to a person. |
| `order_reprint` | Factory | **Approval required**; never more units than were ordered. |
| `check_reply_style` | Support style guide (pure logic) | Flags apologies and negative phrasing, replies over 120 words and replies that skip the customer's name. |
| `apply_proof_revision` | Image studio | Only size, cut shape, border, rotation and background; shapes and sizes each product can take; text and color changes are queued for a designer. |
| `audit_store` | Marketplace | Read only. |
| `screen_listing` | Marketplace, protected marks register | Read only; flags protected brands and claims of a license. |
| `publish_listing` | Marketplace | Screens again itself and refuses anything flagged, whatever the agent was told. |
| `production_status`, `capacity_forecast` | Factory floor | Read only. |
| `reroute_job` | Factory floor | **Approval required**; only to another station that prints the product. |
| `propose_deal` | Storefront deals | **Approval required**; up to 7 days; refused below a 20% gross margin. |
| `fetch_reviews` | Review feed | Read only. |
| `file_defect_report` | Quality tickets | Routed to the team that owns the cause. |
| `screen_giveaway_entries` | Giveaways | Returns masked emails only, so the result is safe to post in chat. |

Every side-effecting tool derives an idempotency key from the run and call ID,
so a retried run never refunds twice.

A rule that must hold no matter what the model does belongs in the tool, not
the prompt: `publish_listing` is the clearest case, and scenario 09 hires an
agent told to publish everything to prove it.

The marketplace, factory floor, review feed and giveaways are small demo
tables (migration 00003) behind their own ports, `tools.Marketplace`,
`tools.Factory`, `tools.Reviews` and `tools.Giveaways`. Swapping one for the
real service is a new adapter; no tool changes.

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
