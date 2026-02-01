import { useState } from 'react';
import type { ObjectiveDraft } from '../lib/types';

interface ObjectiveDraftCardProps {
  draft: ObjectiveDraft;
  onAccept: (draft: ObjectiveDraft, selectedOptional: number[]) => Promise<void>;
  onReject: (draftId: string) => void;
  isAccepting?: boolean;
}

// Hat badge component
function HatBadge({ hat }: { hat: string }) {
  const colors: Record<string, string> = {
    architect: 'bg-purple-600',
    implementer: 'bg-blue-600',
    reviewer: 'bg-green-600',
  };

  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${colors[hat] || 'bg-gray-600'}`}>
      {hat}
    </span>
  );
}

export function ObjectiveDraftCard({ draft, onAccept, onReject, isAccepting }: ObjectiveDraftCardProps) {
  const [selectedOptional, setSelectedOptional] = useState<number[]>([]);
  const [isExpanded, setIsExpanded] = useState(true);

  const handleOptionalToggle = (index: number) => {
    setSelectedOptional((prev) =>
      prev.includes(index) ? prev.filter((i) => i !== index) : [...prev, index]
    );
  };

  const handleAccept = async () => {
    await onAccept(draft, selectedOptional);
  };

  return (
    <div className="bg-gray-800 border border-gray-700 rounded-lg overflow-hidden">
      {/* Header */}
      <div
        className="flex items-center justify-between p-3 cursor-pointer hover:bg-gray-750"
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-yellow-500">📋</span>
          <span className="font-medium truncate">{draft.title}</span>
          <HatBadge hat={draft.hat} />
        </div>
        <svg
          className={`w-5 h-5 text-gray-400 transition-transform ${isExpanded ? 'rotate-180' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </div>

      {/* Expanded Content */}
      {isExpanded && (
        <div className="border-t border-gray-700 p-3 space-y-3">
          {/* Description */}
          {draft.description && (
            <p className="text-sm text-gray-300">{draft.description}</p>
          )}

          {/* Checklist */}
          <div className="space-y-2">
            {/* Must-have items */}
            {draft.checklist.must_have.length > 0 && (
              <div>
                <h4 className="text-xs font-semibold text-gray-400 mb-1">Required</h4>
                <ul className="space-y-1">
                  {draft.checklist.must_have.map((item, idx) => (
                    <li key={idx} className="flex items-start gap-2 text-sm">
                      <span className="text-green-500 mt-0.5">✓</span>
                      <span className="text-gray-300">{item}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {/* Optional items */}
            {draft.checklist.optional && draft.checklist.optional.length > 0 && (
              <div>
                <h4 className="text-xs font-semibold text-gray-400 mb-1">Optional (select to include)</h4>
                <ul className="space-y-1">
                  {draft.checklist.optional.map((item, idx) => (
                    <li key={idx} className="flex items-start gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={selectedOptional.includes(idx)}
                        onChange={() => handleOptionalToggle(idx)}
                        className="mt-1 rounded border-gray-600 bg-gray-700 text-blue-500 focus:ring-blue-500 focus:ring-offset-gray-800"
                      />
                      <span className={selectedOptional.includes(idx) ? 'text-gray-300' : 'text-gray-500'}>
                        {item}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>

          {/* Dependencies */}
          {draft.blocked_by && draft.blocked_by.length > 0 && (
            <div className="text-xs text-yellow-500">
              Blocked by: {draft.blocked_by.join(', ')}
            </div>
          )}

          {/* Budget estimates and auto-start */}
          <div className="flex items-center justify-between text-xs">
            <span className={draft.auto_start ? 'text-green-500' : 'text-gray-500'}>
              {draft.auto_start ? '⚡ Auto-start enabled' : '⏸ Manual start'}
            </span>
            {(draft.estimated_iterations || draft.estimated_budget) && (
              <div className="flex items-center gap-3 text-gray-400">
                {draft.estimated_iterations && (
                  <span title="Estimated iterations">
                    ~{draft.estimated_iterations} iterations
                  </span>
                )}
                {draft.estimated_budget && (
                  <span title="Estimated cost" className="text-yellow-500">
                    ~${draft.estimated_budget.toFixed(2)}
                  </span>
                )}
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex gap-2 pt-2">
            <button
              onClick={handleAccept}
              disabled={isAccepting}
              className="flex-1 bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white text-sm font-medium py-2 px-3 rounded-lg transition-colors"
            >
              {isAccepting ? 'Creating...' : 'Accept Objective'}
            </button>
            <button
              onClick={() => onReject(draft.draft_id)}
              disabled={isAccepting}
              className="px-3 py-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded-lg transition-colors"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
