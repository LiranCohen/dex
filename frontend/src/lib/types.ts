// API response types matching the Go backend

export interface Task {
  ID: string;
  ProjectID: string;
  QuestID: string | null;
  GitHubIssueNumber: number | null;
  Title: string;
  Description: string | null;
  ParentID: string | null;
  Type: 'epic' | 'feature' | 'bug' | 'task' | 'chore';
  Hat: string | null;
  Priority: number;
  AutonomyLevel: number;
  Status: TaskStatus;
  BaseBranch: string;
  WorktreePath: string | null;
  BranchName: string | null;
  PRNumber: number | null;
  TokenBudget: number | null;
  TokenUsed: number;
  TimeBudgetMin: number | null;
  TimeUsedMin: number;
  DollarBudget: number | null;
  DollarUsed: number;
  CreatedAt: string;
  StartedAt: string | null;
  CompletedAt: string | null;
}

export type TaskStatus =
  | 'pending'
  | 'planning'
  | 'blocked'
  | 'ready'
  | 'running'
  | 'paused'
  | 'quarantined'
  | 'completed'
  | 'completed_with_issues'
  | 'cancelled';

export interface Session {
  ID: string;
  TaskID: string;
  Hat: string;
  ClaudeSessionID: string | null;
  Status: SessionStatus;
  WorktreePath: string;
  IterationCount: number;
  MaxIterations: number;
  CompletionPromise: string | null;
  TokensUsed: number;
  TokensBudget: number | null;
  DollarsUsed: number;
  DollarsBudget: number | null;
  CreatedAt: string;
  StartedAt: string | null;
  EndedAt: string | null;
  Outcome: string | null;
}

export type SessionStatus = 'pending' | 'running' | 'paused' | 'completed' | 'failed';

export interface Approval {
  id: string;
  task_id?: string;
  session_id?: string;
  type: string;  // 'commit' | 'hat_transition' | 'pr' | 'merge' | 'conflict_resolution'
  title: string;
  description?: string;
  data?: unknown;
  status: 'pending' | 'approved' | 'rejected';
  created_at: string;
  resolved_at?: string;
}

export interface ApprovalsResponse {
  approvals: Approval[];
  count: number;
}

export interface SystemStatus {
  status: 'healthy' | 'unhealthy';
  timestamp: string;
  version: string;
  database: 'connected' | 'disconnected';
  error?: string;
}

export interface TasksResponse {
  tasks: Task[];
  count: number;
}

// WebSocket event types
export interface WebSocketEvent {
  type: string;
  payload: unknown;
  timestamp: string;
}

export interface TaskEvent extends WebSocketEvent {
  type: 'task.created' | 'task.updated' | 'task.completed' | 'task.deleted';
  payload: {
    task_id: string;
    task?: Task;
  };
}

export interface SessionEvent extends WebSocketEvent {
  type: 'session.started' | 'session.iteration' | 'session.completed';
  payload: {
    session_id: string;
    task_id: string;
    iteration?: number;
    session?: Session;
  };
}

export interface ApprovalEvent extends WebSocketEvent {
  type: 'approval.required';
  payload: {
    approval_id: string;
    type: string;
    title: string;
  };
}

// Activity event types
export type ActivityEventType =
  | 'user_message'
  | 'assistant_response'
  | 'tool_call'
  | 'tool_result'
  | 'completion_signal'
  | 'hat_transition'
  | 'debug_log'
  | 'checklist_update';

// Activity event from API
export interface Activity {
  id: string;
  session_id: string;
  iteration: number;
  event_type: ActivityEventType;
  content?: string;
  tokens_input?: number;
  tokens_output?: number;
  created_at: string;
}

// Parsed content for tool events
export interface ToolCallContent {
  name: string;
  input: Record<string, unknown>;
}

export interface ToolResultContent {
  name: string;
  result: { Output: string; IsError: boolean };
}

export interface HatTransitionContent {
  from_hat: string;
  to_hat: string;
}

export interface DebugLogContent {
  level: 'info' | 'warn' | 'error';
  message: string;
  duration_ms?: number;
  details?: unknown;
}

// API response for activity
export interface ActivityResponse {
  activity: Activity[];
  summary: {
    total_iterations: number;
    total_tokens: number;
    total_sessions?: number;
    completion_reason?: string;
  };
}

// Planning types
export type PlanningStatus = 'processing' | 'awaiting_response' | 'completed' | 'skipped';

export interface PlanningSession {
  id: string;
  task_id: string;
  status: PlanningStatus;
  original_prompt: string;
  refined_prompt?: string;
  pending_checklist?: PendingChecklist;
  created_at: string;
}

export interface PlanningMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  created_at: string;
}

export interface PlanningResponse {
  session: PlanningSession;
  messages: PlanningMessage[];
}

// Checklist types
export type ChecklistItemStatus = 'pending' | 'in_progress' | 'done' | 'failed' | 'skipped';

// Pending checklist during planning (before acceptance)
export interface PendingChecklist {
  must_have: string[];
  optional: string[];
}

// Checklist item after acceptance (no category/selected - all items are equally required)
export interface ChecklistItem {
  id: string;
  checklist_id: string;
  parent_id?: string;
  description: string;
  status: ChecklistItemStatus;
  verification_notes?: string;
  completed_at?: string;
  sort_order: number;
}

export interface Checklist {
  id: string;
  task_id: string;
  created_at: string;
}

export interface ChecklistSummary {
  total: number;
  done: number;
  failed: number;
  pending: number;
  all_done: boolean;
}

export interface ChecklistResponse {
  checklist: Checklist;
  items: ChecklistItem[];
  summary: ChecklistSummary;
}

// Checklist WebSocket event
export interface ChecklistEvent extends WebSocketEvent {
  type: 'checklist.updated';
  payload: {
    task_id: string;
    checklist_id: string;
    item: ChecklistItem;
  };
}

// Activity event type for checklist updates
export interface ChecklistUpdateContent {
  item_id: string;
  description: string;
  status: ChecklistItemStatus;
  notes?: string;
}

// Quest types
export type QuestStatus = 'active' | 'completed';
export type QuestModel = 'sonnet' | 'opus';

export interface QuestSummary {
  total_tasks: number;
  completed_tasks: number;
  running_tasks: number;
  failed_tasks: number;
  blocked_tasks: number;
  pending_tasks: number;
}

export interface Quest {
  id: string;
  project_id: string;
  title?: string;
  status: QuestStatus;
  model: QuestModel;
  auto_start_default: boolean;
  created_at: string;
  completed_at?: string;
  summary?: QuestSummary;
}

export interface QuestMessage {
  id: string;
  quest_id: string;
  role: 'user' | 'assistant';
  content: string;
  created_at: string;
}

export interface QuestResponse {
  quest: Quest;
  messages: QuestMessage[];
}

// Objective draft proposed by Dex during Quest conversation
export interface ObjectiveDraft {
  draft_id: string;
  title: string;
  description: string;
  hat: string;
  checklist: {
    must_have: string[];
    optional?: string[];
  };
  blocked_by?: string[];
  auto_start: boolean;
  estimated_iterations?: number;
  estimated_budget?: number; // estimated cost in dollars
}

// Pre-flight check result
export interface PreflightCheck {
  ok: boolean;
  warnings?: string[];
}

// Quest template for reusable quest patterns
export interface QuestTemplate {
  id: string;
  project_id: string;
  name: string;
  description?: string;
  initial_prompt: string;
  created_at: string;
}

// Quest WebSocket events
export interface QuestEvent extends WebSocketEvent {
  type: 'quest.created' | 'quest.message' | 'quest.completed' | 'quest.reopened' | 'quest.deleted' | 'quest.objective_draft' | 'quest.question' | 'quest.ready';
  payload: {
    quest_id: string;
    project_id?: string;
    message?: QuestMessage;
    draft?: ObjectiveDraft;
    question?: { question: string; options?: string[] };
    drafts?: string[];
    summary?: string;
  };
}
