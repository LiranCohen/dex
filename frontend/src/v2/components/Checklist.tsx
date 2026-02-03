import type { ChecklistItem, ChecklistSummary, ChecklistItemStatus } from '../../lib/types';

// Re-export for convenience
export type { ChecklistItem };

interface ChecklistProps {
  items: ChecklistItem[];
  summary?: ChecklistSummary;
  emptyMessage?: string;
}

function getStatusInfo(status: ChecklistItemStatus): { icon: string; label: string; className: string } {
  switch (status) {
    case 'done':
      return { icon: '✓', label: 'Completed', className: 'complete' };
    case 'in_progress':
      return { icon: '●', label: 'In progress', className: 'active' };
    case 'failed':
      return { icon: '✗', label: 'Failed', className: 'error' };
    case 'skipped':
      return { icon: '—', label: 'Skipped', className: 'skipped' };
    case 'pending':
    default:
      return { icon: '◯', label: 'Pending', className: 'pending' };
  }
}

function getStatusIcon(status: ChecklistItemStatus) {
  const info = getStatusInfo(status);
  return (
    <span
      className={`v2-checklist-item__icon v2-checklist-item__icon--${info.className}`}
      role="img"
      aria-label={info.label}
    >
      {info.icon}
    </span>
  );
}

function getStatusLabel(status: ChecklistItemStatus) {
  if (status === 'pending') return null;
  const info = getStatusInfo(status);
  return (
    <span className={`v2-status-label v2-status-label--${status === 'done' ? 'complete' : status === 'in_progress' ? 'running' : status}`}>
      {info.label}
    </span>
  );
}

interface ProgressBarProps {
  total: number;
  done: number;
  failed: number;
  inProgress?: number;
}

function ProgressBar({ total, done, failed, inProgress = 0 }: ProgressBarProps) {
  if (total === 0) return null;

  const donePercent = (done / total) * 100;
  const failedPercent = (failed / total) * 100;
  const inProgressPercent = (inProgress / total) * 100;
  const isComplete = done + failed >= total;

  return (
    <div
      className="v2-progress-bar"
      role="progressbar"
      aria-valuenow={done}
      aria-valuemin={0}
      aria-valuemax={total}
      aria-label={`${done} of ${total} items complete${failed > 0 ? `, ${failed} failed` : ''}`}
    >
      <div className="v2-progress-bar__track">
        {/* Done segment */}
        <div
          className="v2-progress-bar__segment v2-progress-bar__segment--done"
          style={{ width: `${donePercent}%` }}
        />
        {/* Failed segment */}
        {failed > 0 && (
          <div
            className="v2-progress-bar__segment v2-progress-bar__segment--failed"
            style={{ width: `${failedPercent}%`, left: `${donePercent}%` }}
          />
        )}
        {/* In progress segment */}
        {inProgress > 0 && (
          <div
            className="v2-progress-bar__segment v2-progress-bar__segment--active"
            style={{ width: `${inProgressPercent}%`, left: `${donePercent + failedPercent}%` }}
          />
        )}
      </div>
      {isComplete && failed === 0 && (
        <span className="v2-progress-bar__complete-icon" aria-hidden="true">✓</span>
      )}
    </div>
  );
}

export function Checklist({ items, summary, emptyMessage = '// no checklist items' }: ChecklistProps) {
  if (items.length === 0) {
    return <p className="v2-empty-hint">{emptyMessage}</p>;
  }

  const isCompleted = (status: ChecklistItemStatus) => status === 'done' || status === 'skipped';

  // Count items by status for progress bar
  const inProgressCount = items.filter(i => i.status === 'in_progress').length;

  // Sort items: failed first, then in_progress, then pending, then done/skipped
  const sortedItems = [...items].sort((a, b) => {
    const priority: Record<ChecklistItemStatus, number> = {
      failed: 0,
      in_progress: 1,
      pending: 2,
      done: 3,
      skipped: 4,
    };
    return (priority[a.status] ?? 5) - (priority[b.status] ?? 5);
  });

  // Group failed items for highlighting
  const failedItems = sortedItems.filter(i => i.status === 'failed');
  const otherItems = sortedItems.filter(i => i.status !== 'failed');
  const hasFailed = failedItems.length > 0;

  return (
    <div className="v2-checklist-container">
      {/* Progress bar and summary */}
      {summary && (
        <div className="v2-checklist-header">
          <ProgressBar
            total={summary.total}
            done={summary.done}
            failed={summary.failed}
            inProgress={inProgressCount}
          />
          <div className="v2-checklist-summary">
            <span className={`v2-checklist-summary__count ${summary.all_done ? 'v2-checklist-summary__count--complete' : ''}`}>
              {summary.done}/{summary.total}
            </span>
            <span className="v2-checklist-summary__label">
              {summary.all_done ? 'All complete' : 'completed'}
            </span>
            {summary.failed > 0 && (
              <span className="v2-checklist-summary__failed">
                <span className="v2-checklist-summary__failed-icon" aria-hidden="true">✗</span>
                {summary.failed} failed
              </span>
            )}
          </div>
        </div>
      )}

      {/* Failed items section - highlighted */}
      {hasFailed && (
        <div className="v2-checklist-failed-section" role="alert">
          <div className="v2-checklist-failed-section__header">
            <span className="v2-checklist-failed-section__icon" aria-hidden="true">⚠</span>
            <span className="v2-label v2-checklist-failed-section__label">Failed Items</span>
          </div>
          <div className="v2-card v2-checklist v2-checklist--failed">
            {failedItems.map((item) => (
              <div key={item.id} className="v2-checklist-item v2-checklist-item--error">
                {getStatusIcon(item.status)}
                <div className="v2-checklist-item__content">
                  <span className="v2-checklist-item__text">
                    {item.description}
                  </span>
                  {getStatusLabel(item.status)}
                  {item.verification_notes && (
                    <p className="v2-checklist-item__notes">{item.verification_notes}</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Other items */}
      <div className="v2-card v2-checklist">
        {otherItems.map((item) => (
          <div key={item.id} className={`v2-checklist-item ${item.status === 'in_progress' ? 'v2-checklist-item--active' : ''}`}>
            {getStatusIcon(item.status)}
            <div className="v2-checklist-item__content">
              <span className={`v2-checklist-item__text ${isCompleted(item.status) ? 'v2-checklist-item__text--complete' : ''}`}>
                {item.description}
              </span>
              {getStatusLabel(item.status)}
              {item.verification_notes && (
                <p className="v2-checklist-item__notes">{item.verification_notes}</p>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
