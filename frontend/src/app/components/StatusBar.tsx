type StatusType = 'active' | 'pending' | 'complete' | 'error';

interface StatusBarProps {
  status: StatusType;
  pulse?: boolean;
  className?: string;
}

const statusLabels: Record<StatusType, string> = {
  active: 'Active',
  pending: 'Pending',
  complete: 'Complete',
  error: 'Error',
};

export function StatusBar({ status, pulse = false, className = '' }: StatusBarProps) {
  const classes = [
    'app-status-bar',
    `app-status-bar--${status}`,
    pulse && status === 'active' ? 'app-status-bar--pulse' : '',
    className,
  ].filter(Boolean).join(' ');

  return (
    <div
      className={classes}
      role="status"
      aria-label={`Status: ${statusLabels[status]}`}
    />
  );
}
