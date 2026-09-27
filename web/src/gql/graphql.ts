/* eslint-disable */
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
export type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]?: Maybe<T[SubKey]> };
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]: Maybe<T[SubKey]> };
export type MakeEmpty<T extends { [key: string]: unknown }, K extends keyof T> = { [_ in K]?: never };
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  /** Arbitrary JSON: run inputs, tool arguments, step details. */
  JSON: { input: unknown; output: unknown; }
  Time: { input: string; output: string; }
  /** An amount of US dollars as a decimal string with up to six places, e.g. "0.012500". */
  USD: { input: string; output: string; }
};

export type Agent = {
  __typename?: 'Agent';
  activeExperiment?: Maybe<Experiment>;
  budget: Budget;
  createdAt: Scalars['Time']['output'];
  department: Department;
  description: Scalars['String']['output'];
  evaluations: Array<Evaluation>;
  id: Scalars['ID']['output'];
  /** The system prompt: the agent's job description. */
  instructions: Scalars['String']['output'];
  /** Human minutes one accepted run replaces; turns runs into hours saved. */
  minutesSavedPerRun: Scalars['Float']['output'];
  model: Model;
  name: Scalars['String']['output'];
  /** Department lead accountable for this agent's results. */
  owner: Scalars['String']['output'];
  runs: RunConnection;
  scorecard: Scorecard;
  slug: Scalars['String']['output'];
  status: AgentStatus;
  tools: Array<Tool>;
  /** Event types that start a run automatically, e.g. ticket.created. */
  triggers: Array<Scalars['String']['output']>;
  updatedAt: Scalars['Time']['output'];
  /** Pass back as `expectedVersion` when updating. */
  version: Scalars['Int']['output'];
};


export type AgentEvaluationsArgs = {
  last?: InputMaybe<Scalars['Int']['input']>;
};


export type AgentRunsArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  first?: InputMaybe<Scalars['Int']['input']>;
  status?: InputMaybe<Array<RunStatus>>;
};


export type AgentScorecardArgs = {
  windowDays?: InputMaybe<Scalars['Int']['input']>;
};

export type AgentPayload = {
  __typename?: 'AgentPayload';
  agent?: Maybe<Agent>;
  userErrors: Array<UserError>;
};

export type AgentStatus =
  | 'ACTIVE'
  | 'DRAFT'
  | 'PROBATION'
  | 'RETIRED';

export type Budget = {
  __typename?: 'Budget';
  maxCost: Scalars['USD']['output'];
  maxSteps: Scalars['Int']['output'];
};

export type CreateAgentInput = {
  department: Department;
  description?: InputMaybe<Scalars['String']['input']>;
  instructions: Scalars['String']['input'];
  maxCost: Scalars['USD']['input'];
  maxSteps: Scalars['Int']['input'];
  minutesSavedPerRun: Scalars['Float']['input'];
  /** provider/name, e.g. anthropic/claude-opus-5 or xai/grok-4 */
  model: Scalars['String']['input'];
  name: Scalars['String']['input'];
  owner: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  tools: Array<Scalars['String']['input']>;
  triggers?: InputMaybe<Array<Scalars['String']['input']>>;
};

export type CrewSummary = {
  __typename?: 'CrewSummary';
  activeAgents: Scalars['Int']['output'];
  awaitingApproval: Scalars['Int']['output'];
  hoursSavedLast30Days: Scalars['Float']['output'];
  netValueLast30Days: Scalars['USD']['output'];
  onProbation: Scalars['Int']['output'];
  retired: Scalars['Int']['output'];
  runsLast7Days: Scalars['Int']['output'];
  spendLast30Days: Scalars['USD']['output'];
};

export type Decision =
  | 'INSUFFICIENT_DATA'
  | 'KEEP'
  | 'PROBATION'
  | 'REINSTATE'
  | 'RETIRE';

export type Department =
  | 'CUSTOMER_EXPERIENCE'
  | 'FINANCE'
  | 'MARKETING'
  | 'OPERATIONS'
  | 'PREPRESS';

export type EvaluateCrewPayload = {
  __typename?: 'EvaluateCrewPayload';
  evaluations: Array<Evaluation>;
  userErrors: Array<UserError>;
};

export type Evaluation = {
  __typename?: 'Evaluation';
  agent: Agent;
  agentId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  decision: Decision;
  id: Scalars['ID']['output'];
  reason: Scalars['String']['output'];
  scorecard: Scorecard;
  statusAfter: AgentStatus;
  statusBefore: AgentStatus;
};

export type Experiment = {
  __typename?: 'Experiment';
  agent: Agent;
  agentId: Scalars['ID']['output'];
  challenger: Model;
  challengerScorecard: Scorecard;
  champion: Model;
  championScorecard: Scorecard;
  concludedAt?: Maybe<Scalars['Time']['output']>;
  createdAt: Scalars['Time']['output'];
  hypothesis: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  outcome: Scalars['String']['output'];
  status: ExperimentStatus;
  trafficPercent: Scalars['Int']['output'];
};

export type ExperimentPayload = {
  __typename?: 'ExperimentPayload';
  experiment?: Maybe<Experiment>;
  userErrors: Array<UserError>;
};

export type ExperimentStatus =
  | 'PROMOTED'
  | 'REJECTED'
  | 'RUNNING';

export type Level =
  | 'HIGH'
  | 'LOW'
  | 'MEDIUM';

export type Model = {
  __typename?: 'Model';
  /** provider/name, e.g. anthropic/claude-opus-5 */
  id: Scalars['String']['output'];
  name: Scalars['String']['output'];
  provider: ModelProvider;
};

export type ModelOption = {
  __typename?: 'ModelOption';
  /** Credentials are configured, or the sandbox will serve it. */
  available: Scalars['Boolean']['output'];
  /** List price in USD per million input tokens. */
  inputPerMTok: Scalars['Float']['output'];
  label: Scalars['String']['output'];
  model: Model;
  /** List price in USD per million output tokens. */
  outputPerMTok: Scalars['Float']['output'];
};

export type ModelProvider =
  | 'ANTHROPIC'
  | 'OPENAI'
  | 'OPEN_SOURCE'
  | 'SANDBOX'
  | 'XAI';

export type Mutation = {
  __typename?: 'Mutation';
  activateAgent: AgentPayload;
  /** Let a paused run perform its sensitive tool call. */
  approveToolCall: RunPayload;
  cancelRun: RunPayload;
  /** Compare the arms and promote the challenger if it won. `force` closes an inconclusive trial. */
  concludeExperiment: ExperimentPayload;
  /** Create an agent as a DRAFT. Activate it to put it to work. */
  createAgent: AgentPayload;
  /** Refuse the paused tool call. The agent is told why and carries on. */
  denyToolCall: RunPayload;
  /** Apply the retirement policy to every agent on duty. */
  evaluateCrew: EvaluateCrewPayload;
  moveOpportunity: OpportunityPayload;
  retireAgent: AgentPayload;
  reviewRun: RunPayload;
  /** Send a share of an agent's runs to a challenger model. */
  startExperiment: ExperimentPayload;
  /** Queue a run. Repeating a call with the same `idempotencyKey` returns the original run. */
  startRun: RunPayload;
  submitOpportunity: OpportunityPayload;
  /** Change an agent. Fails with CONFLICT if `expectedVersion` is stale. */
  updateAgent: AgentPayload;
};


export type MutationActivateAgentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationApproveToolCallArgs = {
  runId: Scalars['ID']['input'];
};


export type MutationCancelRunArgs = {
  runId: Scalars['ID']['input'];
};


export type MutationConcludeExperimentArgs = {
  force?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
};


export type MutationCreateAgentArgs = {
  input: CreateAgentInput;
};


export type MutationDenyToolCallArgs = {
  reason: Scalars['String']['input'];
  runId: Scalars['ID']['input'];
};


export type MutationMoveOpportunityArgs = {
  agentId?: InputMaybe<Scalars['ID']['input']>;
  id: Scalars['ID']['input'];
  status: OpportunityStatus;
};


export type MutationRetireAgentArgs = {
  id: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
};


export type MutationReviewRunArgs = {
  input: ReviewRunInput;
};


export type MutationStartExperimentArgs = {
  input: StartExperimentInput;
};


export type MutationStartRunArgs = {
  input: StartRunInput;
};


export type MutationSubmitOpportunityArgs = {
  input: SubmitOpportunityInput;
};


export type MutationUpdateAgentArgs = {
  input: UpdateAgentInput;
};

export type Opportunity = {
  __typename?: 'Opportunity';
  agent?: Maybe<Agent>;
  agentId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  dataSensitivity: Level;
  department: Department;
  errorCost: Level;
  /** 0.3 to 1: discount for sensitive data and costly mistakes. */
  feasibility: Scalars['Float']['output'];
  /** Human hours per week the task takes today. */
  hoursPerWeek: Scalars['Float']['output'];
  id: Scalars['ID']['output'];
  minutesPerTask: Scalars['Float']['output'];
  problem: Scalars['String']['output'];
  /** hoursPerWeek x feasibility. The backlog is sorted by it. */
  score: Scalars['Float']['output'];
  status: OpportunityStatus;
  submittedBy: Scalars['String']['output'];
  title: Scalars['String']['output'];
  weeklyVolume: Scalars['Int']['output'];
};

export type OpportunityPayload = {
  __typename?: 'OpportunityPayload';
  opportunity?: Maybe<Opportunity>;
  userErrors: Array<UserError>;
};

export type OpportunityStatus =
  | 'APPROVED'
  | 'DECLINED'
  | 'SHIPPED'
  | 'SUBMITTED';

export type PageInfo = {
  __typename?: 'PageInfo';
  endCursor?: Maybe<Scalars['String']['output']>;
  hasNextPage: Scalars['Boolean']['output'];
};

export type PendingToolCall = {
  __typename?: 'PendingToolCall';
  arguments: Scalars['JSON']['output'];
  callId: Scalars['String']['output'];
  tool: Tool;
};

export type Query = {
  __typename?: 'Query';
  /** Look an agent up by id or by slug. */
  agent?: Maybe<Agent>;
  agents: Array<Agent>;
  /** Headline numbers for the whole crew. */
  crew: CrewSummary;
  experiments: Array<Experiment>;
  /** Models that can power an agent, with list prices. */
  models: Array<ModelOption>;
  /** Intake requests from department leads, highest score first. */
  opportunities: Array<Opportunity>;
  run?: Maybe<Run>;
  runs: RunConnection;
  /** Every tool an agent can be given. */
  tools: Array<Tool>;
  /** The operator making the request. */
  viewer: Viewer;
};


export type QueryAgentArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryAgentsArgs = {
  department?: InputMaybe<Department>;
  status?: InputMaybe<Array<AgentStatus>>;
};


export type QueryExperimentsArgs = {
  agentId?: InputMaybe<Scalars['ID']['input']>;
  status?: InputMaybe<ExperimentStatus>;
};


export type QueryOpportunitiesArgs = {
  department?: InputMaybe<Department>;
  status?: InputMaybe<OpportunityStatus>;
};


export type QueryRunArgs = {
  id: Scalars['ID']['input'];
};


export type QueryRunsArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  agentId?: InputMaybe<Scalars['ID']['input']>;
  first?: InputMaybe<Scalars['Int']['input']>;
  status?: InputMaybe<Array<RunStatus>>;
};

export type Review = {
  __typename?: 'Review';
  at: Scalars['Time']['output'];
  note: Scalars['String']['output'];
  reviewer: Scalars['String']['output'];
  verdict: Verdict;
};

export type ReviewRunInput = {
  note?: InputMaybe<Scalars['String']['input']>;
  runId: Scalars['ID']['input'];
  verdict: Verdict;
};

export type Run = {
  __typename?: 'Run';
  agent: Agent;
  agentId: Scalars['ID']['output'];
  cost: Scalars['USD']['output'];
  createdAt: Scalars['Time']['output'];
  durationSeconds?: Maybe<Scalars['Float']['output']>;
  experimentId?: Maybe<Scalars['ID']['output']>;
  failureReason: Scalars['String']['output'];
  finishedAt?: Maybe<Scalars['Time']['output']>;
  id: Scalars['ID']['output'];
  input: Scalars['JSON']['output'];
  model: Model;
  output: Scalars['String']['output'];
  /** The tool call waiting for a human, when status is AWAITING_APPROVAL. */
  pendingToolCall?: Maybe<PendingToolCall>;
  review?: Maybe<Review>;
  startedAt?: Maybe<Scalars['Time']['output']>;
  status: RunStatus;
  steps: Array<RunStep>;
  trigger: RunTrigger;
  turns: Scalars['Int']['output'];
  usage: Usage;
  variant: Variant;
};

export type RunConnection = {
  __typename?: 'RunConnection';
  edges: Array<RunEdge>;
  pageInfo: PageInfo;
};

export type RunEdge = {
  __typename?: 'RunEdge';
  cursor: Scalars['String']['output'];
  node: Run;
};

export type RunPayload = {
  __typename?: 'RunPayload';
  run?: Maybe<Run>;
  userErrors: Array<UserError>;
};

export type RunStatus =
  | 'AWAITING_APPROVAL'
  | 'CANCELLED'
  | 'FAILED'
  | 'QUEUED'
  | 'RUNNING'
  | 'SUCCEEDED';

export type RunStep = {
  __typename?: 'RunStep';
  at: Scalars['Time']['output'];
  cost: Scalars['USD']['output'];
  detail: Scalars['JSON']['output'];
  index: Scalars['Int']['output'];
  kind: StepKind;
  latencyMs: Scalars['Int']['output'];
  summary: Scalars['String']['output'];
  toolName?: Maybe<Scalars['String']['output']>;
};

export type RunTrigger =
  | 'EVENT'
  | 'MANUAL'
  | 'SCHEDULE';

export type Scorecard = {
  __typename?: 'Scorecard';
  /** Share of reviewed output that was kept (edited counts half). Null until something was reviewed. */
  acceptanceRate?: Maybe<Scalars['Float']['output']>;
  avgDurationSeconds: Scalars['Float']['output'];
  costPerRun: Scalars['USD']['output'];
  failed: Scalars['Int']['output'];
  finishedRuns: Scalars['Int']['output'];
  hoursSaved: Scalars['Float']['output'];
  /** Value delivered minus model spend. Negative means the agent costs more than it saves. */
  netValue: Scalars['USD']['output'];
  reviewed: Scalars['Int']['output'];
  succeeded: Scalars['Int']['output'];
  successRate: Scalars['Float']['output'];
  totalCost: Scalars['USD']['output'];
  valueDelivered: Scalars['USD']['output'];
  windowDays: Scalars['Int']['output'];
};

export type StartExperimentInput = {
  agentId: Scalars['ID']['input'];
  /** provider/name of the challenger model. */
  challenger: Scalars['String']['input'];
  hypothesis: Scalars['String']['input'];
  /** Share of runs sent to the challenger, 1 to 50. */
  trafficPercent: Scalars['Int']['input'];
};

export type StartRunInput = {
  /** Give agentId or agentSlug. */
  agentId?: InputMaybe<Scalars['ID']['input']>;
  agentSlug?: InputMaybe<Scalars['String']['input']>;
  /** Retrying with the same key returns the first run instead of starting another. */
  idempotencyKey?: InputMaybe<Scalars['String']['input']>;
  /** The task, as a JSON object. */
  input: Scalars['JSON']['input'];
};

export type StepKind =
  | 'APPROVAL_DENIED'
  | 'APPROVAL_GRANTED'
  | 'APPROVAL_REQUESTED'
  | 'COMPLETED'
  | 'FAILED'
  | 'MODEL_TURN'
  | 'TOOL_CALL';

export type SubmitOpportunityInput = {
  dataSensitivity: Level;
  department: Department;
  errorCost: Level;
  minutesPerTask: Scalars['Float']['input'];
  problem: Scalars['String']['input'];
  title: Scalars['String']['input'];
  weeklyVolume: Scalars['Int']['input'];
};

export type Tool = {
  __typename?: 'Tool';
  description: Scalars['String']['output'];
  /** JSON Schema the arguments must satisfy. */
  inputSchema: Scalars['JSON']['output'];
  name: Scalars['String']['output'];
  /** Runs pause for a human before this tool executes. */
  requiresApproval: Scalars['Boolean']['output'];
};

/** Omitted fields keep their current value. */
export type UpdateAgentInput = {
  department?: InputMaybe<Department>;
  description?: InputMaybe<Scalars['String']['input']>;
  expectedVersion: Scalars['Int']['input'];
  id: Scalars['ID']['input'];
  instructions?: InputMaybe<Scalars['String']['input']>;
  maxCost?: InputMaybe<Scalars['USD']['input']>;
  maxSteps?: InputMaybe<Scalars['Int']['input']>;
  minutesSavedPerRun?: InputMaybe<Scalars['Float']['input']>;
  model?: InputMaybe<Scalars['String']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  owner?: InputMaybe<Scalars['String']['input']>;
  tools?: InputMaybe<Array<Scalars['String']['input']>>;
  triggers?: InputMaybe<Array<Scalars['String']['input']>>;
};

export type Usage = {
  __typename?: 'Usage';
  cachedInputTokens: Scalars['Int']['output'];
  inputTokens: Scalars['Int']['output'];
  outputTokens: Scalars['Int']['output'];
};

export type UserError = {
  __typename?: 'UserError';
  code: UserErrorCode;
  /** Path to the offending input field, e.g. ["input", "tools", "1"]. Empty when not tied to a field. */
  field: Array<Scalars['String']['output']>;
  message: Scalars['String']['output'];
};

export type UserErrorCode =
  | 'CONFLICT'
  | 'INVALID_FORMAT'
  | 'INVALID_TRANSITION'
  | 'INVALID_VALUE'
  | 'NOT_FOUND'
  | 'OUT_OF_RANGE'
  | 'REQUIRED'
  | 'TOO_LONG';

export type Variant =
  | 'CHALLENGER'
  | 'CHAMPION';

export type Verdict =
  | 'ACCEPTED'
  | 'EDITED'
  | 'REJECTED';

export type Viewer = {
  __typename?: 'Viewer';
  email: Scalars['String']['output'];
};

export type NavCountsQueryVariables = Exact<{ [key: string]: never; }>;


export type NavCountsQuery = { __typename?: 'Query', crew: { __typename?: 'CrewSummary', awaitingApproval: number } };

export type AgentQueryVariables = Exact<{
  slug: Scalars['String']['input'];
}>;


export type AgentQuery = { __typename?: 'Query', agent?: { __typename?: 'Agent', id: string, slug: string, name: string, description: string, department: Department, owner: string, instructions: string, status: AgentStatus, version: number, triggers: Array<string>, minutesSavedPerRun: number, model: { __typename?: 'Model', id: string }, budget: { __typename?: 'Budget', maxSteps: number, maxCost: string }, tools: Array<{ __typename?: 'Tool', name: string, description: string, requiresApproval: boolean }>, scorecard: { __typename?: 'Scorecard', finishedRuns: number, successRate: number, reviewed: number, acceptanceRate?: number | null, totalCost: string, costPerRun: string, hoursSaved: number, valueDelivered: string, netValue: string, avgDurationSeconds: number }, activeExperiment?: { __typename?: 'Experiment', id: string, trafficPercent: number, hypothesis: string, createdAt: string, champion: { __typename?: 'Model', id: string }, challenger: { __typename?: 'Model', id: string }, championScorecard: { __typename?: 'Scorecard', finishedRuns: number, successRate: number, acceptanceRate?: number | null, costPerRun: string, netValue: string }, challengerScorecard: { __typename?: 'Scorecard', finishedRuns: number, successRate: number, acceptanceRate?: number | null, costPerRun: string, netValue: string } } | null, evaluations: Array<{ __typename?: 'Evaluation', id: string, decision: Decision, reason: string, statusBefore: AgentStatus, statusAfter: AgentStatus, createdAt: string }>, runs: { __typename?: 'RunConnection', edges: Array<{ __typename?: 'RunEdge', node: { __typename?: 'Run', id: string, status: RunStatus, trigger: RunTrigger, variant: Variant, cost: string, createdAt: string, durationSeconds?: number | null, model: { __typename?: 'Model', id: string }, review?: { __typename?: 'Review', verdict: Verdict } | null } }>, pageInfo: { __typename?: 'PageInfo', hasNextPage: boolean, endCursor?: string | null } } } | null, models: Array<{ __typename?: 'ModelOption', label: string, available: boolean, outputPerMTok: number, model: { __typename?: 'Model', id: string } }> };

export type ActivateAgentMutationVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type ActivateAgentMutation = { __typename?: 'Mutation', activateAgent: { __typename?: 'AgentPayload', agent?: { __typename?: 'Agent', id: string, status: AgentStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type RetireAgentMutationVariables = Exact<{
  id: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
}>;


export type RetireAgentMutation = { __typename?: 'Mutation', retireAgent: { __typename?: 'AgentPayload', agent?: { __typename?: 'Agent', id: string, status: AgentStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type StartRunMutationVariables = Exact<{
  input: StartRunInput;
}>;


export type StartRunMutation = { __typename?: 'Mutation', startRun: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string } | null, userErrors: Array<{ __typename?: 'UserError', field: Array<string>, code: UserErrorCode, message: string }> } };

export type StartExperimentMutationVariables = Exact<{
  input: StartExperimentInput;
}>;


export type StartExperimentMutation = { __typename?: 'Mutation', startExperiment: { __typename?: 'ExperimentPayload', experiment?: { __typename?: 'Experiment', id: string } | null, userErrors: Array<{ __typename?: 'UserError', field: Array<string>, code: UserErrorCode, message: string }> } };

export type ConcludeExperimentMutationVariables = Exact<{
  id: Scalars['ID']['input'];
  force?: InputMaybe<Scalars['Boolean']['input']>;
}>;


export type ConcludeExperimentMutation = { __typename?: 'Mutation', concludeExperiment: { __typename?: 'ExperimentPayload', experiment?: { __typename?: 'Experiment', id: string, status: ExperimentStatus, outcome: string } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type PendingApprovalsQueryVariables = Exact<{ [key: string]: never; }>;


export type PendingApprovalsQuery = { __typename?: 'Query', runs: { __typename?: 'RunConnection', edges: Array<{ __typename?: 'RunEdge', node: { __typename?: 'Run', id: string, createdAt: string, agent: { __typename?: 'Agent', name: string, slug: string, owner: string }, pendingToolCall?: { __typename?: 'PendingToolCall', arguments: unknown, tool: { __typename?: 'Tool', name: string, description: string } } | null } }> } };

export type ApproveFromQueueMutationVariables = Exact<{
  runId: Scalars['ID']['input'];
}>;


export type ApproveFromQueueMutation = { __typename?: 'Mutation', approveToolCall: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string, status: RunStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type DenyFromQueueMutationVariables = Exact<{
  runId: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
}>;


export type DenyFromQueueMutation = { __typename?: 'Mutation', denyToolCall: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string, status: RunStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type CrewQueryVariables = Exact<{ [key: string]: never; }>;


export type CrewQuery = { __typename?: 'Query', crew: { __typename?: 'CrewSummary', activeAgents: number, onProbation: number, retired: number, runsLast7Days: number, awaitingApproval: number, hoursSavedLast30Days: number, spendLast30Days: string, netValueLast30Days: string }, agents: Array<{ __typename?: 'Agent', id: string, slug: string, name: string, department: Department, status: AgentStatus, model: { __typename?: 'Model', id: string }, activeExperiment?: { __typename?: 'Experiment', id: string, challenger: { __typename?: 'Model', id: string } } | null, scorecard: { __typename?: 'Scorecard', finishedRuns: number, successRate: number, acceptanceRate?: number | null, costPerRun: string, hoursSaved: number, netValue: string } }> };

export type EvaluateCrewMutationVariables = Exact<{ [key: string]: never; }>;


export type EvaluateCrewMutation = { __typename?: 'Mutation', evaluateCrew: { __typename?: 'EvaluateCrewPayload', evaluations: Array<{ __typename?: 'Evaluation', id: string, decision: Decision, reason: string, statusBefore: AgentStatus, statusAfter: AgentStatus, agent: { __typename?: 'Agent', slug: string, name: string } }>, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type BacklogQueryVariables = Exact<{ [key: string]: never; }>;


export type BacklogQuery = { __typename?: 'Query', opportunities: Array<{ __typename?: 'Opportunity', id: string, title: string, problem: string, department: Department, submittedBy: string, weeklyVolume: number, minutesPerTask: number, hoursPerWeek: number, feasibility: number, score: number, status: OpportunityStatus, agent?: { __typename?: 'Agent', slug: string, name: string } | null }>, agents: Array<{ __typename?: 'Agent', id: string, name: string }> };

export type SubmitOpportunityMutationVariables = Exact<{
  input: SubmitOpportunityInput;
}>;


export type SubmitOpportunityMutation = { __typename?: 'Mutation', submitOpportunity: { __typename?: 'OpportunityPayload', opportunity?: { __typename?: 'Opportunity', id: string, score: number } | null, userErrors: Array<{ __typename?: 'UserError', field: Array<string>, code: UserErrorCode, message: string }> } };

export type MoveOpportunityMutationVariables = Exact<{
  id: Scalars['ID']['input'];
  status: OpportunityStatus;
  agentId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type MoveOpportunityMutation = { __typename?: 'Mutation', moveOpportunity: { __typename?: 'OpportunityPayload', opportunity?: { __typename?: 'Opportunity', id: string, status: OpportunityStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type AgentOptionsQueryVariables = Exact<{ [key: string]: never; }>;


export type AgentOptionsQuery = { __typename?: 'Query', tools: Array<{ __typename?: 'Tool', name: string, description: string, requiresApproval: boolean }>, models: Array<{ __typename?: 'ModelOption', label: string, available: boolean, inputPerMTok: number, outputPerMTok: number, model: { __typename?: 'Model', id: string, provider: ModelProvider } }> };

export type CreateAgentMutationVariables = Exact<{
  input: CreateAgentInput;
}>;


export type CreateAgentMutation = { __typename?: 'Mutation', createAgent: { __typename?: 'AgentPayload', agent?: { __typename?: 'Agent', id: string, slug: string } | null, userErrors: Array<{ __typename?: 'UserError', field: Array<string>, code: UserErrorCode, message: string }> } };

export type RunQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type RunQuery = { __typename?: 'Query', run?: { __typename?: 'Run', id: string, status: RunStatus, trigger: RunTrigger, variant: Variant, input: unknown, output: string, failureReason: string, turns: number, cost: string, createdAt: string, durationSeconds?: number | null, usage: { __typename?: 'Usage', inputTokens: number, cachedInputTokens: number, outputTokens: number }, model: { __typename?: 'Model', id: string }, agent: { __typename?: 'Agent', slug: string, name: string }, pendingToolCall?: { __typename?: 'PendingToolCall', callId: string, arguments: unknown, tool: { __typename?: 'Tool', name: string, description: string } } | null, review?: { __typename?: 'Review', verdict: Verdict, reviewer: string, note: string, at: string } | null, steps: Array<{ __typename?: 'RunStep', index: number, kind: StepKind, toolName?: string | null, summary: string, detail: unknown, latencyMs: number, cost: string, at: string }> } | null };

export type ApproveToolCallMutationVariables = Exact<{
  runId: Scalars['ID']['input'];
}>;


export type ApproveToolCallMutation = { __typename?: 'Mutation', approveToolCall: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string, status: RunStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type DenyToolCallMutationVariables = Exact<{
  runId: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
}>;


export type DenyToolCallMutation = { __typename?: 'Mutation', denyToolCall: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string, status: RunStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type ReviewRunMutationVariables = Exact<{
  input: ReviewRunInput;
}>;


export type ReviewRunMutation = { __typename?: 'Mutation', reviewRun: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string, status: RunStatus, review?: { __typename?: 'Review', verdict: Verdict, reviewer: string, note: string, at: string } | null } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };

export type CancelRunMutationVariables = Exact<{
  runId: Scalars['ID']['input'];
}>;


export type CancelRunMutation = { __typename?: 'Mutation', cancelRun: { __typename?: 'RunPayload', run?: { __typename?: 'Run', id: string, status: RunStatus } | null, userErrors: Array<{ __typename?: 'UserError', message: string }> } };


export const NavCountsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"NavCounts"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"crew"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"awaitingApproval"}}]}}]}}]} as unknown as DocumentNode<NavCountsQuery, NavCountsQueryVariables>;
export const AgentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Agent"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"slug"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"agent"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"slug"},"value":{"kind":"Variable","name":{"kind":"Name","value":"slug"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"department"}},{"kind":"Field","name":{"kind":"Name","value":"owner"}},{"kind":"Field","name":{"kind":"Name","value":"instructions"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"triggers"}},{"kind":"Field","name":{"kind":"Name","value":"minutesSavedPerRun"}},{"kind":"Field","name":{"kind":"Name","value":"model"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"budget"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"maxSteps"}},{"kind":"Field","name":{"kind":"Name","value":"maxCost"}}]}},{"kind":"Field","name":{"kind":"Name","value":"tools"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"requiresApproval"}}]}},{"kind":"Field","name":{"kind":"Name","value":"scorecard"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"windowDays"},"value":{"kind":"IntValue","value":"30"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"finishedRuns"}},{"kind":"Field","name":{"kind":"Name","value":"successRate"}},{"kind":"Field","name":{"kind":"Name","value":"reviewed"}},{"kind":"Field","name":{"kind":"Name","value":"acceptanceRate"}},{"kind":"Field","name":{"kind":"Name","value":"totalCost"}},{"kind":"Field","name":{"kind":"Name","value":"costPerRun"}},{"kind":"Field","name":{"kind":"Name","value":"hoursSaved"}},{"kind":"Field","name":{"kind":"Name","value":"valueDelivered"}},{"kind":"Field","name":{"kind":"Name","value":"netValue"}},{"kind":"Field","name":{"kind":"Name","value":"avgDurationSeconds"}}]}},{"kind":"Field","name":{"kind":"Name","value":"activeExperiment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"trafficPercent"}},{"kind":"Field","name":{"kind":"Name","value":"hypothesis"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"champion"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"challenger"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"championScorecard"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"finishedRuns"}},{"kind":"Field","name":{"kind":"Name","value":"successRate"}},{"kind":"Field","name":{"kind":"Name","value":"acceptanceRate"}},{"kind":"Field","name":{"kind":"Name","value":"costPerRun"}},{"kind":"Field","name":{"kind":"Name","value":"netValue"}}]}},{"kind":"Field","name":{"kind":"Name","value":"challengerScorecard"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"finishedRuns"}},{"kind":"Field","name":{"kind":"Name","value":"successRate"}},{"kind":"Field","name":{"kind":"Name","value":"acceptanceRate"}},{"kind":"Field","name":{"kind":"Name","value":"costPerRun"}},{"kind":"Field","name":{"kind":"Name","value":"netValue"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"evaluations"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"last"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"decision"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"statusBefore"}},{"kind":"Field","name":{"kind":"Name","value":"statusAfter"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"runs"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"15"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"trigger"}},{"kind":"Field","name":{"kind":"Name","value":"variant"}},{"kind":"Field","name":{"kind":"Name","value":"cost"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"durationSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"model"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"review"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"verdict"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"models"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"model"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"available"}},{"kind":"Field","name":{"kind":"Name","value":"outputPerMTok"}}]}}]}}]} as unknown as DocumentNode<AgentQuery, AgentQueryVariables>;
export const ActivateAgentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ActivateAgent"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"activateAgent"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<ActivateAgentMutation, ActivateAgentMutationVariables>;
export const RetireAgentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RetireAgent"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"retireAgent"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<RetireAgentMutation, RetireAgentMutationVariables>;
export const StartRunDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartRun"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartRunInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startRun"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"field"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<StartRunMutation, StartRunMutationVariables>;
export const StartExperimentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartExperiment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartExperimentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startExperiment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"experiment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"field"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<StartExperimentMutation, StartExperimentMutationVariables>;
export const ConcludeExperimentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ConcludeExperiment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"force"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"concludeExperiment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"force"},"value":{"kind":"Variable","name":{"kind":"Name","value":"force"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"experiment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<ConcludeExperimentMutation, ConcludeExperimentMutationVariables>;
export const PendingApprovalsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PendingApprovals"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"runs"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"status"},"value":{"kind":"ListValue","values":[{"kind":"EnumValue","value":"AWAITING_APPROVAL"}]}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"50"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"owner"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pendingToolCall"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"arguments"}},{"kind":"Field","name":{"kind":"Name","value":"tool"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]}}]}}]}}]}}]} as unknown as DocumentNode<PendingApprovalsQuery, PendingApprovalsQueryVariables>;
export const ApproveFromQueueDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ApproveFromQueue"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approveToolCall"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<ApproveFromQueueMutation, ApproveFromQueueMutationVariables>;
export const DenyFromQueueDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DenyFromQueue"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyToolCall"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<DenyFromQueueMutation, DenyFromQueueMutationVariables>;
export const CrewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Crew"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"crew"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"activeAgents"}},{"kind":"Field","name":{"kind":"Name","value":"onProbation"}},{"kind":"Field","name":{"kind":"Name","value":"retired"}},{"kind":"Field","name":{"kind":"Name","value":"runsLast7Days"}},{"kind":"Field","name":{"kind":"Name","value":"awaitingApproval"}},{"kind":"Field","name":{"kind":"Name","value":"hoursSavedLast30Days"}},{"kind":"Field","name":{"kind":"Name","value":"spendLast30Days"}},{"kind":"Field","name":{"kind":"Name","value":"netValueLast30Days"}}]}},{"kind":"Field","name":{"kind":"Name","value":"agents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"department"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"model"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"activeExperiment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"challenger"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"scorecard"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"windowDays"},"value":{"kind":"IntValue","value":"30"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"finishedRuns"}},{"kind":"Field","name":{"kind":"Name","value":"successRate"}},{"kind":"Field","name":{"kind":"Name","value":"acceptanceRate"}},{"kind":"Field","name":{"kind":"Name","value":"costPerRun"}},{"kind":"Field","name":{"kind":"Name","value":"hoursSaved"}},{"kind":"Field","name":{"kind":"Name","value":"netValue"}}]}}]}}]}}]} as unknown as DocumentNode<CrewQuery, CrewQueryVariables>;
export const EvaluateCrewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EvaluateCrew"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"evaluateCrew"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"evaluations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"decision"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"statusBefore"}},{"kind":"Field","name":{"kind":"Name","value":"statusAfter"}},{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<EvaluateCrewMutation, EvaluateCrewMutationVariables>;
export const BacklogDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Backlog"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"opportunities"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"problem"}},{"kind":"Field","name":{"kind":"Name","value":"department"}},{"kind":"Field","name":{"kind":"Name","value":"submittedBy"}},{"kind":"Field","name":{"kind":"Name","value":"weeklyVolume"}},{"kind":"Field","name":{"kind":"Name","value":"minutesPerTask"}},{"kind":"Field","name":{"kind":"Name","value":"hoursPerWeek"}},{"kind":"Field","name":{"kind":"Name","value":"feasibility"}},{"kind":"Field","name":{"kind":"Name","value":"score"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"agents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"status"},"value":{"kind":"ListValue","values":[{"kind":"EnumValue","value":"ACTIVE"},{"kind":"EnumValue","value":"PROBATION"}]}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<BacklogQuery, BacklogQueryVariables>;
export const SubmitOpportunityDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SubmitOpportunity"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SubmitOpportunityInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"submitOpportunity"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"opportunity"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"field"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<SubmitOpportunityMutation, SubmitOpportunityMutationVariables>;
export const MoveOpportunityDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MoveOpportunity"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"status"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"OpportunityStatus"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"agentId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"moveOpportunity"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"status"},"value":{"kind":"Variable","name":{"kind":"Name","value":"status"}}},{"kind":"Argument","name":{"kind":"Name","value":"agentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"agentId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"opportunity"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<MoveOpportunityMutation, MoveOpportunityMutationVariables>;
export const AgentOptionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AgentOptions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tools"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"requiresApproval"}}]}},{"kind":"Field","name":{"kind":"Name","value":"models"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"model"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"provider"}}]}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"available"}},{"kind":"Field","name":{"kind":"Name","value":"inputPerMTok"}},{"kind":"Field","name":{"kind":"Name","value":"outputPerMTok"}}]}}]}}]} as unknown as DocumentNode<AgentOptionsQuery, AgentOptionsQueryVariables>;
export const CreateAgentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateAgent"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateAgentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createAgent"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"field"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<CreateAgentMutation, CreateAgentMutationVariables>;
export const RunDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Run"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"trigger"}},{"kind":"Field","name":{"kind":"Name","value":"variant"}},{"kind":"Field","name":{"kind":"Name","value":"input"}},{"kind":"Field","name":{"kind":"Name","value":"output"}},{"kind":"Field","name":{"kind":"Name","value":"failureReason"}},{"kind":"Field","name":{"kind":"Name","value":"turns"}},{"kind":"Field","name":{"kind":"Name","value":"cost"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"durationSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"usage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inputTokens"}},{"kind":"Field","name":{"kind":"Name","value":"cachedInputTokens"}},{"kind":"Field","name":{"kind":"Name","value":"outputTokens"}}]}},{"kind":"Field","name":{"kind":"Name","value":"model"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"agent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pendingToolCall"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callId"}},{"kind":"Field","name":{"kind":"Name","value":"arguments"}},{"kind":"Field","name":{"kind":"Name","value":"tool"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"review"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"reviewer"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"at"}}]}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"index"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"toolName"}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","name":{"kind":"Name","value":"detail"}},{"kind":"Field","name":{"kind":"Name","value":"latencyMs"}},{"kind":"Field","name":{"kind":"Name","value":"cost"}},{"kind":"Field","name":{"kind":"Name","value":"at"}}]}}]}}]}}]} as unknown as DocumentNode<RunQuery, RunQueryVariables>;
export const ApproveToolCallDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ApproveToolCall"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approveToolCall"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<ApproveToolCallMutation, ApproveToolCallMutationVariables>;
export const DenyToolCallDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DenyToolCall"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyToolCall"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<DenyToolCallMutation, DenyToolCallMutationVariables>;
export const ReviewRunDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReviewRun"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ReviewRunInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reviewRun"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"review"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"reviewer"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"at"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<ReviewRunMutation, ReviewRunMutationVariables>;
export const CancelRunDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelRun"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelRun"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"userErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<CancelRunMutation, CancelRunMutationVariables>;