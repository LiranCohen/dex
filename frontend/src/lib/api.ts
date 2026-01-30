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
        error.message = data.message || data.error || response.statusText;
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

export type { ApiError };
