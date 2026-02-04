import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { Link } from 'react-router-dom';
import { Header, StatusBar, SearchInput, LoadingState, ConnectionStatusBanner, useToast } from '../components';
import type { SearchInputRef } from '../components/SearchInput';
import { api, fetchApprovals } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import type { Task, Approval, WebSocketEvent } from '../../lib/types';

type FilterStatus = 'all' | 'running' | 'pending' | 'completed' | 'failed';

function getTaskStatus(status: string): 'active' | 'pending' | 'complete' | 'error' {
  switch (status) {
    case 'running':
      return 'active';
    case 'completed':
      return 'complete';
    case 'failed':
    case 'cancelled':
      return 'error';
    default:
      return 'pending';
  }
}

function matchesFilter(task: Task, filter: FilterStatus): boolean {
  if (filter === 'all') return true;
  if (filter === 'running') return task.Status === 'running';
  if (filter === 'pending') return ['pending', 'ready', 'planning', 'paused'].includes(task.Status);
  if (filter === 'completed') return task.Status === 'completed';
  if (filter === 'failed') return ['failed', 'cancelled'].includes(task.Status);
  return true;
}

function matchesSearch(task: Task, search: string): boolean {
  if (!search.trim()) return true;
  const searchLower = search.toLowerCase();
  return (
    task.Title.toLowerCase().includes(searchLower) ||
    (task.Description?.toLowerCase().includes(searchLower) ?? false) ||
    (task.QuestTitle?.toLowerCase().includes(searchLower) ?? false)
  );
}

export function AllObjectives() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [approvalCount, setApprovalCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<FilterStatus>('all');
  const [search, setSearch] = useState('');
  const searchInputRef = useRef<SearchInputRef>(null);
  const isMountedRef = useRef(true);
  const { subscribe, connectionState, connectionQuality, latency, reconnectAttempts, reconnect } = useWebSocket();
  const { showToast } = useToast();

  // Track mount state
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  // "/" key to focus search
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }
      if (e.key === '/') {
        e.preventDefault();
        searchInputRef.current?.focus();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, []);

  const loadData = useCallback(async () => {
    try {
      const [tasksData, approvalsData] = await Promise.all([
        api.get<{ tasks: Task[] }>('/tasks'),
        fetchApprovals(),
      ]);
      setTasks(tasksData.tasks || []);
      setApprovalCount((approvalsData.approvals || []).filter((a: Approval) => a.status === 'pending').length);
    } catch (err) {
      console.error('Failed to load tasks:', err);
      showToast('Failed to load objectives', 'error');
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // WebSocket updates
  useEffect(() => {
    console.log('[AllObjectives] Setting up WebSocket subscription');
    const unsubscribe = subscribe((event: WebSocketEvent) => {
      console.log('[AllObjectives] Received event:', event.type);
      // Skip if unmounted
      if (!isMountedRef.current) {
        console.log('[AllObjectives] Skipping - unmounted');
        return;
      }

      // Listen for both task and session events - session events indicate status changes
      if (event.type.startsWith('task.') || event.type.startsWith('session.')) {
        console.log('[AllObjectives] Reloading data for event:', event.type);
        loadData();
      }
      if (event.type.startsWith('approval.')) {
        fetchApprovals()
          .then((data) => {
            // Check again after async operation
            if (!isMountedRef.current) return;
            setApprovalCount((data.approvals || []).filter((a: Approval) => a.status === 'pending').length);
          })
          .catch((err) => {
            console.error('Failed to fetch approvals:', err);
          });
      }
    });
    return () => {
      console.log('[AllObjectives] Cleaning up WebSocket subscription');
      unsubscribe();
    };
  }, [subscribe, loadData]);

  const filteredTasks = useMemo(() =>
    tasks.filter((t) => matchesFilter(t, filter) && matchesSearch(t, search)),
    [tasks, filter, search]
  );

  // Group by quest
  const { tasksByQuest, orphanTasks } = useMemo(() => {
    const byQuest = new Map<string, { questTitle: string; tasks: Task[] }>();
    const orphans: Task[] = [];

    filteredTasks.forEach((task) => {
      if (task.QuestID) {
        const existing = byQuest.get(task.QuestID);
        if (existing) {
          existing.tasks.push(task);
        } else {
          byQuest.set(task.QuestID, {
            questTitle: task.QuestTitle || 'Unknown Quest',
            tasks: [task],
          });
        }
      } else {
        orphans.push(task);
      }
    });

    return { tasksByQuest: byQuest, orphanTasks: orphans };
  }, [filteredTasks]);

  const filters: { key: FilterStatus; label: string }[] = [
    { key: 'all', label: 'All' },
    { key: 'running', label: 'Running' },
    { key: 'pending', label: 'Pending' },
    { key: 'completed', label: 'Complete' },
    { key: 'failed', label: 'Failed' },
  ];

  if (loading) {
    return (
      <div className="app-root">
        <Header backLink={{ to: '/', label: 'Back' }} inboxCount={0} />
        <main className="app-content">
          <LoadingState message="Loading objectives..." size="large" />
        </main>
      </div>
    );
  }

  return (
    <div className="app-root">
      <Header backLink={{ to: '/', label: 'Back' }} inboxCount={approvalCount} />

      <ConnectionStatusBanner
        connectionState={connectionState}
        connectionQuality={connectionQuality}
        latency={latency}
        reconnectAttempts={reconnectAttempts}
        onReconnect={reconnect}
      />

      <main className="app-content">
        <div className="app-all-objectives-header">
          <div className="app-all-objectives-header__top">
            <h1 className="app-page-title">All Objectives</h1>
            <SearchInput
              ref={searchInputRef}
              value={search}
              onChange={setSearch}
              placeholder="Search objectives..."
              onEscape={() => setSearch('')}
            />
          </div>

          {/* Filters */}
          <div className="app-filter-bar" role="group" aria-label="Filter objectives by status">
            {filters.map((f) => (
              <button
                key={f.key}
                type="button"
                className={`app-btn ${filter === f.key ? 'app-btn--secondary app-filter-bar__btn--active' : 'app-btn--ghost'}`}
                onClick={() => setFilter(f.key)}
                aria-pressed={filter === f.key}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        {filteredTasks.length === 0 ? (
          <div className="app-empty-centered">
            <p>[ {search ? 'no matching objectives' : 'empty'} ]</p>
          </div>
        ) : (
          <div className="app-objectives-groups">
            {/* Grouped by quest */}
            {Array.from(tasksByQuest.entries()).map(([questId, { questTitle, tasks: questTasks }]) => (
              <div key={questId} className="app-objectives-group">
                <Link
                  to={`/quests/${questId}`}
                  className="app-objectives-group__title"
                >
                  Quest: {questTitle}
                </Link>
                <div className="app-card app-objectives-group__list">
                  {questTasks.map((task) => (
                    <Link
                      key={task.ID}
                      to={`/objectives/${task.ID}`}
                      className="app-objectives-list-item"
                    >
                      <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
                      <span className="app-objectives-list-item__title">{task.Title}</span>
                      <span className="app-label">{task.Status}</span>
                    </Link>
                  ))}
                </div>
              </div>
            ))}

            {/* Orphan tasks (no quest) */}
            {orphanTasks.length > 0 && (
              <div className="app-objectives-group">
                <span className="app-objectives-group__title app-objectives-group__title--muted">
                  Standalone Objectives
                </span>
                <div className="app-card app-objectives-group__list">
                  {orphanTasks.map((task) => (
                    <Link
                      key={task.ID}
                      to={`/objectives/${task.ID}`}
                      className="app-objectives-list-item"
                    >
                      <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
                      <span className="app-objectives-list-item__title">{task.Title}</span>
                      <span className="app-label">{task.Status}</span>
                    </Link>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </main>
    </div>
  );
}
