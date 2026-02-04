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
    <div className="app-empty-state" role="status">
      <p className="app-empty-state__message">[ {message} ]</p>
      {hint && <p className="app-empty-state__hint">{hint}</p>}
      {action && (
        <button
          type="button"
          className="app-btn app-btn--primary app-empty-state__action"
          onClick={action.onClick}
        >
          {action.label}
        </button>
      )}
    </div>
  );
}
