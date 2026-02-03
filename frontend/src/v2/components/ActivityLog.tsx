import { useState, useMemo, memo } from 'react';
import { formatTimeWithSeconds, formatRelativeTime } from '../utils/formatters';
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

// Grouped tool activity (call + result)
interface GroupedToolActivity {
  type: 'grouped_tool';
  id: string;
  toolName: string;
  toolCall: Activity;
  toolResult?: Activity;
  iteration: number;
  status: 'running' | 'complete' | 'error';
  created_at: string;
}

type DisplayItem = Activity | GroupedToolActivity;

function isGroupedTool(item: DisplayItem): item is GroupedToolActivity {
  return 'type' in item && item.type === 'grouped_tool';
}

// Parse tool name from activity content
function getToolName(activity: Activity): string | null {
  if (activity.content) {
    try {
      const parsed = JSON.parse(activity.content);
      if (parsed.name) return parsed.name;
      if (parsed.tool) return parsed.tool;
    } catch {
      // Fall through
    }
  }
  return null;
}

// Group consecutive tool_call and tool_result by tool name
function groupActivities(items: Activity[]): DisplayItem[] {
  const result: DisplayItem[] = [];
  const pendingToolCalls = new Map<string, Activity>(); // toolName -> tool_call activity

  for (const item of items) {
    if (item.event_type === 'tool_call') {
      const toolName = getToolName(item);
      if (toolName) {
        // Store this tool call, waiting for its result
        pendingToolCalls.set(toolName, item);
        // Add a grouped item for now (will be updated when result arrives)
        result.push({
          type: 'grouped_tool',
          id: `grouped-${item.id}`,
          toolName,
          toolCall: item,
          iteration: item.iteration,
          status: 'running',
          created_at: item.created_at,
        });
      } else {
        result.push(item);
      }
    } else if (item.event_type === 'tool_result') {
      const toolName = getToolName(item);
      if (toolName && pendingToolCalls.has(toolName)) {
        // Find the corresponding grouped item and update it
        const groupedIndex = result.findIndex(
          (r) => isGroupedTool(r) && r.toolName === toolName && !r.toolResult
        );
        if (groupedIndex !== -1) {
          const grouped = result[groupedIndex] as GroupedToolActivity;
          // Parse result to check for errors
          let isError = false;
          try {
            const parsed = JSON.parse(item.content || '{}');
            isError = parsed.result?.IsError === true;
          } catch {
            // Ignore parse errors
          }
          result[groupedIndex] = {
            ...grouped,
            toolResult: item,
            status: isError ? 'error' : 'complete',
          };
          pendingToolCalls.delete(toolName);
        } else {
          result.push(item);
        }
      } else {
        result.push(item);
      }
    } else {
      result.push(item);
    }
  }

  return result;
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

const GroupedToolItem = memo(function GroupedToolItem({ group }: { group: GroupedToolActivity }) {
  const [isExpanded, setIsExpanded] = useState(false);

  const statusIcon = group.status === 'running' ? '⚙' : group.status === 'error' ? '✗' : '✓';
  const statusClass = `v2-activity-item--tool_${group.status}`;

  // Memoize parsed tool input and output
  const { input, output, isError } = useMemo(() => {
    let parsedInput: Record<string, unknown> | null = null;
    let parsedOutput: string | null = null;
    let parsedIsError = false;

    try {
      const callParsed = JSON.parse(group.toolCall.content || '{}');
      parsedInput = callParsed.input;
    } catch {
      // Ignore
    }

    if (group.toolResult?.content) {
      try {
        const resultParsed = JSON.parse(group.toolResult.content);
        if (resultParsed.result?.Output) {
          parsedOutput = resultParsed.result.Output;
          parsedIsError = resultParsed.result.IsError === true;
        } else if (typeof resultParsed.result === 'string') {
          parsedOutput = resultParsed.result;
        }
      } catch {
        // Ignore
      }
    }

    return { input: parsedInput, output: parsedOutput, isError: parsedIsError };
  }, [group.toolCall.content, group.toolResult?.content]);

  return (
    <div className={`v2-activity-group ${statusClass} ${isExpanded ? 'v2-activity-group--expanded' : ''}`}>
      <button
        className="v2-activity-group__header"
        onClick={() => setIsExpanded(!isExpanded)}
        type="button"
      >
        <span className="v2-activity-item__icon">{statusIcon}</span>
        <span className="v2-activity-item__time" title={formatTimeWithSeconds(group.created_at)}>
          {formatRelativeTime(group.created_at)}
        </span>
        <span className="v2-activity-item__content">{group.toolName}</span>
        <span className={`v2-activity-group__status v2-activity-group__status--${group.status}`}>
          {group.status === 'running' ? 'running' : group.status === 'error' ? 'failed' : 'done'}
        </span>
        <span className="v2-activity-group__toggle">{isExpanded ? '▼' : '▶'}</span>
      </button>

      {isExpanded && (
        <div className="v2-activity-group__details">
          {input && (
            <div className="v2-activity-group__section">
              <span className="v2-activity-group__label">Input</span>
              <pre className="v2-activity-group__code">
                {JSON.stringify(input, null, 2)}
              </pre>
            </div>
          )}
          {output && (
            <div className="v2-activity-group__section">
              <span className={`v2-activity-group__label ${isError ? 'v2-activity-group__label--error' : ''}`}>
                {isError ? 'Error' : 'Output'}
              </span>
              <pre className={`v2-activity-group__code ${isError ? 'v2-activity-group__code--error' : ''}`}>
                {output.length > 500 ? output.substring(0, 500) + '...' : output}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
});

export const ActivityLog = memo(function ActivityLog({ items, summary, isRunning, emptyMessage = '// no activity yet' }: ActivityLogProps) {
  // Group tool activities
  const groupedItems = useMemo(() => {
    // Filter out debug logs first
    const visibleItems = items.filter(a => a.event_type !== 'debug_log');
    return groupActivities(visibleItems);
  }, [items]);

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
        {groupedItems.map((item) =>
          isGroupedTool(item) ? (
            <GroupedToolItem key={item.id} group={item} />
          ) : (
            <div key={item.id} className={`v2-activity-item v2-activity-item--${item.event_type}`}>
              <span className="v2-activity-item__icon">{getEventIcon(item.event_type)}</span>
              <span className="v2-activity-item__time" title={formatTimeWithSeconds(item.created_at)}>
                {formatRelativeTime(item.created_at)}
              </span>
              <span className="v2-activity-item__content">{formatContent(item)}</span>
              {item.hat && <span className="v2-activity-item__hat">{item.hat}</span>}
            </div>
          )
        )}

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
});
