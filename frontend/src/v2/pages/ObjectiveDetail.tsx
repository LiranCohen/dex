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
  ObjectiveActions,
  type ChecklistItem,
  type Activity,
} from '../components';
import { api, fetchApprovals } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { getTaskStatus } from '../utils/formatters';
import type { Task, Approval, WebSocketEvent } from '../../lib/types';

export function ObjectiveDetail() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<Task | null>(null);
  const [checklist, setChecklist] = useState<ChecklistItem[]>([]);
  const [activity, setActivity] = useState<Activity[]>([]);
  const [approvalCount, setApprovalCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showCancelConfirm, setShowCancelConfirm] = useState(false);
  const { subscribe } = useWebSocket();
  const { showToast } = useToast();

  const loadData = useCallback(async () => {
    if (!id) return;
    try {
      const [taskData, checklistData, activityData, approvalsData] = await Promise.all([
        api.get<{ task: Task }>(`/tasks/${id}`),
        api.get<{ items: ChecklistItem[] }>(`/tasks/${id}/checklist`).catch(() => ({ items: [] })),
        api.get<{ activities: Activity[] }>(`/tasks/${id}/activity`).catch(() => ({ activities: [] })),
        fetchApprovals(),
      ]);
      setTask(taskData.task);
      setChecklist(checklistData.items || []);
      setActivity(activityData.activities || []);
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
              <span className="v2-label">{task.Status.toUpperCase()}</span>
            </div>
          </div>

          <ObjectiveActions
            status={task.Status}
            actionLoading={actionLoading}
            onStart={handleStart}
            onPause={handlePause}
            onResume={handleResume}
            onCancel={() => setShowCancelConfirm(true)}
          />
        </div>

        {/* Description */}
        {task.Description && (
          <p className="v2-objective-description">{task.Description}</p>
        )}

        {/* Checklist */}
        <div className="v2-objective-section">
          <div className="v2-label v2-objective-section__title">Checklist</div>
          <Checklist items={checklist} />
        </div>

        {/* Activity */}
        <div className="v2-objective-section">
          <div className="v2-label v2-objective-section__title">Activity</div>
          <ActivityLog items={activity} />
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
