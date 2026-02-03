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
    'v2-status-bar',
    `v2-status-bar--${status}`,
    pulse && status === 'active' ? 'v2-status-bar--pulse' : '',
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
