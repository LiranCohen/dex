interface EmptyStateProps {
  message?: string;
  hint?: string;
  action?: {
    label: string;
    onClick: () => void;
  };
}

export function EmptyState({
  message = 'Nothing here yet',
  hint,
  action,
}: EmptyStateProps) {
  return (
    <div className="v2-empty-state" role="status">
      <p className="v2-empty-state__message">[ {message} ]</p>
      {hint && <p className="v2-empty-state__hint">{hint}</p>}
      {action && (
        <button
          type="button"
          className="v2-btn v2-btn--primary v2-empty-state__action"
          onClick={action.onClick}
        >
          {action.label}
        </button>
      )}
    </div>
  );
}
