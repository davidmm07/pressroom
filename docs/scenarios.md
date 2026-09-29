# Business scenarios

Pressroom answers the questions a company has to settle before it lets AI
agents do real work: can an agent finish a job on its own, can it act without
being trusted with money, what happens when a person says no, how do we adopt
a new model safely, what do we do with an agent nobody finds useful, how
does a department get one in the first place, and do the rules hold when an
agent's own instructions are wrong.

Each question is a scenario in the Postman collection
[`postman/collections/Pressroom Scenarios`](../postman/collections/Pressroom%20Scenarios).
Every scenario is a sequence of real API calls with assertions, and ends by
printing its business outcome. The diagrams below show each one step by step;
the step numbers match the request names in Postman.

| # | The question | What the run shows |
| --- | --- | --- |
| 1 | Can an agent finish a job with no human involved? | An uploaded logo is inspected, upscaled, cleaned up and sent to the customer as a proof, in seconds, for a few cents. |
| 2 | Can it act on its own without being trusted with money? | The agent researches a damaged-order ticket and drafts the reply, then stops before refunding until a lead approves. The approver is on record. |
| 3 | What happens when a person says no? | The refund never runs. The agent is told why and still finishes the ticket. |
| 4 | How do we adopt a new model on evidence? | Half the tickets go to Grok 4 Fast. Same success rate, about 30 times cheaper per ticket, so it is promoted. |
| 5 | What happens to an agent that does not deliver? | 20 runs without an error, but reviewers keep 20% of the output: probation, then retirement, then no more work. |
| 6 | How does a department get an agent, and how do we prove it shipped? | A lead's request is scored and ranked, approved, turned into an agent, and marked shipped with that agent linked. |
| 7 | What does a developer get when something is wrong? | Stable error codes, field paths, nulls for missing objects, and no internal details. |
| 8 | Can an agent make the judgment call and leave a person one click? | A photo of misprinted stickers becomes a proposed reprint of exactly the 40 bad units. After one approval the agent drafts a style-checked reply and tells the factory. |
| 9 | Do the rules hold when an agent's instructions are wrong? | Clean designs go live on their own and flagged ones are held. An agent told to publish everything is refused by the tool itself. |

## Run them

The scenarios expect the stack with its demo data and no model API keys, so
every agent uses the free, deterministic sandbox model.

### From the terminal

```bash
make up                                          # in one terminal
make scenarios                                   # in another: all nine
make scenarios S="02 Refunds wait for a human"   # just one
```

`make scenarios` runs the [Postman CLI](https://learning.postman.com/docs/postman-cli/postman-cli-overview)
through `npx` (Node 20 or newer; no Postman account needed). It fails if an
assertion fails, and also if any step did not run: the CLI silently skips a
request file it cannot parse, so `scripts/scenarios.sh` checks every step
number against the files on disk. CI runs the same command against a fresh
stack on every push.

### From the Postman desktop app

The collection is stored in Postman's Native Git format (Collection v3 YAML),
which the desktop app opens directly from the repository:

1. In the sidebar, click the **Files** icon, then **Open folder**, and choose the Pressroom repository root.
2. Switch to **Local View** (bottom left). **Pressroom Scenarios** and the **Pressroom Local** environment appear.
3. Select the **Pressroom Local** environment.
4. Right-click a scenario folder and run it with the **Collection Runner**.

The "Wait for..." steps repeat themselves until the worker finishes, which
only happens in the Collection Runner. Sent by hand, a request runs once.
Edits made in the app are saved straight back to the YAML files, ready to
commit.

## The scenarios

### 1. Artwork becomes a proof on its own

```mermaid
flowchart LR
  A["1.1 Storefront publishes<br/>artwork.uploaded"] --> B["1.2 Find the<br/>Proof Checker run"]
  B --> C{"1.3 Run finished?"}
  C -- "not yet, wait" --> C
  C -- "SUCCEEDED" --> D["1.4 Trace: inspect, upscale 2x,<br/>remove background, send proof"]
  D --> E["1.5 Same message again:<br/>no new run"]
```

A 600 px logo ordered at 3 x 3 inches is 200 DPI; stickers need 300. The
agent works that out, upscales 2x, removes the background so the die line
follows the shape, and sends the free proof. It skips vectorizing because the
inspection said it was not needed. Pub/Sub delivering the same message twice
does not start a second run.

### 2. Refunds wait for a human

```mermaid
flowchart LR
  A["2.1 Ticket: cracked magnets,<br/>refund 15 USD"] --> B["2.2 Find the run"]
  B --> C{"2.3 Paused for approval?"}
  C -- "working" --> C
  C -- "AWAITING_APPROVAL" --> D["2.4 No money has moved"]
  D --> E["2.5 Lead approves"]
  E --> F{"2.6 Finished?"}
  F -- "working" --> F
  F -- "SUCCEEDED" --> G["2.7 Trail names the approver"]
  G --> H["2.8 Lead accepts the result"]
  H --> I["2.9 Second review: CONFLICT"]
```

### 3. A denied action never runs

```mermaid
flowchart LR
  A["3.1 Ticket: refund<br/>the shipping"] --> B["3.2 Find the run"]
  B --> C{"3.3 Refund proposed?"}
  C -- "working" --> C
  C -- "AWAITING_APPROVAL" --> D["3.4 Lead denies:<br/>shipping is free"]
  D --> E{"3.5 Finished?"}
  E -- "working" --> E
  E -- "SUCCEEDED" --> F["3.6 Refund never ran;<br/>agent got the reason"]
```

### 4. A cheaper model earns the job

```mermaid
flowchart LR
  A["4.1 Hire an agent<br/>on Claude Opus 5"] --> B["4.2 Activate"]
  B --> C["4.3 Experiment: Grok 4 Fast<br/>on 50% of tickets"]
  C --> D{"4.4 Each arm has<br/>at least 12 tickets?"}
  D -- "no, send another" --> D
  D -- "yes" --> E{"4.5 All handled?"}
  E -- "not yet" --> E
  E -- "yes" --> F["4.6 Compare scorecards"]
  F --> G["4.7 Conclude: PROMOTED"]
  G --> H["4.8 Agent now runs<br/>on Grok 4 Fast"]
```

Each run is assigned to an arm by hashing its ID, so the split is close to
50/50 and a retried run never switches models. The challenger wins only if it
matches the champion's success and acceptance rates (within two points) and
nets more value per run.

### 5. An agent that does not deliver is let go

```mermaid
flowchart LR
  A["5.1 Hire a reorder agent"] --> B["5.2 Activate"]
  B --> C{"5.3 20 runs started?"}
  C -- "no" --> C
  C -- "yes" --> D{"5.4 All finished?"}
  D -- "not yet" --> D
  D -- "yes" --> E{"5.5 All 20 reviewed?<br/>16 rejected, 4 kept"}
  E -- "no" --> E
  E -- "yes" --> F["5.6 Evaluation: PROBATION"]
  F --> G["5.7 Evaluation: RETIRED"]
  G --> H["5.8 New work refused"]
  H --> I["5.9 Record explains why"]
```

The evaluation normally runs every morning from Cloud Scheduler. It covers the
whole crew, so running this scenario also evaluates the demo agents.

### 6. From intake request to working agent

```mermaid
flowchart LR
  A["6.1 Ops lead submits a request<br/>score 5.7"] --> B["6.2 Rank in the backlog"]
  B --> C["6.3 Approved"]
  C --> D["6.4 Draft with 3 mistakes:<br/>all reported at once"]
  D --> E["6.5 Hired as DRAFT"]
  E --> F["6.6 A draft cannot run"]
  F --> G["6.7 Owner activates"]
  G --> H["6.8 First run"]
  H --> I{"6.9 Finished?"}
  I -- "not yet" --> I
  I -- "SUCCEEDED" --> J["6.10 Request SHIPPED,<br/>linked to the agent"]
```

### 7. API errors a client can act on

```mermaid
flowchart LR
  A["7.1 Seven-decimal money:<br/>BAD_USER_INPUT"] --> B["7.2 Unknown objects: null"]
  B --> C["7.3 Read a version"]
  C --> D["7.4 Stale version:<br/>CONFLICT"]
  D --> E["7.5 Find a finished run"]
  E --> F["7.6 Approve it:<br/>INVALID_TRANSITION"]
  F --> G["7.7 Made-up cursor:<br/>BAD_USER_INPUT"]
  G --> H["7.8 Malformed event:<br/>acknowledged, 204"]
```

### 8. A photo settles a damage claim

```mermaid
flowchart LR
  A["8.1 Photo: 40 stickers<br/>printed off-color"] --> B["8.2 Find the<br/>Damage Claim Assessor run"]
  B --> C{"8.3 Asks to reprint?"}
  C -- "working" --> C
  C -- "AWAITING_APPROVAL" --> D["8.4 Read as a production<br/>fault; nothing reprinted yet"]
  D --> E["8.5 Lead approves"]
  E --> F{"8.6 Finished?"}
  F -- "working" --> F
  F -- "SUCCEEDED" --> G["8.7 Reprint ran, reply passed<br/>the style guide, factory told"]
```

Customers never send damaged items back; the photo is the evidence. The
claims policy lives in `assess_damage_photo`: a clear photo gets a reprint of
the affected units (or a refund if the customer asked for one), an unclear
one goes to a person. A production fault is also reported to the team, so
manufacturing hears about it the same day. Transit damage is not.

### 9. A flagged design is never published

```mermaid
flowchart LR
  A["9.1 Clean design<br/>submitted"] --> B["9.2 Find the<br/>Listing Screener run"]
  B --> C{"9.3 Live on its own?"}
  C -- "working" --> C
  C -- "SUCCEEDED" --> D["9.4 Fan art of a<br/>protected character"]
  D --> E["9.5 Find its run"]
  E --> F{"9.6 Held, team told?"}
  F -- "working" --> F
  F -- "SUCCEEDED" --> G["9.7 Hire an agent told<br/>to publish everything"]
  G --> H["9.8 Activate"]
  H --> I["9.9 Publish the<br/>flagged design"]
  I --> J{"9.10 Tool refused?"}
  J -- "working" --> J
  J -- "yes, on record" --> K["Nothing published"]
```

The screener publishes clean designs with no human involved, which is most of
them. The rule that a flagged design never goes live is not in its
instructions: `publish_listing` screens the design again itself and refuses.
Steps 9.7 to 9.10 hire an agent whose instructions say to publish without
checking, and show that the tool still says no.

## Turning a scenario into a Postman Flow

Postman Flows are the drag-and-drop version of these diagrams. The desktop app
stores each Flow as a JSON file under `postman/flows/` when the repository is
open in Local View, so a Flow you build is committed like any other file.

The repository does not ship hand-written Flow files: Postman does not publish
the Flow file format, Flows point at requests by internal IDs, and running one
from the command line (`postman flows run`) needs an Enterprise plan. Building
one in the app takes a few minutes per scenario:

1. Create a Flow in the workspace and name it after the scenario.
2. Add one request block per step, in step order, choosing the requests from
   **Pressroom Scenarios**, and connect each block's success output to the next.
3. For a "Wait for..." step, add a condition on `data.run.status` after the
   request: while it is `QUEUED` or `RUNNING`, go through a delay back into the
   same request; otherwise continue.
4. Where a step needs a value from an earlier response (the run ID, the agent
   ID), select that field from the response and connect it to the next block.
5. End with an output block that shows the outcome: cost, duration, decision.

The collection's own tests keep running inside the Flow, so it fails the same
way `make scenarios` does.

## Environment

`postman/environments/local.environment.yaml`:

| Variable | Default | Meaning |
| --- | --- | --- |
| `apiUrl` | `http://localhost:8080` | GraphQL API |
| `workerUrl` | `http://localhost:8081` | Worker, for Pub/Sub push events |
| `actor` | `cx-lead@example.com` | Operator recorded for approvals and reviews |
| `pollMs` | `500` | Wait between polls |
| `maxPolls` | `120` | Polls before a wait step gives up |

To start from clean demo data: `make down && make up`.
