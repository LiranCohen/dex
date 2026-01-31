import { useState, useEffect, useRef, useCallback } from 'react';
import { fetchTaskActivity } from '../lib/api';
import type { Activity, WebSocketEvent, SessionEvent } from '../lib/types';
import { useWebSocket } from '../hooks/useWebSocket';
import { ActivityItem } from './ActivityItem';

interface ActivityFeedProps {
  taskId: string;
  isRunning: boolean;
}

export function ActivityFeed({ taskId, isRunning }: ActivityFeedProps) {
  const [activities, setActivities] = useState<Activity[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [summary, setSummary] = useState<{
    total_iterations: number;
    total_tokens: number;
  } | null>(null);

  const scrollRef = useRef<HTMLDivElement>(null);
  const { subscribe } = useWebSocket();

  // Fetch activities from API
  const loadActivity = useCallback(async () => {
    try {
      const data = await fetchTaskActivity(taskId);
      setActivities(data.activity || []);
      setSummary(data.summary);
      setError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load activity';
      setError(message);
    }
  }, [taskId]);

  // Initial load
  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      await loadActivity();
      setLoading(false);
    };
    fetchData();
  }, [loadActivity]);

  // Subscribe to WebSocket for session and activity events
  const handleWebSocketEvent = useCallback(
    (event: WebSocketEvent) => {
      // Handle real-time activity events
      if (event.type === 'activity.new') {
        const payload = event.payload as { task_id: string; activity: Activity };
        if (payload.task_id === taskId && payload.activity) {
          setActivities((prev) => [...prev, payload.activity]);
        }
        return;
      }

      // Handle session events to trigger refetch (for backwards compatibility)
      if (event.type.startsWith('session.')) {
        const sessionEvent = event as SessionEvent;
        if (sessionEvent.payload.task_id === taskId) {
          // Refetch activity on session events for this task
          loadActivity();
        }
      }
    },
    [taskId, loadActivity]
  );

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Auto-scroll to bottom when new activities arrive while running
  useEffect(() => {
    if (isRunning && scrollRef.current) {
      scrollRef.current.scrollTo({
        top: scrollRef.current.scrollHeight,
        behavior: 'smooth',
      });
    }
  }, [activities, isRunning]);

  if (loading) {
    return (
      <div className="space-y-3">
        {/* Skeleton loading */}
        {[1, 2, 3].map((i) => (
          <div key={i} className="animate-pulse">
            <div className="h-16 bg-gray-700 rounded-lg"></div>
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-900/30 border border-red-600 rounded-lg p-4">
        <p className="text-red-400 text-sm">{error}</p>
        <button
          onClick={() => loadActivity()}
          className="text-sm text-red-300 hover:text-red-200 mt-2 underline"
        >
          Retry
        </button>
      </div>
    );
  }

  if (activities.length === 0) {
    return (
      <div className="text-center py-8">
        <svg
          className="w-10 h-10 mx-auto text-gray-600 mb-3"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"
          />
        </svg>
        <p className="text-gray-500 text-sm">No activity yet</p>
        <p className="text-gray-600 text-xs mt-1">
          Activity will appear here when the task runs
        </p>
      </div>
    );
  }

  // Group activities by iteration for visual separation
  const groupedActivities: { iteration: number; items: Activity[] }[] = [];
  let currentIteration = -1;

  for (const activity of activities) {
    if (activity.iteration !== currentIteration) {
      currentIteration = activity.iteration;
      groupedActivities.push({ iteration: currentIteration, items: [] });
    }
    groupedActivities[groupedActivities.length - 1].items.push(activity);
  }

  return (
    <div className="space-y-4">
      {/* Summary stats */}
      {summary && (summary.total_iterations > 0 || summary.total_tokens > 0) && (
        <div className="flex gap-4 text-xs text-gray-500 pb-2 border-b border-gray-700">
          {summary.total_iterations > 0 && (
            <span>Iterations: {summary.total_iterations}</span>
          )}
          {summary.total_tokens > 0 && (
            <span>Tokens: {summary.total_tokens.toLocaleString()}</span>
          )}
        </div>
      )}

      {/* Activity list */}
      <div
        ref={scrollRef}
        className="space-y-3 max-h-[500px] overflow-y-auto pr-1"
      >
        {groupedActivities.map((group, groupIndex) => (
          <div key={`group-${group.iteration}-${groupIndex}`}>
            {/* Iteration separator (only show if not first iteration 0) */}
            {group.iteration > 0 && groupIndex > 0 && (
              <div className="flex items-center gap-2 py-2">
                <div className="flex-1 border-t border-gray-700"></div>
                <span className="text-xs text-gray-500 font-medium">
                  Iteration {group.iteration}
                </span>
                <div className="flex-1 border-t border-gray-700"></div>
              </div>
            )}

            {/* Activity items in this iteration */}
            <div className="space-y-2">
              {group.items.map((activity) => (
                <ActivityItem key={activity.id} activity={activity} />
              ))}
            </div>
          </div>
        ))}

        {/* Running indicator */}
        {isRunning && (
          <div className="flex items-center gap-2 py-2 text-green-400">
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
            <span className="text-sm">Task running...</span>
          </div>
        )}
      </div>
    </div>
  );
}
