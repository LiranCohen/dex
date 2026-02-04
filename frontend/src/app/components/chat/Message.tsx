import { useState, useRef, useCallback, type ReactNode } from 'react';

type MessageStatus = 'sending' | 'sent' | 'error';

export type ErrorType = 'billing_error' | 'rate_limit' | 'network' | 'validation' | 'unknown';

export interface MessageErrorInfo {
  type: ErrorType;
  message: string;
  retryable: boolean;
  details?: string;
}

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
  errorInfo?: MessageErrorInfo;
  toolCalls?: ToolCall[];
  isStreaming?: boolean;
  onRetry?: () => void;
  onCopy?: () => void;
  children: ReactNode;
}

// Get user-friendly error message based on error type
function getErrorDisplay(errorInfo: MessageErrorInfo): { icon: string; label: string; description: string } {
  switch (errorInfo.type) {
    case 'billing_error':
      return {
        icon: '💳',
        label: 'Billing issue',
        description: errorInfo.message || 'Credit balance too low. Please add credits to continue.',
      };
    case 'rate_limit':
      return {
        icon: '⏱',
        label: 'Rate limited',
        description: errorInfo.message || 'Too many requests. Please wait a moment and try again.',
      };
    case 'network':
      return {
        icon: '🔌',
        label: 'Connection error',
        description: errorInfo.message || 'Unable to reach the server. Check your connection.',
      };
    case 'validation':
      return {
        icon: '⚠',
        label: 'Invalid request',
        description: errorInfo.message || 'The message could not be processed.',
      };
    default:
      return {
        icon: '✗',
        label: 'Failed to send',
        description: errorInfo.message || 'An unexpected error occurred.',
      };
  }
}

export function Message({
  sender,
  timestamp,
  status = 'sent',
  errorInfo,
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

  // Handle keyboard focus for accessibility
  const handleFocus = useCallback(() => setShowActions(true), []);
  const handleBlur = useCallback((e: React.FocusEvent) => {
    // Only hide if focus is leaving the message entirely
    if (!e.currentTarget.contains(e.relatedTarget)) {
      setShowActions(false);
    }
  }, []);

  return (
    <div
      className={`app-message app-message--${sender} ${status === 'error' ? 'app-message--error' : ''} ${isStreaming ? 'app-message--streaming' : ''}`}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
      onFocus={handleFocus}
      onBlur={handleBlur}
      role="article"
      aria-label={`Message from ${senderLabel}${timestamp ? `, sent at ${timestamp}` : ''}`}
    >
      <span className={`app-message__handle app-message__handle--${sender}`}>
        {'<'}{senderLabel}{'>'}
      </span>

      <div className="app-message__body">
        <div ref={contentRef} className="app-message__content">
          {children}
          {isStreaming && <span className="app-cursor" />}
        </div>

        {/* Inline tool calls */}
        {toolCalls.length > 0 && (
          <div className="app-message__tools">
            {toolCalls.map((tool) => (
              <div
                key={tool.id}
                className={`app-tool-activity app-tool-activity--${tool.status}`}
              >
                {tool.status === 'running' && (
                  <div className="app-tool-activity__spinner" />
                )}
                {tool.status === 'complete' && (
                  <span className="app-tool-activity__icon">·</span>
                )}
                {tool.status === 'error' && (
                  <span className="app-tool-activity__icon">✗</span>
                )}
                <span className="app-tool-activity__name">{tool.name}</span>
                {tool.description && (
                  <span className="app-tool-activity__desc">{tool.description}</span>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Error state with detailed feedback */}
        {status === 'error' && (() => {
          const error = errorInfo || { type: 'unknown' as ErrorType, message: '', retryable: true };
          const display = getErrorDisplay(error);
          return (
            <div className={`app-message__error app-message__error--${error.type}`} role="alert">
              <div className="app-message__error-header">
                <span className="app-message__error-icon" aria-hidden="true">{display.icon}</span>
                <span className="app-message__error-label">{display.label}</span>
              </div>
              <p className="app-message__error-desc">{display.description}</p>
              <div className="app-message__error-actions">
                {error.retryable && onRetry && (
                  <button
                    type="button"
                    className="app-message__retry"
                    onClick={onRetry}
                    aria-label="Retry sending message"
                  >
                    Retry
                  </button>
                )}
                {error.type === 'billing_error' && (
                  <a
                    href="https://console.anthropic.com"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="app-message__error-link"
                  >
                    Add credits →
                  </a>
                )}
              </div>
            </div>
          );
        })()}
      </div>

      {/* Actions and timestamp - show on hover or focus */}
      <div className={`app-message__meta ${showActions ? 'app-message__meta--visible' : ''}`}>
        {status === 'sent' && sender === 'assistant' && (
          <button
            type="button"
            className={`app-message__action ${showActions ? 'app-message__action--visible' : ''}`}
            onClick={handleCopy}
            aria-label={copied ? 'Copied to clipboard' : 'Copy message to clipboard'}
            title={copied ? 'Copied!' : 'Copy message'}
          >
            {copied ? '✓' : '⎘'}
          </button>
        )}
        {timestamp && !isStreaming && (
          <time className="app-timestamp" aria-label={`Sent at ${timestamp}`}>
            {timestamp}
          </time>
        )}
      </div>
    </div>
  );
}
