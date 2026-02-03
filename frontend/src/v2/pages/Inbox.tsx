import { useState, useEffect, useCallback, useMemo } from 'react';
import { Link } from 'react-router-dom';
import { Header, KeyboardShortcuts, SkeletonList, Button, useToast } from '../components';
import { fetchApprovals, approveApproval, rejectApproval } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';
import type { Approval, WebSocketEvent } from '../../lib/types';

function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - date.getTime();

  if (diff < 60000) return 'just now';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
  return date.toLocaleDateString();
}

function getApprovalTypeLabel(type: string): string {
  switch (type) {
    case 'commit':
      return 'Commit';
    case 'pr':
      return 'Pull Request';
    case 'merge':
      return 'Merge';
    case 'hat_transition':
      return 'Role Change';
    default:
      return type;
  }
}

export function Inbox() {
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [loading, setLoading] = useState(true);
  const [processing, setProcessing] = useState<Set<string>>(new Set());
  const [showShortcuts, setShowShortcuts] = useState(false);
  const { subscribe } = useWebSocket();
  const { showToast } = useToast();

  // Keyboard navigation items
  const navItems = useMemo(() =>
    approvals.map((approval) => ({
      id: approval.id,
    })),
    [approvals]
  );

  const { selectedIndex } = useKeyboardNavigation({
    onHelp: () => setShowShortcuts(true),
    items: navItems,
    enabled: !loading,
  });

  const loadData = useCallback(async () => {
    try {
      const data = await fetchApprovals();
      setApprovals((data.approvals || []).filter((a: Approval) => a.status === 'pending'));
    } catch (err) {
      console.error('Failed to load approvals:', err);
      showToast('Failed to load inbox', 'error');
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
      if (event.type.startsWith('approval.')) {
        loadData();
      }
    });
    return unsubscribe;
  }, [subscribe, loadData]);

  const handleApprove = useCallback(async (id: string) => {
    setProcessing((prev) => new Set(prev).add(id));
    try {
      await approveApproval(id);
      setApprovals((prev) => prev.filter((a) => a.id !== id));
      showToast('Approved successfully', 'success');
    } catch (err) {
      console.error('Failed to approve:', err);
      showToast('Failed to approve', 'error');
    } finally {
      setProcessing((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }, [showToast]);

  const handleReject = useCallback(async (id: string) => {
    setProcessing((prev) => new Set(prev).add(id));
    try {
      await rejectApproval(id);
      setApprovals((prev) => prev.filter((a) => a.id !== id));
      showToast('Rejected', 'info');
    } catch (err) {
      console.error('Failed to reject:', err);
      showToast('Failed to reject', 'error');
    } finally {
      setProcessing((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }, [showToast]);

  // Handle 'a' for approve, 'r' for reject on selected item
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (selectedIndex < 0 || selectedIndex >= approvals.length) return;
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;

      const selectedApproval = approvals[selectedIndex];
      if (!selectedApproval || processing.has(selectedApproval.id)) return;

      if (e.key === 'a') {
        e.preventDefault();
        handleApprove(selectedApproval.id);
      } else if (e.key === 'r') {
        e.preventDefault();
        handleReject(selectedApproval.id);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [selectedIndex, approvals, processing, handleApprove, handleReject]);

  const pendingCount = approvals.length;

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

  return (
    <div className="v2-root">
      <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={pendingCount} />

      <main className="v2-content">
        <h1 className="v2-page-title">Inbox</h1>

        {approvals.length === 0 ? (
          <div className="v2-empty-state">
            <p>Nothing needs attention</p>
          </div>
        ) : (
          <div className="v2-quest-list">
            {approvals.map((approval, index) => {
              const isProcessing = processing.has(approval.id);
              const isSelected = selectedIndex === index;
              return (
                <div key={approval.id} className={`v2-card v2-inbox-card ${isSelected ? 'v2-card--selected' : ''}`}>
                  {/* Type label */}
                  <div className="v2-label v2-label--accent">
                    APPROVAL · {getApprovalTypeLabel(approval.type)}
                  </div>

                  {/* Title */}
                  <h3 className="v2-quest-card__title">
                    {approval.title}
                  </h3>

                  {/* Description */}
                  {approval.description && (
                    <p className="v2-quest-card__progress">
                      {approval.description}
                    </p>
                  )}

                  {/* Context */}
                  {approval.task_id && (
                    <Link to={`/v2/objectives/${approval.task_id}`} className="v2-header__back">
                      View Objective →
                    </Link>
                  )}

                  {/* Actions */}
                  <div className="v2-inbox-card__footer">
                    <span className="v2-timestamp">{formatTime(approval.created_at)}</span>
                    <div className="v2-inbox-card__actions">
                      <Button
                        variant="ghost"
                        onClick={() => handleReject(approval.id)}
                        loading={isProcessing}
                        disabled={isProcessing}
                      >
                        Reject
                      </Button>
                      <Button
                        variant="primary"
                        onClick={() => handleApprove(approval.id)}
                        loading={isProcessing}
                        disabled={isProcessing}
                      >
                        Approve
                      </Button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </main>

      <KeyboardShortcuts isOpen={showShortcuts} onClose={() => setShowShortcuts(false)} />
    </div>
  );
}
