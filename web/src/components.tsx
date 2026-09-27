import type { ReactNode } from 'react';
import type { CombinedError } from 'urql';
import { humanize } from './lib/format';

const tones: Record<string, string> = {
  ACTIVE: 'good',
  SUCCEEDED: 'good',
  ACCEPTED: 'good',
  PROMOTED: 'good',
  SHIPPED: 'good',
  KEEP: 'good',
  REINSTATE: 'good',
  PROBATION: 'warn',
  AWAITING_APPROVAL: 'warn',
  EDITED: 'warn',
  RUNNING: 'info',
  QUEUED: 'info',
  SUBMITTED: 'info',
  APPROVED: 'info',
  DRAFT: 'muted',
  INSUFFICIENT_DATA: 'muted',
  CANCELLED: 'muted',
  RETIRED: 'bad',
  RETIRE: 'bad',
  FAILED: 'bad',
  REJECTED: 'bad',
  DECLINED: 'bad',
};

export function Badge({ value }: { value: string }) {
  return <span className={`badge badge-${tones[value] ?? 'muted'}`}>{humanize(value)}</span>;
}

export function Tile({ label, value, hint, tone }: { label: string; value: ReactNode; hint?: string; tone?: 'good' | 'bad' }) {
  return (
    <div className={`tile ${tone ? `tile-${tone}` : ''}`}>
      <div className="tile-label">{label}</div>
      <div className="tile-value">{value}</div>
      {hint && <div className="tile-hint">{hint}</div>}
    </div>
  );
}

export function ErrorBanner({ error }: { error?: CombinedError | string | null }) {
  if (!error) return null;
  const message = typeof error === 'string' ? error : (error.graphQLErrors[0]?.message ?? error.message);
  return (
    <div className="banner banner-bad" role="alert">
      {message}
    </div>
  );
}

export function Field({ label, error, hint, children }: { label: string; error?: string; hint?: string; children: ReactNode }) {
  return (
    <label className={`field ${error ? 'field-error' : ''}`}>
      <span className="field-label">{label}</span>
      {children}
      {error ? <span className="field-message">{error}</span> : hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}

export function JSONBlock({ value }: { value: unknown }) {
  return <pre className="json">{JSON.stringify(value, null, 2)}</pre>;
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}

export function PageHeader({ title, subtitle, actions }: { title: ReactNode; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="page-header">
      <div>
        <h1>{title}</h1>
        {subtitle && <p className="subtitle">{subtitle}</p>}
      </div>
      {actions && <div className="actions">{actions}</div>}
    </header>
  );
}
