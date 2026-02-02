import { memo, useState, useCallback } from 'react';
import { MarkdownContent } from './MarkdownContent';
import { ToolActivity } from './ToolActivity';
import { stripSignals } from './utils';
import type { QuestMessage, QuestToolCall } from '../../lib/types';

interface MessageBubbleProps {
  message: QuestMessage;
  isStreaming?: boolean;
  showTimestamp?: boolean;
  onRetry?: () => void;
}

// Format timestamp for display
function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export const MessageBubble = memo(function MessageBubble({
  message,
  isStreaming = false,
  showTimestamp = true,
  onRetry,
}: MessageBubbleProps) {
  const [copied, setCopied] = useState(false);
  const [isHovering, setIsHovering] = useState(false);

  const handleCopy = useCallback(async () => {
    if (isStreaming) return;

    try {
      await navigator.clipboard.writeText(message.content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }, [message.content, isStreaming]);

  const isUser = message.role === 'user';
  const displayContent = isUser ? message.content : stripSignals(message.content);
  const toolCalls = message.tool_calls || [];

  return (
    <div
      className={`flex flex-col ${isUser ? 'items-end' : 'items-start'} mb-4`}
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => setIsHovering(false)}
    >
      {/* Message bubble */}
      <div
        className={`
          relative max-w-[85%] rounded-lg px-4 py-3
          ${isUser
            ? 'bg-blue-600 text-white'
            : 'bg-gray-800 text-gray-100 border border-gray-700'
          }
        `}
      >
        {/* Role indicator for assistant */}
        {!isUser && (
          <div className="flex items-center gap-2 mb-2 text-xs text-gray-400">
            <span className="text-purple-400">Dex</span>
            {isStreaming && (
              <span className="flex items-center gap-1">
                <span className="w-1.5 h-1.5 bg-blue-400 rounded-full animate-pulse" />
                <span>Thinking...</span>
              </span>
            )}
          </div>
        )}

        {/* Content */}
        {displayContent ? (
          isUser ? (
            <p className="whitespace-pre-wrap break-words">{displayContent}</p>
          ) : (
            <MarkdownContent content={displayContent} isStreaming={isStreaming} />
          )
        ) : isStreaming ? (
          <div className="flex items-center gap-2 text-gray-400">
            <span className="w-2 h-2 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
            <span className="w-2 h-2 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
            <span className="w-2 h-2 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
          </div>
        ) : null}

        {/* Tool calls for assistant messages */}
        {!isUser && toolCalls.length > 0 && (
          <div className="mt-3 space-y-2">
            {toolCalls.map((tc: QuestToolCall, idx: number) => (
              <ToolActivity
                key={`${tc.tool_name}-${idx}`}
                toolCall={tc}
                status="complete"
              />
            ))}
          </div>
        )}

        {/* Action buttons (visible on hover for assistant messages) */}
        {!isUser && isHovering && !isStreaming && (
          <div className="absolute -bottom-8 left-0 flex items-center gap-2">
            <button
              onClick={handleCopy}
              className="text-xs text-gray-500 hover:text-gray-300 px-2 py-1 rounded hover:bg-gray-800 transition-colors"
              title="Copy message"
            >
              {copied ? 'Copied!' : 'Copy'}
            </button>
            {onRetry && (
              <button
                onClick={onRetry}
                className="text-xs text-gray-500 hover:text-gray-300 px-2 py-1 rounded hover:bg-gray-800 transition-colors"
                title="Retry"
              >
                Retry
              </button>
            )}
          </div>
        )}
      </div>

      {/* Timestamp */}
      {showTimestamp && (
        <div className={`text-xs text-gray-600 mt-1 ${isUser ? 'mr-1' : 'ml-1'}`}>
          {formatTime(message.created_at)}
        </div>
      )}
    </div>
  );
});
