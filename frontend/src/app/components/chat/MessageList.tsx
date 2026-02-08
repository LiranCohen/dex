import { useState, useRef, useCallback, useEffect, memo } from 'react';
import { Message, type MessageErrorInfo } from './Message';
import { MarkdownContent } from './MarkdownContent';
import { ToolActivity } from './ToolActivity';
import { ProposedObjective } from './ProposedObjective';
import { ScrollIndicator } from './ScrollIndicator';
import { formatTime } from '../../utils/formatters';
import type { QuestMessage, ObjectiveDraft } from '../../../lib/types';

export type { MessageErrorInfo } from './Message';

interface ActiveTool {
  tool: string;
  status: 'running' | 'complete' | 'error';
}

export interface AcceptedDraft {
  draft: ObjectiveDraft;
  taskId?: string;
}

interface MessageListProps {
  messages: QuestMessage[];
  searchQuery?: string;
  failedMessages: Set<string>;
  messageErrors: Map<string, MessageErrorInfo>;
  activeTools: Map<string, ActiveTool>;
  streamingContent: string;
  sending: boolean;
  pendingDrafts: Map<string, ObjectiveDraft>;
  acceptedDrafts: Map<string, AcceptedDraft>;
  rejectedDrafts?: Map<string, ObjectiveDraft>;
  acceptingDrafts?: Set<string>;
  acceptingAll?: boolean;
  onRetry: (msg: QuestMessage) => void;
  onCopy: (content: string) => void;
  onAcceptDraft: (key: string, draft: ObjectiveDraft, selectedOptionalIndices: number[]) => void;
  onRejectDraft: (key: string) => void;
  onUndoReject?: (key: string) => void;
  onAcceptAll?: () => void;
  onRejectAll?: () => void;
}

// Highlight search matches in text
function highlightMatches(text: string, query: string): React.ReactNode {
  if (!query.trim()) return text;

  const parts = text.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi'));
  return parts.map((part, i) =>
    part.toLowerCase() === query.toLowerCase() ? (
      <mark key={i} className="app-search-highlight">{part}</mark>
    ) : (
      part
    )
  );
}

// Helper to compare Sets by content
function areSetsEqual<T>(a: Set<T>, b: Set<T>): boolean {
  if (a.size !== b.size) return false;
  for (const item of a) {
    if (!b.has(item)) return false;
  }
  return true;
}

// Helper to compare Maps by keys (shallow comparison)
function areMapsEqualByKeys<K, V>(a: Map<K, V>, b: Map<K, V>): boolean {
  if (a.size !== b.size) return false;
  for (const key of a.keys()) {
    if (!b.has(key)) return false;
  }
  return true;
}

// Custom comparison for memoization - compare Map/Set contents
function arePropsEqual(prevProps: MessageListProps, nextProps: MessageListProps): boolean {
  // Compare primitive props and arrays by reference
  if (
    prevProps.messages !== nextProps.messages ||
    prevProps.searchQuery !== nextProps.searchQuery ||
    prevProps.streamingContent !== nextProps.streamingContent ||
    prevProps.sending !== nextProps.sending ||
    prevProps.acceptingAll !== nextProps.acceptingAll ||
    prevProps.onRetry !== nextProps.onRetry ||
    prevProps.onCopy !== nextProps.onCopy ||
    prevProps.onAcceptDraft !== nextProps.onAcceptDraft ||
    prevProps.onRejectDraft !== nextProps.onRejectDraft ||
    prevProps.onAcceptAll !== nextProps.onAcceptAll ||
    prevProps.onRejectAll !== nextProps.onRejectAll
  ) {
    return false;
  }

  // Compare Sets by content
  if (!areSetsEqual(prevProps.failedMessages, nextProps.failedMessages)) return false;

  // Compare Maps by keys (values are derived from keys in our use case)
  if (!areMapsEqualByKeys(prevProps.messageErrors, nextProps.messageErrors)) return false;
  if (!areMapsEqualByKeys(prevProps.activeTools, nextProps.activeTools)) return false;
  if (!areMapsEqualByKeys(prevProps.pendingDrafts, nextProps.pendingDrafts)) return false;
  if (!areMapsEqualByKeys(prevProps.acceptedDrafts, nextProps.acceptedDrafts)) return false;

  // Compare optional rejected drafts
  const prevRejected = prevProps.rejectedDrafts ?? new Map();
  const nextRejected = nextProps.rejectedDrafts ?? new Map();
  if (!areMapsEqualByKeys(prevRejected, nextRejected)) return false;

  // Compare optional Set
  const prevAccepting = prevProps.acceptingDrafts ?? new Set();
  const nextAccepting = nextProps.acceptingDrafts ?? new Set();
  if (!areSetsEqual(prevAccepting, nextAccepting)) return false;

  return true;
}

export const MessageList = memo(function MessageList({
  messages,
  searchQuery = '',
  failedMessages,
  messageErrors,
  activeTools,
  streamingContent,
  sending,
  pendingDrafts,
  acceptedDrafts,
  rejectedDrafts = new Map(),
  acceptingDrafts = new Set(),
  acceptingAll = false,
  onRetry,
  onCopy,
  onAcceptDraft,
  onRejectDraft,
  onUndoReject,
  onAcceptAll,
  onRejectAll,
}: MessageListProps) {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const [showScrollIndicator, setShowScrollIndicator] = useState(false);
  // Track if user was at bottom before content changed
  const wasAtBottomRef = useRef(true);
  const prevMessagesLengthRef = useRef(messages.length);

  const scrollToBottom = useCallback(() => {
    // Guard against unmounted state
    if (!messagesEndRef.current) return;
    messagesEndRef.current.scrollIntoView({ behavior: 'smooth' });
    setShowScrollIndicator(false);
  }, []);

  const checkIfAtBottom = useCallback(() => {
    const container = messagesContainerRef.current;
    if (!container) return true;
    const threshold = 100;
    return container.scrollHeight - container.scrollTop - container.clientHeight < threshold;
  }, []);

  const handleScroll = useCallback(() => {
    const isAtBottom = checkIfAtBottom();
    wasAtBottomRef.current = isAtBottom;
    if (isAtBottom) {
      setShowScrollIndicator(false);
    }
  }, [checkIfAtBottom]);

  useEffect(() => {
    // Only auto-scroll if user was at bottom before the update
    // or if this is a new message from the user (messages length increased)
    const isNewMessage = messages.length > prevMessagesLengthRef.current;
    prevMessagesLengthRef.current = messages.length;

    if (wasAtBottomRef.current || isNewMessage) {
      scrollToBottom();
    } else if (messages.length > 0) {
      setShowScrollIndicator(true);
    }
  }, [messages, streamingContent, scrollToBottom]);

  if (messages.length === 0 && !sending) {
    return (
      <div className="app-quest-messages">
        <div className="app-quest-empty">
          <div className="app-quest-empty__icon">🧭</div>
          <h3 className="app-quest-empty__title">Start a conversation</h3>
          <p className="app-quest-empty__desc">
            Describe what you want to accomplish. Dex will help you break it down into actionable objectives.
          </p>
          <div className="app-quest-empty__tips">
            <p className="app-quest-empty__tip">
              <span className="app-quest-empty__tip-icon">💡</span>
              <span>Be specific about your goals and requirements</span>
            </p>
            <p className="app-quest-empty__tip">
              <span className="app-quest-empty__tip-icon">💡</span>
              <span>Dex can search files, explore code, and research the web</span>
            </p>
            <p className="app-quest-empty__tip">
              <span className="app-quest-empty__tip-icon">💡</span>
              <span>You'll get objective proposals to review before work begins</span>
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      ref={messagesContainerRef}
      onScroll={handleScroll}
      className="app-quest-messages"
    >
      <div className="app-quest-messages__list">
        {messages.map((msg) => {
          const isFailed = failedMessages.has(msg.id);

          return (
            <Message
              key={msg.id}
              sender={msg.role === 'user' ? 'user' : 'assistant'}
              timestamp={formatTime(msg.created_at)}
              status={isFailed ? 'error' : undefined}
              errorInfo={isFailed ? messageErrors.get(msg.id) : undefined}
              onRetry={isFailed ? () => onRetry(msg) : undefined}
              onCopy={msg.role === 'assistant' ? () => onCopy(msg.content) : undefined}
            >
              {msg.role === 'user' ? (
                <p style={{ whiteSpace: 'pre-wrap' }}>{highlightMatches(msg.content, searchQuery)}</p>
              ) : (
                <MarkdownContent content={msg.content} searchQuery={searchQuery} />
              )}
            </Message>
          );
        })}

        {/* Active tools */}
        {activeTools.size > 0 && (
          <>
            {Array.from(activeTools.values()).map((tool) => (
              <ToolActivity
                key={tool.tool}
                status={tool.status}
                tool={tool.tool}
                description={`${tool.status === 'running' ? 'Running' : tool.status === 'complete' ? 'Completed' : 'Failed'} ${tool.tool}`}
              />
            ))}
          </>
        )}

        {/* Streaming response */}
        {sending && streamingContent && streamingContent.trim() && (
          <Message sender="assistant" isStreaming>
            <MarkdownContent content={streamingContent} isStreaming />
          </Message>
        )}

        {/* Accepted drafts - show as confirmed */}
        {acceptedDrafts.size > 0 && Array.from(acceptedDrafts.entries()).map(([key, { draft, taskId }]) => (
          <div key={key} className="app-accepted-objective">
            <div className="app-accepted-objective__header">
              <span className="app-label" style={{ color: 'var(--status-complete)' }}>✓ Objective Created</span>
            </div>
            <div className="app-accepted-objective__title">{draft.title}</div>
            {taskId && (
              <a href={`/objectives/${taskId}`} className="app-link app-accepted-objective__link">
                View objective →
              </a>
            )}
          </div>
        ))}

        {/* Recently rejected drafts with undo option */}
        {rejectedDrafts.size > 0 && Array.from(rejectedDrafts.entries()).map(([key, draft]) => (
          <div key={key} className="app-rejected-objective">
            <div className="app-rejected-objective__content">
              <span className="app-label" style={{ color: 'var(--text-tertiary)' }}>✗ Rejected</span>
              <span className="app-rejected-objective__title">{draft.title}</span>
            </div>
            {onUndoReject && (
              <button
                type="button"
                className="app-btn app-btn--ghost app-rejected-objective__undo"
                onClick={() => onUndoReject(key)}
              >
                <span className="app-rejected-objective__undo-text">Undo</span>
                <span className="app-rejected-objective__undo-countdown" aria-hidden="true" />
              </button>
            )}
          </div>
        ))}

        {/* Pending drafts */}
        {pendingDrafts.size > 0 && (
          <>
            {/* Accept All / Reject All buttons when multiple drafts */}
            {pendingDrafts.size > 1 && (onAcceptAll || onRejectAll) && (
              <div className="app-drafts-header">
                <span className="app-label">{pendingDrafts.size} Proposed Objectives</span>
                <div className="app-drafts-header__actions">
                  {onRejectAll && (
                    <button
                      className="app-btn app-btn--ghost"
                      onClick={onRejectAll}
                      disabled={acceptingAll}
                    >
                      Reject All
                    </button>
                  )}
                  {onAcceptAll && (
                    <button
                      className={`app-btn app-btn--primary ${acceptingAll ? 'app-btn--loading' : ''}`}
                      onClick={onAcceptAll}
                      disabled={acceptingAll}
                      aria-busy={acceptingAll}
                    >
                      {acceptingAll ? (
                        <>
                          <span className="app-btn__spinner" aria-hidden="true" />
                          Accepting...
                        </>
                      ) : (
                        'Accept All'
                      )}
                    </button>
                  )}
                </div>
              </div>
            )}
            {Array.from(pendingDrafts.entries()).map(([key, draft]) => {
              const isAccepting = acceptingDrafts.has(key) || acceptingAll;
              const mustHaveItems = (draft.checklist?.must_have || []).map((item, i) => ({
                id: `must-${i}`,
                text: item,
                isOptional: false,
              }));
              const optionalItems = (draft.checklist?.optional || []).map((item, i) => ({
                id: `opt-${i}`,
                text: item,
                isOptional: true,
              }));
              return (
                <ProposedObjective
                  key={key}
                  title={draft.title}
                  description={draft.description}
                  checklist={[...mustHaveItems, ...optionalItems]}
                  status={isAccepting ? 'accepting' : 'pending'}
                  onAccept={(selectedOptionalIndices) => onAcceptDraft(key, draft, selectedOptionalIndices)}
                  onReject={() => onRejectDraft(key)}
                />
              );
            })}
          </>
        )}

        {/* Thinking indicator - shown when waiting for response (not yet streaming) */}
        {sending && !streamingContent && activeTools.size === 0 && (
          <div className="app-thinking">
            <div className="app-thinking__header">
              <span className="app-thinking__sender">Dex</span>
              <span className="app-thinking__status">
                <span className="app-thinking__pulse" />
                <span>Thinking...</span>
              </span>
            </div>
            <div className="app-thinking__dots">
              <span className="app-thinking__dot" style={{ animationDelay: '0ms' }} />
              <span className="app-thinking__dot" style={{ animationDelay: '150ms' }} />
              <span className="app-thinking__dot" style={{ animationDelay: '300ms' }} />
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      <ScrollIndicator visible={showScrollIndicator} onClick={scrollToBottom} />
    </div>
  );
}, arePropsEqual);
