// Presentation helpers. Money arrives as a decimal string (the USD scalar)
// and is only turned into a number at the very last step, for display.

const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });
const usdPrecise = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

/** "0.012500" -> "$0.0125"; amounts of a dollar or more use cents. */
export function formatUSD(decimal: string | null | undefined): string {
  if (decimal == null || decimal === '') return '-';
  const n = Number(decimal);
  if (!Number.isFinite(n)) return '-';
  return Math.abs(n) >= 1 || n === 0 ? usd.format(n) : usdPrecise.format(n);
}

/** 0.873 -> "87%"; null -> "-". */
export function formatPercent(rate: number | null | undefined): string {
  if (rate == null) return '-';
  return `${Math.round(rate * 100)}%`;
}

export function formatHours(hours: number): string {
  return hours >= 10 ? `${Math.round(hours)} h` : `${hours.toFixed(1)} h`;
}

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds == null) return '-';
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`;
  const m = Math.floor(seconds / 60);
  return `${m} min ${Math.round(seconds % 60)} s`;
}

const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

/** ISO timestamp -> "3 hours ago". `now` is injectable for tests. */
export function timeAgo(iso: string, now: Date = new Date()): string {
  const diff = (new Date(iso).getTime() - now.getTime()) / 1000;
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ];
  for (const [unit, secs] of units) {
    if (Math.abs(diff) >= secs) return relative.format(Math.round(diff / secs), unit);
  }
  return 'just now';
}

/** SCREAMING_SNAKE enum values -> "Customer experience". */
export function humanize(value: string): string {
  const s = value.toLowerCase().replaceAll('_', ' ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function prettyJSON(value: unknown): string {
  return JSON.stringify(value, null, 2);
}
