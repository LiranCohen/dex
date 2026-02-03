import { formatTimeWithSeconds } from '../utils/formatters';
import type { Activity, ActivityEventType } from '../../lib/types';

// Re-export for convenience
export type { Activity };

interface ActivityLogProps {
  items: Activity[];
  summary?: {
    total_iterations: number;
    total_tokens: number;
  };
  isRunning?: boolean;
  emptyMessage?: string;
}

function getEventIcon(eventType: ActivityEventType) {
  switch (eventType) {
    case 'user_message':
      return '→';
    case 'assistant_response':
      return '←';
    case 'tool_call':
      return '⚙';
    case 'tool_result':
      return '✓';
    case 'completion_signal':
      return '●';
    case 'hat_transition':
      return '◆';
    case 'checklist_update':
      return '☐';
    case 'debug_log':
      return '#';
    default:
      return '·';
  }
}

function getEventLabel(eventType: ActivityEventType) {
  switch (eventType) {
    case 'user_message':
      return 'User';
    case 'assistant_response':
      return 'Response';
    case 'tool_call':
      return 'Tool';
    case 'tool_result':
      return 'Result';
    case 'completion_signal':
      return 'Complete';
    case 'hat_transition':
      return 'Phase';
    case 'checklist_update':
      return 'Checklist';
    case 'debug_log':
      return 'Debug';
    default:
      return eventType;
  }
}

function formatContent(activity: Activity): string {
  if (activity.content) {
    // Try to parse JSON content for tool calls
    if (activity.event_type === 'tool_call' || activity.event_type === 'tool_result') {
      try {
        const parsed = JSON.parse(activity.content);
        if (parsed.name) return parsed.name;
        if (parsed.tool) return parsed.tool;
      } catch {
        // Fall through to raw content
      }
    }
    // Truncate long content
    return activity.content.length > 100
      ? activity.content.substring(0, 100) + '...'
      : activity.content;
  }
  return getEventLabel(activity.event_type);
}

export function ActivityLog({ items, summary, isRunning, emptyMessage = '// no activity yet' }: ActivityLogProps) {
  if (items.length === 0) {
    return (
      <div className="v2-activity-empty">
        <p className="v2-empty-hint">{emptyMessage}</p>
        {isRunning && (
          <div className="v2-activity-running">
            <span className="v2-activity-running__dot"></span>
            <span>Task running...</span>
          </div>
        )}
      </div>
    );
  }

  // Filter out debug logs by default
  const visibleItems = items.filter(a => a.event_type !== 'debug_log');

  return (
    <div className="v2-activity-container">
      {/* Summary stats */}
      {summary && (summary.total_iterations > 0 || summary.total_tokens > 0) && (
        <div className="v2-activity-summary">
          {summary.total_iterations > 0 && (
            <span className="v2-label">Iterations: {summary.total_iterations}</span>
          )}
          {summary.total_tokens > 0 && (
            <span className="v2-label">Tokens: {summary.total_tokens.toLocaleString()}</span>
          )}
        </div>
      )}

      <div className="v2-card v2-activity-log">
        {visibleItems.map((item) => (
          <div key={item.id} className={`v2-activity-item v2-activity-item--${item.event_type}`}>
            <span className="v2-activity-item__icon">{getEventIcon(item.event_type)}</span>
            <span className="v2-activity-item__time">{formatTimeWithSeconds(item.created_at)}</span>
            <span className="v2-activity-item__content">{formatContent(item)}</span>
            {item.hat && <span className="v2-activity-item__hat">{item.hat}</span>}
          </div>
        ))}

        {/* Running indicator */}
        {isRunning && (
          <div className="v2-activity-item v2-activity-item--running">
            <span className="v2-activity-running__dot"></span>
            <span>Task running...</span>
          </div>
        )}
      </div>
    </div>
  );
}
