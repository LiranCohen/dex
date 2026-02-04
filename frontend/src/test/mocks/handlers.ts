import { http, HttpResponse } from 'msw';
import {
  mockQuests,
  mockQuest,
  mockTasks,
  mockTask,
  mockApprovals,
  mockMessages,
} from './data';

const API_BASE = '/api/v1';

export const handlers = [
  // Quests - by project
  http.get(`${API_BASE}/projects/:projectId/quests`, () => {
    return HttpResponse.json(mockQuests);
  }),

  http.post(`${API_BASE}/projects/:projectId/quests`, async () => {
    return HttpResponse.json({ ...mockQuest, id: `quest-${Date.now()}` });
  }),

  // Quests - by quest ID
  http.get(`${API_BASE}/quests/:questId`, ({ params }) => {
    const quest = mockQuests.find((q) => q.id === params.questId);
    if (!quest) {
      return new HttpResponse(null, { status: 404 });
    }
    return HttpResponse.json({
      quest,
      messages: mockMessages,
    });
  }),

  http.delete(`${API_BASE}/quests/:questId`, () => {
    return HttpResponse.json({ message: 'Quest deleted' });
  }),

  http.post(`${API_BASE}/quests/:questId/messages`, () => {
    return HttpResponse.json({ message: { id: 'msg-new', quest_id: 'quest-1', role: 'user', content: 'test', created_at: new Date().toISOString() } });
  }),

  http.post(`${API_BASE}/quests/:questId/complete`, () => {
    return HttpResponse.json({ ...mockQuest, status: 'completed' });
  }),

  http.post(`${API_BASE}/quests/:questId/reopen`, () => {
    return HttpResponse.json({ ...mockQuest, status: 'active' });
  }),

  http.post(`${API_BASE}/quests/:questId/cancel`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.get(`${API_BASE}/quests/:questId/tasks`, () => {
    return HttpResponse.json(mockTasks);
  }),

  http.post(`${API_BASE}/quests/:questId/objectives`, () => {
    return HttpResponse.json({ task: mockTask, message: 'Objective created' });
  }),

  // Tasks (Objectives)
  http.get(`${API_BASE}/tasks`, () => {
    return HttpResponse.json({
      tasks: mockTasks,
      count: mockTasks.length,
    });
  }),

  http.get(`${API_BASE}/tasks/:taskId`, ({ params }) => {
    const task = mockTasks.find((t) => t.ID === params.taskId);
    if (!task) {
      return new HttpResponse(null, { status: 404 });
    }
    // Return task directly (component expects Task, not { task })
    return HttpResponse.json(task);
  }),

  http.get(`${API_BASE}/tasks/:taskId/checklist`, () => {
    return HttpResponse.json({
      checklist: { id: 'checklist-1', task_id: 'task-1', created_at: '2024-01-01T00:00:00Z' },
      items: [
        { id: 'item-1', checklist_id: 'checklist-1', description: 'Create login endpoint', status: 'done', sort_order: 0 },
        { id: 'item-2', checklist_id: 'checklist-1', description: 'Add JWT token generation', status: 'in_progress', sort_order: 1 },
        { id: 'item-3', checklist_id: 'checklist-1', description: 'Implement auth middleware', status: 'pending', sort_order: 2 },
      ],
      summary: { total: 3, done: 1, failed: 0, all_done: false },
    });
  }),

  http.post(`${API_BASE}/tasks/:taskId/start`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.post(`${API_BASE}/tasks/:taskId/stop`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.post(`${API_BASE}/tasks/:taskId/pause`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.post(`${API_BASE}/tasks/:taskId/resume`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.post(`${API_BASE}/tasks/:taskId/cancel`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.get(`${API_BASE}/tasks/:taskId/activity`, () => {
    return HttpResponse.json({
      activity: [
        { id: 'activity-1', session_id: 'session-1', iteration: 0, event_type: 'user_message', content: 'Task started', created_at: '2024-01-01T01:00:00Z' },
        { id: 'activity-2', session_id: 'session-1', iteration: 1, event_type: 'tool_call', content: 'Read file main.ts', created_at: '2024-01-01T01:01:00Z' },
      ],
      summary: { total_iterations: 1, total_tokens: 1000 },
    });
  }),

  // Approvals
  http.get(`${API_BASE}/approvals`, () => {
    return HttpResponse.json({
      approvals: mockApprovals,
      count: mockApprovals.length,
    });
  }),

  http.post(`${API_BASE}/approvals/:approvalId/approve`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.post(`${API_BASE}/approvals/:approvalId/reject`, () => {
    return HttpResponse.json({ success: true });
  }),

  // Objective drafts
  http.post(`${API_BASE}/quests/:questId/drafts/:draftId/accept`, () => {
    return HttpResponse.json({ task: mockTask });
  }),

  http.post(`${API_BASE}/quests/:questId/drafts/:draftId/reject`, () => {
    return HttpResponse.json({ success: true });
  }),

  // Questions
  http.post(`${API_BASE}/quests/:questId/answer`, () => {
    return HttpResponse.json({ success: true });
  }),

  // System status
  http.get(`${API_BASE}/status`, () => {
    return HttpResponse.json({
      status: 'healthy',
      timestamp: new Date().toISOString(),
      version: '1.0.0',
      database: 'connected',
    });
  }),

  // Projects
  http.get(`${API_BASE}/projects`, () => {
    return HttpResponse.json({
      projects: [
        {
          ID: 'proj_default',
          Name: 'Default Project',
          RepoPath: '.',
          GitHubOwner: null,
          GitHubRepo: null,
          RemoteOrigin: null,
          RemoteUpstream: null,
          DefaultBranch: 'main',
          CreatedAt: '2024-01-01T00:00:00Z',
        },
      ],
      count: 1,
    });
  }),
];
