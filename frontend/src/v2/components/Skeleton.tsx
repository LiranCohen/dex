interface SkeletonProps {
  variant?: 'text' | 'title' | 'card' | 'avatar' | 'button';
  width?: string;
  height?: string;
  className?: string;
}

export function Skeleton({ variant = 'text', width, height, className = '' }: SkeletonProps) {
  const classes = [
    'v2-skeleton',
    `v2-skeleton--${variant}`,
    className,
  ].filter(Boolean).join(' ');

  return (
    <div
      className={classes}
      style={{ width, height }}
      aria-hidden="true"
    />
  );
}

// Pre-built skeleton patterns
export function SkeletonCard() {
  return (
    <div className="v2-card v2-skeleton-card" aria-hidden="true">
      <Skeleton variant="text" width="60%" />
      <Skeleton variant="text" width="80%" />
      <Skeleton variant="text" width="40%" />
    </div>
  );
}

export function SkeletonList({ count = 3 }: { count?: number }) {
  return (
    <div className="v2-skeleton-list" aria-label="Loading..." role="status">
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} />
      ))}
    </div>
  );
}

export function SkeletonMessage() {
  return (
    <div className="v2-message v2-skeleton-message" aria-hidden="true">
      <div className="v2-message__header">
        <Skeleton variant="text" width="60px" />
        <Skeleton variant="text" width="40px" />
      </div>
      <div className="v2-message__content">
        <Skeleton variant="text" width="100%" />
        <Skeleton variant="text" width="90%" />
        <Skeleton variant="text" width="70%" />
      </div>
    </div>
  );
}

interface LoadingStateProps {
  message?: string;
  size?: 'small' | 'medium' | 'large';
}

export function LoadingState({ message = 'Loading...', size = 'medium' }: LoadingStateProps) {
  return (
    <div className={`v2-loading-state v2-loading-state--${size}`} role="status" aria-live="polite">
      <div className="v2-loading-state__spinner" aria-hidden="true">
        <span className="v2-loading-state__dot" style={{ animationDelay: '0ms' }} />
        <span className="v2-loading-state__dot" style={{ animationDelay: '150ms' }} />
        <span className="v2-loading-state__dot" style={{ animationDelay: '300ms' }} />
      </div>
      <span className="v2-loading-state__message">{message}</span>
    </div>
  );
}
