import type { ChecklistItem, ChecklistSummary, ChecklistItemStatus, PendingChecklist } from '../lib/types';

// Props for displaying a pending checklist during planning (before acceptance)
interface PendingChecklistDisplayProps {
  pendingChecklist: PendingChecklist;
  selectedOptional: number[];
  onOptionalToggle: (index: number, selected: boolean) => void;
}

// Props for displaying an accepted checklist (during/after execution)
interface ChecklistDisplayProps {
  items: ChecklistItem[];
  summary?: ChecklistSummary;
}

// Status icons
function PendingIcon() {
  return (
    <svg className="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <circle cx="12" cy="12" r="10" strokeWidth={2} />
    </svg>
  );
}

function InProgressIcon() {
  return (
    <svg className="w-5 h-5 text-blue-400 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <circle cx="12" cy="12" r="10" strokeWidth={2} strokeDasharray="60 40" />
    </svg>
  );
}

function DoneIcon() {
  return (
    <svg className="w-5 h-5 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function FailedIcon() {
  return (
    <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function SkippedIcon() {
  return (
    <svg className="w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 12H4" />
    </svg>
  );
}

function getStatusIcon(status: ChecklistItemStatus) {
  switch (status) {
    case 'pending':
      return <PendingIcon />;
    case 'in_progress':
      return <InProgressIcon />;
    case 'done':
      return <DoneIcon />;
    case 'failed':
      return <FailedIcon />;
    case 'skipped':
      return <SkippedIcon />;
    default:
      return <PendingIcon />;
  }
}

function getStatusBadge(status: ChecklistItemStatus) {
  const styles: Record<ChecklistItemStatus, string> = {
    pending: 'bg-gray-700 text-gray-300',
    in_progress: 'bg-blue-900 text-blue-200',
    done: 'bg-green-900 text-green-200',
    failed: 'bg-red-900 text-red-200',
    skipped: 'bg-gray-800 text-gray-400',
  };

  return (
    <span className={`text-xs px-2 py-0.5 rounded ${styles[status]}`}>
      {status.replace('_', ' ')}
    </span>
  );
}

// Row for accepted checklist items (during/after execution)
function ChecklistItemRow({ item }: { item: ChecklistItem }) {
  const isCompleted = item.status === 'done' || item.status === 'skipped';

  return (
    <div
      className={`flex items-start gap-3 p-3 rounded-lg border ${
        isCompleted
          ? 'border-gray-700 bg-gray-800/30'
          : item.status === 'failed'
          ? 'border-red-800 bg-red-900/10'
          : 'border-gray-700 bg-gray-800/50'
      }`}
    >
      {/* Status icon */}
      <div className="mt-0.5">{getStatusIcon(item.status)}</div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span
            className={`${
              isCompleted ? 'text-gray-400 line-through' : 'text-gray-200'
            }`}
          >
            {item.description}
          </span>
          {item.status !== 'pending' && getStatusBadge(item.status)}
        </div>

        {/* Verification notes */}
        {item.verification_notes && (
          <p className="mt-1 text-sm text-gray-400">
            {item.verification_notes}
          </p>
        )}
      </div>
    </div>
  );
}

// Display for pending checklist during planning phase
export function PendingChecklistDisplay({
  pendingChecklist,
  selectedOptional,
  onOptionalToggle,
}: PendingChecklistDisplayProps) {
  return (
    <div className="space-y-6">
      {/* Must-have section */}
      {pendingChecklist.must_have.length > 0 && (
        <div>
          <h4 className="text-sm font-medium text-gray-400 mb-3 flex items-center gap-2">
            <svg className="w-4 h-4 text-amber-400" fill="currentColor" viewBox="0 0 20 20">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            Required Steps
          </h4>
          <div className="space-y-2">
            {pendingChecklist.must_have.map((description, index) => (
              <div
                key={`must-have-${index}`}
                className="flex items-start gap-3 p-3 rounded-lg border border-gray-700 bg-gray-800/50"
              >
                <input
                  type="checkbox"
                  checked={true}
                  disabled
                  className="mt-1 w-5 h-5 rounded border-gray-600 bg-gray-700 text-blue-500 opacity-50"
                />
                <span className="text-gray-200">{description}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Optional section */}
      {pendingChecklist.optional.length > 0 && (
        <div>
          <h4 className="text-sm font-medium text-gray-400 mb-3 flex items-center gap-2">
            <svg className="w-4 h-4 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
            </svg>
            Optional Enhancements
          </h4>
          <div className="space-y-2">
            {pendingChecklist.optional.map((description, index) => (
              <div
                key={`optional-${index}`}
                className="flex items-start gap-3 p-3 rounded-lg border border-gray-700 bg-gray-800/50"
              >
                <input
                  type="checkbox"
                  checked={selectedOptional.includes(index)}
                  onChange={(e) => onOptionalToggle(index, e.target.checked)}
                  className="mt-1 w-5 h-5 rounded border-gray-600 bg-gray-700 text-blue-500 focus:ring-blue-500 focus:ring-offset-gray-900"
                />
                <span className="text-gray-200">{description}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Empty state */}
      {pendingChecklist.must_have.length === 0 && pendingChecklist.optional.length === 0 && (
        <div className="text-center py-8 text-gray-500">
          No checklist items
        </div>
      )}
    </div>
  );
}

// Display for accepted checklist (during/after execution)
export function ChecklistDisplay({ items, summary }: ChecklistDisplayProps) {
  return (
    <div className="space-y-4">
      {/* Summary bar */}
      {summary && (
        <div className="flex items-center gap-2 text-sm">
          <span className="text-gray-400">Progress:</span>
          <span className={summary.all_done ? 'text-green-400' : 'text-gray-300'}>
            {summary.done}/{summary.total} completed
          </span>
          {summary.failed > 0 && (
            <span className="text-red-400">({summary.failed} failed)</span>
          )}
        </div>
      )}

      {/* Flat list of items */}
      <div className="space-y-2">
        {items.map((item) => (
          <ChecklistItemRow key={item.id} item={item} />
        ))}
      </div>

      {/* Empty state */}
      {items.length === 0 && (
        <div className="text-center py-8 text-gray-500">
          No checklist items
        </div>
      )}
    </div>
  );
}

// Compact version for inline display
export function ChecklistSummaryBadge({ summary }: { summary: ChecklistSummary }) {
  const allDone = summary.all_done;
  const hasIssues = summary.failed > 0;

  return (
    <div
      className={`inline-flex items-center gap-1.5 px-2 py-1 rounded text-xs ${
        allDone
          ? 'bg-green-900/50 text-green-300'
          : hasIssues
          ? 'bg-red-900/50 text-red-300'
          : 'bg-gray-700 text-gray-300'
      }`}
    >
      {allDone ? <DoneIcon /> : hasIssues ? <FailedIcon /> : <PendingIcon />}
      <span>
        {summary.done}/{summary.total}
      </span>
    </div>
  );
}
