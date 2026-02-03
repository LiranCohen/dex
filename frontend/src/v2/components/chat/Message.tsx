import { useState, useRef, useCallback, type ReactNode } from 'react';

type MessageStatus = 'sending' | 'sent' | 'error';

interface ToolCall {
  id: string;
  name: string;
  status: 'running' | 'complete' | 'error';
  description?: string;
}

interface MessageProps {
  sender: 'user' | 'assistant';
  timestamp?: string;
  status?: MessageStatus;
  toolCalls?: ToolCall[];
  isStreaming?: boolean;
  onRetry?: () => void;
  onCopy?: () => void;
  children: ReactNode;
}

export function Message({
  sender,
  timestamp,
  status = 'sent',
  toolCalls = [],
  isStreaming = false,
  onRetry,
  onCopy,
  children,
}: MessageProps) {
  const [showActions, setShowActions] = useState(false);
  const [copied, setCopied] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const senderLabel = sender === 'user' ? 'you' : 'dex';

  const handleCopy = useCallback(async () => {
    if (contentRef.current) {
      const text = contentRef.current.innerText;
      try {
        await navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
        onCopy?.();
      } catch {
        console.error('Failed to copy');
      }
    }
  }, [onCopy]);

  return (
    <div
      className={`v2-message v2-message--${sender} ${status === 'error' ? 'v2-message--error' : ''} ${isStreaming ? 'v2-message--streaming' : ''}`}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
    >
      <span className={`v2-message__handle v2-message__handle--${sender}`}>
        {'<'}{senderLabel}{'>'}
      </span>

      <div className="v2-message__body">
        <div ref={contentRef} className="v2-message__content">
          {children}
          {isStreaming && <span className="v2-cursor" />}
        </div>

        {/* Inline tool calls */}
        {toolCalls.length > 0 && (
          <div className="v2-message__tools">
            {toolCalls.map((tool) => (
              <div
                key={tool.id}
                className={`v2-tool-activity v2-tool-activity--${tool.status}`}
              >
                {tool.status === 'running' && (
                  <div className="v2-tool-activity__spinner" />
                )}
                {tool.status === 'complete' && (
                  <span className="v2-tool-activity__icon">·</span>
                )}
                {tool.status === 'error' && (
                  <span className="v2-tool-activity__icon">✗</span>
                )}
                <span className="v2-tool-activity__name">{tool.name}</span>
                {tool.description && (
                  <span className="v2-tool-activity__desc">{tool.description}</span>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Error state with retry */}
        {status === 'error' && (
          <div className="v2-message__error" role="alert">
            <span className="v2-message__error-text">✗ Failed to send</span>
            {onRetry && (
              <button
                type="button"
                className="v2-message__retry"
                onClick={onRetry}
                aria-label="Retry sending message"
              >
                Retry
              </button>
            )}
          </div>
        )}
      </div>

      {/* Actions and timestamp - show on hover */}
      <div className={`v2-message__meta ${showActions ? 'v2-message__meta--visible' : ''}`}>
        {showActions && status === 'sent' && sender === 'assistant' && (
          <button
            type="button"
            className="v2-message__action"
            onClick={handleCopy}
            aria-label={copied ? 'Copied' : 'Copy message'}
          >
            {copied ? '✓' : '⎘'}
          </button>
        )}
        {timestamp && !isStreaming && (
          <span className="v2-timestamp">{timestamp}</span>
        )}
      </div>
    </div>
  );
}
