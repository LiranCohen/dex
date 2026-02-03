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

export function ContextUsageBar({ context, className = '' }: ContextUsageBarProps) {
  if (!context) {
    return null;
  }

  const { used_tokens, max_tokens, usage_percent, status } = context;

  // Format token count (e.g., 125000 -> "125k")
  const formatTokens = (tokens: number) => {
    if (tokens >= 1000) {
      return `${Math.round(tokens / 1000)}k`;
    }
    return tokens.toString();
  };

  return (
    <div className={`v2-context-bar ${className}`}>
      <div className="v2-context-bar__label">
        <span className="v2-context-bar__title">Context</span>
        <span className="v2-context-bar__tokens">
          {formatTokens(used_tokens)} / {formatTokens(max_tokens)}
        </span>
      </div>
      <div className="v2-context-bar__track">
        <div
          className={`v2-context-bar__fill v2-context-bar__fill--${status}`}
          style={{ width: `${Math.min(usage_percent, 100)}%` }}
        />
      </div>
      <span className={`v2-context-bar__percent v2-context-bar__percent--${status}`}>
        {usage_percent}%
      </span>
    </div>
  );
}
