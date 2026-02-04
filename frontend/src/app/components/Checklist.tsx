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
      className={`app-checklist-item__icon app-checklist-item__icon--${info.className}`}
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
    <span className={`app-status-label app-status-label--${status === 'done' ? 'complete' : status === 'in_progress' ? 'running' : status}`}>
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
      className="app-progress-bar"
      role="progressbar"
      aria-valuenow={done}
      aria-valuemin={0}
      aria-valuemax={total}
      aria-label={`${done} of ${total} items complete${failed > 0 ? `, ${failed} failed` : ''}`}
    >
      <div className="app-progress-bar__track">
        {/* Done segment */}
        <div
          className="app-progress-bar__segment app-progress-bar__segment--done"
          style={{ width: `${donePercent}%` }}
        />
        {/* Failed segment */}
        {failed > 0 && (
          <div
            className="app-progress-bar__segment app-progress-bar__segment--failed"
            style={{ width: `${failedPercent}%`, left: `${donePercent}%` }}
          />
        )}
        {/* In progress segment */}
        {inProgress > 0 && (
          <div
            className="app-progress-bar__segment app-progress-bar__segment--active"
            style={{ width: `${inProgressPercent}%`, left: `${donePercent + failedPercent}%` }}
          />
        )}
      </div>
      {isComplete && failed === 0 && (
        <span className="app-progress-bar__complete-icon" aria-hidden="true">✓</span>
      )}
    </div>
  );
}

export function Checklist({ items, summary, emptyMessage = '// no checklist items' }: ChecklistProps) {
  if (items.length === 0) {
    return <p className="app-empty-hint">{emptyMessage}</p>;
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
    <div className="app-checklist-container">
      {/* Progress bar and summary */}
      {summary && (
        <div className="app-checklist-header">
          <ProgressBar
            total={summary.total}
            done={summary.done}
            failed={summary.failed}
            inProgress={inProgressCount}
          />
          <div className="app-checklist-summary">
            <span className={`app-checklist-summary__count ${summary.all_done ? 'app-checklist-summary__count--complete' : ''}`}>
              {summary.done}/{summary.total}
            </span>
            <span className="app-checklist-summary__label">
              {summary.all_done ? 'All complete' : 'completed'}
            </span>
            {summary.failed > 0 && (
              <span className="app-checklist-summary__failed">
                <span className="app-checklist-summary__failed-icon" aria-hidden="true">✗</span>
                {summary.failed} failed
              </span>
            )}
          </div>
        </div>
      )}

      {/* Failed items section - highlighted */}
      {hasFailed && (
        <div className="app-checklist-failed-section" role="alert">
          <div className="app-checklist-failed-section__header">
            <span className="app-checklist-failed-section__icon" aria-hidden="true">⚠</span>
            <span className="app-label app-checklist-failed-section__label">Failed Items</span>
          </div>
          <div className="app-card app-checklist app-checklist--failed">
            {failedItems.map((item) => (
              <div key={item.id} className="app-checklist-item app-checklist-item--error">
                {getStatusIcon(item.status)}
                <div className="app-checklist-item__content">
                  <span className="app-checklist-item__text">
                    {item.description}
                  </span>
                  {getStatusLabel(item.status)}
                  {item.verification_notes && (
                    <p className="app-checklist-item__notes">{item.verification_notes}</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Other items */}
      <div className="app-card app-checklist">
        {otherItems.map((item) => (
          <div key={item.id} className={`app-checklist-item ${item.status === 'in_progress' ? 'app-checklist-item--active' : ''}`}>
            {getStatusIcon(item.status)}
            <div className="app-checklist-item__content">
              <span className={`app-checklist-item__text ${isCompleted(item.status) ? 'app-checklist-item__text--complete' : ''}`}>
                {item.description}
              </span>
              {getStatusLabel(item.status)}
              {item.verification_notes && (
                <p className="app-checklist-item__notes">{item.verification_notes}</p>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
