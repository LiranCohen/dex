import { useState, useEffect, useCallback, useRef } from 'react';
import { Routes, Route, Navigate, useNavigate, useParams, Link } from 'react-router-dom';
import { useAuthStore } from './stores/auth';
import { api, fetchApprovals, approveApproval, rejectApproval, fetchQuests, createQuest, fetchQuest, fetchQuestTasks, sendQuestMessage, completeQuest, reopenQuest, deleteQuest, createObjective, updateQuestModel, fetchPreflightCheck } from './lib/api';
import { useWebSocket } from './hooks/useWebSocket';
import { Onboarding } from './components/Onboarding';
import { ActivityFeed } from './components/ActivityFeed';
import { PlanningPanel } from './components/PlanningPanel';
import { ObjectiveDraftCard } from './components/ObjectiveDraftCard';
import { ToolCallList } from './components/ToolCallList';
import type { Task, TasksResponse, SystemStatus, TaskStatus, WebSocketEvent, SessionEvent, Approval, Quest, QuestMessage, QuestResponse, ObjectiveDraft, QuestModel, PreflightCheck } from './lib/types';

// Setup status type
interface SetupStatus {
  passkey_registered: boolean;
  github_token_set: boolean;
  github_app_set: boolean;
  anthropic_key_set: boolean;
  setup_complete: boolean;
}

// WebAuthn helper to convert base64url to ArrayBuffer
function base64urlToBuffer(base64url: string): ArrayBuffer {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const binary = atob(base64 + padding);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

// WebAuthn helper to convert ArrayBuffer to base64url
function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const base64 = btoa(binary);
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

function LoginPage() {
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isCheckingStatus, setIsCheckingStatus] = useState(true);
  const [isConfigured, setIsConfigured] = useState(false);

  const setToken = useAuthStore((state) => state.setToken);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const navigate = useNavigate();

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  // Check if passkeys are configured
  useEffect(() => {
    const checkStatus = async () => {
      try {
        const status = await api.get<{ configured: boolean }>('/auth/passkey/status');
        setIsConfigured(status.configured);
      } catch (err) {
        console.error('Failed to check passkey status:', err);
        setError('Failed to connect to server');
      } finally {
        setIsCheckingStatus(false);
      }
    };
    checkStatus();
  }, []);

  // Handle passkey registration (first time setup)
  const handleRegister = async () => {
    setError(null);
    setIsLoading(true);

    try {
      // 1. Begin registration - get options from server
      // go-webauthn wraps the options in a "publicKey" field
      const beginResponse = await api.post<{
        session_id: string;
        user_id: string;
        options: { publicKey: PublicKeyCredentialCreationOptions };
      }>('/auth/passkey/register/begin');

      // 2. Convert base64url fields to ArrayBuffer for WebAuthn API
      const options = beginResponse.options.publicKey;
      const publicKeyOptions: PublicKeyCredentialCreationOptions = {
        ...options,
        challenge: base64urlToBuffer(options.challenge as unknown as string),
        user: {
          ...options.user,
          id: base64urlToBuffer(options.user.id as unknown as string),
        },
        excludeCredentials: options.excludeCredentials?.map((cred) => ({
          ...cred,
          id: base64urlToBuffer(cred.id as unknown as string),
        })),
      };

      // 3. Create credential using WebAuthn API
      const credential = await navigator.credentials.create({
        publicKey: publicKeyOptions,
      }) as PublicKeyCredential;

      if (!credential) {
        throw new Error('Failed to create credential');
      }

      const attestationResponse = credential.response as AuthenticatorAttestationResponse;

      // 4. Send credential to server to complete registration
      // session_id and user_id go in query params, credential in body
      const finishResponse = await api.post<{ token: string; user_id: string }>(
        `/auth/passkey/register/finish?session_id=${encodeURIComponent(beginResponse.session_id)}&user_id=${encodeURIComponent(beginResponse.user_id)}`,
        {
          id: credential.id,
          rawId: bufferToBase64url(credential.rawId),
          type: credential.type,
          response: {
            attestationObject: bufferToBase64url(attestationResponse.attestationObject),
            clientDataJSON: bufferToBase64url(attestationResponse.clientDataJSON),
          },
        }
      );

      // 5. Store JWT and navigate
      setToken(finishResponse.token, finishResponse.user_id);
      navigate('/', { replace: true });
    } catch (err: unknown) {
      let message = 'Registration failed';
      if (err instanceof Error) {
        message = err.message;
      } else if (err && typeof err === 'object' && 'message' in err) {
        message = String((err as { message: unknown }).message);
      }
      console.error('Registration error:', err);
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  // Handle passkey login
  const handleLogin = async () => {
    setError(null);
    setIsLoading(true);

    try {
      // 1. Begin login - get options from server
      // go-webauthn wraps the options in a "publicKey" field
      const beginResponse = await api.post<{
        session_id: string;
        user_id: string;
        options: { publicKey: PublicKeyCredentialRequestOptions };
      }>('/auth/passkey/login/begin');

      // 2. Convert base64url fields to ArrayBuffer for WebAuthn API
      const options = beginResponse.options.publicKey;
      const publicKeyOptions: PublicKeyCredentialRequestOptions = {
        ...options,
        challenge: base64urlToBuffer(options.challenge as unknown as string),
        allowCredentials: options.allowCredentials?.map((cred) => ({
          ...cred,
          id: base64urlToBuffer(cred.id as unknown as string),
        })),
      };

      // 3. Get credential using WebAuthn API
      const credential = await navigator.credentials.get({
        publicKey: publicKeyOptions,
      }) as PublicKeyCredential;

      if (!credential) {
        throw new Error('Failed to get credential');
      }

      const assertionResponse = credential.response as AuthenticatorAssertionResponse;

      // 4. Send assertion to server to complete login
      // session_id and user_id go in query params, credential in body
      const finishResponse = await api.post<{ token: string; user_id: string }>(
        `/auth/passkey/login/finish?session_id=${encodeURIComponent(beginResponse.session_id)}&user_id=${encodeURIComponent(beginResponse.user_id)}`,
        {
          id: credential.id,
          rawId: bufferToBase64url(credential.rawId),
          type: credential.type,
          response: {
            authenticatorData: bufferToBase64url(assertionResponse.authenticatorData),
            clientDataJSON: bufferToBase64url(assertionResponse.clientDataJSON),
            signature: bufferToBase64url(assertionResponse.signature),
            userHandle: assertionResponse.userHandle
              ? bufferToBase64url(assertionResponse.userHandle)
              : null,
          },
        }
      );

      // 5. Store JWT and navigate
      setToken(finishResponse.token, finishResponse.user_id);
      navigate('/', { replace: true });
    } catch (err: unknown) {
      let message = 'Authentication failed';
      if (err instanceof Error) {
        message = err.message;
      } else if (err && typeof err === 'object' && 'message' in err) {
        message = String((err as { message: unknown }).message);
      }
      console.error('Login error:', err);
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  if (isCheckingStatus) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-4xl font-bold mb-2">Poindexter</h1>
          <p className="text-gray-400">Your AI Orchestration Genius</p>
        </div>

        <div className="bg-gray-800 rounded-lg p-6">
          {error && (
            <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          {isConfigured ? (
            // Login with existing passkey
            <div className="space-y-4">
              <div className="text-center mb-6">
                <svg
                  className="w-16 h-16 mx-auto text-blue-500 mb-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={1.5}
                    d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
                  />
                </svg>
                <p className="text-gray-300">
                  Use your passkey to sign in
                </p>
              </div>

              <button
                onClick={handleLogin}
                disabled={isLoading}
                className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isLoading ? (
                  <>
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
                    Authenticating...
                  </>
                ) : (
                  <>
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"
                      />
                    </svg>
                    Sign in with Passkey
                  </>
                )}
              </button>
            </div>
          ) : (
            // First time setup - register passkey
            <div className="space-y-4">
              <div className="text-center mb-6">
                <svg
                  className="w-16 h-16 mx-auto text-green-500 mb-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={1.5}
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                  />
                </svg>
                <h2 className="text-xl font-semibold mb-2">Welcome to Poindexter</h2>
                <p className="text-gray-400 text-sm">
                  Set up a passkey to secure your account. You'll use Face ID, Touch ID, or your device's security to sign in.
                </p>
              </div>

              <button
                onClick={handleRegister}
                disabled={isLoading}
                className="w-full bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isLoading ? (
                  <>
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
                    Setting up...
                  </>
                ) : (
                  <>
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"
                      />
                    </svg>
                    Set up Passkey
                  </>
                )}
              </button>

              <p className="text-xs text-gray-500 text-center">
                Passkeys are more secure than passwords and easier to use.
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// Status badge component for tasks
function StatusBadge({ status }: { status: TaskStatus }) {
  const colors: Record<TaskStatus, string> = {
    pending: 'bg-gray-600',
    planning: 'bg-purple-600',
    blocked: 'bg-yellow-600',
    ready: 'bg-blue-600',
    running: 'bg-green-600',
    paused: 'bg-orange-600',
    quarantined: 'bg-red-600',
    completed: 'bg-emerald-600',
    completed_with_issues: 'bg-amber-600',
    cancelled: 'bg-gray-500',
  };

  return (
    <span className={`px-2 py-1 rounded text-xs font-medium ${colors[status] || 'bg-gray-600'}`}>
      {status}
    </span>
  );
}

// Priority indicator
function PriorityDot({ priority }: { priority: number }) {
  const colors: Record<number, string> = {
    1: 'bg-red-500',
    2: 'bg-orange-500',
    3: 'bg-yellow-500',
    4: 'bg-blue-500',
    5: 'bg-gray-500',
  };

  return (
    <span
      className={`inline-block w-2 h-2 rounded-full ${colors[priority] || 'bg-gray-500'}`}
      title={`Priority ${priority}`}
    />
  );
}

function DashboardPage() {
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const logout = useAuthStore((state) => state.logout);
  const navigate = useNavigate();
  const { connected, subscribe } = useWebSocket();

  // Fetch initial data
  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      setError(null);

      try {
        const [status, tasksData] = await Promise.all([
          api.get<SystemStatus>('/system/status'),
          api.get<TasksResponse>('/tasks'),
        ]);

        setSystemStatus(status);
        setTasks(tasksData.tasks || []);
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to fetch data';
        setError(message);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  // Handle WebSocket events for real-time updates
  const handleWebSocketEvent = useCallback((event: WebSocketEvent) => {
    if (event.type.startsWith('task.')) {
      // Refetch tasks on any task event
      api.get<TasksResponse>('/tasks').then((data) => {
        setTasks(data.tasks || []);
      });
    }
  }, []);

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  const handleLogout = () => {
    logout();
    navigate('/login', { replace: true });
  };

  // Calculate stats
  const runningTasks = tasks.filter((t) => t.Status === 'running').length;
  const pendingTasks = tasks.filter((t) => t.Status === 'pending' || t.Status === 'ready').length;
  const completedTasks = tasks.filter((t) => t.Status === 'completed').length;

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading dashboard...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <h1 className="text-xl font-bold">Poindexter</h1>
            <Link to="/quests" className="text-gray-400 hover:text-white text-sm transition-colors">
              Quests
            </Link>
            <Link to="/tasks" className="text-gray-400 hover:text-white text-sm transition-colors">
              Objectives
            </Link>
            <Link to="/approvals" className="text-gray-400 hover:text-white text-sm transition-colors">
              Approvals
            </Link>
            <div className="flex items-center gap-2 text-sm">
              <span
                className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`}
              />
              <span className="text-gray-400">{connected ? 'Live' : 'Offline'}</span>
            </div>
          </div>
          <button
            onClick={handleLogout}
            className="text-gray-400 hover:text-white text-sm transition-colors"
          >
            Logout
          </button>
        </div>
      </header>

      <main className="p-4 max-w-4xl mx-auto">
        {error && (
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {/* System Status */}
        <div className="bg-gray-800 rounded-lg p-4 mb-4">
          <h2 className="text-lg font-semibold mb-3">System Status</h2>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div className="bg-gray-700 rounded-lg p-3 text-center">
              <p className="text-2xl font-bold text-green-400">{runningTasks}</p>
              <p className="text-xs text-gray-400">Running</p>
            </div>
            <div className="bg-gray-700 rounded-lg p-3 text-center">
              <p className="text-2xl font-bold text-blue-400">{pendingTasks}</p>
              <p className="text-xs text-gray-400">Queued</p>
            </div>
            <div className="bg-gray-700 rounded-lg p-3 text-center">
              <p className="text-2xl font-bold text-emerald-400">{completedTasks}</p>
              <p className="text-xs text-gray-400">Completed</p>
            </div>
            <div className="bg-gray-700 rounded-lg p-3 text-center">
              <p className="text-2xl font-bold">{tasks.length}</p>
              <p className="text-xs text-gray-400">Total</p>
            </div>
          </div>
          {systemStatus && (
            <p className="text-xs text-gray-500 mt-3">
              v{systemStatus.version} &middot; DB: {systemStatus.database}
            </p>
          )}
        </div>

        {/* Objectives List */}
        <div className="bg-gray-800 rounded-lg p-4">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Objectives</h2>
            <div className="flex items-center gap-3">
              <Link
                to="/quests"
                className="bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-3 py-1.5 rounded-lg transition-colors"
              >
                + Start Quest
              </Link>
              <Link
                to="/tasks"
                className="text-blue-400 hover:text-blue-300 text-sm transition-colors"
              >
                View All
              </Link>
            </div>
          </div>

          {tasks.length === 0 ? (
            <div className="text-center py-8">
              <p className="text-gray-400 mb-2">No objectives yet</p>
              <p className="text-sm text-gray-500">Start a quest to plan your work with Dex</p>
            </div>
          ) : (
            <div className="space-y-2">
              {tasks.slice(0, 10).map((task) => (
                <Link
                  key={task.ID}
                  to={`/tasks/${task.ID}`}
                  className="block bg-gray-700 hover:bg-gray-600 rounded-lg p-3 transition-colors"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <PriorityDot priority={task.Priority} />
                        <span className="font-medium truncate">{task.Title}</span>
                      </div>
                      <div className="flex items-center gap-2 text-xs text-gray-400">
                        <span className="uppercase">{task.Type}</span>
                        {task.Hat && (
                          <>
                            <span>&middot;</span>
                            <span>{task.Hat}</span>
                          </>
                        )}
                      </div>
                    </div>
                    <StatusBadge status={task.Status} />
                  </div>
                </Link>
              ))}
              {tasks.length > 10 && (
                <p className="text-center text-sm text-gray-500 pt-2">
                  +{tasks.length - 10} more objectives
                </p>
              )}
            </div>
          )}
        </div>

      </main>
    </div>
  );
}

function TaskListPage() {
  const navigate = useNavigate();
  const { isAuthenticated } = useAuthStore();
  const { connected, subscribe } = useWebSocket();

  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>('all');

  // Fetch tasks with optional status filter
  const fetchTasksData = useCallback(async () => {
    try {
      const params = statusFilter !== 'all' ? `?status=${statusFilter}` : '';
      const data = await api.get<TasksResponse>(`/tasks${params}`);
      setTasks(data.tasks || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load tasks');
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);

  // Redirect if not authenticated and fetch tasks on mount
  useEffect(() => {
    if (!isAuthenticated) {
      navigate('/login');
      return;
    }
    setLoading(true);
    fetchTasksData();
  }, [isAuthenticated, navigate, fetchTasksData]);

  // Subscribe to WebSocket for task events
  const handleWebSocketEvent = useCallback((event: WebSocketEvent) => {
    if (event.type.startsWith('task.')) {
      // Refetch tasks on any task event
      fetchTasksData();
    }
  }, [fetchTasksData]);

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Status filter options
  const statusOptions: { value: string; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'pending', label: 'Pending' },
    { value: 'planning', label: 'Planning' },
    { value: 'ready', label: 'Ready' },
    { value: 'running', label: 'Running' },
    { value: 'paused', label: 'Paused' },
    { value: 'completed', label: 'Completed' },
    { value: 'cancelled', label: 'Cancelled' },
    { value: 'failed', label: 'Failed' },
    { value: 'blocked', label: 'Blocked' },
    { value: 'quarantined', label: 'Quarantined' },
  ];

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading tasks...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between max-w-4xl mx-auto">
          <Link to="/" className="text-blue-400 hover:text-blue-300 text-sm flex items-center gap-1">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Dashboard
          </Link>
          <h1 className="text-lg font-semibold">Objectives</h1>
          <div className="flex items-center gap-2 text-sm">
            <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-gray-400">{connected ? 'Live' : 'Offline'}</span>
          </div>
        </div>
      </header>

      <main className="p-4 max-w-4xl mx-auto">
        {error && (
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {/* Filter Controls */}
        <div className="bg-gray-800 rounded-lg p-4 mb-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <label htmlFor="status-filter" className="text-sm text-gray-400">
                Filter by status:
              </label>
              <select
                id="status-filter"
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value)}
                className="bg-gray-700 text-white rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {statusOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
            <p className="text-sm text-gray-500">
              {tasks.length} objective{tasks.length !== 1 ? 's' : ''}
            </p>
          </div>
        </div>

        {/* Task List */}
        <div className="bg-gray-800 rounded-lg p-4">
          {tasks.length === 0 ? (
            <div className="text-center py-8">
              <svg
                className="w-12 h-12 mx-auto text-gray-600 mb-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
                />
              </svg>
              <p className="text-gray-400 mb-2">No objectives found</p>
              <p className="text-sm text-gray-500">
                {statusFilter !== 'all'
                  ? `No ${statusFilter} objectives. Try a different filter.`
                  : 'Create a task from the dashboard to get started.'}
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {tasks.map((task) => (
                <Link
                  key={task.ID}
                  to={`/tasks/${task.ID}`}
                  className="block bg-gray-700 hover:bg-gray-600 rounded-lg p-3 transition-colors"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <PriorityDot priority={task.Priority} />
                        <span className="font-medium truncate">{task.Title}</span>
                      </div>
                      <div className="flex items-center gap-2 text-xs text-gray-400">
                        <span className="uppercase">{task.Type}</span>
                        {task.Hat && (
                          <>
                            <span>&middot;</span>
                            <span>{task.Hat}</span>
                          </>
                        )}
                        {task.Description && (
                          <>
                            <span>&middot;</span>
                            <span className="truncate max-w-xs">{task.Description}</span>
                          </>
                        )}
                      </div>
                    </div>
                    <StatusBadge status={task.Status} />
                  </div>
                </Link>
              ))}
            </div>
          )}
        </div>
      </main>
    </div>
  );
}

function TaskDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<Task | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isStarting, setIsStarting] = useState(false);
  const [isPausing, setIsPausing] = useState(false);
  const [isResuming, setIsResuming] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);

  // Guard to prevent clicking multiple action buttons simultaneously
  const isActioning = isStarting || isPausing || isResuming || isCancelling;

  const [sessionInfo, setSessionInfo] = useState<{
    sessionId: string | null;
    iterationCount: number;
  }>({ sessionId: null, iterationCount: 0 });

  const { connected, subscribe } = useWebSocket();

  // Fetch task on mount
  useEffect(() => {
    const fetchTask = async () => {
      if (!id) return;

      setLoading(true);
      setError(null);

      try {
        const taskData = await api.get<Task>(`/tasks/${id}`);
        setTask(taskData);
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to fetch task';
        setError(message);
      } finally {
        setLoading(false);
      }
    };

    fetchTask();
  }, [id]);

  // Subscribe to WebSocket for session events
  const handleWebSocketEvent = useCallback((event: WebSocketEvent) => {
    if (!id) return;

    // Handle session events for this task
    if (event.type.startsWith('session.')) {
      const sessionEvent = event as SessionEvent;
      if (sessionEvent.payload.task_id === id) {
        if (event.type === 'session.started') {
          setSessionInfo({
            sessionId: sessionEvent.payload.session_id,
            iterationCount: 0,
          });
          // Refetch task to get updated status
          api.get<Task>(`/tasks/${id}`).then(setTask).catch(console.error);
        } else if (event.type === 'session.iteration') {
          setSessionInfo((prev) => ({
            ...prev,
            iterationCount: sessionEvent.payload.iteration || prev.iterationCount + 1,
          }));
        } else if (event.type === 'session.completed') {
          // Refetch task to get final status
          api.get<Task>(`/tasks/${id}`).then(setTask).catch(console.error);
        }
      }
    }

    // Handle task updates
    if (event.type.startsWith('task.') && (event.payload as { task_id?: string }).task_id === id) {
      api.get<Task>(`/tasks/${id}`).then(setTask).catch(console.error);
    }
  }, [id]);

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Handler for when planning is accepted or skipped
  // MUST be defined before early returns to satisfy React hooks rules
  const handlePlanningComplete = useCallback(async () => {
    if (!id) return;
    try {
      const taskData = await api.get<Task>(`/tasks/${id}`);
      setTask(taskData);
    } catch (err) {
      console.error('Failed to refresh task after planning:', err);
    }
  }, [id]);

  // Start task handler
  const handleStartTask = async () => {
    if (!id) return;

    setIsStarting(true);
    setError(null);

    try {
      await api.post(`/tasks/${id}/start`, {});
      // Refetch task to get updated status
      const taskData = await api.get<Task>(`/tasks/${id}`);
      setTask(taskData);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start task';
      setError(message);
    } finally {
      setIsStarting(false);
    }
  };

  // Pause task handler
  const handlePauseTask = async () => {
    if (!id) return;

    setIsPausing(true);
    setError(null);

    try {
      await api.post(`/tasks/${id}/pause`, {});
      // Refetch task to get updated status
      const taskData = await api.get<Task>(`/tasks/${id}`);
      setTask(taskData);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to pause task';
      setError(message);
    } finally {
      setIsPausing(false);
    }
  };

  // Resume task handler
  const handleResumeTask = async () => {
    if (!id) return;

    setIsResuming(true);
    setError(null);

    try {
      await api.post(`/tasks/${id}/resume`, {});
      // Refetch task to get updated status
      const taskData = await api.get<Task>(`/tasks/${id}`);
      setTask(taskData);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to resume task';
      setError(message);
    } finally {
      setIsResuming(false);
    }
  };

  // Cancel task handler
  const handleCancelTask = async () => {
    if (!id) return;

    // Confirm cancellation
    if (!window.confirm('Are you sure you want to cancel this task? This cannot be undone.')) {
      return;
    }

    setIsCancelling(true);
    setError(null);

    try {
      await api.post(`/tasks/${id}/cancel`, {});
      // Refetch task to get updated status
      const taskData = await api.get<Task>(`/tasks/${id}`);
      setTask(taskData);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to cancel task';
      setError(message);
    } finally {
      setIsCancelling(false);
    }
  };

  // Format timestamp for display
  const formatDate = (dateString: string | null) => {
    if (!dateString) return '-';
    return new Date(dateString).toLocaleString();
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading objective...</p>
        </div>
      </div>
    );
  }

  if (error && !task) {
    return (
      <div className="min-h-screen bg-gray-900 text-white p-4">
        <div className="max-w-2xl mx-auto">
          <Link to="/" className="text-blue-400 hover:text-blue-300 text-sm mb-4 inline-block">
            &larr; Back to Dashboard
          </Link>
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-4">
            <p className="text-red-400">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (!task) {
    return (
      <div className="min-h-screen bg-gray-900 text-white p-4">
        <div className="max-w-2xl mx-auto">
          <Link to="/" className="text-blue-400 hover:text-blue-300 text-sm mb-4 inline-block">
            &larr; Back to Dashboard
          </Link>
          <p className="text-gray-400">Objective not found</p>
        </div>
      </div>
    );
  }

  const isPlanning = task.Status === 'planning';
  const canStart = task.Status === 'pending' || task.Status === 'ready';
  const isRunning = task.Status === 'running';
  const isPaused = task.Status === 'paused';
  const isComplete = task.Status === 'completed' || task.Status === 'cancelled';

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between max-w-2xl mx-auto">
          <Link to="/" className="text-blue-400 hover:text-blue-300 text-sm flex items-center gap-1">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Dashboard
          </Link>
          <div className="flex items-center gap-2 text-sm">
            <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-gray-400">{connected ? 'Live' : 'Offline'}</span>
          </div>
        </div>
      </header>

      <main className="p-4 max-w-2xl mx-auto">
        {error && (
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {/* Task Header */}
        <div className="bg-gray-800 rounded-lg p-4 mb-4">
          <div className="flex items-start justify-between gap-4 mb-4">
            <div className="flex-1">
              <div className="flex items-center gap-3 mb-2">
                <PriorityDot priority={task.Priority} />
                <h1 className="text-xl font-bold">{task.Title}</h1>
              </div>
              <StatusBadge status={task.Status} />
            </div>
          </div>

          {task.Description && (
            <p className="text-gray-300 mb-4">{task.Description}</p>
          )}

          {/* Task Metadata */}
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 text-sm">
            <div>
              <span className="text-gray-500">Type</span>
              <p className="text-gray-300 uppercase">{task.Type}</p>
            </div>
            <div>
              <span className="text-gray-500">Priority</span>
              <p className="text-gray-300">{task.Priority}</p>
            </div>
            <div>
              <span className="text-gray-500">Autonomy</span>
              <p className="text-gray-300">{task.AutonomyLevel}</p>
            </div>
            {task.Hat && (
              <div>
                <span className="text-gray-500">Hat</span>
                <p className="text-gray-300">{task.Hat}</p>
              </div>
            )}
            <div>
              <span className="text-gray-500">Created</span>
              <p className="text-gray-300 text-xs">{formatDate(task.CreatedAt)}</p>
            </div>
            {task.StartedAt && (
              <div>
                <span className="text-gray-500">Started</span>
                <p className="text-gray-300 text-xs">{formatDate(task.StartedAt)}</p>
              </div>
            )}
          </div>
        </div>

        {/* Planning Panel (always shown if session exists, read-only when not in planning) */}
        <div className="mb-4">
          <PlanningPanel
            taskId={task.ID}
            readOnly={!isPlanning}
            onPlanAccepted={handlePlanningComplete}
            onPlanSkipped={handlePlanningComplete}
          />
        </div>

        {/* Action Buttons */}
        <div className="bg-gray-800 rounded-lg p-4 mb-4">
          <h2 className="text-sm font-semibold text-gray-400 mb-3">Actions</h2>
          <div className="flex gap-3">
            {canStart && (
              <button
                onClick={handleStartTask}
                disabled={isActioning}
                className="flex-1 bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
              >
                {isStarting ? 'Starting...' : 'Start Objective'}
              </button>
            )}
            {isRunning && (
              <>
                <button
                  onClick={handlePauseTask}
                  disabled={isActioning}
                  className="flex-1 bg-orange-600 hover:bg-orange-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                >
                  {isPausing ? 'Pausing...' : 'Pause'}
                </button>
                <button
                  onClick={handleCancelTask}
                  disabled={isActioning}
                  className="flex-1 bg-red-600 hover:bg-red-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                >
                  {isCancelling ? 'Cancelling...' : 'Cancel'}
                </button>
              </>
            )}
            {isPaused && (
              <>
                <button
                  onClick={handleResumeTask}
                  disabled={isActioning}
                  className="flex-1 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                >
                  {isResuming ? 'Resuming...' : 'Resume'}
                </button>
                <button
                  onClick={handleCancelTask}
                  disabled={isActioning}
                  className="flex-1 bg-red-600 hover:bg-red-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                >
                  {isCancelling ? 'Cancelling...' : 'Cancel'}
                </button>
              </>
            )}
            {isComplete && (
              <p className="text-gray-500 text-sm">Task {task.Status}</p>
            )}
          </div>
        </div>

        {/* Session Info (when running) */}
        {(isRunning || sessionInfo.sessionId) && (
          <div className="bg-gray-800 rounded-lg p-4 mb-4">
            <h2 className="text-sm font-semibold text-gray-400 mb-3">Session</h2>
            <div className="space-y-2">
              {isRunning && (
                <div className="flex items-center gap-2">
                  <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
                  <span className="text-green-400">Session running</span>
                </div>
              )}
              {sessionInfo.sessionId && (
                <div className="text-sm">
                  <span className="text-gray-500">Session ID: </span>
                  <span className="text-gray-300 font-mono text-xs">{sessionInfo.sessionId}</span>
                </div>
              )}
              {task.Hat && (
                <div className="text-sm">
                  <span className="text-gray-500">Current Hat: </span>
                  <span className="text-gray-300">{task.Hat}</span>
                </div>
              )}
              {sessionInfo.iterationCount > 0 && (
                <div className="text-sm">
                  <span className="text-gray-500">Iterations: </span>
                  <span className="text-gray-300">{sessionInfo.iterationCount}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Activity Feed */}
        <div className="bg-gray-800 rounded-lg p-4 mb-4">
          <h2 className="text-sm font-semibold text-gray-400 mb-3">Activity</h2>
          <ActivityFeed taskId={task.ID} isRunning={isRunning} />
        </div>

        {/* Worktree Info */}
        {task.WorktreePath && (
          <div className="bg-gray-800 rounded-lg p-4 mb-4">
            <h2 className="text-sm font-semibold text-gray-400 mb-3">Worktree</h2>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-gray-500">Path: </span>
                <span className="text-gray-300 font-mono text-xs">{task.WorktreePath}</span>
              </div>
              {task.BranchName && (
                <div>
                  <span className="text-gray-500">Branch: </span>
                  <span className="text-gray-300 font-mono">{task.BranchName}</span>
                </div>
              )}
              {task.PRNumber && (
                <div>
                  <span className="text-gray-500">PR: </span>
                  <span className="text-blue-400">#{task.PRNumber}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Budget Info */}
        {(task.TokenBudget || task.DollarBudget) && (
          <div className="bg-gray-800 rounded-lg p-4">
            <h2 className="text-sm font-semibold text-gray-400 mb-3">Budget</h2>
            <div className="grid grid-cols-2 gap-4 text-sm">
              {task.TokenBudget && (
                <div>
                  <span className="text-gray-500">Tokens</span>
                  <p className="text-gray-300">
                    {task.TokenUsed.toLocaleString()} / {task.TokenBudget.toLocaleString()}
                  </p>
                </div>
              )}
              {task.DollarBudget && (
                <div>
                  <span className="text-gray-500">Cost</span>
                  <p className="text-gray-300">
                    ${task.DollarUsed.toFixed(2)} / ${task.DollarBudget.toFixed(2)}
                  </p>
                </div>
              )}
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

// Quest status badge component
function QuestStatusBadge({ status }: { status: 'active' | 'completed' }) {
  const colors = {
    active: 'bg-green-600',
    completed: 'bg-gray-600',
  };

  return (
    <span className={`px-2 py-1 rounded text-xs font-medium ${colors[status]}`}>
      {status}
    </span>
  );
}

// Progress bar component
function ProgressBar({ completed, total }: { completed: number; total: number }) {
  const percentage = total > 0 ? (completed / total) * 100 : 0;

  return (
    <div className="w-full bg-gray-700 rounded-full h-2">
      <div
        className="bg-green-500 h-2 rounded-full transition-all duration-300"
        style={{ width: `${percentage}%` }}
      />
    </div>
  );
}

// Default project ID (will be replaced with actual project selection later)
const DEFAULT_PROJECT_ID = 'proj_default';

function QuestsPage() {
  const [quests, setQuests] = useState<Quest[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCreating, setIsCreating] = useState(false);

  const { connected, subscribe } = useWebSocket();
  const navigate = useNavigate();

  // Fetch quests on mount
  const loadQuests = useCallback(async () => {
    try {
      const data = await fetchQuests(DEFAULT_PROJECT_ID);
      setQuests(data);
      setError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch quests';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadQuests();
  }, [loadQuests]);

  // Subscribe to WebSocket for quest events
  const handleWebSocketEvent = useCallback((event: WebSocketEvent) => {
    if (event.type.startsWith('quest.')) {
      loadQuests();
    }
  }, [loadQuests]);

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Create new quest
  const handleCreateQuest = async () => {
    setIsCreating(true);
    setError(null);

    try {
      const quest = await createQuest(DEFAULT_PROJECT_ID);
      navigate(`/quests/${quest.id}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create quest';
      setError(message);
    } finally {
      setIsCreating(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading quests...</p>
        </div>
      </div>
    );
  }

  const activeQuests = quests.filter((q) => q.status === 'active');
  const completedQuests = quests.filter((q) => q.status === 'completed');

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between max-w-4xl mx-auto">
          <div className="flex items-center gap-4">
            <Link to="/" className="text-blue-400 hover:text-blue-300 text-sm flex items-center gap-1">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
              </svg>
              Dashboard
            </Link>
            <h1 className="text-lg font-semibold">Quests</h1>
          </div>
          <div className="flex items-center gap-4">
            <button
              onClick={handleCreateQuest}
              disabled={isCreating}
              className="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 text-white text-sm font-medium px-3 py-1.5 rounded-lg transition-colors"
            >
              {isCreating ? 'Creating...' : '+ New Quest'}
            </button>
            <div className="flex items-center gap-2 text-sm">
              <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
              <span className="text-gray-400">{connected ? 'Live' : 'Offline'}</span>
            </div>
          </div>
        </div>
      </header>

      <main className="p-4 max-w-4xl mx-auto">
        {error && (
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {/* Active Quests */}
        {activeQuests.length > 0 && (
          <div className="mb-6">
            <h2 className="text-sm font-semibold text-gray-400 mb-3">Active Quests</h2>
            <div className="space-y-3">
              {activeQuests.map((quest) => (
                <Link
                  key={quest.id}
                  to={`/quests/${quest.id}`}
                  className="block bg-gray-800 hover:bg-gray-700 rounded-lg p-4 transition-colors"
                >
                  <div className="flex items-start justify-between gap-3 mb-2">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-lg">⚔️</span>
                        <span className="font-medium truncate">
                          {quest.title || 'Untitled Quest'}
                        </span>
                      </div>
                      {quest.summary && (
                        <p className="text-sm text-gray-400">
                          {quest.summary.total_tasks} objective{quest.summary.total_tasks !== 1 ? 's' : ''} &middot;{' '}
                          {quest.summary.running_tasks > 0 && `${quest.summary.running_tasks} running, `}
                          {quest.summary.completed_tasks} completed
                          {quest.summary.total_dollars_used > 0 && (
                            <span className="text-yellow-500"> &middot; ${quest.summary.total_dollars_used.toFixed(2)}</span>
                          )}
                        </p>
                      )}
                    </div>
                    <QuestStatusBadge status={quest.status} />
                  </div>
                  {quest.summary && quest.summary.total_tasks > 0 && (
                    <ProgressBar
                      completed={quest.summary.completed_tasks}
                      total={quest.summary.total_tasks}
                    />
                  )}
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Completed Quests */}
        {completedQuests.length > 0 && (
          <div className="mb-6">
            <h2 className="text-sm font-semibold text-gray-400 mb-3">Completed Quests</h2>
            <div className="space-y-3">
              {completedQuests.map((quest) => (
                <Link
                  key={quest.id}
                  to={`/quests/${quest.id}`}
                  className="block bg-gray-800 hover:bg-gray-700 rounded-lg p-4 transition-colors opacity-75"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-lg">✓</span>
                        <span className="font-medium truncate">
                          {quest.title || 'Untitled Quest'}
                        </span>
                      </div>
                      {quest.summary && (
                        <p className="text-sm text-gray-400">
                          {quest.summary.total_tasks} objective{quest.summary.total_tasks !== 1 ? 's' : ''} completed
                          {quest.summary.total_dollars_used > 0 && (
                            <span className="text-yellow-500"> &middot; ${quest.summary.total_dollars_used.toFixed(2)}</span>
                          )}
                        </p>
                      )}
                    </div>
                    <QuestStatusBadge status={quest.status} />
                  </div>
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Empty State */}
        {quests.length === 0 && (
          <div className="bg-gray-800 rounded-lg p-8 text-center">
            <svg
              className="w-12 h-12 mx-auto text-gray-600 mb-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"
              />
            </svg>
            <p className="text-gray-400 mb-2">No quests yet</p>
            <p className="text-sm text-gray-500 mb-4">
              Start a quest to chat with Dex and plan your objectives
            </p>
            <button
              onClick={handleCreateQuest}
              disabled={isCreating}
              className="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 text-white font-medium px-4 py-2 rounded-lg transition-colors"
            >
              {isCreating ? 'Creating...' : 'Start New Quest'}
            </button>
          </div>
        )}
      </main>
    </div>
  );
}

// Helper to parse objective drafts from message content
function parseObjectiveDrafts(content: string): ObjectiveDraft[] {
  const drafts: ObjectiveDraft[] = [];
  const regex = /OBJECTIVE_DRAFT:\s*(\{[\s\S]*?\})\s*(?=OBJECTIVE_DRAFT:|QUESTION:|QUEST_READY:|$)/g;
  let match;

  while ((match = regex.exec(content)) !== null) {
    try {
      const draft = JSON.parse(match[1]);
      if (draft.title && draft.draft_id) {
        // Default auto_start to true if not specified
        if (draft.auto_start === undefined) {
          draft.auto_start = true;
        }
        drafts.push(draft);
      }
    } catch {
      // Skip malformed JSON
    }
  }

  return drafts;
}

// Helper to format signals for display
// Questions are formatted inline for history, drafts are shown in sidebar
function formatMessageContent(content: string): string {
  // Remove OBJECTIVE_DRAFT signals (shown in sidebar)
  let formatted = content.replace(/OBJECTIVE_DRAFT:\s*\{[\s\S]*?\}\s*(?=OBJECTIVE_DRAFT:|QUESTION:|QUEST_READY:|$)/g, '');

  // Format QUESTION signals as readable text for message history
  formatted = formatted.replace(/QUESTION:\s*(\{[^}]*\})/g, (_match, jsonStr) => {
    try {
      const q = JSON.parse(jsonStr);
      let questionText = `\n**${q.question}**`;
      if (q.options && q.options.length > 0) {
        questionText += '\n' + q.options.map((opt: string) => `• ${opt}`).join('\n');
      }
      return questionText;
    } catch {
      return '';
    }
  });

  // Remove QUEST_READY signals
  formatted = formatted.replace(/QUEST_READY:\s*\{[^}]*\}/g, '');
  // Clean up extra whitespace
  return formatted.trim();
}

// Question type for quest conversations
interface QuestQuestion {
  question: string;
  options?: string[];
}

// Helper to parse questions from message content
function parseQuestions(content: string): QuestQuestion[] {
  const questions: QuestQuestion[] = [];
  const regex = /QUESTION:\s*(\{[^}]*\})/g;
  let match;

  while ((match = regex.exec(content)) !== null) {
    try {
      const q = JSON.parse(match[1]);
      if (q.question) {
        questions.push(q);
      }
    } catch {
      // Skip malformed JSON
    }
  }

  return questions;
}

function QuestDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [quest, setQuest] = useState<Quest | null>(null);
  const [messages, setMessages] = useState<QuestMessage[]>([]);
  const [drafts, setDrafts] = useState<ObjectiveDraft[]>([]);
  const [questions, setQuestions] = useState<QuestQuestion[]>([]);
  const [acceptingDraft, setAcceptingDraft] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [messageInput, setMessageInput] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [isCompleting, setIsCompleting] = useState(false);
  const [isReopening, setIsReopening] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isUpdatingModel, setIsUpdatingModel] = useState(false);
  const [preflight, setPreflight] = useState<PreflightCheck | null>(null);

  // Track accepted/rejected drafts to avoid re-showing them after loadQuest
  const handledDraftIds = useRef<Set<string>>(new Set());
  // Ref for textarea to reset height after sending
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const { connected, subscribe } = useWebSocket();
  const navigate = useNavigate();

  // Fetch quest on mount
  const loadQuest = useCallback(async () => {
    if (!id) return;

    try {
      const data: QuestResponse = await fetchQuest(id);
      setQuest(data.quest);
      setMessages(data.messages);

      // Fetch quest tasks to filter out already-accepted drafts
      let existingTaskTitles: Set<string> = new Set();
      try {
        const tasks = await fetchQuestTasks(id);
        existingTaskTitles = new Set(tasks.map((t) => t.Title));
      } catch {
        // Continue without task filtering if fetch fails
      }

      // Parse drafts and questions from existing messages
      const allDrafts: ObjectiveDraft[] = [];
      let latestQuestions: QuestQuestion[] = [];
      let lastAssistantHadDraft = false;

      for (const msg of data.messages) {
        if (msg.role === 'assistant') {
          const msgDrafts = parseObjectiveDrafts(msg.content);
          // Filter out drafts that have been:
          // 1. Accepted/rejected in this session (handledDraftIds)
          // 2. Already converted to tasks (matching title)
          const unhandledDrafts = msgDrafts.filter(
            (d) => !handledDraftIds.current.has(d.draft_id) && !existingTaskTitles.has(d.title)
          );
          allDrafts.push(...unhandledDrafts);

          // Track if this message has drafts or questions
          const msgQuestions = parseQuestions(msg.content);
          if (msgDrafts.length > 0) {
            // This message has drafts - clear any previous questions
            latestQuestions = [];
            lastAssistantHadDraft = true;
          } else if (msgQuestions.length > 0) {
            // This message has questions (no drafts) - these are the latest
            latestQuestions = msgQuestions;
            lastAssistantHadDraft = false;
          }
        }
      }
      setDrafts(allDrafts);
      // Only show questions if:
      // 1. The last assistant message had questions (not drafts)
      // 2. There are still pending drafts OR no drafts have been proposed yet
      setQuestions(lastAssistantHadDraft ? [] : latestQuestions);

      // Fetch preflight checks if quest is active
      if (data.quest.status === 'active') {
        try {
          const preflightData = await fetchPreflightCheck(id);
          setPreflight(preflightData);
        } catch {
          // Preflight check is optional, don't block on failure
          setPreflight(null);
        }
      }

      setError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch quest';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    loadQuest();
  }, [loadQuest]);

  // Subscribe to WebSocket for quest events
  const handleWebSocketEvent = useCallback((event: WebSocketEvent) => {
    if (!id) return;

    const payload = event.payload as { quest_id?: string; message?: QuestMessage; draft?: ObjectiveDraft };
    if (payload.quest_id !== id) return;

    if (event.type === 'quest.message') {
      const msg = payload.message;
      if (msg) {
        setMessages((prev) => [...prev, msg]);
        // Parse drafts and questions from new assistant messages
        if (msg.role === 'assistant') {
          const newDrafts = parseObjectiveDrafts(msg.content);
          if (newDrafts.length > 0) {
            // Message has drafts - add them and clear questions
            setDrafts((prev) => [...prev, ...newDrafts]);
            setQuestions([]);
          } else {
            // Message has no drafts - check for questions
            const newQuestions = parseQuestions(msg.content);
            setQuestions(newQuestions);
          }
        } else {
          // User message clears pending questions
          setQuestions([]);
        }
      }
    } else if (event.type === 'quest.question') {
      const q = payload as { question?: QuestQuestion };
      if (q.question) {
        setQuestions([q.question]);
      }
    } else if (event.type === 'quest.objective_draft') {
      if (payload.draft) {
        setDrafts((prev) => {
          // Replace if exists, otherwise add
          const exists = prev.find((d) => d.draft_id === payload.draft!.draft_id);
          if (exists) {
            return prev.map((d) => (d.draft_id === payload.draft!.draft_id ? payload.draft! : d));
          }
          return [...prev, payload.draft!];
        });
      }
    } else if (event.type === 'task.created') {
      // Reload quest to get updated summary
      loadQuest();
    } else if (event.type.startsWith('quest.')) {
      loadQuest();
    }
  }, [id, loadQuest]);

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Send message
  const handleSendMessage = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedMessage = messageInput.trim();
    if (!id || !trimmedMessage || isSending) return;

    // Clear input immediately for better UX
    setMessageInput('');
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
    }

    setIsSending(true);
    setError(null);
    // Clear questions immediately when user sends a message
    setQuestions([]);

    try {
      await sendQuestMessage(id, trimmedMessage);
      // Message will be added via WebSocket event
    } catch (err) {
      // err could be ApiError (with message property) or Error
      const apiErr = err as { message?: string };
      const message = apiErr?.message || (err instanceof Error ? err.message : 'Failed to send message');
      setError(message);
      // Restore the message on error so user doesn't lose it
      setMessageInput(trimmedMessage);
    } finally {
      setIsSending(false);
    }
  };

  // Complete quest
  const handleCompleteQuest = async () => {
    if (!id) return;

    setIsCompleting(true);
    setError(null);

    try {
      const updatedQuest = await completeQuest(id);
      setQuest(updatedQuest);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to complete quest';
      setError(message);
    } finally {
      setIsCompleting(false);
    }
  };

  // Reopen quest
  const handleReopenQuest = async () => {
    if (!id) return;

    setIsReopening(true);
    setError(null);

    try {
      const updatedQuest = await reopenQuest(id);
      setQuest(updatedQuest);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to reopen quest';
      setError(message);
    } finally {
      setIsReopening(false);
    }
  };

  // Update quest model
  const handleUpdateModel = async (model: QuestModel) => {
    if (!id || !quest || quest.model === model) return;

    setIsUpdatingModel(true);
    setError(null);

    try {
      const updatedQuest = await updateQuestModel(id, model);
      setQuest(updatedQuest);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update model';
      setError(message);
    } finally {
      setIsUpdatingModel(false);
    }
  };

  // Delete quest
  const handleDeleteQuest = async () => {
    if (!id) return;

    if (!window.confirm('Are you sure you want to delete this quest? This cannot be undone.')) {
      return;
    }

    setIsDeleting(true);
    setError(null);

    try {
      await deleteQuest(id);
      navigate('/quests');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete quest';
      setError(message);
    } finally {
      setIsDeleting(false);
    }
  };

  // Accept draft and create objective
  const handleAcceptDraft = async (draft: ObjectiveDraft, selectedOptional: number[]) => {
    if (!id) return;

    setAcceptingDraft(draft.draft_id);
    setError(null);

    try {
      console.log('Creating objective with auto_start:', draft.auto_start);
      const result = await createObjective(id, draft, selectedOptional);
      console.log('Create objective result:', result);

      // Check for auto-start error
      if ('auto_start_error' in result) {
        console.error('Auto-start failed:', result.auto_start_error);
        setError(`Objective created but auto-start failed: ${result.auto_start_error}`);
      }

      // Mark draft as handled so it won't reappear after loadQuest
      handledDraftIds.current.add(draft.draft_id);
      // Remove the accepted draft from the list
      setDrafts((prev) => prev.filter((d) => d.draft_id !== draft.draft_id));
      // Clear questions since we've moved past the planning stage
      setQuestions([]);
      // Reload quest to get updated summary
      loadQuest();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create objective';
      setError(message);
    } finally {
      setAcceptingDraft(null);
    }
  };

  // Reject draft (just remove from UI)
  const handleRejectDraft = (draftId: string) => {
    // Mark draft as handled so it won't reappear after loadQuest
    handledDraftIds.current.add(draftId);
    setDrafts((prev) => prev.filter((d) => d.draft_id !== draftId));
  };

  // Format timestamp
  const formatTime = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading quest...</p>
        </div>
      </div>
    );
  }

  if (!quest) {
    return (
      <div className="min-h-screen bg-gray-900 text-white p-4">
        <div className="max-w-4xl mx-auto">
          <Link to="/quests" className="text-blue-400 hover:text-blue-300 text-sm mb-4 inline-block">
            &larr; Back to Quests
          </Link>
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-4">
            <p className="text-red-400">{error || 'Quest not found'}</p>
          </div>
        </div>
      </div>
    );
  }

  const isActive = quest.status === 'active';

  return (
    <div className="min-h-screen bg-gray-900 text-white flex flex-col">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between max-w-4xl mx-auto">
          <div className="flex items-center gap-4">
            <Link to="/quests" className="text-blue-400 hover:text-blue-300 text-sm flex items-center gap-1">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
              </svg>
              Quests
            </Link>
            <h1 className="text-lg font-semibold truncate max-w-md">
              {quest.title || 'New Quest'}
            </h1>
            <QuestStatusBadge status={quest.status} />
          </div>
          <div className="flex items-center gap-3">
            {/* Model selector */}
            {isActive && (
              <select
                value={quest.model}
                onChange={(e) => handleUpdateModel(e.target.value as QuestModel)}
                disabled={isUpdatingModel}
                className="bg-gray-700 border border-gray-600 text-white text-sm rounded-lg px-2 py-1.5 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
                title="Select AI model for this quest"
              >
                <option value="sonnet">Sonnet</option>
                <option value="opus">Opus</option>
              </select>
            )}
            {isActive ? (
              <button
                onClick={handleCompleteQuest}
                disabled={isCompleting}
                className="bg-green-600 hover:bg-green-700 disabled:bg-gray-700 text-white text-sm font-medium px-3 py-1.5 rounded-lg transition-colors"
              >
                {isCompleting ? 'Completing...' : 'Complete Quest'}
              </button>
            ) : (
              <button
                onClick={handleReopenQuest}
                disabled={isReopening}
                className="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 text-white text-sm font-medium px-3 py-1.5 rounded-lg transition-colors"
              >
                {isReopening ? 'Reopening...' : 'Reopen Quest'}
              </button>
            )}
            <button
              onClick={handleDeleteQuest}
              disabled={isDeleting || (quest.summary?.total_tasks ?? 0) > 0}
              title={(quest.summary?.total_tasks ?? 0) > 0 ? 'Cannot delete quest with objectives' : 'Delete quest'}
              className="text-red-400 hover:text-red-300 disabled:text-gray-600 disabled:cursor-not-allowed text-sm transition-colors"
            >
              {isDeleting ? 'Deleting...' : 'Delete'}
            </button>
            <div className="flex items-center gap-2 text-sm">
              <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
              <span className="text-gray-400">{connected ? 'Live' : 'Offline'}</span>
            </div>
          </div>
        </div>
      </header>

      {/* Preflight Warnings */}
      {preflight && !preflight.ok && preflight.warnings && preflight.warnings.length > 0 && (
        <div className="bg-yellow-900/50 border-b border-yellow-500 px-4 py-2">
          <div className="max-w-4xl mx-auto flex items-center gap-2">
            <svg className="w-5 h-5 text-yellow-500 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
            <span className="text-yellow-400 text-sm">
              {preflight.warnings.join(' · ')}
            </span>
          </div>
        </div>
      )}

      {/* Main Content */}
      <div className="flex-1 flex max-w-4xl mx-auto w-full">
        {/* Chat Area */}
        <div className="flex-1 flex flex-col">
          {error && (
            <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 m-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          {/* Messages - aligned to bottom */}
          <div className="flex-1 overflow-y-auto p-4 flex flex-col justify-end">
            {messages.length === 0 ? (
              <div className="text-center py-8">
                <p className="text-gray-400 mb-2">Start your quest</p>
                <p className="text-sm text-gray-500">
                  Describe what you want to accomplish and Dex will help plan the objectives
                </p>
              </div>
            ) : (
              <div className="space-y-4">
              {messages.map((msg) => {
                // Format assistant messages and skip if empty (e.g., only contained objective drafts)
                const displayContent = msg.role === 'assistant' ? formatMessageContent(msg.content) : msg.content;
                if (!displayContent) return null;

                return (
                  <div
                    key={msg.id}
                    className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
                  >
                    <div
                      className={`max-w-[80%] rounded-lg p-3 ${
                        msg.role === 'user'
                          ? 'bg-blue-600 text-white'
                          : 'bg-gray-800 text-gray-200'
                      }`}
                    >
                      {/* Tool calls (assistant only) */}
                      {msg.role === 'assistant' && msg.tool_calls && msg.tool_calls.length > 0 && (
                        <div className="mb-3">
                          <ToolCallList toolCalls={msg.tool_calls} />
                        </div>
                      )}
                      <p className="whitespace-pre-wrap">{displayContent}</p>
                      <p className={`text-xs mt-1 ${msg.role === 'user' ? 'text-blue-200' : 'text-gray-500'}`}>
                        {formatTime(msg.created_at)}
                      </p>
                    </div>
                  </div>
                );
              })}
              </div>
            )}
          </div>

          {/* Questions from Dex */}
          {questions.length > 0 && isActive && (
            <div className="p-4 border-t border-gray-700 bg-gray-800/50">
              {questions.map((q, idx) => (
                <div key={idx} className="mb-3 last:mb-0">
                  <p className="text-gray-300 mb-2 font-medium">{q.question}</p>
                  {q.options && q.options.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {q.options.map((opt, optIdx) => (
                        <button
                          key={optIdx}
                          type="button"
                          onClick={() => setMessageInput(opt)}
                          className="bg-gray-700 hover:bg-gray-600 text-gray-200 px-3 py-1.5 rounded-lg text-sm transition-colors"
                        >
                          {opt}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Input Area */}
          {isActive && (
            <form onSubmit={handleSendMessage} className="p-4 border-t border-gray-700">
              <div className="flex gap-3 items-end">
                <textarea
                  ref={textareaRef}
                  value={messageInput}
                  onChange={(e) => {
                    setMessageInput(e.target.value);
                    // Auto-resize: reset height then set to scrollHeight
                    e.target.style.height = 'auto';
                    e.target.style.height = Math.min(e.target.scrollHeight, window.innerWidth < 768 ? 120 : 200) + 'px';
                  }}
                  onKeyDown={(e) => {
                    // Check if mobile (touch device)
                    const isMobile = 'ontouchstart' in window || navigator.maxTouchPoints > 0;

                    if (e.key === 'Enter') {
                      if (isMobile) {
                        // Mobile: Enter = new line (don't prevent default)
                        return;
                      }
                      // Desktop: Enter = send, Shift+Enter = new line
                      if (!e.shiftKey) {
                        e.preventDefault();
                        if (messageInput.trim() && !isSending) {
                          handleSendMessage(e);
                        }
                      }
                    }
                  }}
                  placeholder="Describe what you want to accomplish..."
                  className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent resize-none min-h-[42px] max-h-[120px] md:max-h-[200px] overflow-y-auto"
                  disabled={isSending}
                  rows={1}
                />
                <button
                  type="submit"
                  disabled={isSending || !messageInput.trim()}
                  className="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium p-2 rounded-lg transition-colors flex-shrink-0"
                  aria-label="Send message"
                >
                  {isSending ? (
                    <svg className="w-6 h-6 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                    </svg>
                  ) : (
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
                    </svg>
                  )}
                </button>
              </div>
            </form>
          )}
        </div>

        {/* Sidebar - Drafts and Objectives */}
        {(drafts.length > 0 || (quest.summary && quest.summary.total_tasks > 0)) && (
          <div className="w-80 border-l border-gray-700 p-4 overflow-y-auto">
            {/* Draft Objectives */}
            {drafts.length > 0 && (
              <div className="mb-6">
                <h2 className="text-sm font-semibold text-gray-400 mb-3">
                  Proposed Objectives ({drafts.length})
                </h2>
                <div className="space-y-3">
                  {drafts.map((draft) => (
                    <ObjectiveDraftCard
                      key={draft.draft_id}
                      draft={draft}
                      onAccept={handleAcceptDraft}
                      onReject={handleRejectDraft}
                      isAccepting={acceptingDraft === draft.draft_id}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* Existing Objectives Summary */}
            {quest.summary && quest.summary.total_tasks > 0 && (
              <div>
                <h2 className="text-sm font-semibold text-gray-400 mb-3">
                  Objectives ({quest.summary.total_tasks})
                </h2>
                <div className="space-y-2">
                  <div className="text-sm">
                    <div className="flex justify-between text-gray-400">
                      <span>Completed</span>
                      <span>{quest.summary.completed_tasks}</span>
                    </div>
                    <div className="flex justify-between text-gray-400">
                      <span>Running</span>
                      <span>{quest.summary.running_tasks}</span>
                    </div>
                    <div className="flex justify-between text-gray-400">
                      <span>Pending</span>
                      <span>{quest.summary.pending_tasks}</span>
                    </div>
                    {quest.summary.blocked_tasks > 0 && (
                      <div className="flex justify-between text-yellow-400">
                        <span>Blocked</span>
                        <span>{quest.summary.blocked_tasks}</span>
                      </div>
                    )}
                    {quest.summary.failed_tasks > 0 && (
                      <div className="flex justify-between text-red-400">
                        <span>Failed</span>
                        <span>{quest.summary.failed_tasks}</span>
                      </div>
                    )}
                    {quest.summary.total_dollars_used > 0 && (
                      <div className="flex justify-between text-yellow-500 pt-2 border-t border-gray-700 mt-2">
                        <span>Total Cost</span>
                        <span>${quest.summary.total_dollars_used.toFixed(2)}</span>
                      </div>
                    )}
                  </div>
                  <div className="pt-2">
                    <ProgressBar
                      completed={quest.summary.completed_tasks}
                      total={quest.summary.total_tasks}
                    />
                  </div>
                  <Link
                    to={`/tasks?quest=${quest.id}`}
                    className="block text-sm text-blue-400 hover:text-blue-300 pt-2"
                  >
                    View all objectives →
                  </Link>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// Approval type badge component
function ApprovalTypeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    commit: 'bg-purple-600',
    hat_transition: 'bg-blue-600',
    pr: 'bg-green-600',
    merge: 'bg-emerald-600',
    conflict_resolution: 'bg-orange-600',
  };

  return (
    <span className={`px-2 py-1 rounded text-xs font-medium ${colors[type] || 'bg-gray-600'}`}>
      {type.replace('_', ' ')}
    </span>
  );
}

function ApprovalsPage() {
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actioning, setActioning] = useState<{ [id: string]: 'approving' | 'rejecting' }>({});

  const { connected, subscribe } = useWebSocket();

  // Fetch approvals on mount
  const loadApprovals = useCallback(async () => {
    try {
      const data = await fetchApprovals();
      // Filter to show only pending approvals
      setApprovals(data.approvals.filter((a) => a.status === 'pending'));
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch approvals';
      setError(message);
    }
  }, []);

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      setError(null);
      await loadApprovals();
      setLoading(false);
    };

    fetchData();
  }, [loadApprovals]);

  // Subscribe to WebSocket for approval events
  const handleWebSocketEvent = useCallback((event: WebSocketEvent) => {
    if (event.type.startsWith('approval.')) {
      // Refetch approvals on any approval event
      loadApprovals();
    }
  }, [loadApprovals]);

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Handle approve
  const handleApprove = async (id: string) => {
    setActioning((prev) => ({ ...prev, [id]: 'approving' }));
    setError(null);

    try {
      await approveApproval(id);
      await loadApprovals();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to approve';
      setError(message);
    } finally {
      setActioning((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
    }
  };

  // Handle reject
  const handleReject = async (id: string) => {
    setActioning((prev) => ({ ...prev, [id]: 'rejecting' }));
    setError(null);

    try {
      await rejectApproval(id);
      await loadApprovals();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to reject';
      setError(message);
    } finally {
      setActioning((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
    }
  };

  // Format timestamp for display
  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading approvals...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between max-w-2xl mx-auto">
          <Link to="/" className="text-blue-400 hover:text-blue-300 text-sm flex items-center gap-1">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Dashboard
          </Link>
          <div className="flex items-center gap-2 text-sm">
            <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-gray-400">{connected ? 'Live' : 'Offline'}</span>
          </div>
        </div>
      </header>

      <main className="p-4 max-w-2xl mx-auto">
        <h1 className="text-2xl font-bold mb-4">Approvals</h1>

        {error && (
          <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {approvals.length === 0 ? (
          <div className="bg-gray-800 rounded-lg p-8 text-center">
            <svg
              className="w-12 h-12 mx-auto text-gray-600 mb-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <p className="text-gray-400 mb-2">No pending approvals</p>
            <p className="text-sm text-gray-500">You're all caught up!</p>
          </div>
        ) : (
          <div className="space-y-4">
            {approvals.map((approval) => {
              const isActioning = !!actioning[approval.id];
              const actionType = actioning[approval.id];

              return (
                <div key={approval.id} className="bg-gray-800 rounded-lg p-4">
                  <div className="flex items-start justify-between gap-4 mb-3">
                    <div className="flex-1">
                      <div className="flex items-center gap-2 mb-2">
                        <ApprovalTypeBadge type={approval.type} />
                        <h3 className="font-medium">{approval.title}</h3>
                      </div>
                      {approval.description && (
                        <p className="text-gray-400 text-sm mb-2">{approval.description}</p>
                      )}
                      <p className="text-xs text-gray-500">
                        Created: {formatDate(approval.created_at)}
                      </p>
                    </div>
                  </div>

                  <div className="flex gap-3">
                    <button
                      onClick={() => handleApprove(approval.id)}
                      disabled={isActioning}
                      className="flex-1 bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                    >
                      {actionType === 'approving' ? 'Approving...' : 'Approve'}
                    </button>
                    <button
                      onClick={() => handleReject(approval.id)}
                      disabled={isActioning}
                      className="flex-1 bg-red-600 hover:bg-red-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                    >
                      {actionType === 'rejecting' ? 'Rejecting...' : 'Reject'}
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </main>
    </div>
  );
}

// Protected route wrapper that also handles onboarding
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const [_setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [showOnboarding, setShowOnboarding] = useState(false);

  useEffect(() => {
    if (isAuthenticated) {
      checkSetupStatus();
    }
  }, [isAuthenticated]);

  const checkSetupStatus = async () => {
    setIsLoading(true);
    try {
      const status = await api.get<SetupStatus>('/setup/status');
      setSetupStatus(status);

      // Show onboarding if setup is not complete
      // Check for either GitHub auth method (app or token)
      const hasGitHubAuth = status.github_token_set || status.github_app_set;
      if (!status.setup_complete && (!hasGitHubAuth || !status.anthropic_key_set)) {
        setShowOnboarding(true);
      }
    } catch (err) {
      console.error('Failed to check setup status:', err);
      // Don't block user if setup check fails
    } finally {
      setIsLoading(false);
    }
  };

  const handleOnboardingComplete = () => {
    setShowOnboarding(false);
    checkSetupStatus();
  };

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading...</p>
        </div>
      </div>
    );
  }

  if (showOnboarding) {
    return <Onboarding onComplete={handleOnboardingComplete} />;
  }

  return <>{children}</>;
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <DashboardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/tasks"
        element={
          <ProtectedRoute>
            <TaskListPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/tasks/:id"
        element={
          <ProtectedRoute>
            <TaskDetailPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/approvals"
        element={
          <ProtectedRoute>
            <ApprovalsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/quests"
        element={
          <ProtectedRoute>
            <QuestsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/quests/:id"
        element={
          <ProtectedRoute>
            <QuestDetailPage />
          </ProtectedRoute>
        }
      />
      {/* Catch-all redirect to dashboard */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
