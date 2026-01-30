import { useState, useEffect, useCallback } from 'react';
import { Routes, Route, Navigate, useNavigate, useParams, Link } from 'react-router-dom';
import { useAuthStore } from './stores/auth';
import { api } from './lib/api';
import { useWebSocket } from './hooks/useWebSocket';
import type { Task, TasksResponse, SystemStatus, TaskStatus, WebSocketEvent, SessionEvent } from './lib/types';
import {
  validateMnemonic,
  generatePassphrase,
  deriveKeypair,
  sign,
  bytesToHex,
  hexToBytes,
} from './lib/crypto';

function LoginPage() {
  const [passphrase, setPassphrase] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [showGenerated, setShowGenerated] = useState(false);
  const [generatedPhrase, setGeneratedPhrase] = useState<string | null>(null);

  const setToken = useAuthStore((state) => state.setToken);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const navigate = useNavigate();

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  const handleGeneratePassphrase = () => {
    const newPhrase = generatePassphrase();
    setGeneratedPhrase(newPhrase);
    setShowGenerated(true);
    setPassphrase(newPhrase);
    setError(null);
  };

  const handleUseGenerated = () => {
    setShowGenerated(false);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      // 1. Validate mnemonic format
      const normalizedPhrase = passphrase.trim().toLowerCase().replace(/\s+/g, ' ');
      if (!validateMnemonic(normalizedPhrase)) {
        throw new Error('Invalid passphrase. Please enter a valid 24-word BIP39 mnemonic.');
      }

      // 2. Derive Ed25519 keypair from mnemonic
      const { publicKey, privateKey } = deriveKeypair(normalizedPhrase);

      // 3. Request challenge from server
      const challengeResponse = await api.post<{ challenge: string; expires_in: number }>(
        '/auth/challenge'
      );

      // 4. Sign challenge with private key
      const challengeBytes = hexToBytes(challengeResponse.challenge);
      const signature = sign(challengeBytes, privateKey);

      // 5. Verify with server
      const verifyResponse = await api.post<{ token: string; user_id: string }>('/auth/verify', {
        public_key: bytesToHex(publicKey),
        signature: bytesToHex(signature),
        challenge: challengeResponse.challenge,
      });

      // 6. Store JWT and navigate
      setToken(verifyResponse.token, verifyResponse.user_id);
      setPassphrase(''); // Clear sensitive data from memory
      navigate('/', { replace: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Authentication failed';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-4xl font-bold mb-2">Poindexter</h1>
          <p className="text-gray-400">Your AI Orchestration Genius</p>
        </div>

        {showGenerated && generatedPhrase ? (
          <div className="bg-gray-800 rounded-lg p-6 mb-6">
            <div className="flex items-center gap-2 mb-4">
              <svg
                className="w-5 h-5 text-yellow-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
              <span className="text-yellow-500 font-semibold">Save This Passphrase</span>
            </div>
            <p className="text-gray-300 text-sm mb-4">
              This is your only way to access Poindexter. Write it down and store it somewhere safe.
              It cannot be recovered.
            </p>
            <div className="bg-gray-900 rounded p-4 mb-4 font-mono text-sm break-words leading-relaxed">
              {generatedPhrase}
            </div>
            <button
              onClick={handleUseGenerated}
              className="w-full bg-blue-600 hover:bg-blue-700 text-white font-semibold py-3 px-4 rounded-lg transition-colors"
            >
              I've Saved It - Continue
            </button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-6">
            <div>
              <label htmlFor="passphrase" className="block text-sm font-medium text-gray-300 mb-2">
                Enter your 24-word passphrase
              </label>
              <textarea
                id="passphrase"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                placeholder="word1 word2 word3 ... word24"
                rows={4}
                className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent resize-none font-mono text-sm"
                disabled={isLoading}
              />
            </div>

            {error && (
              <div className="bg-red-900/50 border border-red-500 rounded-lg p-3">
                <p className="text-red-400 text-sm">{error}</p>
              </div>
            )}

            <button
              type="submit"
              disabled={isLoading || !passphrase.trim()}
              className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors"
            >
              {isLoading ? 'Signing In...' : 'Sign In'}
            </button>

            <div className="text-center">
              <button
                type="button"
                onClick={handleGeneratePassphrase}
                className="text-blue-400 hover:text-blue-300 text-sm transition-colors"
              >
                First time? Generate a new passphrase
              </button>
            </div>
          </form>
        )}
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
                  className="block bg-gray-700 hover:bg-gray-650 rounded-lg p-3 transition-colors"
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
  return (
    <div className="min-h-screen bg-gray-900 text-white p-8">
      <h1 className="text-3xl font-bold mb-4">Tasks</h1>
      <p className="text-gray-400">(Task list UI coming soon)</p>
    </div>
  );
}

function TaskDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<Task | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isStarting, setIsStarting] = useState(false);
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
      await api.post(`/tasks/${id}/start`, {
        project_path: '/Users/liran/src/dex',
        base_branch: 'main',
      });
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
                disabled={isStarting}
                className="flex-1 bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg transition-colors"
              >
                {isStarting ? 'Starting...' : 'Start Task'}
              </button>
            )}
            {isRunning && (
              <button
                disabled
                className="flex-1 bg-orange-600/50 cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg"
                title="Pause functionality coming soon"
              >
                Pause (coming soon)
              </button>
            )}
            {isPaused && (
              <button
                disabled
                className="flex-1 bg-blue-600/50 cursor-not-allowed text-white font-medium py-2 px-4 rounded-lg"
                title="Resume functionality coming soon"
              >
                Resume (coming soon)
              </button>
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

// Protected route wrapper
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
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
      {/* Catch-all redirect to dashboard */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
