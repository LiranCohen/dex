import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import {
  Header,
  StatusBar,
  LoadingState,
  ConfirmModal,
  useToast,
  Checklist,
  ActivityLog,
  ContextUsageBar,
  ObjectiveActions,
  DependencyGraph,
} from '../components';
import { api, fetchApprovals, fetchChecklist, fetchTaskActivity, fetchQuestTasks } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { getTaskStatus } from '../utils/formatters';
import type {
  Task,
  Approval,
  WebSocketEvent,
  ChecklistItem,
  ChecklistSummary,
  Activity,
  ActivityResponse,
} from '../../lib/types';

// Type guard for context status
function isContextStatus(obj: unknown): obj is {
  used_tokens: number;
  max_tokens: number;
  usage_percent: number;
  status: 'ok' | 'warning' | 'critical';
} {
  return typeof obj === 'object' && obj !== null &&
    'used_tokens' in obj && 'max_tokens' in obj &&
    'usage_percent' in obj && 'status' in obj;
}

export function ObjectiveDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [task, setTask] = useState<Task | null>(null);
  const [questTasks, setQuestTasks] = useState<Task[]>([]);
  const [checklist, setChecklist] = useState<ChecklistItem[]>([]);
  const [checklistSummary, setChecklistSummary] = useState<ChecklistSummary | undefined>(undefined);
  const [activity, setActivity] = useState<Activity[]>([]);
  const [activitySummary, setActivitySummary] = useState<ActivityResponse['summary'] | undefined>(undefined);
  const [approvalCount, setApprovalCount] = useState(0);
  const [contextStatus, setContextStatus] = useState<{
    used_tokens: number;
    max_tokens: number;
    usage_percent: number;
    status: 'ok' | 'warning' | 'critical';
  } | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showCancelConfirm, setShowCancelConfirm] = useState(false);
  const { subscribe } = useWebSocket();
  const { showToast } = useToast();

  // Track if we've already shown the critical context warning
  const hasShownCriticalWarning = useRef(false);
  // Track mount state for safe WebSocket handler updates
  const isMountedRef = useRef(true);

  // Track mount state
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  // Calculate prev/next objective navigation
  const { prevObjective, nextObjective, objectivePosition } = useMemo(() => {
    if (!id || questTasks.length === 0) {
      return { prevObjective: null, nextObjective: null, objectivePosition: null };
    }
    const currentIndex = questTasks.findIndex((t) => t.ID === id);
    if (currentIndex === -1) {
      return { prevObjective: null, nextObjective: null, objectivePosition: null };
    }
    return {
      prevObjective: currentIndex > 0 ? questTasks[currentIndex - 1] : null,
      nextObjective: currentIndex < questTasks.length - 1 ? questTasks[currentIndex + 1] : null,
      objectivePosition: { current: currentIndex + 1, total: questTasks.length },
    };
  }, [id, questTasks]);

  // Keyboard navigation for prev/next
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }
      if (e.key === 'ArrowLeft' && prevObjective) {
        e.preventDefault();
        navigate(`/v2/objectives/${prevObjective.ID}`);
      } else if (e.key === 'ArrowRight' && nextObjective) {
        e.preventDefault();
        navigate(`/v2/objectives/${nextObjective.ID}`);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [prevObjective, nextObjective, navigate]);

  const loadData = useCallback(async () => {
    if (!id) return;
    try {
      // Use Promise.allSettled for graceful partial failure handling
      const results = await Promise.allSettled([
        api.get<Task>(`/tasks/${id}`),
        fetchChecklist(id),
        fetchTaskActivity(id),
        fetchApprovals(),
      ]);

      const [taskResult, checklistResult, activityResult, approvalsResult] = results;

      // Task is required - if it fails, show error and return
      if (taskResult.status === 'rejected') {
        console.error('Failed to load task:', taskResult.reason);
        showToast('Failed to load objective', 'error');
        setLoading(false);
        return;
      }
      const taskData = taskResult.value;
      setTask(taskData);

      // Fetch quest tasks if this task belongs to a quest (for looking up blocked-by names)
      if (taskData.QuestID) {
        try {
          const questTasksData = await fetchQuestTasks(taskData.QuestID);
          setQuestTasks(questTasksData || []);
        } catch (err) {
          console.error('Failed to load quest tasks:', err);
          // Non-critical - just use empty array
          setQuestTasks([]);
        }
      }

      // Checklist - optional, use defaults on failure
      if (checklistResult.status === 'fulfilled') {
        setChecklist(checklistResult.value.items || []);
        setChecklistSummary(checklistResult.value.summary);
      } else {
        console.error('Failed to load checklist:', checklistResult.reason);
        setChecklist([]);
        setChecklistSummary({ total: 0, done: 0, failed: 0, pending: 0, all_done: false });
      }

      // Activity - optional, use defaults on failure
      if (activityResult.status === 'fulfilled') {
        setActivity(activityResult.value.activity || []);
        setActivitySummary(activityResult.value.summary);
      } else {
        console.error('Failed to load activity:', activityResult.reason);
        setActivity([]);
        setActivitySummary({ total_iterations: 0, total_tokens: 0 });
      }

      // Approvals - optional, use default on failure
      if (approvalsResult.status === 'fulfilled') {
        setApprovalCount((approvalsResult.value.approvals || []).filter((a: Approval) => a.status === 'pending').length);
      } else {
        console.error('Failed to load approvals:', approvalsResult.reason);
        // Keep previous approval count on failure
      }
    } catch (err) {
      console.error('Failed to load objective:', err);
      showToast('Failed to load objective', 'error');
    } finally {
      setLoading(false);
    }
  }, [id, showToast]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // WebSocket updates
  useEffect(() => {
    const unsubscribe = subscribe((event: WebSocketEvent) => {
      // Skip state updates if component is unmounted
      if (!id || !isMountedRef.current) return;

      const payload = event.payload as Record<string, unknown>;
      const taskId = payload?.task_id;
      if (taskId === id) {
        if (event.type === 'checklist.updated') {
          loadData();
        } else if (event.type.startsWith('task.') || event.type.startsWith('session.')) {
          loadData();
        }
      }

      // Capture context status from session.iteration events
      if (event.type === 'session.iteration' && isContextStatus(payload?.context)) {
        const ctx = payload.context;
        setContextStatus(ctx);

        // Show warning toast when context usage becomes critical (only once per session)
        if (ctx.status === 'critical' && !hasShownCriticalWarning.current) {
          hasShownCriticalWarning.current = true;
          showToast('Context usage is critical - task may need to summarize soon', 'info');
        }
      }

      // Clear context when session completes
      if (event.type === 'session.completed') {
        setContextStatus(undefined);
        hasShownCriticalWarning.current = false; // Reset for next session
      }

      if (event.type.startsWith('approval.')) {
        fetchApprovals()
          .then((data) => {
            // Skip if unmounted
            if (!isMountedRef.current) return;
            setApprovalCount((data.approvals || []).filter((a: Approval) => a.status === 'pending').length);
          })
          .catch((err) => {
            console.error('Failed to fetch approvals:', err);
            // Silently fail - approval count is not critical
          });
      }
    });

    return unsubscribe;
  }, [id, subscribe, loadData]);

  const handlePause = async () => {
    if (!id || actionLoading) return;
    setActionLoading('pause');
    try {
      await api.post(`/tasks/${id}/pause`);
      showToast('Objective paused', 'success');
      loadData();
    } catch (err) {
      console.error('Failed to pause:', err);
      showToast('Failed to pause objective', 'error');
    } finally {
      setActionLoading(null);
    }
  };

  const handleResume = async () => {
    if (!id || actionLoading) return;
    setActionLoading('resume');
    try {
      await api.post(`/tasks/${id}/resume`);
      showToast('Objective resumed', 'success');
      loadData();
    } catch (err) {
      console.error('Failed to resume:', err);
      showToast('Failed to resume objective', 'error');
    } finally {
      setActionLoading(null);
    }
  };

  const handleCancelConfirm = async () => {
    if (!id) return;
    setShowCancelConfirm(false);
    setActionLoading('cancel');
    try {
      await api.post(`/tasks/${id}/cancel`);
      showToast('Objective cancelled', 'success');
      loadData();
    } catch (err) {
      console.error('Failed to cancel:', err);
      showToast('Failed to cancel objective', 'error');
    } finally {
      setActionLoading(null);
    }
  };

  const handleStart = async () => {
    if (!id || actionLoading) return;
    setActionLoading('start');
    try {
      await api.post(`/tasks/${id}/start`);
      showToast('Objective started', 'success');
      loadData();
    } catch (err) {
      console.error('Failed to start:', err);
      showToast('Failed to start objective', 'error');
    } finally {
      setActionLoading(null);
    }
  };

  if (loading) {
    return (
      <div className="v2-root">
        <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={0} />
        <main className="v2-content">
          <LoadingState message="Loading objective..." size="large" />
        </main>
      </div>
    );
  }

  if (!task) {
    return (
      <div className="v2-root">
        <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={0} />
        <main className="v2-content">
          <p className="v2-empty-hint">Objective not found</p>
        </main>
      </div>
    );
  }

  const backLink = task.QuestID
    ? { to: `/v2/quests/${task.QuestID}`, label: 'Quest' }
    : { to: '/v2', label: 'Back' };

  return (
    <div className="v2-root">
      <Header backLink={backLink} inboxCount={approvalCount} />

      <main className="v2-content">
        {/* Header */}
        <div className="v2-objective-header">
          <div className="v2-objective-header__info">
            <h1 className="v2-page-title">{task.Title}</h1>
            <div className="v2-objective-header__status">
              <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
              <span className="v2-label">{(task.Status || 'unknown').toUpperCase()}</span>
            </div>
          </div>

          {/* Prev/Next Navigation */}
          {objectivePosition && (
            <div className="v2-objective-nav">
              <Link
                to={prevObjective ? `/v2/objectives/${prevObjective.ID}` : '#'}
                className={`v2-objective-nav__btn ${!prevObjective ? 'v2-objective-nav__btn--disabled' : ''}`}
                aria-label={prevObjective ? `Previous: ${prevObjective.Title}` : 'No previous objective'}
                aria-disabled={!prevObjective}
                onClick={(e) => !prevObjective && e.preventDefault()}
                title={prevObjective?.Title}
              >
                &larr;
              </Link>
              <span className="v2-objective-nav__position">
                {objectivePosition.current} / {objectivePosition.total}
              </span>
              <Link
                to={nextObjective ? `/v2/objectives/${nextObjective.ID}` : '#'}
                className={`v2-objective-nav__btn ${!nextObjective ? 'v2-objective-nav__btn--disabled' : ''}`}
                aria-label={nextObjective ? `Next: ${nextObjective.Title}` : 'No next objective'}
                aria-disabled={!nextObjective}
                onClick={(e) => !nextObjective && e.preventDefault()}
                title={nextObjective?.Title}
              >
                &rarr;
              </Link>
            </div>
          )}

          <ObjectiveActions
            status={task.Status}
            actionLoading={actionLoading}
            isBlocked={task.IsBlocked}
            onStart={handleStart}
            onPause={handlePause}
            onResume={handleResume}
            onCancel={() => setShowCancelConfirm(true)}
          />
        </div>

        {/* Context usage bar - shown when task is running */}
        {task.Status === 'running' && contextStatus && (
          <ContextUsageBar context={contextStatus} />
        )}

        {/* Blocked status indicator (derived from dependencies) */}
        {task.IsBlocked && (
          <div className="v2-blocked-notice">
            <span className="v2-label">Waiting for dependencies</span>
            <p className="v2-blocked-notice__text">
              This objective will automatically start when its blocking objectives complete.
            </p>
            {task.BlockedBy && task.BlockedBy.length > 0 && (
              <div className="v2-blocked-notice__blockers">
                <span className="v2-blocked-notice__blockers-label">Blocked by:</span>
                <ul className="v2-blocked-notice__blockers-list">
                  {task.BlockedBy.map((blockerId) => {
                    const blocker = questTasks.find((t) => t.ID === blockerId);
                    return (
                      <li key={blockerId}>
                        <Link
                          to={`/v2/objectives/${blockerId}`}
                          className="v2-link"
                        >
                          {blocker?.Title || `Objective ${blockerId.slice(0, 8)}...`}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </div>
            )}
          </div>
        )}

        {/* Description */}
        {task.Description && (
          <p className="v2-objective-description">{task.Description}</p>
        )}

        {/* Checklist */}
        <div className="v2-objective-section">
          <div className="v2-label v2-objective-section__title">Checklist</div>
          <Checklist items={checklist} summary={checklistSummary} />
        </div>

        {/* Activity */}
        <div className="v2-objective-section">
          <div className="v2-label v2-objective-section__title">Activity</div>
          <ActivityLog
            items={activity}
            summary={activitySummary}
            isRunning={task.Status === 'running'}
          />
        </div>

        {/* Dependencies - show if there are any related tasks with dependencies */}
        {questTasks.length > 1 && questTasks.some((t) => t.BlockedBy && t.BlockedBy.length > 0) && (
          <div className="v2-objective-section">
            <div className="v2-label v2-objective-section__title">Dependencies</div>
            <DependencyGraph tasks={questTasks} currentTaskId={task.ID} />
          </div>
        )}
      </main>

      {/* Cancel confirmation modal */}
      <ConfirmModal
        isOpen={showCancelConfirm}
        title="Cancel Objective"
        message="Are you sure you want to cancel this objective? This action cannot be undone."
        confirmLabel="Cancel Objective"
        cancelLabel="Keep Running"
        variant="danger"
        onConfirm={handleCancelConfirm}
        onCancel={() => setShowCancelConfirm(false)}
      />
    </div>
  );
}
