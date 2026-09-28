import { useEffect, useState, type FormEvent } from 'react';
import { Link, useParams } from 'react-router';
import { useMutation, useQuery } from 'urql';
import { Badge, Empty, ErrorBanner, Field, JSONBlock, PageHeader, Tile } from '../components';
import { graphql } from '../gql';
import type { Verdict } from '../gql/graphql';
import { formatDuration, formatUSD, humanize, timeAgo } from '../lib/format';

const RunQuery = graphql(`
  query Run($id: ID!) {
    run(id: $id) {
      id
      status
      trigger
      variant
      input
      output
      failureReason
      turns
      cost
      createdAt
      durationSeconds
      usage {
        inputTokens
        cachedInputTokens
        outputTokens
      }
      model {
        id
      }
      agent {
        slug
        name
      }
      pendingToolCall {
        callId
        arguments
        tool {
          name
          description
        }
      }
      review {
        verdict
        reviewer
        note
        at
      }
      steps {
        index
        kind
        toolName
        summary
        detail
        latencyMs
        cost
        at
      }
    }
  }
`);

const ApproveToolCall = graphql(`
  mutation ApproveToolCall($runId: ID!) {
    approveToolCall(runId: $runId) {
      run {
        id
        status
      }
      userErrors {
        message
      }
    }
  }
`);

const DenyToolCall = graphql(`
  mutation DenyToolCall($runId: ID!, $reason: String!) {
    denyToolCall(runId: $runId, reason: $reason) {
      run {
        id
        status
      }
      userErrors {
        message
      }
    }
  }
`);

const ReviewRun = graphql(`
  mutation ReviewRun($input: ReviewRunInput!) {
    reviewRun(input: $input) {
      run {
        id
        status
        review {
          verdict
          reviewer
          note
          at
        }
      }
      userErrors {
        message
      }
    }
  }
`);

const CancelRun = graphql(`
  mutation CancelRun($runId: ID!) {
    cancelRun(runId: $runId) {
      run {
        id
        status
      }
      userErrors {
        message
      }
    }
  }
`);

const inFlight = new Set(['QUEUED', 'RUNNING']);

export function RunPage() {
  const { id = '' } = useParams();
  const [{ data, error, fetching }, refetch] = useQuery({ query: RunQuery, variables: { id } });
  const [approval, approve] = useMutation(ApproveToolCall);
  const [denial, deny] = useMutation(DenyToolCall);
  const [cancellation, cancel] = useMutation(CancelRun);
  const run = data?.run;

  // Poll while the worker is busy with this run.
  useEffect(() => {
    if (!run || !inFlight.has(run.status)) return;
    const timer = setInterval(() => refetch({ requestPolicy: 'network-only' }), 1500);
    return () => clearInterval(timer);
  }, [run, refetch]);

  if (fetching && !data) return <p className="muted">Loading...</p>;
  if (error) return <ErrorBanner error={error} />;
  if (!run) return <Empty>Run not found.</Empty>;

  const act = async (action: () => Promise<unknown>) => {
    await action();
    refetch({ requestPolicy: 'network-only' });
  };
  const userError =
    approval.data?.approveToolCall.userErrors[0]?.message ??
    denial.data?.denyToolCall.userErrors[0]?.message ??
    cancellation.data?.cancelRun.userErrors[0]?.message;

  return (
    <>
      <PageHeader
        title={
          <>
            Run by <Link to={`/agents/${run.agent.slug}`}>{run.agent.name}</Link> <Badge value={run.status} />
          </>
        }
        subtitle={`${humanize(run.trigger)} run, ${timeAgo(run.createdAt)}, on ${run.model.id}${run.variant === 'CHALLENGER' ? ' (challenger)' : ''}`}
        actions={
          (inFlight.has(run.status) || run.status === 'AWAITING_APPROVAL') && (
            <button className="button button-quiet" onClick={() => act(() => cancel({ runId: run.id }))}>
              Cancel run
            </button>
          )
        }
      />
      <ErrorBanner error={approval.error ?? denial.error ?? cancellation.error ?? userError} />

      {run.pendingToolCall && (
        <section className="card card-warn">
          <h2>
            Waiting for approval: <code>{run.pendingToolCall.tool.name}</code>
          </h2>
          <p className="muted">{run.pendingToolCall.tool.description}</p>
          <JSONBlock value={run.pendingToolCall.arguments} />
          <div className="actions">
            <button className="button" onClick={() => act(() => approve({ runId: run.id }))} disabled={approval.fetching}>
              Approve
            </button>
            <button
              className="button button-danger"
              onClick={() => {
                const reason = window.prompt('Why deny this action? The agent will read your reason.');
                if (reason) act(() => deny({ runId: run.id, reason }));
              }}
            >
              Deny
            </button>
          </div>
        </section>
      )}

      <section className="tiles">
        <Tile label="Model turns" value={run.turns} />
        <Tile label="Cost" value={formatUSD(run.cost)} />
        <Tile label="Tokens in / cached / out" value={`${run.usage.inputTokens} / ${run.usage.cachedInputTokens} / ${run.usage.outputTokens}`} />
        <Tile label="Duration" value={formatDuration(run.durationSeconds)} />
      </section>

      <div className="columns">
        <section className="card">
          <h2>Input</h2>
          <JSONBlock value={run.input} />
        </section>
        <section className="card">
          <h2>Outcome</h2>
          {run.status === 'SUCCEEDED' && <pre className="prose">{run.output}</pre>}
          {run.status === 'FAILED' && <div className="banner banner-bad">{run.failureReason}</div>}
          {inFlight.has(run.status) && <p className="muted">The agent is working. This page refreshes on its own.</p>}
          {run.review ? (
            <p>
              <Badge value={run.review.verdict} /> by {run.review.reviewer} {timeAgo(run.review.at)}
              {run.review.note && <span className="muted">: {run.review.note}</span>}
            </p>
          ) : (
            run.status === 'SUCCEEDED' && <ReviewForm runId={run.id} />
          )}
        </section>
      </div>

      <section className="card">
        <h2>Trace</h2>
        <ol className="trace">
          {run.steps.map((s) => (
            <li key={s.index} className={`step step-${s.kind.toLowerCase()}`}>
              <div className="step-head">
                <Badge value={s.kind} /> <span>{s.summary}</span>
                <span className="muted small">
                  {[s.latencyMs > 0 && `${s.latencyMs} ms`, Number(s.cost) > 0 && formatUSD(s.cost)].filter(Boolean).join(', ')}
                </span>
              </div>
              <StepDetail detail={s.detail} />
            </li>
          ))}
        </ol>
      </section>
    </>
  );
}

function StepDetail({ detail }: { detail: unknown }) {
  const [open, setOpen] = useState(false);
  if (!detail || typeof detail !== 'object' || Object.keys(detail).length === 0) return null;
  return (
    <>
      <button className="link small" onClick={() => setOpen(!open)}>
        {open ? 'Hide details' : 'Show details'}
      </button>
      {open && <JSONBlock value={detail} />}
    </>
  );
}

function ReviewForm({ runId }: { runId: string }) {
  const [note, setNote] = useState('');
  const [result, review] = useMutation(ReviewRun);
  const submit = (verdict: Verdict) => (e: FormEvent) => {
    e.preventDefault();
    review({ input: { runId, verdict, note } });
  };
  return (
    <form className="form">
      <Field label="Was this output useful?" hint="Reviews drive the acceptance rate that keeps agents on the crew.">
        <input value={note} placeholder="Optional note" onChange={(e) => setNote(e.target.value)} />
      </Field>
      <ErrorBanner error={result.error ?? result.data?.reviewRun.userErrors[0]?.message} />
      <div className="actions">
        <button className="button" onClick={submit('ACCEPTED')}>
          Accept
        </button>
        <button className="button button-quiet" onClick={submit('EDITED')}>
          Needed edits
        </button>
        <button className="button button-danger" onClick={submit('REJECTED')}>
          Reject
        </button>
      </div>
    </form>
  );
}
