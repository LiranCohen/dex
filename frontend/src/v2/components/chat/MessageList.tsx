import { useState, useRef, useCallback, useEffect, memo } from 'react';
import { Message, type MessageErrorInfo } from './Message';
import { MarkdownContent } from './MarkdownContent';
import { ToolActivity } from './ToolActivity';
import { ProposedObjective } from './ProposedObjective';
import { QuestionPrompt } from './QuestionPrompt';
import { ScrollIndicator } from './ScrollIndicator';
import { formatTime } from '../../utils/formatters';
import { stripSignals } from '../../../components/QuestChat';
import type { QuestMessage, ObjectiveDraft } from '../../../lib/types';
import type { QuestQuestion } from '../../../components/QuestChat';

export type { MessageErrorInfo } from './Message';

interface ActiveTool {
  tool: string;
  status: 'running' | 'complete' | 'error';
}

export interface AnsweredQuestion {
  question: QuestQuestion;
  answerId: string;
  answer: string;
  messageId?: string; // ID of the message that triggered this question
}

export interface AcceptedDraft {
  draft: ObjectiveDraft;
  taskId?: string;
}

export interface PendingQuestionWithMessage extends QuestQuestion {
  messageId?: string; // ID of the message that triggered this question
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
  pendingQuestions: PendingQuestionWithMessage[];
  answeredQuestions: AnsweredQuestion[];
  acceptedDrafts: Map<string, AcceptedDraft>;
  rejectedDrafts?: Map<string, ObjectiveDraft>;
  acceptingDrafts?: Set<string>;
  acceptingAll?: boolean;
  onRetry: (msg: QuestMessage) => void;
  onCopy: (content: string) => void;
  onAcceptDraft: (key: string, draft: ObjectiveDraft, selectedOptionalIndices: number[]) => void;
  onRejectDraft: (key: string) => void;
  onUndoReject?: (key: string) => void;
  onAnswerQuestion: (answer: string, optionId: string) => void;
  onAcceptAll?: () => void;
  onRejectAll?: () => void;
}

// Highlight search matches in text
function highlightMatches(text: string, query: string): React.ReactNode {
  if (!query.trim()) return text;

  const parts = text.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi'));
  return parts.map((part, i) =>
    part.toLowerCase() === query.toLowerCase() ? (
      <mark key={i} className="v2-search-highlight">{part}</mark>
    ) : (
      part
    )
  );
}

// Check if streaming content contains objective or question signals
function hasStreamingSignal(content: string): 'objective' | 'question' | null {
  if (content.includes('<objective_draft>') || content.includes('title":')) {
    return 'objective';
  }
  if (content.includes('<question>') || content.includes('"question":')) {
    return 'question';
  }
  return null;
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
    prevProps.pendingQuestions !== nextProps.pendingQuestions ||
    prevProps.answeredQuestions !== nextProps.answeredQuestions ||
    prevProps.acceptingAll !== nextProps.acceptingAll ||
    prevProps.onRetry !== nextProps.onRetry ||
    prevProps.onCopy !== nextProps.onCopy ||
    prevProps.onAcceptDraft !== nextProps.onAcceptDraft ||
    prevProps.onRejectDraft !== nextProps.onRejectDraft ||
    prevProps.onAnswerQuestion !== nextProps.onAnswerQuestion ||
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
  pendingQuestions,
  answeredQuestions,
  acceptedDrafts,
  rejectedDrafts = new Map(),
  acceptingDrafts = new Set(),
  acceptingAll = false,
  onRetry,
  onCopy,
  onAcceptDraft,
  onRejectDraft,
  onUndoReject,
  onAnswerQuestion,
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
      <div className="v2-quest-messages">
        <div className="v2-quest-empty">
          <div className="v2-quest-empty__icon">🧭</div>
          <h3 className="v2-quest-empty__title">Start a conversation</h3>
          <p className="v2-quest-empty__desc">
            Describe what you want to accomplish. Dex will help you break it down into actionable objectives.
          </p>
          <div className="v2-quest-empty__tips">
            <p className="v2-quest-empty__tip">
              <span className="v2-quest-empty__tip-icon">💡</span>
              <span>Be specific about your goals and requirements</span>
            </p>
            <p className="v2-quest-empty__tip">
              <span className="v2-quest-empty__tip-icon">💡</span>
              <span>Dex can search files, explore code, and research the web</span>
            </p>
            <p className="v2-quest-empty__tip">
              <span className="v2-quest-empty__tip-icon">💡</span>
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
      className="v2-quest-messages"
    >
      <div className="v2-quest-messages__list">
        {messages.map((msg, msgIndex) => {
          const isFailed = failedMessages.has(msg.id);
          // Strip signals from content before rendering
          const displayContent = stripSignals(msg.content);

          // Find answered questions associated with this message
          const questionsForThisMessage = answeredQuestions.filter(aq => aq.messageId === msg.id);

          // For the last assistant message, also show pending questions
          const isLastAssistantMessage = msg.role === 'assistant' &&
            (msgIndex === messages.length - 1 ||
            !messages.slice(msgIndex + 1).some(m => m.role === 'assistant'));
          const pendingQuestionsForMessage = isLastAssistantMessage
            ? pendingQuestions.filter(q => !q.messageId || q.messageId === msg.id)
            : [];

          return (
            <div key={msg.id}>
              <Message
                sender={msg.role === 'user' ? 'user' : 'assistant'}
                timestamp={formatTime(msg.created_at)}
                status={isFailed ? 'error' : undefined}
                errorInfo={isFailed ? messageErrors.get(msg.id) : undefined}
                onRetry={isFailed ? () => onRetry(msg) : undefined}
                onCopy={msg.role === 'assistant' ? () => onCopy(displayContent) : undefined}
              >
                {msg.role === 'user' ? (
                  <p style={{ whiteSpace: 'pre-wrap' }}>{highlightMatches(displayContent, searchQuery)}</p>
                ) : (
                  <MarkdownContent content={displayContent} searchQuery={searchQuery} />
                )}
              </Message>

              {/* Answered questions from this message */}
              {questionsForThisMessage.map((aq, i) => (
                <QuestionPrompt
                  key={`answered-${msg.id}-${i}`}
                  question={aq.question.question}
                  options={(aq.question.options || []).map((opt, j) => ({
                    id: `${j}`,
                    title: opt,
                    description: '',
                  }))}
                  onSelect={() => {}}
                  disabled={true}
                  answeredId={aq.answerId}
                  customAnswer={aq.answer}
                />
              ))}

              {/* Pending questions for this message */}
              {pendingQuestionsForMessage.map((q, i) => (
                <QuestionPrompt
                  key={`pending-${msg.id}-${i}`}
                  question={q.question}
                  options={(q.options || []).map((opt, j) => ({
                    id: `${j}`,
                    title: opt,
                    description: '',
                  }))}
                  onSelect={(optId) => {
                    const answer = q.options?.[parseInt(optId)] || '';
                    onAnswerQuestion(answer, optId);
                  }}
                  onCustomAnswer={(answer) => {
                    onAnswerQuestion(answer, 'custom');
                  }}
                  disabled={sending}
                />
              ))}
            </div>
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
        {sending && streamingContent && (() => {
          const signalType = hasStreamingSignal(streamingContent);
          const strippedContent = stripSignals(streamingContent);

          // Show placeholder for objectives/questions being streamed
          if (signalType === 'objective') {
            return (
              <div className="v2-streaming-placeholder">
                <div className="v2-streaming-placeholder__icon">[+]</div>
                <div className="v2-streaming-placeholder__text">
                  <span className="v2-label v2-label--warm">Preparing objective...</span>
                  <div className="v2-thinking__dots">
                    <span className="v2-thinking__dot" style={{ animationDelay: '0ms' }} />
                    <span className="v2-thinking__dot" style={{ animationDelay: '150ms' }} />
                    <span className="v2-thinking__dot" style={{ animationDelay: '300ms' }} />
                  </div>
                </div>
              </div>
            );
          }

          if (signalType === 'question') {
            return (
              <div className="v2-streaming-placeholder">
                <div className="v2-streaming-placeholder__icon">[?]</div>
                <div className="v2-streaming-placeholder__text">
                  <span className="v2-label v2-label--accent">Preparing question...</span>
                  <div className="v2-thinking__dots">
                    <span className="v2-thinking__dot" style={{ animationDelay: '0ms' }} />
                    <span className="v2-thinking__dot" style={{ animationDelay: '150ms' }} />
                    <span className="v2-thinking__dot" style={{ animationDelay: '300ms' }} />
                  </div>
                </div>
              </div>
            );
          }

          // Regular streaming content
          if (strippedContent.trim()) {
            return (
              <Message sender="assistant" isStreaming>
                <MarkdownContent content={strippedContent} isStreaming />
              </Message>
            );
          }

          return null;
        })()}

        {/* Accepted drafts - show as confirmed */}
        {acceptedDrafts.size > 0 && Array.from(acceptedDrafts.entries()).map(([key, { draft, taskId }]) => (
          <div key={key} className="v2-accepted-objective">
            <div className="v2-accepted-objective__header">
              <span className="v2-label" style={{ color: 'var(--status-complete)' }}>✓ Objective Created</span>
            </div>
            <div className="v2-accepted-objective__title">{draft.title}</div>
            {taskId && (
              <a href={`/v2/objectives/${taskId}`} className="v2-link v2-accepted-objective__link">
                View objective →
              </a>
            )}
          </div>
        ))}

        {/* Recently rejected drafts with undo option */}
        {rejectedDrafts.size > 0 && Array.from(rejectedDrafts.entries()).map(([key, draft]) => (
          <div key={key} className="v2-rejected-objective">
            <div className="v2-rejected-objective__content">
              <span className="v2-label" style={{ color: 'var(--text-tertiary)' }}>✗ Rejected</span>
              <span className="v2-rejected-objective__title">{draft.title}</span>
            </div>
            {onUndoReject && (
              <button
                type="button"
                className="v2-btn v2-btn--ghost v2-rejected-objective__undo"
                onClick={() => onUndoReject(key)}
              >
                <span className="v2-rejected-objective__undo-text">Undo</span>
                <span className="v2-rejected-objective__undo-countdown" aria-hidden="true" />
              </button>
            )}
          </div>
        ))}

        {/* Pending drafts */}
        {pendingDrafts.size > 0 && (
          <>
            {/* Accept All / Reject All buttons when multiple drafts */}
            {pendingDrafts.size > 1 && (onAcceptAll || onRejectAll) && (
              <div className="v2-drafts-header">
                <span className="v2-label">{pendingDrafts.size} Proposed Objectives</span>
                <div className="v2-drafts-header__actions">
                  {onRejectAll && (
                    <button
                      className="v2-btn v2-btn--ghost"
                      onClick={onRejectAll}
                      disabled={acceptingAll}
                    >
                      Reject All
                    </button>
                  )}
                  {onAcceptAll && (
                    <button
                      className={`v2-btn v2-btn--primary ${acceptingAll ? 'v2-btn--loading' : ''}`}
                      onClick={onAcceptAll}
                      disabled={acceptingAll}
                      aria-busy={acceptingAll}
                    >
                      {acceptingAll ? (
                        <>
                          <span className="v2-btn__spinner" aria-hidden="true" />
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

        {/* Questions without a message ID (fallback for legacy questions) */}
        {answeredQuestions.filter(aq => !aq.messageId).map((aq, i) => (
          <QuestionPrompt
            key={`answered-orphan-${i}`}
            question={aq.question.question}
            options={(aq.question.options || []).map((opt, j) => ({
              id: `${j}`,
              title: opt,
              description: '',
            }))}
            onSelect={() => {}}
            disabled={true}
            answeredId={aq.answerId}
            customAnswer={aq.answer}
          />
        ))}

        {/* Thinking indicator - shown when waiting for response (not yet streaming) */}
        {sending && !streamingContent && activeTools.size === 0 && (
          <div className="v2-thinking">
            <div className="v2-thinking__header">
              <span className="v2-thinking__sender">Dex</span>
              <span className="v2-thinking__status">
                <span className="v2-thinking__pulse" />
                <span>Thinking...</span>
              </span>
            </div>
            <div className="v2-thinking__dots">
              <span className="v2-thinking__dot" style={{ animationDelay: '0ms' }} />
              <span className="v2-thinking__dot" style={{ animationDelay: '150ms' }} />
              <span className="v2-thinking__dot" style={{ animationDelay: '300ms' }} />
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      <ScrollIndicator visible={showScrollIndicator} onClick={scrollToBottom} />
    </div>
  );
}, arePropsEqual);
