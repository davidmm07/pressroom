import { Link } from 'react-router';
import { useMutation, useQuery } from 'urql';
import { Badge, ErrorBanner, PageHeader, Tile } from '../components';
import { graphql } from '../gql';
import { formatHours, formatPercent, formatUSD, humanize } from '../lib/format';

const CrewQuery = graphql(`
  query Crew {
    crew {
      activeAgents
      onProbation
      retired
      runsLast7Days
      awaitingApproval
      hoursSavedLast30Days
      spendLast30Days
      netValueLast30Days
    }
    agents {
      id
      slug
      name
      department
      status
      model {
        id
      }
      activeExperiment {
        id
        challenger {
          id
        }
      }
      scorecard(windowDays: 30) {
        finishedRuns
        successRate
        acceptanceRate
        costPerRun
        hoursSaved
        netValue
      }
    }
  }
`);

const EvaluateCrew = graphql(`
  mutation EvaluateCrew {
    evaluateCrew {
      evaluations {
        id
        decision
        reason
        statusBefore
        statusAfter
        agent {
          slug
          name
        }
      }
      userErrors {
        message
      }
    }
  }
`);

export function CrewPage() {
  const [{ data, fetching, error }] = useQuery({ query: CrewQuery, requestPolicy: 'cache-and-network' });
  const [evaluation, evaluate] = useMutation(EvaluateCrew);
  const changes = evaluation.data?.evaluateCrew.evaluations.filter((e) => e.statusBefore !== e.statusAfter) ?? [];

  return (
    <>
      <PageHeader
        title="Crew"
        subtitle="Every agent on the payroll, what it costs and whether it earns its place. Figures cover the last 30 days."
        actions={
          <button className="button" onClick={() => evaluate({})} disabled={evaluation.fetching}>
            {evaluation.fetching ? 'Evaluating...' : 'Evaluate crew'}
          </button>
        }
      />
      <ErrorBanner error={error ?? evaluation.error} />

      {evaluation.data && (
        <div className="banner banner-info">
          <strong>Evaluation complete.</strong>{' '}
          {changes.length === 0
            ? 'Every agent kept its status.'
            : changes.map((e) => (
                <span key={e.id} className="change">
                  <Link to={`/agents/${e.agent.slug}`}>{e.agent.name}</Link>: {humanize(e.statusBefore)} to{' '}
                  <Badge value={e.statusAfter} /> ({e.reason})
                </span>
              ))}
        </div>
      )}

      {data && (
        <section className="tiles">
          <Tile label="On duty" value={data.crew.activeAgents} hint={`${data.crew.onProbation} on probation, ${data.crew.retired} retired`} />
          <Tile label="Runs, last 7 days" value={data.crew.runsLast7Days} />
          <Tile label="Hours saved" value={formatHours(data.crew.hoursSavedLast30Days)} hint="accepted work only" />
          <Tile label="Model spend" value={formatUSD(data.crew.spendLast30Days)} />
          <Tile
            label="Net value"
            value={formatUSD(data.crew.netValueLast30Days)}
            tone={Number(data.crew.netValueLast30Days) >= 0 ? 'good' : 'bad'}
            hint="value of hours saved minus spend"
          />
          <Tile
            label="Waiting on you"
            value={<Link to="/approvals">{data.crew.awaitingApproval}</Link>}
            hint="tool calls to approve"
          />
        </section>
      )}

      <section className="card">
        <table className="table">
          <thead>
            <tr>
              <th>Agent</th>
              <th>Status</th>
              <th>Model</th>
              <th className="num">Runs</th>
              <th className="num">Success</th>
              <th className="num">Accepted</th>
              <th className="num">Cost / run</th>
              <th className="num">Net value</th>
            </tr>
          </thead>
          <tbody>
            {fetching && !data && (
              <tr>
                <td colSpan={8} className="muted">
                  Loading the crew...
                </td>
              </tr>
            )}
            {data?.agents.map((a) => (
              <tr key={a.id}>
                <td>
                  <Link to={`/agents/${a.slug}`} className="strong">
                    {a.name}
                  </Link>
                  <div className="muted small">{humanize(a.department)}</div>
                </td>
                <td>
                  <Badge value={a.status} />
                </td>
                <td>
                  <code>{a.model.id}</code>
                  {a.activeExperiment && <div className="small experiment-tag">trialling {a.activeExperiment.challenger.id}</div>}
                </td>
                <td className="num">{a.scorecard.finishedRuns}</td>
                <td className="num">{formatPercent(a.scorecard.finishedRuns ? a.scorecard.successRate : null)}</td>
                <td className="num">{formatPercent(a.scorecard.acceptanceRate)}</td>
                <td className="num">{formatUSD(a.scorecard.costPerRun)}</td>
                <td className={`num ${Number(a.scorecard.netValue) < 0 ? 'negative' : ''}`}>{formatUSD(a.scorecard.netValue)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </>
  );
}
