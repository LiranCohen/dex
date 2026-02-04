interface SkeletonProps {
  variant?: 'text' | 'title' | 'card' | 'avatar' | 'button';
  width?: string;
  height?: string;
  className?: string;
}

export function Skeleton({ variant = 'text', width, height, className = '' }: SkeletonProps) {
  const classes = [
    'app-skeleton',
    `app-skeleton--${variant}`,
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
    <div className="app-card app-skeleton-card" aria-hidden="true">
      <Skeleton variant="text" width="60%" />
      <Skeleton variant="text" width="80%" />
      <Skeleton variant="text" width="40%" />
    </div>
  );
}

export function SkeletonList({ count = 3 }: { count?: number }) {
  return (
    <div className="app-skeleton-list" aria-label="Loading..." role="status">
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} />
      ))}
    </div>
  );
}

export function SkeletonMessage() {
  return (
    <div className="app-message app-skeleton-message" aria-hidden="true">
      <div className="app-message__header">
        <Skeleton variant="text" width="60px" />
        <Skeleton variant="text" width="40px" />
      </div>
      <div className="app-message__content">
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
    <div className={`app-loading-state app-loading-state--${size}`} role="status" aria-live="polite">
      <div className="app-loading-state__spinner" aria-hidden="true">
        <span className="app-loading-state__dot" style={{ animationDelay: '0ms' }} />
        <span className="app-loading-state__dot" style={{ animationDelay: '150ms' }} />
        <span className="app-loading-state__dot" style={{ animationDelay: '300ms' }} />
      </div>
      <span className="app-loading-state__message">{message}</span>
    </div>
  );
}
