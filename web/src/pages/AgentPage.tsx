import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { useMutation, useQuery } from 'urql';
import { Badge, Empty, ErrorBanner, Field, PageHeader, Tile } from '../components';
import { graphql } from '../gql';
import { formatDuration, formatHours, formatPercent, formatUSD, humanize, timeAgo } from '../lib/format';
import { experimentSchema, jsonObject, serverFieldErrors, zodFieldErrors, type FieldErrors } from '../lib/validation';

const AgentQuery = graphql(`
  query Agent($slug: String!) {
    agent(slug: $slug) {
      id
      slug
      name
      description
      department
      owner
      instructions
      status
      version
      triggers
      minutesSavedPerRun
      model {
        id
      }
      budget {
        maxSteps
        maxCost
      }
      tools {
        name
        description
        requiresApproval
      }
      scorecard(windowDays: 30) {
        finishedRuns
        successRate
        reviewed
        acceptanceRate
        totalCost
        costPerRun
        hoursSaved
        valueDelivered
        netValue
        avgDurationSeconds
      }
      activeExperiment {
        id
        trafficPercent
        hypothesis
        createdAt
        champion {
          id
        }
        challenger {
          id
        }
        championScorecard {
          finishedRuns
          successRate
          acceptanceRate
          costPerRun
          netValue
        }
        challengerScorecard {
          finishedRuns
          successRate
          acceptanceRate
          costPerRun
          netValue
        }
      }
      evaluations(last: 5) {
        id
        decision
        reason
        statusBefore
        statusAfter
        createdAt
      }
      runs(first: 15) {
        edges {
          node {
            id
            status
            trigger
            variant
            cost
            createdAt
            durationSeconds
            model {
              id
            }
            review {
              verdict
            }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
    models {
      model {
        id
      }
      label
      available
      outputPerMTok
    }
  }
`);

const ActivateAgent = graphql(`
  mutation ActivateAgent($id: ID!) {
    activateAgent(id: $id) {
      agent {
        id
        status
      }
      userErrors {
        message
      }
    }
  }
`);

const RetireAgent = graphql(`
  mutation RetireAgent($id: ID!, $reason: String!) {
    retireAgent(id: $id, reason: $reason) {
      agent {
        id
        status
      }
      userErrors {
        message
      }
    }
  }
`);

const StartRun = graphql(`
  mutation StartRun($input: StartRunInput!) {
    startRun(input: $input) {
      run {
        id
      }
      userErrors {
        field
        code
        message
      }
    }
  }
`);

const StartExperiment = graphql(`
  mutation StartExperiment($input: StartExperimentInput!) {
    startExperiment(input: $input) {
      experiment {
        id
      }
      userErrors {
        field
        code
        message
      }
    }
  }
`);

const ConcludeExperiment = graphql(`
  mutation ConcludeExperiment($id: ID!, $force: Boolean) {
    concludeExperiment(id: $id, force: $force) {
      experiment {
        id
        status
        outcome
      }
      userErrors {
        message
      }
    }
  }
`);

export function AgentPage() {
  const { slug = '' } = useParams();
  const navigate = useNavigate();
  const [{ data, error, fetching }, refetch] = useQuery({ query: AgentQuery, variables: { slug } });
  const [activation, activate] = useMutation(ActivateAgent);
  const [retirement, retire] = useMutation(RetireAgent);
  const [conclusion, conclude] = useMutation(ConcludeExperiment);
  const [outcome, setOutcome] = useState<string | null>(null);

  if (fetching && !data) return <p className="muted">Loading...</p>;
  if (error) return <ErrorBanner error={error} />;
  const agent = data?.agent;
  if (!agent) return <Empty>No agent called "{slug}".</Empty>;

  const card = agent.scorecard;
  const onDuty = agent.status === 'ACTIVE' || agent.status === 'PROBATION';
  const exp = agent.activeExperiment;
  const mutationError =
    activation.error ?? retirement.error ?? conclusion.error ??
    activation.data?.activateAgent.userErrors[0]?.message ??
    retirement.data?.retireAgent.userErrors[0]?.message ??
    conclusion.data?.concludeExperiment.userErrors[0]?.message;

  const onRetire = async () => {
    const reason = window.prompt(`Why retire ${agent.name}? This cannot be undone.`);
    if (reason) await retire({ id: agent.id, reason });
  };

  const onConclude = async (force: boolean) => {
    const result = await conclude({ id: exp!.id, force });
    const done = result.data?.concludeExperiment.experiment;
    if (done) {
      setOutcome(`${humanize(done.status)}: ${done.outcome}`);
      refetch({ requestPolicy: 'network-only' });
    }
  };

  return (
    <>
      <PageHeader
        title={
          <>
            {agent.name} <Badge value={agent.status} />
          </>
        }
        subtitle={`${humanize(agent.department)} · owned by ${agent.owner} · ${agent.description}`}
        actions={
          <>
            {agent.status === 'DRAFT' && (
              <button className="button" onClick={() => activate({ id: agent.id })}>
                Activate
              </button>
            )}
            {agent.status !== 'RETIRED' && (
              <button className="button button-danger" onClick={onRetire}>
                Retire
              </button>
            )}
          </>
        }
      />
      <ErrorBanner error={mutationError} />
      {outcome && <div className="banner banner-info">{outcome}</div>}

      <section className="tiles">
        <Tile label="Runs (30 days)" value={card.finishedRuns} hint={`avg ${formatDuration(card.avgDurationSeconds)}`} />
        <Tile label="Success" value={formatPercent(card.finishedRuns ? card.successRate : null)} hint="target 85%" />
        <Tile label="Accepted" value={formatPercent(card.acceptanceRate)} hint={`${card.reviewed} reviewed · target 70%`} />
        <Tile label="Hours saved" value={formatHours(card.hoursSaved)} hint={`${agent.minutesSavedPerRun} min per accepted run`} />
        <Tile label="Spend" value={formatUSD(card.totalCost)} hint={`${formatUSD(card.costPerRun)} per run`} />
        <Tile label="Net value" value={formatUSD(card.netValue)} tone={Number(card.netValue) >= 0 ? 'good' : 'bad'} />
      </section>

      <div className="columns">
        <section className="card">
          <h2>Model</h2>
          <p>
            <code>{agent.model.id}</code> · budget {agent.budget.maxSteps} steps, {formatUSD(agent.budget.maxCost)} per run
          </p>
          {exp ? (
            <>
              <h3>Experiment: {exp.challenger.id} on {exp.trafficPercent}% of runs</h3>
              <p className="muted">
                {exp.hypothesis} · started {timeAgo(exp.createdAt)}
              </p>
              <table className="table compact">
                <thead>
                  <tr>
                    <th>Arm</th>
                    <th className="num">Runs</th>
                    <th className="num">Success</th>
                    <th className="num">Accepted</th>
                    <th className="num">Cost / run</th>
                    <th className="num">Net value</th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    { arm: `Champion · ${exp.champion.id}`, c: exp.championScorecard },
                    { arm: `Challenger · ${exp.challenger.id}`, c: exp.challengerScorecard },
                  ].map(({ arm, c }) => (
                    <tr key={arm}>
                      <td>{arm}</td>
                      <td className="num">{c.finishedRuns}</td>
                      <td className="num">{formatPercent(c.finishedRuns ? c.successRate : null)}</td>
                      <td className="num">{formatPercent(c.acceptanceRate)}</td>
                      <td className="num">{formatUSD(c.costPerRun)}</td>
                      <td className="num">{formatUSD(c.netValue)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <div className="actions">
                <button className="button" onClick={() => onConclude(false)} disabled={conclusion.fetching}>
                  Conclude experiment
                </button>
                <button className="button button-quiet" onClick={() => onConclude(true)} disabled={conclusion.fetching}>
                  Stop early, keep champion
                </button>
              </div>
            </>
          ) : (
            onDuty && <ExperimentForm agentId={agent.id} currentModel={agent.model.id} models={data.models} onStarted={() => refetch({ requestPolicy: 'network-only' })} />
          )}
        </section>

        <section className="card">
          <h2>Run it now</h2>
          {onDuty ? (
            <RunForm agentId={agent.id} onStarted={(id) => navigate(`/runs/${id}`)} />
          ) : (
            <p className="muted">Only agents on duty can run.</p>
          )}
          <h3>Tools</h3>
          <ul className="tools">
            {agent.tools.map((t) => (
              <li key={t.name}>
                <code>{t.name}</code> {t.requiresApproval && <span className="badge badge-warn">needs approval</span>}
                <div className="muted small">{t.description}</div>
              </li>
            ))}
          </ul>
          {agent.triggers.length > 0 && (
            <p className="small">
              Starts automatically on {agent.triggers.map((t) => <code key={t}>{t}</code>)}
            </p>
          )}
        </section>
      </div>

      <section className="card">
        <h2>Recent runs</h2>
        {agent.runs.edges.length === 0 ? (
          <Empty>No runs yet.</Empty>
        ) : (
          <table className="table compact">
            <thead>
              <tr>
                <th>Run</th>
                <th>Status</th>
                <th>Trigger</th>
                <th>Model</th>
                <th className="num">Cost</th>
                <th className="num">Duration</th>
                <th>Review</th>
              </tr>
            </thead>
            <tbody>
              {agent.runs.edges.map(({ node: r }) => (
                <tr key={r.id}>
                  <td>
                    <Link to={`/runs/${r.id}`}>{timeAgo(r.createdAt)}</Link>
                  </td>
                  <td>
                    <Badge value={r.status} />
                  </td>
                  <td>{humanize(r.trigger)}</td>
                  <td>
                    <code>{r.model.id}</code>
                    {r.variant === 'CHALLENGER' && <span className="small experiment-tag"> challenger</span>}
                  </td>
                  <td className="num">{formatUSD(r.cost)}</td>
                  <td className="num">{formatDuration(r.durationSeconds)}</td>
                  <td>{r.review ? <Badge value={r.review.verdict} /> : <span className="muted">-</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <div className="columns">
        <section className="card">
          <h2>Evaluations</h2>
          {agent.evaluations.length === 0 ? (
            <Empty>Not evaluated yet. Evaluations run daily, or from the Crew page.</Empty>
          ) : (
            <ul className="timeline">
              {agent.evaluations.map((e) => (
                <li key={e.id}>
                  <Badge value={e.decision} /> <span className="muted small">{timeAgo(e.createdAt)}</span>
                  <div className="small">
                    {e.reason}
                    {e.statusBefore !== e.statusAfter && ` (${humanize(e.statusBefore)} to ${humanize(e.statusAfter)})`}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section className="card">
          <h2>Instructions</h2>
          <pre className="prose">{agent.instructions}</pre>
        </section>
      </div>
    </>
  );
}

function RunForm({ agentId, onStarted }: { agentId: string; onStarted: (id: string) => void }) {
  const [input, setInput] = useState('{\n  "orderId": "SM-1042"\n}');
  const [errors, setErrors] = useState<FieldErrors>({});
  const [result, startRun] = useMutation(StartRun);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const parsed = jsonObject.safeParse(input);
    if (!parsed.success) return setErrors({ input: zodFieldErrors(parsed.error)._ ?? 'invalid' });
    setErrors({});
    const res = await startRun({ input: { agentId, input: parsed.data, idempotencyKey: crypto.randomUUID() } });
    const payload = res.data?.startRun;
    if (payload?.run) onStarted(payload.run.id);
    else if (payload) setErrors(serverFieldErrors(payload.userErrors));
  };

  return (
    <form onSubmit={submit} className="form">
      <Field label="Task input (JSON object)" error={errors.input ?? errors._}>
        <textarea rows={5} value={input} onChange={(e) => setInput(e.target.value)} spellCheck={false} className="mono" />
      </Field>
      <ErrorBanner error={result.error} />
      <button className="button" disabled={result.fetching}>
        Start run
      </button>
    </form>
  );
}

function ExperimentForm({
  agentId,
  currentModel,
  models,
  onStarted,
}: {
  agentId: string;
  currentModel: string;
  models: { model: { id: string }; label: string; available: boolean; outputPerMTok: number }[];
  onStarted: () => void;
}) {
  const [form, setForm] = useState({ challenger: '', trafficPercent: '20', hypothesis: '' });
  const [errors, setErrors] = useState<FieldErrors>({});
  const [result, start] = useMutation(StartExperiment);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const parsed = experimentSchema.safeParse(form);
    if (!parsed.success) return setErrors(zodFieldErrors(parsed.error));
    setErrors({});
    const res = await start({ input: { agentId, ...parsed.data } });
    const payload = res.data?.startExperiment;
    if (payload?.experiment) onStarted();
    else if (payload) setErrors(serverFieldErrors(payload.userErrors));
  };

  return (
    <form onSubmit={submit} className="form">
      <h3>Trial a challenger model</h3>
      <p className="muted small">A share of runs goes to the challenger. Conclude once both arms have 10 runs to adopt it on evidence.</p>
      <Field label="Challenger" error={errors.challenger ?? errors._}>
        <select value={form.challenger} onChange={(e) => setForm({ ...form, challenger: e.target.value })}>
          <option value="">Pick a model...</option>
          {models
            .filter((m) => m.model.id !== currentModel)
            .map((m) => (
              <option key={m.model.id} value={m.model.id}>
                {m.label} ({m.model.id}, ${m.outputPerMTok}/M out){m.available ? '' : ' - sandbox'}
              </option>
            ))}
        </select>
      </Field>
      <div className="row">
        <Field label="Traffic %" error={errors.trafficPercent}>
          <input type="number" min={1} max={50} value={form.trafficPercent} onChange={(e) => setForm({ ...form, trafficPercent: e.target.value })} />
        </Field>
        <Field label="Hypothesis" error={errors.hypothesis}>
          <input value={form.hypothesis} placeholder="Cheaper at the same quality" onChange={(e) => setForm({ ...form, hypothesis: e.target.value })} />
        </Field>
      </div>
      <ErrorBanner error={result.error} />
      <button className="button" disabled={result.fetching}>
        Start experiment
      </button>
    </form>
  );
}
