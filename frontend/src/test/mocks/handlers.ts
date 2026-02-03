import { http, HttpResponse } from 'msw';
import {
  mockQuests,
  mockQuest,
  mockTasks,
  mockTask,
  mockApprovals,
  mockMessages,
  mockChecklistResponse,
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
    return HttpResponse.json({ task });
  }),

  http.get(`${API_BASE}/tasks/:taskId/checklist`, () => {
    // Transform mock data to match ObjectiveDetail's expected format
    return HttpResponse.json({
      items: [
        { id: 'item-1', task_id: 'task-1', description: 'Create login endpoint', is_completed: true, is_optional: false },
        { id: 'item-2', task_id: 'task-1', description: 'Add JWT token generation', is_completed: false, is_optional: false },
        { id: 'item-3', task_id: 'task-1', description: 'Implement auth middleware', is_completed: false, is_optional: false },
      ],
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
      activities: [
        { id: 'activity-1', type: 'started', content: 'Task started', created_at: '2024-01-01T01:00:00Z' },
        { id: 'activity-2', type: 'tool_call', content: 'Read file main.ts', created_at: '2024-01-01T01:01:00Z' },
      ],
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
];
