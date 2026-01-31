import { useState, useEffect, useCallback } from 'react';
import { Routes, Route, Navigate, useNavigate, useParams, Link } from 'react-router-dom';
import { useAuthStore } from './stores/auth';
import { api, fetchApprovals, approveApproval, rejectApproval } from './lib/api';
import { useWebSocket } from './hooks/useWebSocket';
import { Onboarding } from './components/Onboarding';
import { ActivityFeed } from './components/ActivityFeed';
import type { Task, TasksResponse, SystemStatus, TaskStatus, WebSocketEvent, SessionEvent, Approval } from './lib/types';

// Setup status type
interface SetupStatus {
  passkey_registered: boolean;
  github_token_set: boolean;
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
    blocked: 'bg-yellow-600',
    ready: 'bg-blue-600',
    running: 'bg-green-600',
    paused: 'bg-orange-600',
    quarantined: 'bg-red-600',
    completed: 'bg-emerald-600',
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

// Task creation form state type
interface CreateTaskForm {
  title: string;
  description: string;
  priority: number;
  autonomy: string;
}

const initialFormState: CreateTaskForm = {
  title: '',
  description: '',
  priority: 3,
  autonomy: 'suggest',
};

function DashboardPage() {
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Task creation modal state
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [createForm, setCreateForm] = useState<CreateTaskForm>(initialFormState);
  const [createError, setCreateError] = useState<string | null>(null);
  const [isCreating, setIsCreating] = useState(false);

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

  // Task creation handlers
  const handleOpenCreateModal = () => {
    setCreateForm(initialFormState);
    setCreateError(null);
    setShowCreateModal(true);
  };

  const handleCloseCreateModal = () => {
    setShowCreateModal(false);
    setCreateForm(initialFormState);
    setCreateError(null);
  };

  const handleCreateFormChange = (
    field: keyof CreateTaskForm,
    value: string | number
  ) => {
    setCreateForm((prev) => ({ ...prev, [field]: value }));
  };

  const handleCreateTask = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError(null);

    // Validate
    if (!createForm.title.trim()) {
      setCreateError('Title is required');
      return;
    }

    if (createForm.priority < 1 || createForm.priority > 5) {
      setCreateError('Priority must be between 1 and 5');
      return;
    }

    setIsCreating(true);

    try {
      await api.post('/tasks', {
        title: createForm.title.trim(),
        description: createForm.description.trim() || null,
        priority: createForm.priority,
        autonomy: createForm.autonomy,
        project_id: 1, // Default project for now
      });

      // Refetch tasks
      const tasksData = await api.get<TasksResponse>('/tasks');
      setTasks(tasksData.tasks || []);

      // Close modal
      handleCloseCreateModal();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create task';
      setCreateError(message);
    } finally {
      setIsCreating(false);
    }
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

        {/* Tasks List */}
        <div className="bg-gray-800 rounded-lg p-4">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Tasks</h2>
            <div className="flex items-center gap-3">
              <button
                onClick={handleOpenCreateModal}
                className="bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-3 py-1.5 rounded-lg transition-colors"
              >
                + Create Task
              </button>
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
              <p className="text-gray-400 mb-2">No tasks yet</p>
              <p className="text-sm text-gray-500">Create a task to get started</p>
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
                  +{tasks.length - 10} more tasks
                </p>
              )}
            </div>
          )}
        </div>

        {/* Create Task Modal */}
        {showCreateModal && (
          <div className="fixed inset-0 bg-black/60 flex items-center justify-center p-4 z-50">
            <div className="bg-gray-800 rounded-lg w-full max-w-md">
              <div className="flex items-center justify-between p-4 border-b border-gray-700">
                <h3 className="text-lg font-semibold">Create Task</h3>
                <button
                  onClick={handleCloseCreateModal}
                  className="text-gray-400 hover:text-white transition-colors"
                >
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>

              <form onSubmit={handleCreateTask} className="p-4 space-y-4">
                {createError && (
                  <div className="bg-red-900/50 border border-red-500 rounded-lg p-3">
                    <p className="text-red-400 text-sm">{createError}</p>
                  </div>
                )}

                <div>
                  <label htmlFor="task-title" className="block text-sm font-medium text-gray-300 mb-1">
                    Title <span className="text-red-400">*</span>
                  </label>
                  <input
                    id="task-title"
                    type="text"
                    value={createForm.title}
                    onChange={(e) => handleCreateFormChange('title', e.target.value)}
                    placeholder="Enter task title"
                    className="w-full bg-gray-700 border border-gray-600 rounded-lg px-3 py-2 text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                    disabled={isCreating}
                    autoFocus
                  />
                </div>

                <div>
                  <label htmlFor="task-description" className="block text-sm font-medium text-gray-300 mb-1">
                    Description
                  </label>
                  <textarea
                    id="task-description"
                    value={createForm.description}
                    onChange={(e) => handleCreateFormChange('description', e.target.value)}
                    placeholder="Describe the task (optional)"
                    rows={3}
                    className="w-full bg-gray-700 border border-gray-600 rounded-lg px-3 py-2 text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent resize-none"
                    disabled={isCreating}
                  />
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label htmlFor="task-priority" className="block text-sm font-medium text-gray-300 mb-1">
                      Priority
                    </label>
                    <select
                      id="task-priority"
                      value={createForm.priority}
                      onChange={(e) => handleCreateFormChange('priority', parseInt(e.target.value, 10))}
                      className="w-full bg-gray-700 border border-gray-600 rounded-lg px-3 py-2 text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      disabled={isCreating}
                    >
                      <option value={1}>1 - Critical</option>
                      <option value={2}>2 - High</option>
                      <option value={3}>3 - Medium</option>
                      <option value={4}>4 - Low</option>
                      <option value={5}>5 - Lowest</option>
                    </select>
                  </div>

                  <div>
                    <label htmlFor="task-autonomy" className="block text-sm font-medium text-gray-300 mb-1">
                      Autonomy
                    </label>
                    <select
                      id="task-autonomy"
                      value={createForm.autonomy}
                      onChange={(e) => handleCreateFormChange('autonomy', e.target.value)}
                      className="w-full bg-gray-700 border border-gray-600 rounded-lg px-3 py-2 text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      disabled={isCreating}
                    >
                      <option value="full">Full</option>
                      <option value="suggest">Suggest</option>
                      <option value="supervised">Supervised</option>
                      <option value="manual">Manual</option>
                    </select>
                  </div>
                </div>

                <div className="flex gap-3 pt-2">
                  <button
                    type="button"
                    onClick={handleCloseCreateModal}
                    className="flex-1 bg-gray-600 hover:bg-gray-500 text-white font-medium py-2 px-4 rounded-lg transition-colors"
                    disabled={isCreating}
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={isCreating || !createForm.title.trim()}
                    className="flex-1 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
                  >
                    {isCreating ? 'Creating...' : 'Create Task'}
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
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
          <h1 className="text-lg font-semibold">Tasks</h1>
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
              {tasks.length} task{tasks.length !== 1 ? 's' : ''}
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
              <p className="text-gray-400 mb-2">No tasks found</p>
              <p className="text-sm text-gray-500">
                {statusFilter !== 'all'
                  ? `No ${statusFilter} tasks. Try a different filter.`
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
          <p className="text-gray-400">Loading task...</p>
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
          <p className="text-gray-400">Task not found</p>
        </div>
      </div>
    );
  }

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
                {isStarting ? 'Starting...' : 'Start Task'}
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
      if (!status.setup_complete && (!status.github_token_set || !status.anthropic_key_set)) {
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
      {/* Catch-all redirect to dashboard */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
