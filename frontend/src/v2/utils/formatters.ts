/**
 * Format a date string to time only (HH:MM format)
 */
export function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
}

/**
 * Format a date string to time with seconds (HH:MM:SS format)
 */
export function formatTimeWithSeconds(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
}

/**
 * Format a date string to relative time (e.g., "just now", "2m ago", "1h ago")
 */
export function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHours = Math.floor(diffMin / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSec < 10) {
    return 'just now';
  } else if (diffSec < 60) {
    return `${diffSec}s ago`;
  } else if (diffMin < 60) {
    return `${diffMin}m ago`;
  } else if (diffHours < 24) {
    return `${diffHours}h ago`;
  } else if (diffDays === 1) {
    return 'yesterday';
  } else if (diffDays < 7) {
    return `${diffDays}d ago`;
  } else {
    // For older dates, show the date
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  }
}

/**
 * Map task status to StatusBar status
 */
export function getTaskStatus(status: string): 'active' | 'pending' | 'complete' | 'error' {
  switch (status) {
    case 'running':
      return 'active';
    case 'completed':
      return 'complete';
    case 'failed':
    case 'cancelled':
      return 'error';
    default:
      return 'pending';
  }
}
