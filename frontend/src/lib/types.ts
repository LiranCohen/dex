// API response types matching the Go backend

export interface Task {
  ID: string;
  ProjectID: string;
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
  | 'blocked'
  | 'ready'
  | 'running'
  | 'paused'
  | 'quarantined'
  | 'completed'
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
