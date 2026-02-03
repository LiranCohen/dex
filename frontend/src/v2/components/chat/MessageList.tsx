import { useState, useRef, useCallback, useEffect } from 'react';
import { Message } from './Message';
import { MarkdownContent } from './MarkdownContent';
import { ToolActivity } from './ToolActivity';
import { ProposedObjective } from './ProposedObjective';
import { QuestionPrompt } from './QuestionPrompt';
import { ScrollIndicator } from './ScrollIndicator';
import { formatTime } from '../../utils/formatters';
import { stripSignals } from '../../../components/QuestChat';
import type { QuestMessage, ObjectiveDraft } from '../../../lib/types';
import type { QuestQuestion } from '../../../components/QuestChat';

interface ActiveTool {
  tool: string;
  status: 'running' | 'complete' | 'error';
}

interface MessageListProps {
  messages: QuestMessage[];
  failedMessages: Set<string>;
  activeTools: Map<string, ActiveTool>;
  streamingContent: string;
  sending: boolean;
  pendingDrafts: Map<string, ObjectiveDraft>;
  pendingQuestions: QuestQuestion[];
  answeredQuestionId: string | null;
  onRetry: (msg: QuestMessage) => void;
  onCopy: (content: string) => void;
  onAcceptDraft: (key: string, draft: ObjectiveDraft) => void;
  onRejectDraft: (key: string) => void;
  onAnswerQuestion: (answer: string, optionId: string) => void;
  onAcceptAll?: () => void;
}

export function MessageList({
  messages,
  failedMessages,
  activeTools,
  streamingContent,
  sending,
  pendingDrafts,
  pendingQuestions,
  answeredQuestionId,
  onRetry,
  onCopy,
  onAcceptDraft,
  onRejectDraft,
  onAnswerQuestion,
  onAcceptAll,
}: MessageListProps) {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const [showScrollIndicator, setShowScrollIndicator] = useState(false);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    setShowScrollIndicator(false);
  };

  const checkIfAtBottom = useCallback(() => {
    const container = messagesContainerRef.current;
    if (!container) return true;
    const threshold = 100;
    return container.scrollHeight - container.scrollTop - container.clientHeight < threshold;
  }, []);

  const handleScroll = useCallback(() => {
    if (checkIfAtBottom()) {
      setShowScrollIndicator(false);
    }
  }, [checkIfAtBottom]);

  useEffect(() => {
    if (checkIfAtBottom()) {
      scrollToBottom();
    } else if (messages.length > 0) {
      setShowScrollIndicator(true);
    }
  }, [messages, streamingContent, checkIfAtBottom]);

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
        {messages.map((msg) => {
          const isFailed = failedMessages.has(msg.id);
          // Strip signals from content before rendering
          const displayContent = stripSignals(msg.content);
          return (
            <Message
              key={msg.id}
              sender={msg.role === 'user' ? 'user' : 'assistant'}
              timestamp={formatTime(msg.created_at)}
              status={isFailed ? 'error' : undefined}
              onRetry={isFailed ? () => onRetry(msg) : undefined}
              onCopy={msg.role === 'assistant' ? () => onCopy(displayContent) : undefined}
            >
              {msg.role === 'user' ? (
                <p style={{ whiteSpace: 'pre-wrap' }}>{displayContent}</p>
              ) : (
                <MarkdownContent content={displayContent} />
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
        {sending && streamingContent && (
          <Message sender="assistant" isStreaming>
            <MarkdownContent content={stripSignals(streamingContent)} isStreaming />
          </Message>
        )}

        {/* Pending drafts */}
        {pendingDrafts.size > 0 && (
          <>
            {/* Accept All button when multiple drafts */}
            {pendingDrafts.size > 1 && onAcceptAll && (
              <div className="v2-drafts-header">
                <span className="v2-label">{pendingDrafts.size} Proposed Objectives</span>
                <button
                  className="v2-btn v2-btn--primary"
                  onClick={onAcceptAll}
                >
                  Accept All
                </button>
              </div>
            )}
            {Array.from(pendingDrafts.entries()).map(([key, draft]) => {
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
                  onAccept={() => onAcceptDraft(key, draft)}
                  onReject={() => onRejectDraft(key)}
                />
              );
            })}
          </>
        )}

        {/* Pending questions */}
        {pendingQuestions.length > 0 && pendingQuestions.map((q, i) => (
          <QuestionPrompt
            key={i}
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
            disabled={sending}
            answeredId={answeredQuestionId || undefined}
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
}
