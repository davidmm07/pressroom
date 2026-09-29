import { describe, expect, it } from 'vitest';
import { agentSchema, jsonObject, opportunitySchema, serverFieldErrors, zodFieldErrors } from './validation';

describe('jsonObject', () => {
  it('accepts objects and rejects everything else', () => {
    expect(jsonObject.parse('{"orderId":"ORD-1042"}')).toEqual({ orderId: 'ORD-1042' });
    expect(jsonObject.safeParse('[1,2]').success).toBe(false);
    expect(jsonObject.safeParse('{"broken":').success).toBe(false);
  });
});

describe('agentSchema', () => {
  const valid = {
    slug: 'proof-checker',
    name: 'Proof Checker',
    description: '',
    department: 'PREPRESS',
    owner: 'lead@example.com',
    instructions: 'Check artwork.',
    model: 'anthropic/claude-opus-5',
    tools: ['inspect_artwork'],
    triggers: 'artwork.uploaded, ticket.created',
    maxSteps: '8',
    maxCost: '0.75',
    minutesSavedPerRun: '6',
  };

  it('parses form strings into API values', () => {
    const out = agentSchema.parse(valid);
    expect(out.triggers).toEqual(['artwork.uploaded', 'ticket.created']);
    expect(out.maxSteps).toBe(8);
    expect(out.maxCost).toBe('0.75');
  });

  it('reports every invalid field', () => {
    const result = agentSchema.safeParse({ ...valid, slug: 'Bad Slug', tools: [], maxCost: '0.1234567', maxSteps: '99' });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(Object.keys(zodFieldErrors(result.error)).sort()).toEqual(['maxCost', 'maxSteps', 'slug', 'tools']);
    }
  });
});

describe('opportunitySchema', () => {
  it('rejects fractional weekly volumes', () => {
    const result = opportunitySchema.safeParse({
      title: 'x', problem: 'y', department: 'FINANCE', weeklyVolume: '2.5', minutesPerTask: '10',
      dataSensitivity: 'LOW', errorCost: 'LOW',
    });
    expect(result.success).toBe(false);
  });
});

describe('serverFieldErrors', () => {
  it('maps userError paths onto form fields', () => {
    expect(
      serverFieldErrors([
        { field: ['input', 'tools', '1'], code: 'INVALID_VALUE', message: 'unknown tool "launch_rockets"' },
        { field: ['input', 'slug'], code: 'INVALID_VALUE', message: '"x" is already taken' },
        { field: [], code: 'CONFLICT', message: 'agent is at version 3, not 2' },
      ]),
    ).toEqual({
      tools: 'unknown tool "launch_rockets"',
      slug: '"x" is already taken',
      _: 'agent is at version 3, not 2',
    });
  });
});
