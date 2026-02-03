import { useState, useEffect, useCallback, useMemo } from 'react';
import { Link } from 'react-router-dom';
import { Header, StatusBar, SearchInput, SkeletonList, useToast } from '../components';
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
  const { subscribe } = useWebSocket();
  const { showToast } = useToast();

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
    const unsubscribe = subscribe((event: WebSocketEvent) => {
      if (event.type.startsWith('task.')) {
        loadData();
      }
      if (event.type.startsWith('approval.')) {
        fetchApprovals().then((data) => {
          setApprovalCount((data.approvals || []).filter((a: Approval) => a.status === 'pending').length);
        });
      }
    });
    return unsubscribe;
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
      <div className="v2-root">
        <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={0} />
        <main className="v2-content">
          <SkeletonList count={5} />
        </main>
      </div>
    );
  }

  return (
    <div className="v2-root">
      <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={approvalCount} />

      <main className="v2-content">
        <div className="v2-all-objectives-header">
          <div className="v2-all-objectives-header__top">
            <h1 className="v2-page-title">All Objectives</h1>
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search objectives..."
              onEscape={() => setSearch('')}
            />
          </div>

          {/* Filters */}
          <div className="v2-filter-bar" role="group" aria-label="Filter objectives by status">
            {filters.map((f) => (
              <button
                key={f.key}
                type="button"
                className={`v2-btn ${filter === f.key ? 'v2-btn--secondary v2-filter-bar__btn--active' : 'v2-btn--ghost'}`}
                onClick={() => setFilter(f.key)}
                aria-pressed={filter === f.key}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        {filteredTasks.length === 0 ? (
          <div className="v2-empty-centered">
            <p>[ {search ? 'no matching objectives' : 'empty'} ]</p>
          </div>
        ) : (
          <div className="v2-objectives-groups">
            {/* Grouped by quest */}
            {Array.from(tasksByQuest.entries()).map(([questId, { questTitle, tasks: questTasks }]) => (
              <div key={questId} className="v2-objectives-group">
                <Link
                  to={`/v2/quests/${questId}`}
                  className="v2-objectives-group__title"
                >
                  Quest: {questTitle}
                </Link>
                <div className="v2-card v2-objectives-group__list">
                  {questTasks.map((task) => (
                    <Link
                      key={task.ID}
                      to={`/v2/objectives/${task.ID}`}
                      className="v2-objectives-list-item"
                    >
                      <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
                      <span className="v2-objectives-list-item__title">{task.Title}</span>
                      <span className="v2-label">{task.Status}</span>
                    </Link>
                  ))}
                </div>
              </div>
            ))}

            {/* Orphan tasks (no quest) */}
            {orphanTasks.length > 0 && (
              <div className="v2-objectives-group">
                <span className="v2-objectives-group__title v2-objectives-group__title--muted">
                  Standalone Objectives
                </span>
                <div className="v2-card v2-objectives-group__list">
                  {orphanTasks.map((task) => (
                    <Link
                      key={task.ID}
                      to={`/v2/objectives/${task.ID}`}
                      className="v2-objectives-list-item"
                    >
                      <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
                      <span className="v2-objectives-list-item__title">{task.Title}</span>
                      <span className="v2-label">{task.Status}</span>
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
