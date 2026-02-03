import { useState, useEffect, useCallback, useRef } from 'react';

interface ChecklistItem {
  id: string;
  text: string;
  isOptional?: boolean;
}

interface ProposedObjectiveProps {
  title: string;
  description: string;
  checklist: ChecklistItem[];
  status?: 'pending' | 'accepted' | 'rejected' | 'accepting' | 'rejecting';
  /** Called with the indices of selected optional items */
  onAccept?: (selectedOptionalIndices: number[]) => void;
  onReject?: () => void;
}

export function ProposedObjective({
  title,
  description,
  checklist,
  status = 'pending',
  onAccept,
  onReject,
}: ProposedObjectiveProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  // Track which optional items are selected (all selected by default)
  const optionalItems = checklist.filter((item) => item.isOptional);
  const [selectedOptional, setSelectedOptional] = useState<Set<number>>(
    () => new Set(optionalItems.map((_, i) => i))
  );

  // Reset selectedOptional when checklist changes (new draft loaded)
  const optionalIds = optionalItems.map((item) => item.id).join(',');
  useEffect(() => {
    setSelectedOptional(new Set(optionalItems.map((_, i) => i)));
  }, [optionalIds]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggleOptionalItem = useCallback((index: number) => {
    setSelectedOptional((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  }, []);

  const selectAllOptional = useCallback(() => {
    setSelectedOptional(new Set(optionalItems.map((_, i) => i)));
  }, [optionalItems]);

  const deselectAllOptional = useCallback(() => {
    setSelectedOptional(new Set());
  }, []);

  // Derive state from props - parent controls accepting/rejecting/accepted/rejected
  const isPending = status === 'pending';
  const isLoading = status === 'accepting' || status === 'rejecting';
  const isAccepted = status === 'accepted';
  const isRejected = status === 'rejected';

  const handleAccept = useCallback(() => {
    if (!onAccept || !isPending) return;
    // Parent will update status to 'accepting' - we just trigger the callback
    onAccept(Array.from(selectedOptional));
  }, [onAccept, isPending, selectedOptional]);

  const handleReject = useCallback(() => {
    if (!onReject || !isPending) return;
    // Parent will handle the rejection
    onReject();
  }, [onReject, isPending]);

  // Keyboard shortcuts: y to accept, n to reject
  // Only responds if this component has focus or is the first pending objective
  useEffect(() => {
    if (!isPending) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Ignore if user is typing
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }

      // Check if this component has focus or contains the focused element
      const hasFocus = containerRef.current?.contains(document.activeElement);

      // If no focus, only respond if this is the first pending objective in the DOM
      if (!hasFocus) {
        const allPendingObjectives = document.querySelectorAll('.v2-proposed:not(.v2-proposed--accepted):not(.v2-proposed--rejected)');
        const isFirst = allPendingObjectives.length > 0 && allPendingObjectives[0] === containerRef.current;
        if (!isFirst) {
          return;
        }
      }

      if (e.key === 'y' || e.key === 'Y') {
        e.preventDefault();
        handleAccept();
      } else if (e.key === 'n' || e.key === 'N') {
        e.preventDefault();
        handleReject();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isPending, handleAccept, handleReject]);

  const mustHaveItems = checklist.filter((item) => !item.isOptional);

  // Calculate summary for display
  const selectedCount = selectedOptional.size;
  const totalOptional = optionalItems.length;

  return (
    <div
      ref={containerRef}
      className={`v2-proposed ${isAccepted ? 'v2-proposed--accepted' : ''} ${isRejected ? 'v2-proposed--rejected' : ''} ${isLoading ? 'v2-proposed--loading' : ''}`}
      role="article"
      aria-label={`Proposed objective: ${title}`}
    >
      <div className="v2-proposed__label" aria-live="polite">
        {isPending && 'Proposed'}
        {status === 'accepting' && 'Accepting...'}
        {status === 'rejecting' && 'Rejecting...'}
        {isAccepted && '✓ Accepted'}
        {isRejected && '✗ Rejected'}
      </div>

      <h3 className="v2-proposed__title">{title}</h3>
      <hr className="v2-proposed__divider" />

      <p className="v2-proposed__description">{description}</p>

      {mustHaveItems.length > 0 && (
        <div className="v2-proposed__section">
          <div className="v2-proposed__section-label v2-proposed__section-label--required">Must have</div>
          <ul className="v2-proposed__checklist" role="list">
            {mustHaveItems.map((item) => (
              <li key={item.id} className="v2-proposed__checklist-item">
                <span className="v2-proposed__checkbox" aria-hidden="true">☐</span>
                <span>{item.text}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {optionalItems.length > 0 && (
        <div className="v2-proposed__section">
          <div className="v2-proposed__section-header">
            <div className="v2-proposed__section-label">Optional</div>
            {isPending && (
              <div className="v2-proposed__section-controls">
                <span className="v2-proposed__section-count">
                  {selectedCount}/{totalOptional} selected
                </span>
                {totalOptional > 1 && (
                  <div className="v2-proposed__bulk-actions">
                    <button
                      type="button"
                      className="v2-proposed__bulk-btn"
                      onClick={selectAllOptional}
                      disabled={selectedCount === totalOptional}
                      aria-label="Select all optional items"
                    >
                      All
                    </button>
                    <button
                      type="button"
                      className="v2-proposed__bulk-btn"
                      onClick={deselectAllOptional}
                      disabled={selectedCount === 0}
                      aria-label="Deselect all optional items"
                    >
                      None
                    </button>
                  </div>
                )}
              </div>
            )}
          </div>
          <ul className="v2-proposed__checklist v2-proposed__checklist--optional" role="list">
            {optionalItems.map((item, index) => {
              const isSelected = selectedOptional.has(index);
              return (
                <li key={item.id} className="v2-proposed__checklist-item">
                  {isPending ? (
                    <button
                      type="button"
                      className={`v2-proposed__toggle ${isSelected ? 'v2-proposed__toggle--selected' : ''}`}
                      onClick={() => toggleOptionalItem(index)}
                      aria-pressed={isSelected}
                      aria-label={`${isSelected ? 'Deselect' : 'Select'}: ${item.text}`}
                    >
                      <span className="v2-proposed__checkbox" aria-hidden="true">
                        {isSelected ? '☑' : '☐'}
                      </span>
                      <span className={isSelected ? '' : 'v2-proposed__text--deselected'}>{item.text}</span>
                    </button>
                  ) : (
                    <>
                      <span className="v2-proposed__checkbox" aria-hidden="true">
                        {isSelected ? '☑' : '☐'}
                      </span>
                      <span className={isSelected ? '' : 'v2-proposed__text--deselected'}>{item.text}</span>
                    </>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      )}

      {/* Show simple list if no distinction */}
      {mustHaveItems.length === 0 && optionalItems.length === 0 && checklist.length > 0 && (
        <ul className="v2-proposed__checklist" role="list">
          {checklist.map((item) => (
            <li key={item.id} className="v2-proposed__checklist-item">
              <span className="v2-proposed__checkbox" aria-hidden="true">☐</span>
              <span>{item.text}</span>
            </li>
          ))}
        </ul>
      )}

      {isPending && (
        <div className="v2-proposed__actions">
          <button
            type="button"
            className="v2-btn v2-btn--ghost"
            onClick={handleReject}
            disabled={isLoading}
            aria-label="Reject objective (press N)"
          >
            Reject
          </button>
          <button
            type="button"
            className="v2-btn v2-btn--primary"
            onClick={handleAccept}
            disabled={isLoading}
            aria-label="Accept objective (press Y)"
          >
            Accept
          </button>
        </div>
      )}

      {isPending && (
        <p className="v2-question__hint">
          Press Y to accept, N to reject
          {optionalItems.length > 0 && ' · Click optional items to toggle'}
        </p>
      )}
    </div>
  );
}
