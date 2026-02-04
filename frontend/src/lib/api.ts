const BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';

export interface ApiError {
  message: string;
  status: number;
  errorType?: string;    // Specific error type (e.g., 'billing_error', 'rate_limit')
  retryable?: boolean;   // Whether the operation can be retried
  data?: Record<string, unknown>;  // Full response data for custom handling
}

export function isApiError(error: unknown): error is ApiError {
  return typeof error === 'object' && error !== null && 'status' in error && 'message' in error;
}

class ApiClient {
  private getToken(): string | null {
    return localStorage.getItem('auth_token');
  }

  private clearAuthAndRedirect(): void {
    // Clear auth state
    localStorage.removeItem('auth_token');
    localStorage.removeItem('auth-storage');
    // Redirect to login if not already there
    if (window.location.pathname !== '/login') {
      window.location.href = '/login';
    }
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const token = this.getToken();
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
    };

    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(`${BASE_URL}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    if (!response.ok) {
      // Handle 401 Unauthorized - clear auth and redirect to login
      if (response.status === 401) {
        this.clearAuthAndRedirect();
      }

      const error: ApiError = {
        message: response.statusText,
        status: response.status,
      };
      try {
        const data = await response.json();
        const msg = data.message || data.error;
        // Ensure message is always a string (prevent React error #310 if object is passed)
        error.message = typeof msg === 'string' ? msg : (msg ? JSON.stringify(msg) : response.statusText);
        // Preserve structured error info for special error types
        if (typeof data.error === 'string') {
          error.errorType = data.error;
        }
        if (typeof data.retryable === 'boolean') {
          error.retryable = data.retryable;
        }
        // Store full data for custom handling (e.g., billing errors with user_message)
        error.data = data;
      } catch {
        // Use default statusText
      }
      throw error;
    }

    // Handle 204 No Content
    if (response.status === 204) {
      return undefined as T;
    }

    return response.json();
  }

  get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }

  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('POST', path, body);
  }

  put<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('PUT', path, body);
  }

  delete<T>(path: string): Promise<T> {
    return this.request<T>('DELETE', path);
  }
}

export const api = new ApiClient();

// Approval API functions
export async function fetchApprovals(): Promise<import('./types').ApprovalsResponse> {
  return api.get('/approvals');
}

export async function fetchApproval(id: string): Promise<import('./types').Approval> {
  return api.get(`/approvals/${id}`);
}

export async function approveApproval(id: string): Promise<void> {
  return api.post(`/approvals/${id}/approve`);
}

export async function rejectApproval(id: string): Promise<void> {
  return api.post(`/approvals/${id}/reject`);
}

// Activity API functions
export async function fetchTaskActivity(taskId: string): Promise<import('./types').ActivityResponse> {
  return api.get(`/tasks/${taskId}/activity`);
}

// Planning API functions
export async function fetchPlanning(taskId: string): Promise<import('./types').PlanningResponse> {
  return api.get(`/tasks/${taskId}/planning`);
}

export async function sendPlanningResponse(taskId: string, response: string): Promise<import('./types').PlanningResponse> {
  return api.post(`/tasks/${taskId}/planning/respond`, { response });
}

export async function acceptPlan(taskId: string): Promise<{ message: string; task_id: string; refined_prompt: string }> {
  return api.post(`/tasks/${taskId}/planning/accept`);
}

export async function skipPlanning(taskId: string): Promise<{ message: string; task_id: string }> {
  return api.post(`/tasks/${taskId}/planning/skip`);
}

// Checklist API functions
export async function fetchChecklist(taskId: string): Promise<import('./types').ChecklistResponse> {
  return api.get(`/tasks/${taskId}/checklist`);
}

export async function acceptChecklist(
  taskId: string,
  selectedOptional: number[]
): Promise<{ message: string; task_id: string }> {
  return api.post(`/tasks/${taskId}/checklist/accept`, { selected_optional: selectedOptional });
}

export async function createRemediation(
  taskId: string
): Promise<{ message: string; task: import('./types').Task; original_task_id: string; issues_count: number }> {
  return api.post(`/tasks/${taskId}/remediate`);
}

// Quest API functions
export async function fetchQuests(projectId: string): Promise<import('./types').Quest[]> {
  return api.get(`/projects/${projectId}/quests`);
}

export async function createQuest(projectId: string, model?: import('./types').QuestModel): Promise<import('./types').Quest> {
  return api.post(`/projects/${projectId}/quests`, model ? { model } : undefined);
}

export async function fetchQuest(questId: string): Promise<import('./types').QuestResponse> {
  return api.get(`/quests/${questId}`);
}

export async function deleteQuest(questId: string): Promise<{ message: string }> {
  return api.delete(`/quests/${questId}`);
}

export async function sendQuestMessage(questId: string, content: string): Promise<{ message: import('./types').QuestMessage }> {
  return api.post(`/quests/${questId}/messages`, { content });
}

export async function completeQuest(questId: string): Promise<import('./types').Quest> {
  return api.post(`/quests/${questId}/complete`);
}

export async function reopenQuest(questId: string): Promise<import('./types').Quest> {
  return api.post(`/quests/${questId}/reopen`);
}

export async function updateQuestModel(questId: string, model: import('./types').QuestModel): Promise<import('./types').Quest> {
  return api.put(`/quests/${questId}/model`, { model });
}

export async function fetchQuestTasks(questId: string): Promise<import('./types').Task[]> {
  return api.get(`/quests/${questId}/tasks`);
}

export interface CreateObjectiveResult {
  message: string;
  task: import('./types').Task;
  auto_started?: boolean;
  auto_start_error?: string;
  worktree_path?: string;
  session_id?: string;
}

export async function createObjective(
  questId: string,
  draft: import('./types').ObjectiveDraft,
  selectedOptional: number[]
): Promise<CreateObjectiveResult> {
  const payload = {
    draft_id: draft.draft_id,
    title: draft.title,
    description: draft.description,
    hat: draft.hat,
    must_have: draft.checklist.must_have,
    optional: draft.checklist.optional || [],
    selected_optional: selectedOptional,
    auto_start: draft.auto_start,
    blocked_by: draft.blocked_by || [],
  };
  console.log('createObjective API call:', { questId, payload });
  const result = await api.post<CreateObjectiveResult>(`/quests/${questId}/objectives`, payload);
  console.log('createObjective API response:', result);
  return result;
}

export interface BatchCreateObjectiveResult {
  message: string;
  tasks: Array<{
    draft_id: string;
    task: import('./types').Task;
    auto_started?: boolean;
    worktree_path?: string;
    session_id?: string;
  }>;
  draft_mapping: Record<string, string>;
  auto_started?: string[];
  auto_start_errors?: string[];
}

export async function createObjectivesBatch(
  questId: string,
  drafts: Array<{ draft: import('./types').ObjectiveDraft; selectedOptional: number[] }>
): Promise<BatchCreateObjectiveResult> {
  const payload = {
    drafts: drafts.map(({ draft, selectedOptional }) => ({
      draft_id: draft.draft_id,
      title: draft.title,
      description: draft.description,
      hat: draft.hat,
      must_have: draft.checklist.must_have,
      optional: draft.checklist.optional || [],
      selected_optional: selectedOptional,
      auto_start: draft.auto_start,
      blocked_by: draft.blocked_by || [],
      complexity: draft.complexity,
      estimated_iterations: draft.estimated_iterations,
      // Repository targeting
      github_owner: draft.github_owner,
      github_repo: draft.github_repo,
      clone_url: draft.clone_url,
    })),
  };
  console.log('createObjectivesBatch API call:', { questId, payload });
  const result = await api.post<BatchCreateObjectiveResult>(`/quests/${questId}/objectives/batch`, payload);
  console.log('createObjectivesBatch API response:', result);
  return result;
}

export async function fetchPreflightCheck(questId: string): Promise<import('./types').PreflightCheck> {
  return api.get(`/quests/${questId}/preflight`);
}

export async function cancelQuestSession(questId: string): Promise<void> {
  // Attempt to cancel the quest session - this endpoint may not exist yet
  // The backend will need to implement POST /quests/:id/cancel
  try {
    await api.post(`/quests/${questId}/cancel`);
  } catch (err) {
    // Silently ignore if endpoint doesn't exist (404)
    // The user can still navigate away to effectively stop processing
    console.warn('Cancel quest session not available:', err);
  }
}

// Quest template API functions
export async function fetchQuestTemplates(projectId: string): Promise<import('./types').QuestTemplate[]> {
  return api.get(`/projects/${projectId}/quest-templates`);
}

export async function createQuestTemplate(
  projectId: string,
  name: string,
  description: string,
  initialPrompt: string
): Promise<import('./types').QuestTemplate> {
  return api.post(`/projects/${projectId}/quest-templates`, { name, description, initial_prompt: initialPrompt });
}

export async function fetchQuestTemplate(templateId: string): Promise<import('./types').QuestTemplate> {
  return api.get(`/quest-templates/${templateId}`);
}

export async function updateQuestTemplate(
  templateId: string,
  name: string,
  description: string,
  initialPrompt: string
): Promise<import('./types').QuestTemplate> {
  return api.put(`/quest-templates/${templateId}`, { name, description, initial_prompt: initialPrompt });
}

export async function deleteQuestTemplate(templateId: string): Promise<{ message: string }> {
  return api.delete(`/quest-templates/${templateId}`);
}
