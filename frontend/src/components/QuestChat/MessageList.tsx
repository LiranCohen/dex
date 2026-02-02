import { useRef, useEffect, useCallback, memo } from 'react';
import { MessageBubble } from './MessageBubble';
import type { QuestMessage } from '../../lib/types';

interface MessageListProps {
  messages: QuestMessage[];
  isStreaming: boolean;
  onScrollToBottom?: () => void;
}

export const MessageList = memo(function MessageList({
  messages,
  isStreaming,
  onScrollToBottom,
}: MessageListProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const isAutoScrollingRef = useRef(true);

  // Check if we're near the bottom
  const isNearBottom = useCallback(() => {
    const container = containerRef.current;
    if (!container) return true;

    const threshold = 100;
    const position = container.scrollTop + container.clientHeight;
    return position >= container.scrollHeight - threshold;
  }, []);

  // Scroll to bottom
  const scrollToBottom = useCallback((smooth = true) => {
    bottomRef.current?.scrollIntoView({
      behavior: smooth ? 'smooth' : 'auto',
      block: 'end',
    });
    onScrollToBottom?.();
  }, [onScrollToBottom]);

  // Auto-scroll when new messages arrive (if already at bottom)
  useEffect(() => {
    if (isAutoScrollingRef.current || isNearBottom()) {
      scrollToBottom(true);
    }
  }, [messages, isStreaming, scrollToBottom, isNearBottom]);

  // Handle scroll events
  const handleScroll = useCallback(() => {
    isAutoScrollingRef.current = isNearBottom();
  }, [isNearBottom]);

  // Determine if we should show timestamp for consecutive messages
  const shouldShowTimestamp = (msg: QuestMessage, prevMsg: QuestMessage | null): boolean => {
    if (!prevMsg) return true;
    if (msg.role !== prevMsg.role) return true;

    // Show timestamp if more than 2 minutes apart
    const curr = new Date(msg.created_at).getTime();
    const prev = new Date(prevMsg.created_at).getTime();
    return curr - prev > 2 * 60 * 1000;
  };

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="flex-1 overflow-y-auto px-4 py-4 space-y-2"
    >
      {messages.length === 0 ? (
        <EmptyState />
      ) : (
        <>
          {messages.map((msg, idx) => (
            <MessageBubble
              key={msg.id}
              message={msg}
              isStreaming={isStreaming && idx === messages.length - 1 && msg.role === 'assistant'}
              showTimestamp={shouldShowTimestamp(msg, idx > 0 ? messages[idx - 1] : null)}
            />
          ))}

          {/* Streaming indicator for when assistant is thinking but no content yet */}
          {isStreaming && messages.length > 0 && messages[messages.length - 1].role === 'user' && (
            <div className="flex items-start mb-4">
              <div className="bg-gray-800 border border-gray-700 rounded-lg px-4 py-3">
                <div className="flex items-center gap-2 mb-2 text-xs text-gray-400">
                  <span className="text-purple-400">Dex</span>
                  <span className="flex items-center gap-1">
                    <span className="w-1.5 h-1.5 bg-blue-400 rounded-full animate-pulse" />
                    <span>Thinking...</span>
                  </span>
                </div>
                <div className="flex items-center gap-2 text-gray-400">
                  <span className="w-2 h-2 bg-gray-500 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                  <span className="w-2 h-2 bg-gray-500 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                  <span className="w-2 h-2 bg-gray-500 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
                </div>
              </div>
            </div>
          )}
        </>
      )}

      {/* Scroll anchor */}
      <div ref={bottomRef} />

      {/* Scroll to bottom button (visible when not at bottom) */}
      {!isAutoScrollingRef.current && messages.length > 5 && (
        <button
          onClick={() => scrollToBottom(true)}
          className="fixed bottom-24 right-8 bg-gray-700 hover:bg-gray-600 text-white rounded-full p-2 shadow-lg transition-colors z-10"
          title="Scroll to bottom"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 14l-7 7m0 0l-7-7m7 7V3" />
          </svg>
        </button>
      )}
    </div>
  );
});

// Empty state when no messages
function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center h-full text-center px-4">
      <div className="text-4xl mb-4">🧭</div>
      <h3 className="text-lg font-medium text-gray-300 mb-2">Start a conversation</h3>
      <p className="text-gray-500 max-w-md">
        Describe what you want to accomplish. Dex will help you break it down into actionable objectives.
      </p>

      <div className="mt-6 space-y-3 text-left text-sm text-gray-400">
        <p className="flex items-start gap-2">
          <span className="text-blue-400">💡</span>
          <span>Be specific about your goals and requirements</span>
        </p>
        <p className="flex items-start gap-2">
          <span className="text-blue-400">💡</span>
          <span>Dex can search files, explore code, and research the web</span>
        </p>
        <p className="flex items-start gap-2">
          <span className="text-blue-400">💡</span>
          <span>You'll get objective proposals to review before work begins</span>
        </p>
      </div>
    </div>
  );
}
