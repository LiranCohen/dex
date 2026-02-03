import { useState, useRef, useCallback, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import { Message } from './Message';
import { ToolActivity } from './ToolActivity';
import { ProposedObjective } from './ProposedObjective';
import { QuestionPrompt } from './QuestionPrompt';
import { ScrollIndicator } from './ScrollIndicator';
import { formatTime } from '../../utils/formatters';
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
  onRetry: (msg: QuestMessage) => void;
  onCopy: (content: string) => void;
  onAcceptDraft: (key: string, draft: ObjectiveDraft) => void;
  onRejectDraft: (key: string) => void;
  onAnswerQuestion: (answer: string) => void;
}

export function MessageList({
  messages,
  failedMessages,
  activeTools,
  streamingContent,
  sending,
  pendingDrafts,
  pendingQuestions,
  onRetry,
  onCopy,
  onAcceptDraft,
  onRejectDraft,
  onAnswerQuestion,
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
          <p>Start by describing what you want to accomplish.</p>
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
          return (
            <Message
              key={msg.id}
              sender={msg.role === 'user' ? 'user' : 'assistant'}
              timestamp={formatTime(msg.created_at)}
              status={isFailed ? 'error' : undefined}
              onRetry={isFailed ? () => onRetry(msg) : undefined}
              onCopy={msg.role === 'assistant' ? () => onCopy(msg.content) : undefined}
            >
              <ReactMarkdown>{msg.content}</ReactMarkdown>
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
          <Message sender="assistant">
            <ReactMarkdown>{streamingContent}</ReactMarkdown>
            <span className="v2-cursor" />
          </Message>
        )}

        {/* Pending drafts */}
        {pendingDrafts.size > 0 && Array.from(pendingDrafts.entries()).map(([key, draft]) => {
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
              onAnswerQuestion(answer);
            }}
            disabled={sending}
          />
        ))}

        {/* Thinking indicator */}
        {sending && !streamingContent && (
          <Message sender="assistant">
            <span className="v2-cursor" />
          </Message>
        )}

        <div ref={messagesEndRef} />
      </div>

      <ScrollIndicator visible={showScrollIndicator} onClick={scrollToBottom} />
    </div>
  );
}
