import { useState, useEffect, useRef, useCallback } from 'react';
import { fetchPlanning, sendPlanningResponse, acceptPlan, skipPlanning } from '../lib/api';
import type { PlanningMessage, PlanningSession, WebSocketEvent } from '../lib/types';
import { useWebSocket } from '../hooks/useWebSocket';

interface PlanningPanelProps {
  taskId: string;
  onPlanAccepted?: () => void;
  onPlanSkipped?: () => void;
}

export function PlanningPanel({ taskId, onPlanAccepted, onPlanSkipped }: PlanningPanelProps) {
  const [session, setSession] = useState<PlanningSession | null>(null);
  const [messages, setMessages] = useState<PlanningMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [response, setResponse] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [accepting, setAccepting] = useState(false);
  const [skipping, setSkipping] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const { subscribe } = useWebSocket();

  // Load planning data
  const loadPlanning = useCallback(async () => {
    try {
      const data = await fetchPlanning(taskId);
      setSession(data.session);
      setMessages(data.messages);
      setError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load planning';
      // Don't show error if no planning session exists
      if (message.includes('not found') || message.includes('404')) {
        setSession(null);
        setMessages([]);
      } else {
        setError(message);
      }
    }
  }, [taskId]);

  // Initial load
  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      await loadPlanning();
      setLoading(false);
    };
    fetchData();
  }, [loadPlanning]);

  // Subscribe to WebSocket for planning events
  const handleWebSocketEvent = useCallback(
    (event: WebSocketEvent) => {
      if (event.type.startsWith('planning.')) {
        const payload = event.payload as { task_id: string };
        if (payload.task_id === taskId) {
          loadPlanning();
        }
      }
    },
    [taskId, loadPlanning]
  );

  useEffect(() => {
    const unsubscribe = subscribe(handleWebSocketEvent);
    return unsubscribe;
  }, [subscribe, handleWebSocketEvent]);

  // Scroll to bottom when messages change
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Handle sending a response
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!response.trim() || submitting) return;

    setSubmitting(true);
    try {
      const data = await sendPlanningResponse(taskId, response.trim());
      setSession(data.session);
      setMessages(data.messages);
      setResponse('');
      setError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to send response';
      setError(message);
    } finally {
      setSubmitting(false);
    }
  };

  // Handle accepting the plan
  const handleAccept = async () => {
    setAccepting(true);
    try {
      await acceptPlan(taskId);
      onPlanAccepted?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to accept plan';
      setError(message);
    } finally {
      setAccepting(false);
    }
  };

  // Handle skipping planning
  const handleSkip = async () => {
    setSkipping(true);
    try {
      await skipPlanning(taskId);
      onPlanSkipped?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to skip planning';
      setError(message);
    } finally {
      setSkipping(false);
    }
  };

  if (loading) {
    return (
      <div className="bg-purple-900/20 border border-purple-600/30 rounded-lg p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-4 bg-purple-800/30 rounded w-3/4"></div>
          <div className="h-4 bg-purple-800/30 rounded w-1/2"></div>
          <div className="h-20 bg-purple-800/30 rounded"></div>
        </div>
      </div>
    );
  }

  if (!session) {
    return null;
  }

  const isCompleted = session.status === 'completed';
  const isProcessing = session.status === 'processing';
  const isAwaitingResponse = session.status === 'awaiting_response';

  return (
    <div className="bg-purple-900/20 border border-purple-600/30 rounded-lg overflow-hidden">
      {/* Header */}
      <div className="bg-purple-900/40 px-4 py-3 border-b border-purple-600/30">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <svg
              className="w-5 h-5 text-purple-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"
              />
            </svg>
            <h3 className="text-purple-200 font-medium">Planning Phase</h3>
          </div>
          <div className="flex items-center gap-2">
            {isProcessing && (
              <span className="flex items-center gap-1.5 text-xs text-purple-400">
                <div className="w-2 h-2 bg-purple-500 rounded-full animate-pulse"></div>
                Analyzing...
              </span>
            )}
            {isAwaitingResponse && (
              <span className="text-xs text-purple-400">Awaiting your response</span>
            )}
            {isCompleted && (
              <span className="flex items-center gap-1.5 text-xs text-green-400">
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
                Plan confirmed
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="p-4 max-h-[400px] overflow-y-auto space-y-4">
        {messages.map((msg) => (
          <div
            key={msg.id}
            className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            <div
              className={`max-w-[80%] rounded-lg px-4 py-3 ${
                msg.role === 'user'
                  ? 'bg-purple-600/40 text-purple-100'
                  : 'bg-gray-700/50 text-gray-200'
              }`}
            >
              <div className="text-xs text-gray-400 mb-1">
                {msg.role === 'user' ? 'You' : 'Planning Assistant'}
              </div>
              <div className="whitespace-pre-wrap text-sm">{msg.content}</div>
            </div>
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>

      {/* Error display */}
      {error && (
        <div className="mx-4 mb-4 p-3 bg-red-900/30 border border-red-600 rounded-lg">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Response input (only show when awaiting response) */}
      {isAwaitingResponse && (
        <form onSubmit={handleSubmit} className="p-4 border-t border-purple-600/30">
          <textarea
            value={response}
            onChange={(e) => setResponse(e.target.value)}
            placeholder="Type your response..."
            className="w-full bg-gray-800 border border-purple-600/30 rounded-lg px-4 py-3 text-gray-200 placeholder-gray-500 focus:outline-none focus:border-purple-500 resize-none"
            rows={3}
            disabled={submitting}
          />
          <div className="flex justify-end mt-3">
            <button
              type="submit"
              disabled={!response.trim() || submitting}
              className="px-4 py-2 bg-purple-600 hover:bg-purple-700 disabled:bg-purple-800 disabled:text-purple-400 text-white rounded-lg text-sm font-medium transition-colors"
            >
              {submitting ? 'Sending...' : 'Send Response'}
            </button>
          </div>
        </form>
      )}

      {/* Action buttons (show when completed or can skip) */}
      <div className="p-4 border-t border-purple-600/30 flex items-center justify-between gap-3">
        <button
          onClick={handleSkip}
          disabled={skipping || accepting}
          className="px-4 py-2 bg-gray-700 hover:bg-gray-600 disabled:bg-gray-800 disabled:text-gray-500 text-gray-200 rounded-lg text-sm font-medium transition-colors"
        >
          {skipping ? 'Skipping...' : 'Skip Planning'}
        </button>
        {(isCompleted || isAwaitingResponse) && (
          <button
            onClick={handleAccept}
            disabled={accepting || skipping}
            className="px-4 py-2 bg-green-600 hover:bg-green-700 disabled:bg-green-800 disabled:text-green-400 text-white rounded-lg text-sm font-medium transition-colors"
          >
            {accepting ? 'Accepting...' : 'Accept & Continue'}
          </button>
        )}
      </div>
    </div>
  );
}
