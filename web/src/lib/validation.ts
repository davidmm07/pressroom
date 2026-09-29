import { z } from 'zod';

// Client-side validation mirrors the server's rules (internal/domain) for
// instant feedback. The server stays the source of truth: whatever it
// returns in userErrors is shown on the same fields.

export const departments = [
  'PREPRESS',
  'MANUFACTURING',
  'CUSTOMER_EXPERIENCE',
  'OPERATIONS',
  'MARKETPLACE',
  'MARKETING',
  'FINANCE',
] as const;
export const levels = ['LOW', 'MEDIUM', 'HIGH'] as const;

/** Parses a textarea into a JSON object, the shape every run input takes. */
export const jsonObject = z.string().transform((text, ctx) => {
  try {
    const value: unknown = JSON.parse(text);
    if (value === null || typeof value !== 'object' || Array.isArray(value)) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'must be a JSON object, e.g. {"orderId": "ORD-1042"}' });
      return z.NEVER;
    }
    return value as Record<string, unknown>;
  } catch {
    ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'is not valid JSON' });
    return z.NEVER;
  }
});

const usd = z
  .string()
  .trim()
  .regex(/^\d+(\.\d{1,6})?$/, 'must be a dollar amount with up to six decimals')
  .refine((v) => Number(v) > 0 && Number(v) <= 25, 'must be more than 0 and at most 25');

export const agentSchema = z.object({
  slug: z
    .string()
    .trim()
    .regex(/^[a-z][a-z0-9-]{2,39}$/, 'must be 3-40 lowercase letters, digits or dashes, starting with a letter'),
  name: z.string().trim().min(1, 'is required').max(80),
  description: z.string().max(500),
  department: z.enum(departments),
  owner: z.string().trim().email('must be a valid email address'),
  instructions: z.string().trim().min(1, 'is required').max(8000),
  model: z.string().regex(/^[a-z_]+\/.+$/, 'pick a model'),
  tools: z.array(z.string()).min(1, 'an agent needs at least one tool').max(20),
  triggers: z
    .string()
    .transform((s) => s.split(',').map((t) => t.trim()).filter(Boolean))
    .pipe(z.array(z.string().regex(/^[a-z]+(\.[a-z_]+)+$/, 'use dotted event types such as ticket.created'))),
  maxSteps: z.coerce.number().int().min(1).max(50),
  maxCost: usd,
  minutesSavedPerRun: z.coerce.number().min(0).max(480),
});
/** Form state: numeric inputs hold the raw text until the schema coerces it. */
export type AgentForm = Omit<z.input<typeof agentSchema>, 'maxSteps' | 'minutesSavedPerRun'> & {
  maxSteps: string;
  minutesSavedPerRun: string;
};

export const opportunitySchema = z.object({
  title: z.string().trim().min(1, 'is required').max(120),
  problem: z.string().trim().min(1, 'is required').max(2000),
  department: z.enum(departments),
  weeklyVolume: z.coerce.number().int('must be a whole number').min(1).max(100_000),
  minutesPerTask: z.coerce.number().min(0.5).max(480),
  dataSensitivity: z.enum(levels),
  errorCost: z.enum(levels),
});
export type OpportunityForm = Omit<z.input<typeof opportunitySchema>, 'weeklyVolume' | 'minutesPerTask'> & {
  weeklyVolume: string;
  minutesPerTask: string;
};

export const experimentSchema = z.object({
  challenger: z.string().min(1, 'pick a challenger model'),
  trafficPercent: z.coerce.number().int().min(1).max(50),
  hypothesis: z.string().trim().min(1, 'is required').max(500),
});

export type FieldErrors = Record<string, string>;

/** Flattens a zod error into { field: message } for form display. */
export function zodFieldErrors(error: z.ZodError): FieldErrors {
  const out: FieldErrors = {};
  for (const issue of error.issues) {
    const key = issue.path.join('.') || '_';
    out[key] ??= issue.message;
  }
  return out;
}

export interface UserError {
  field: string[];
  code: string;
  message: string;
}

/**
 * Maps server userErrors onto form fields: ["input","tools","1"] becomes
 * "tools". Errors not tied to a field land under "_".
 */
export function serverFieldErrors(errors: readonly UserError[]): FieldErrors {
  const out: FieldErrors = {};
  for (const e of errors) {
    const path = e.field[0] === 'input' ? e.field.slice(1) : e.field;
    const key = path.find((p) => !/^\d+$/.test(p)) ?? '_';
    out[key] ??= e.message;
  }
  return out;
}
