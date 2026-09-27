import { Link } from 'react-router';
import { useMutation, useQuery } from 'urql';
import { Empty, ErrorBanner, JSONBlock, PageHeader } from '../components';
import { graphql } from '../gql';
import { timeAgo } from '../lib/format';

const Pending = graphql(`
  query PendingApprovals {
    runs(status: [AWAITING_APPROVAL], first: 50) {
      edges {
        node {
          id
          createdAt
          agent {
            name
            slug
            owner
          }
          pendingToolCall {
            arguments
            tool {
              name
              description
            }
          }
        }
      }
    }
  }
`);

const Approve = graphql(`
  mutation ApproveFromQueue($runId: ID!) {
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

const Deny = graphql(`
  mutation DenyFromQueue($runId: ID!, $reason: String!) {
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

/** The human-in-the-loop queue: every refund and customer email an agent
 * wants to send, waiting for a yes or a no. */
export function ApprovalsPage() {
  const [{ data, error }, refetch] = useQuery({ query: Pending, requestPolicy: 'network-only' });
  const [approval, approve] = useMutation(Approve);
  const [denial, deny] = useMutation(Deny);
  const edges = data?.runs.edges ?? [];

  const after = async (p: Promise<unknown>) => {
    await p;
    refetch({ requestPolicy: 'network-only' });
  };

  return (
    <>
      <PageHeader
        title="Approvals"
        subtitle="Agents pause before anything that moves money or reaches a customer's inbox. Approve to let the run continue; deny and the agent is told why."
      />
      <ErrorBanner
        error={approval.error ?? denial.error ?? approval.data?.approveToolCall.userErrors[0]?.message ?? denial.data?.denyToolCall.userErrors[0]?.message}
      />
      <ErrorBanner error={error} />
      {data && edges.length === 0 && <Empty>Nothing is waiting on you.</Empty>}
      {edges.map(({ node: run }) => (
        <section key={run.id} className="card card-warn">
          <div className="approval-head">
            <h2>
              {run.agent.name} wants to call <code>{run.pendingToolCall?.tool.name}</code>
            </h2>
            <span className="muted small">
              {timeAgo(run.createdAt)} · owner {run.agent.owner} · <Link to={`/runs/${run.id}`}>open run</Link>
            </span>
          </div>
          <p className="muted small">{run.pendingToolCall?.tool.description}</p>
          <JSONBlock value={run.pendingToolCall?.arguments} />
          <div className="actions">
            <button className="button" onClick={() => after(approve({ runId: run.id }))}>
              Approve
            </button>
            <button
              className="button button-danger"
              onClick={() => {
                const reason = window.prompt('Why deny this action? The agent will read your reason.');
                if (reason) after(deny({ runId: run.id, reason }));
              }}
            >
              Deny
            </button>
          </div>
        </section>
      ))}
    </>
  );
}
