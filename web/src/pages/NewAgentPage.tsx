import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { useMutation, useQuery } from 'urql';
import { ErrorBanner, Field, PageHeader } from '../components';
import { graphql } from '../gql';
import { humanize } from '../lib/format';
import { agentSchema, departments, serverFieldErrors, zodFieldErrors, type AgentForm, type FieldErrors } from '../lib/validation';

const Options = graphql(`
  query AgentOptions {
    tools {
      name
      description
      requiresApproval
    }
    models {
      model {
        id
        provider
      }
      label
      available
      inputPerMTok
      outputPerMTok
    }
  }
`);

const CreateAgent = graphql(`
  mutation CreateAgent($input: CreateAgentInput!) {
    createAgent(input: $input) {
      agent {
        id
        slug
      }
      userErrors {
        field
        code
        message
      }
    }
  }
`);

const initial: AgentForm = {
  slug: '',
  name: '',
  description: '',
  department: 'CUSTOMER_EXPERIENCE',
  owner: '',
  instructions: '',
  model: 'anthropic/claude-opus-5',
  tools: [],
  triggers: '',
  maxSteps: '8',
  maxCost: '0.50',
  minutesSavedPerRun: '5',
};

export function NewAgentPage() {
  const navigate = useNavigate();
  const [{ data }] = useQuery({ query: Options });
  const [form, setForm] = useState<AgentForm>(initial);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [result, create] = useMutation(CreateAgent);

  const set = (key: keyof AgentForm) => (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value });
  const toggleTool = (name: string) =>
    setForm({ ...form, tools: form.tools.includes(name) ? form.tools.filter((t) => t !== name) : [...form.tools, name] });

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const parsed = agentSchema.safeParse(form);
    if (!parsed.success) return setErrors(zodFieldErrors(parsed.error));
    setErrors({});
    const res = await create({ input: parsed.data });
    const payload = res.data?.createAgent;
    if (payload?.agent) navigate(`/agents/${payload.agent.slug}`);
    else if (payload) setErrors(serverFieldErrors(payload.userErrors));
  };

  return (
    <>
      <PageHeader
        title="Hire an agent"
        subtitle="New agents start as drafts. The owning lead activates them after reading the instructions."
      />
      <form onSubmit={onSubmit} className="form card narrow" noValidate>
        <div className="row">
          <Field label="Name" error={errors.name}>
            <input value={form.name} onChange={set('name')} placeholder="Shipping Watch" />
          </Field>
          <Field label="Slug" error={errors.slug} hint="Used in URLs and event routing">
            <input value={form.slug} onChange={set('slug')} placeholder="shipping-watch" />
          </Field>
        </div>
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
          <Field label="Owner" error={errors.owner} hint="The lead accountable for its results">
            <input type="email" value={form.owner} onChange={set('owner')} placeholder="ops-lead@example.com" />
          </Field>
        </div>
        <Field label="Description" error={errors.description}>
          <input value={form.description} onChange={set('description')} />
        </Field>
        <Field label="Instructions" error={errors.instructions} hint="The job description the model follows">
          <textarea rows={6} value={form.instructions} onChange={set('instructions')} />
        </Field>
        <Field label="Model" error={errors.model}>
          <select value={form.model} onChange={set('model')}>
            {data?.models.map((m) => (
              <option key={m.model.id} value={m.model.id}>
                {m.label}, ${m.inputPerMTok}/${m.outputPerMTok} per M tokens{m.available ? '' : ' (sandbox)'}
              </option>
            ))}
          </select>
        </Field>
        <fieldset className={`field ${errors.tools ? 'field-error' : ''}`}>
          <legend className="field-label">Tools</legend>
          <div className="tool-grid">
            {data?.tools.map((t) => (
              <label key={t.name} className="tool-option">
                <input type="checkbox" checked={form.tools.includes(t.name)} onChange={() => toggleTool(t.name)} />
                <span>
                  <code>{t.name}</code> {t.requiresApproval && <span className="badge badge-warn">needs approval</span>}
                  <span className="muted small block">{t.description}</span>
                </span>
              </label>
            ))}
          </div>
          {errors.tools && <span className="field-message">{errors.tools}</span>}
        </fieldset>
        <Field label="Triggers" error={errors.triggers} hint="Comma separated event types, e.g. ticket.created">
          <input value={form.triggers} onChange={set('triggers')} />
        </Field>
        <div className="row">
          <Field label="Max steps per run" error={errors.maxSteps}>
            <input inputMode="numeric" value={String(form.maxSteps)} onChange={set('maxSteps')} />
          </Field>
          <Field label="Max cost per run (USD)" error={errors.maxCost}>
            <input inputMode="decimal" value={form.maxCost} onChange={set('maxCost')} />
          </Field>
          <Field label="Minutes saved per run" error={errors.minutesSavedPerRun}>
            <input inputMode="decimal" value={String(form.minutesSavedPerRun)} onChange={set('minutesSavedPerRun')} />
          </Field>
        </div>
        <ErrorBanner error={result.error ?? errors._} />
        <button className="button" disabled={result.fetching}>
          Create draft agent
        </button>
      </form>
    </>
  );
}
