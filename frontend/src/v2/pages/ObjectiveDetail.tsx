import { useState, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import {
  Header,
  StatusBar,
  SkeletonList,
  ConfirmModal,
  useToast,
  Checklist,
  ActivityLog,
  ContextUsageBar,
  ObjectiveActions,
} from '../components';
import { api, fetchApprovals, fetchChecklist, fetchTaskActivity } from '../../lib/api';
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

export function ObjectiveDetail() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<Task | null>(null);
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

  const loadData = useCallback(async () => {
    if (!id) return;
    try {
      const [taskData, checklistData, activityData, approvalsData] = await Promise.all([
        api.get<Task>(`/tasks/${id}`),
        fetchChecklist(id).catch(() => ({ checklist: null, items: [], summary: { total: 0, done: 0, failed: 0, pending: 0, all_done: false } })),
        fetchTaskActivity(id).catch(() => ({ activity: [], summary: { total_iterations: 0, total_tokens: 0 } })),
        fetchApprovals(),
      ]);
      setTask(taskData);
      setChecklist(checklistData.items || []);
      setChecklistSummary(checklistData.summary);
      setActivity(activityData.activity || []);
      setActivitySummary(activityData.summary);
      setApprovalCount((approvalsData.approvals || []).filter((a: Approval) => a.status === 'pending').length);
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
      if (!id) return;

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
      if (event.type === 'session.iteration' && payload?.context) {
        const ctx = payload.context as {
          used_tokens: number;
          max_tokens: number;
          usage_percent: number;
          status: 'ok' | 'warning' | 'critical';
        };
        setContextStatus(ctx);
      }

      // Clear context when session completes
      if (event.type === 'session.completed') {
        setContextStatus(undefined);
      }

      if (event.type.startsWith('approval.')) {
        fetchApprovals().then((data) => {
          setApprovalCount((data.approvals || []).filter((a: Approval) => a.status === 'pending').length);
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
          <SkeletonList count={3} />
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
