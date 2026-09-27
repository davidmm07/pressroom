import { useEffect, useState } from 'react';
import { NavLink, Route, Routes, useLocation } from 'react-router';
import { useQuery } from 'urql';
import { graphql } from './gql';
import { getOperator, setOperator } from './lib/client';
import { AgentPage } from './pages/AgentPage';
import { ApprovalsPage } from './pages/ApprovalsPage';
import { CrewPage } from './pages/CrewPage';
import { IntakePage } from './pages/IntakePage';
import { NewAgentPage } from './pages/NewAgentPage';
import { RunPage } from './pages/RunPage';

const NavCounts = graphql(`
  query NavCounts {
    crew {
      awaitingApproval
    }
  }
`);

export function App() {
  const [{ data }, refetchCounts] = useQuery({ query: NavCounts, requestPolicy: 'cache-and-network' });
  const [operator, setOperatorState] = useState(getOperator);
  const location = useLocation();

  // Approvals arrive from the worker, not from this tab, so poll the count
  // and refresh it whenever the operator moves between pages.
  useEffect(() => {
    refetchCounts({ requestPolicy: 'network-only' });
    const timer = setInterval(() => refetchCounts({ requestPolicy: 'network-only' }), 15_000);
    return () => clearInterval(timer);
  }, [location.pathname, refetchCounts]);
  const waiting = data?.crew.awaitingApproval ?? 0;

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">P</span>
          <div>
            <div className="brand-name">Pressroom</div>
            <div className="brand-tag">AI crew operations</div>
          </div>
        </div>
        <nav>
          <NavLink to="/" end>
            Crew
          </NavLink>
          <NavLink to="/approvals">
            Approvals {waiting > 0 && <span className="count">{waiting}</span>}
          </NavLink>
          <NavLink to="/intake">Intake</NavLink>
          <NavLink to="/agents/new">Hire an agent</NavLink>
        </nav>
        <label className="operator">
          <span>Signed in as</span>
          <input
            value={operator}
            onChange={(e) => {
              setOperatorState(e.target.value);
              setOperator(e.target.value);
            }}
            aria-label="Operator email"
          />
        </label>
      </aside>
      <main className="main">
        <Routes>
          <Route path="/" element={<CrewPage />} />
          <Route path="/approvals" element={<ApprovalsPage />} />
          <Route path="/intake" element={<IntakePage />} />
          <Route path="/agents/new" element={<NewAgentPage />} />
          <Route path="/agents/:slug" element={<AgentPage />} />
          <Route path="/runs/:id" element={<RunPage />} />
          <Route path="*" element={<p>Page not found.</p>} />
        </Routes>
      </main>
    </div>
  );
}
