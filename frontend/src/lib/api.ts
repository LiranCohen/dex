const BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';

interface ApiError {
  message: string;
  status: number;
}

class ApiClient {
  private getToken(): string | null {
    return localStorage.getItem('auth_token');
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
      const error: ApiError = {
        message: response.statusText,
        status: response.status,
      };
      try {
        const data = await response.json();
        const msg = data.message || data.error;
        // Ensure message is always a string (prevent React error #310 if object is passed)
        error.message = typeof msg === 'string' ? msg : (msg ? JSON.stringify(msg) : response.statusText);
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

export async function updateChecklistItem(
  taskId: string,
  itemId: string,
  updates: { selected?: boolean; status?: string; verification_notes?: string }
): Promise<import('./types').ChecklistItem> {
  return api.put(`/tasks/${taskId}/checklist/items/${itemId}`, updates);
}

export async function acceptChecklist(
  taskId: string,
  selectedItems?: string[]
): Promise<{ message: string; task_id: string }> {
  return api.post(`/tasks/${taskId}/checklist/accept`, { selected_items: selectedItems });
}

export async function createRemediation(
  taskId: string
): Promise<{ message: string; task: import('./types').Task; original_task_id: string; issues_count: number }> {
  return api.post(`/tasks/${taskId}/remediate`);
}

export type { ApiError };
