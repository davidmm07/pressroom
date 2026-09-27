import { useState, type FormEvent } from 'react';
import { Link } from 'react-router';
import { useMutation, useQuery } from 'urql';
import { Badge, ErrorBanner, Field, PageHeader } from '../components';
import { graphql } from '../gql';
import type { OpportunityStatus } from '../gql/graphql';
import { humanize } from '../lib/format';
import {
  departments,
  levels,
  opportunitySchema,
  serverFieldErrors,
  zodFieldErrors,
  type FieldErrors,
  type OpportunityForm,
} from '../lib/validation';

const Backlog = graphql(`
  query Backlog {
    opportunities {
      id
      title
      problem
      department
      submittedBy
      weeklyVolume
      minutesPerTask
      hoursPerWeek
      feasibility
      score
      status
      agent {
        slug
        name
      }
    }
    agents(status: [ACTIVE, PROBATION]) {
      id
      name
    }
  }
`);

const Submit = graphql(`
  mutation SubmitOpportunity($input: SubmitOpportunityInput!) {
    submitOpportunity(input: $input) {
      opportunity {
        id
        score
      }
      userErrors {
        field
        code
        message
      }
    }
  }
`);

const Move = graphql(`
  mutation MoveOpportunity($id: ID!, $status: OpportunityStatus!, $agentId: ID) {
    moveOpportunity(id: $id, status: $status, agentId: $agentId) {
      opportunity {
        id
        status
      }
      userErrors {
        message
      }
    }
  }
`);

const empty: OpportunityForm = {
  title: '',
  problem: '',
  department: 'CUSTOMER_EXPERIENCE',
  weeklyVolume: '',
  minutesPerTask: '',
  dataSensitivity: 'LOW',
  errorCost: 'LOW',
};

/** Where department leads describe work an agent might take over, scored so
 * the backlog is ordered by hours won back rather than by who asks loudest. */
export function IntakePage() {
  const [{ data, error }, refetch] = useQuery({ query: Backlog, requestPolicy: 'cache-and-network' });
  const [form, setForm] = useState<OpportunityForm>(empty);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [submission, submit] = useMutation(Submit);
  const [moving, move] = useMutation(Move);

  const set = (key: keyof OpportunityForm) => (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value });

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const parsed = opportunitySchema.safeParse(form);
    if (!parsed.success) return setErrors(zodFieldErrors(parsed.error));
    setErrors({});
    const res = await submit({ input: parsed.data });
    const payload = res.data?.submitOpportunity;
    if (payload?.opportunity) {
      setForm(empty);
      refetch({ requestPolicy: 'network-only' });
    } else if (payload) setErrors(serverFieldErrors(payload.userErrors));
  };

  const onMove = async (id: string, status: OpportunityStatus, agentId?: string) => {
    await move({ id, status, agentId: agentId ?? null });
    refetch({ requestPolicy: 'network-only' });
  };

  return (
    <>
      <PageHeader
        title="Intake"
        subtitle="Score = hours per week the task takes today x feasibility (sensitive data and costly mistakes lower it)."
      />
      <ErrorBanner error={error ?? moving.error ?? moving.data?.moveOpportunity.userErrors[0]?.message} />

      <section className="card">
        <table className="table">
          <thead>
            <tr>
              <th>Request</th>
              <th>Department</th>
              <th className="num">Hours / week</th>
              <th className="num">Feasibility</th>
              <th className="num">Score</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data?.opportunities.map((o) => (
              <tr key={o.id}>
                <td>
                  <div className="strong">{o.title}</div>
                  <div className="muted small">
                    {o.weeklyVolume}/week x {o.minutesPerTask} min · {o.submittedBy}
                  </div>
                </td>
                <td>{humanize(o.department)}</td>
                <td className="num">{o.hoursPerWeek.toFixed(1)}</td>
                <td className="num">{Math.round(o.feasibility * 100)}%</td>
                <td className="num strong">{o.score}</td>
                <td>
                  <Badge value={o.status} />
                  {o.agent && (
                    <div className="small">
                      by <Link to={`/agents/${o.agent.slug}`}>{o.agent.name}</Link>
                    </div>
                  )}
                </td>
                <td>
                  <div className="actions">
                  {o.status === 'SUBMITTED' && (
                    <button className="button button-small" onClick={() => onMove(o.id, 'APPROVED')}>
                      Approve
                    </button>
                  )}
                  {o.status === 'APPROVED' && (
                    <select
                      className="button-small"
                      defaultValue=""
                      onChange={(e) => e.target.value && onMove(o.id, 'SHIPPED', e.target.value)}
                      aria-label="Ship with agent"
                    >
                      <option value="">Shipped by...</option>
                      {data.agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.name}
                        </option>
                      ))}
                    </select>
                  )}
                  {(o.status === 'SUBMITTED' || o.status === 'APPROVED') && (
                    <button className="button button-small button-quiet" onClick={() => onMove(o.id, 'DECLINED')}>
                      Decline
                    </button>
                  )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card narrow">
        <h2>Suggest work for an agent</h2>
        <form onSubmit={onSubmit} className="form" noValidate>
          <Field label="What should the agent take over?" error={errors.title}>
            <input value={form.title} onChange={set('title')} placeholder="Answer where-is-my-order tickets" />
          </Field>
          <Field label="How is it done today?" error={errors.problem}>
            <textarea rows={3} value={form.problem} onChange={set('problem')} />
          </Field>
          <div className="row">
            <Field label="Department" error={errors.department}>
              <select value={form.department} onChange={set('department')}>
                {departments.map((d) => (
                  <option key={d} value={d}>
                    {humanize(d)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Times per week" error={errors.weeklyVolume}>
              <input inputMode="numeric" value={String(form.weeklyVolume)} onChange={set('weeklyVolume')} />
            </Field>
            <Field label="Minutes each" error={errors.minutesPerTask}>
              <input inputMode="decimal" value={String(form.minutesPerTask)} onChange={set('minutesPerTask')} />
            </Field>
          </div>
          <div className="row">
            <Field label="Data sensitivity" hint="Customer PII, payments, artwork rights" error={errors.dataSensitivity}>
              <select value={form.dataSensitivity} onChange={set('dataSensitivity')}>
                {levels.map((l) => (
                  <option key={l}>{l}</option>
                ))}
              </select>
            </Field>
            <Field label="Cost of a mistake" hint="A typo, or a reprint" error={errors.errorCost}>
              <select value={form.errorCost} onChange={set('errorCost')}>
                {levels.map((l) => (
                  <option key={l}>{l}</option>
                ))}
              </select>
            </Field>
          </div>
          <ErrorBanner error={submission.error ?? errors._} />
          <button className="button" disabled={submission.fetching}>
            Submit request
          </button>
        </form>
      </section>
    </>
  );
}
