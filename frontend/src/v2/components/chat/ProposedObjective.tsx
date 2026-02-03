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
  onAccept?: () => void;
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
  const [localStatus, setLocalStatus] = useState(status);
  const containerRef = useRef<HTMLDivElement>(null);

  const handleAccept = useCallback(async () => {
    if (!onAccept || localStatus !== 'pending') return;
    setLocalStatus('accepting');
    try {
      await onAccept();
      setLocalStatus('accepted');
    } catch {
      setLocalStatus('pending');
    }
  }, [onAccept, localStatus]);

  const handleReject = useCallback(async () => {
    if (!onReject || localStatus !== 'pending') return;
    setLocalStatus('rejecting');
    try {
      await onReject();
      setLocalStatus('rejected');
    } catch {
      setLocalStatus('pending');
    }
  }, [onReject, localStatus]);

  // Keyboard shortcuts: y to accept, n to reject
  useEffect(() => {
    if (localStatus !== 'pending') return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Only handle if this proposal is in view or focused
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
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
  }, [localStatus, handleAccept, handleReject]);

  const isPending = localStatus === 'pending';
  const isLoading = localStatus === 'accepting' || localStatus === 'rejecting';
  const isAccepted = localStatus === 'accepted';
  const isRejected = localStatus === 'rejected';

  const mustHaveItems = checklist.filter((item) => !item.isOptional);
  const optionalItems = checklist.filter((item) => item.isOptional);

  return (
    <div
      ref={containerRef}
      className={`v2-proposed ${isAccepted ? 'v2-proposed--accepted' : ''} ${isRejected ? 'v2-proposed--rejected' : ''} ${isLoading ? 'v2-proposed--loading' : ''}`}
      role="article"
      aria-label={`Proposed objective: ${title}`}
    >
      <div className="v2-proposed__label" aria-live="polite">
        {isPending && 'Proposed'}
        {localStatus === 'accepting' && 'Accepting...'}
        {localStatus === 'rejecting' && 'Rejecting...'}
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
          <div className="v2-proposed__section-label">Optional</div>
          <ul className="v2-proposed__checklist v2-proposed__checklist--optional" role="list">
            {optionalItems.map((item) => (
              <li key={item.id} className="v2-proposed__checklist-item">
                <span className="v2-proposed__checkbox" aria-hidden="true">☐</span>
                <span>{item.text}</span>
              </li>
            ))}
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
        </p>
      )}
    </div>
  );
}
