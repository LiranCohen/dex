import { useState } from 'react';
import type { ChecklistItem, ChecklistSummary, Task } from '../lib/types';
import { ChecklistDisplay } from './ChecklistDisplay';
import { createRemediation } from '../lib/api';

interface TaskCompletionPanelProps {
  task: Task;
  checklistItems: ChecklistItem[];
  checklistSummary: ChecklistSummary;
  onRemediationCreated?: (newTask: Task) => void;
  onMarkComplete?: () => void;
}

function WarningIcon() {
  return (
    <svg className="w-6 h-6 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
    </svg>
  );
}

function SuccessIcon() {
  return (
    <svg className="w-6 h-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

export function TaskCompletionPanel({
  task,
  checklistItems,
  checklistSummary,
  onRemediationCreated,
  onMarkComplete,
}: TaskCompletionPanelProps) {
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const hasIssues = task.Status === 'completed_with_issues' || !checklistSummary.all_done;
  const failedItems = checklistItems.filter(
    (item) => item.status === 'failed' || item.status === 'pending'
  );

  const handleCreateRemediation = async () => {
    setCreating(true);
    setError(null);
    try {
      const result = await createRemediation(task.ID);
      onRemediationCreated?.(result.task);
    } catch (err) {
      let message = 'Failed to create remediation task';
      if (err instanceof Error) {
        message = err.message;
      } else if (err && typeof err === 'object' && 'message' in err) {
        const apiErr = err as { message: unknown };
        message = typeof apiErr.message === 'string' ? apiErr.message : 'Failed to create remediation task';
      }
      setError(message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div
      className={`rounded-lg border overflow-hidden ${
        hasIssues
          ? 'bg-amber-900/20 border-amber-600/30'
          : 'bg-green-900/20 border-green-600/30'
      }`}
    >
      {/* Header */}
      <div
        className={`px-4 py-3 border-b ${
          hasIssues
            ? 'bg-amber-900/40 border-amber-600/30'
            : 'bg-green-900/40 border-green-600/30'
        }`}
      >
        <div className="flex items-center gap-3">
          {hasIssues ? <WarningIcon /> : <SuccessIcon />}
          <div>
            <h3
              className={`font-medium ${
                hasIssues ? 'text-amber-200' : 'text-green-200'
              }`}
            >
              {hasIssues ? 'Task Completed with Issues' : 'Task Completed Successfully'}
            </h3>
            {hasIssues && (
              <p className="text-sm text-amber-300/80">
                {failedItems.length} item{failedItems.length !== 1 ? 's' : ''} failed or incomplete
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Checklist status */}
      <div className="p-4">
        <h4 className="text-sm font-medium text-gray-400 mb-3">Checklist Status</h4>
        <ChecklistDisplay
          items={checklistItems}
          summary={checklistSummary}
        />
      </div>

      {/* Failed items detail */}
      {hasIssues && failedItems.length > 0 && (
        <div className="px-4 pb-4">
          <h4 className="text-sm font-medium text-amber-400 mb-2">Issues</h4>
          <div className="space-y-2">
            {failedItems.map((item) => (
              <div
                key={item.id}
                className="bg-amber-900/20 border border-amber-700/30 rounded-lg p-3"
              >
                <div className="flex items-start gap-2">
                  <svg
                    className="w-4 h-4 text-red-400 mt-0.5 flex-shrink-0"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <div>
                    <p className="text-sm text-gray-200">{item.description}</p>
                    {item.verification_notes && (
                      <p className="text-xs text-gray-400 mt-1">
                        {item.verification_notes}
                      </p>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Error display */}
      {error && (
        <div className="mx-4 mb-4 p-3 bg-red-900/30 border border-red-600 rounded-lg">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Action buttons */}
      <div className="p-4 border-t border-gray-700 flex items-center justify-between gap-3">
        {hasIssues ? (
          <>
            <button
              onClick={onMarkComplete}
              className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-lg text-sm font-medium transition-colors"
            >
              Mark as Complete
            </button>
            <button
              onClick={handleCreateRemediation}
              disabled={creating}
              className="px-4 py-2 bg-amber-600 hover:bg-amber-700 disabled:bg-amber-800 disabled:text-amber-400 text-white rounded-lg text-sm font-medium transition-colors"
            >
              {creating ? 'Creating...' : 'Create Remediation Task'}
            </button>
          </>
        ) : (
          <div className="flex items-center gap-2 text-green-400">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
            <span className="text-sm font-medium">All items completed successfully</span>
          </div>
        )}
      </div>
    </div>
  );
}
