import type { ChecklistItem, ChecklistSummary, ChecklistItemStatus } from '../../lib/types';

// Re-export for convenience
export type { ChecklistItem };

interface ChecklistProps {
  items: ChecklistItem[];
  summary?: ChecklistSummary;
  emptyMessage?: string;
}

function getStatusIcon(status: ChecklistItemStatus) {
  switch (status) {
    case 'done':
      return <span className="v2-checklist-item__icon v2-checklist-item__icon--complete">✓</span>;
    case 'in_progress':
      return <span className="v2-checklist-item__icon v2-checklist-item__icon--active">●</span>;
    case 'failed':
      return <span className="v2-checklist-item__icon v2-checklist-item__icon--error">✗</span>;
    case 'skipped':
      return <span className="v2-checklist-item__icon v2-checklist-item__icon--skipped">—</span>;
    case 'pending':
    default:
      return <span className="v2-checklist-item__icon v2-checklist-item__icon--pending">◯</span>;
  }
}

function getStatusLabel(status: ChecklistItemStatus) {
  if (status === 'pending') return null;
  return (
    <span className={`v2-status-label v2-status-label--${status === 'done' ? 'complete' : status === 'in_progress' ? 'running' : status}`}>
      {status.replace('_', ' ')}
    </span>
  );
}

export function Checklist({ items, summary, emptyMessage = '// no checklist items' }: ChecklistProps) {
  if (items.length === 0) {
    return <p className="v2-empty-hint">{emptyMessage}</p>;
  }

  const isCompleted = (status: ChecklistItemStatus) => status === 'done' || status === 'skipped';

  return (
    <div className="v2-checklist-container">
      {/* Summary bar */}
      {summary && (
        <div className="v2-checklist-summary">
          <span className="v2-label">Progress:</span>
          <span className={summary.all_done ? 'v2-highlight' : ''}>
            {summary.done}/{summary.total} completed
          </span>
          {summary.failed > 0 && (
            <span className="v2-checklist-summary__failed">({summary.failed} failed)</span>
          )}
        </div>
      )}

      <div className="v2-card v2-checklist">
        {items.map((item) => (
          <div key={item.id} className={`v2-checklist-item ${item.status === 'failed' ? 'v2-checklist-item--error' : ''}`}>
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
