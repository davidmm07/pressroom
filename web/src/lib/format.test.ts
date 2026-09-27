import { describe, expect, it } from 'vitest';
import { formatDuration, formatPercent, formatUSD, humanize, timeAgo } from './format';

describe('formatUSD', () => {
  it('keeps sub-dollar model costs readable', () => {
    expect(formatUSD('0.012500')).toBe('$0.0125');
  });
  it('uses cents for larger amounts, including negatives', () => {
    expect(formatUSD('1234.500000')).toBe('$1,234.50');
    expect(formatUSD('-12.000000')).toBe('-$12.00');
  });
  it('handles missing values', () => {
    expect(formatUSD(null)).toBe('-');
    expect(formatUSD('not money')).toBe('-');
  });
});

describe('small formatters', () => {
  it('formats rates, durations and enums', () => {
    expect(formatPercent(0.873)).toBe('87%');
    expect(formatPercent(null)).toBe('-');
    expect(formatDuration(4.25)).toBe('4.3 s');
    expect(formatDuration(95)).toBe('1 min 35 s');
    expect(humanize('CUSTOMER_EXPERIENCE')).toBe('Customer experience');
  });
  it('describes time relative to now', () => {
    const now = new Date('2026-09-27T12:00:00Z');
    expect(timeAgo('2026-09-27T09:00:00Z', now)).toBe('3 hours ago');
    expect(timeAgo('2026-09-27T11:59:50Z', now)).toBe('just now');
  });
});
