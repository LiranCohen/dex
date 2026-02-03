interface ContextStatus {
  used_tokens: number;
  max_tokens: number;
  usage_percent: number;
  status: 'ok' | 'warning' | 'critical';
}

interface ContextUsageBarProps {
  context?: ContextStatus;
  className?: string;
}

function getStatusInfo(status: 'ok' | 'warning' | 'critical'): {
  icon: string;
  label: string;
  message?: string;
} {
  switch (status) {
    case 'critical':
      return {
        icon: '⚠',
        label: 'Critical',
        message: 'Context is nearly full. Task may need to summarize soon.',
      };
    case 'warning':
      return {
        icon: '📊',
        label: 'High',
        message: 'Context usage is elevated.',
      };
    default:
      return {
        icon: '📊',
        label: 'Context',
      };
  }
}

export function ContextUsageBar({ context, className = '' }: ContextUsageBarProps) {
  if (!context) {
    return null;
  }

  const { used_tokens, max_tokens, usage_percent, status } = context;
  const statusInfo = getStatusInfo(status);
  const remaining = max_tokens - used_tokens;
  const isCritical = status === 'critical';
  const showWarning = status === 'warning' || status === 'critical';

  // Format token count (e.g., 125000 -> "125k")
  const formatTokens = (tokens: number) => {
    if (tokens >= 1000) {
      return `${Math.round(tokens / 1000)}k`;
    }
    return tokens.toString();
  };

  return (
    <div
      className={`v2-context-bar v2-context-bar--${status} ${className}`}
      role={showWarning ? 'alert' : undefined}
      aria-live={showWarning ? 'polite' : undefined}
    >
      <div className="v2-context-bar__header">
        <div className="v2-context-bar__label">
          <span className={`v2-context-bar__icon ${isCritical ? 'v2-context-bar__icon--pulse' : ''}`} aria-hidden="true">
            {statusInfo.icon}
          </span>
          <span className="v2-context-bar__title">{statusInfo.label}</span>
          <span className="v2-context-bar__tokens">
            {formatTokens(used_tokens)} / {formatTokens(max_tokens)} tokens
          </span>
        </div>
        <span className={`v2-context-bar__percent v2-context-bar__percent--${status}`}>
          {usage_percent}%
        </span>
      </div>

      <div className="v2-context-bar__track">
        <div
          className={`v2-context-bar__fill v2-context-bar__fill--${status}`}
          style={{ width: `${Math.min(usage_percent, 100)}%` }}
          role="progressbar"
          aria-valuenow={usage_percent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={`Context usage: ${usage_percent}%`}
        />
      </div>

      {showWarning && statusInfo.message && (
        <div className="v2-context-bar__warning">
          <span className="v2-context-bar__warning-text">{statusInfo.message}</span>
          <span className="v2-context-bar__remaining">
            ~{formatTokens(remaining)} tokens remaining
          </span>
        </div>
      )}
    </div>
  );
}
