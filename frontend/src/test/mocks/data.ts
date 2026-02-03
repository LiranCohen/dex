import type { Quest, Task, Approval, QuestMessage, ObjectiveDraft, ChecklistItem, ChecklistResponse } from '../../lib/types';

// Mock Quests
export const mockQuest: Quest = {
  id: 'quest-1',
  project_id: 'project-1',
  title: 'Test Quest',
  status: 'active',
  model: 'sonnet',
  auto_start_default: true,
  created_at: '2024-01-01T00:00:00Z',
  summary: {
    total_tasks: 3,
    completed_tasks: 1,
    running_tasks: 1,
    failed_tasks: 0,
    blocked_tasks: 0,
    pending_tasks: 1,
    total_dollars_used: 0.50,
  },
};

export const mockCompletedQuest: Quest = {
  id: 'quest-2',
  project_id: 'project-1',
  title: 'Completed Quest',
  status: 'completed',
  model: 'opus',
  auto_start_default: false,
  created_at: '2024-01-01T00:00:00Z',
  completed_at: '2024-01-02T00:00:00Z',
};

export const mockQuests: Quest[] = [mockQuest, mockCompletedQuest];

// Mock Tasks
export const mockTask: Task = {
  ID: 'task-1',
  ProjectID: 'project-1',
  QuestID: 'quest-1',
  QuestTitle: 'Test Quest',
  GitHubIssueNumber: null,
  Title: 'Implement feature X',
  Description: 'A detailed description of the task',
  ParentID: null,
  Type: 'feature',
  Hat: 'coder',
  Priority: 1,
  AutonomyLevel: 3,
  Status: 'running',
  BaseBranch: 'main',
  WorktreePath: '/path/to/worktree',
  BranchName: 'feature/x',
  PRNumber: null,
  TokenBudget: 100000,
  TokenUsed: 25000,
  InputTokens: 20000,
  OutputTokens: 5000,
  TimeBudgetMin: 60,
  TimeUsedMin: 15,
  DollarBudget: 10.00,
  DollarUsed: 2.50,
  CreatedAt: '2024-01-01T00:00:00Z',
  StartedAt: '2024-01-01T01:00:00Z',
  CompletedAt: null,
  IsBlocked: false,
};

export const mockPendingTask: Task = {
  ...mockTask,
  ID: 'task-2',
  Title: 'Pending task',
  Status: 'pending',
  StartedAt: null,
};

export const mockCompletedTask: Task = {
  ...mockTask,
  ID: 'task-3',
  Title: 'Completed task',
  Status: 'completed',
  CompletedAt: '2024-01-01T02:00:00Z',
};

export const mockTasks: Task[] = [mockTask, mockPendingTask, mockCompletedTask];

// Mock Approvals
export const mockApproval: Approval = {
  id: 'approval-1',
  task_id: 'task-1',
  session_id: 'session-1',
  type: 'commit',
  title: 'Approve commit',
  description: 'feat: add new feature',
  status: 'pending',
  created_at: '2024-01-01T00:00:00Z',
};

export const mockPrApproval: Approval = {
  id: 'approval-2',
  task_id: 'task-1',
  type: 'pr',
  title: 'Approve PR',
  description: 'Create pull request for feature',
  status: 'pending',
  created_at: '2024-01-01T01:00:00Z',
};

export const mockApprovals: Approval[] = [mockApproval, mockPrApproval];

// Mock Quest Messages
export const mockUserMessage: QuestMessage = {
  id: 'msg-1',
  quest_id: 'quest-1',
  role: 'user',
  content: 'Please help me implement feature X',
  created_at: '2024-01-01T00:00:00Z',
};

export const mockAssistantMessage: QuestMessage = {
  id: 'msg-2',
  quest_id: 'quest-1',
  role: 'assistant',
  content: 'I understand you want to implement feature X. Let me analyze the codebase.',
  tool_calls: [
    {
      tool_name: 'Read',
      input: { path: '/src/main.ts' },
      output: '// main file content',
      is_error: false,
      duration_ms: 150,
    },
  ],
  created_at: '2024-01-01T00:01:00Z',
};

export const mockMessages: QuestMessage[] = [mockUserMessage, mockAssistantMessage];

// Mock Objective Draft
export const mockObjectiveDraft: ObjectiveDraft = {
  draft_id: 'draft-1',
  title: 'Implement authentication',
  description: 'Add user authentication with JWT tokens',
  hat: 'coder',
  checklist: {
    must_have: [
      'Create login endpoint',
      'Add JWT token generation',
      'Implement auth middleware',
    ],
    optional: [
      'Add refresh token support',
    ],
  },
  auto_start: true,
  complexity: 'simple',
  estimated_iterations: 5,
  estimated_budget: 2.50,
};

// Mock Checklist
export const mockChecklistItems: ChecklistItem[] = [
  {
    id: 'item-1',
    checklist_id: 'checklist-1',
    description: 'Create login endpoint',
    status: 'done',
    sort_order: 0,
    completed_at: '2024-01-01T01:00:00Z',
  },
  {
    id: 'item-2',
    checklist_id: 'checklist-1',
    description: 'Add JWT token generation',
    status: 'in_progress',
    sort_order: 1,
  },
  {
    id: 'item-3',
    checklist_id: 'checklist-1',
    description: 'Implement auth middleware',
    status: 'pending',
    sort_order: 2,
  },
];

export const mockChecklistResponse: ChecklistResponse = {
  checklist: {
    id: 'checklist-1',
    task_id: 'task-1',
    created_at: '2024-01-01T00:00:00Z',
  },
  items: mockChecklistItems,
  summary: {
    total: 3,
    done: 1,
    failed: 0,
    pending: 2,
    all_done: false,
  },
};
