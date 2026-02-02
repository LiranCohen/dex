import { useState, useCallback, useEffect } from 'react';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import { QuestionPrompt } from './QuestionPrompt';
import type { QuestMessage, WebSocketEvent, QuestToolCall } from '../../lib/types';

interface Question {
  question: string;
  options?: string[];
}

interface QuestChatContainerProps {
  questId: string;
  messages: QuestMessage[];
  questions: Question[];
  isStreaming: boolean;
  error: string | null;
  connected: boolean;
  onSendMessage: (content: string) => Promise<void>;
  subscribe: (handler: (event: WebSocketEvent) => void) => () => void;
}

// Pending tool calls being executed
interface PendingToolCall {
  tool_name: string;
  status: 'running' | 'complete' | 'error';
  input?: Record<string, unknown>;
}

export function QuestChatContainer({
  questId,
  messages,
  questions,
  isStreaming,
  error,
  connected,
  onSendMessage,
  subscribe,
}: QuestChatContainerProps) {
  const [isSending, setIsSending] = useState(false);
  const [pendingToolCalls, setPendingToolCalls] = useState<Map<string, PendingToolCall>>(new Map());

  // Handle WebSocket events for tool calls
  useEffect(() => {
    const handleEvent = (event: WebSocketEvent) => {
      const payload = event.payload as {
        quest_id?: string;
        tool_name?: string;
        status?: string;
        input?: Record<string, unknown>;
        output?: string;
        is_error?: boolean;
      };

      if (payload.quest_id !== questId) return;

      if (event.type === 'quest.tool_call') {
        // Tool execution started
        setPendingToolCalls(prev => {
          const next = new Map(prev);
          next.set(payload.tool_name!, {
            tool_name: payload.tool_name!,
            status: 'running',
            input: payload.input,
          });
          return next;
        });
      } else if (event.type === 'quest.tool_result') {
        // Tool execution completed
        setPendingToolCalls(prev => {
          const next = new Map(prev);
          next.delete(payload.tool_name!);
          return next;
        });
      }
    };

    const unsubscribe = subscribe(handleEvent);
    return unsubscribe;
  }, [questId, subscribe]);

  // Handle sending messages
  const handleSendMessage = useCallback(async (content: string) => {
    if (isSending || isStreaming) return;

    setIsSending(true);
    try {
      await onSendMessage(content);
    } finally {
      setIsSending(false);
    }
  }, [isSending, isStreaming, onSendMessage]);

  // Handle answering questions
  const handleAnswerQuestion = useCallback((answer: string) => {
    handleSendMessage(answer);
  }, [handleSendMessage]);

  // Append pending tool calls as "running" to the last assistant message for display
  const messagesWithPending = [...messages];
  if (pendingToolCalls.size > 0 && messages.length > 0) {
    const lastMsg = messages[messages.length - 1];
    if (lastMsg.role === 'assistant') {
      // Create a modified copy with pending tool calls
      const pendingCalls: QuestToolCall[] = Array.from(pendingToolCalls.values()).map(tc => ({
        tool_name: tc.tool_name,
        input: tc.input || {},
        output: '',
        is_error: false,
        duration_ms: 0,
      }));

      messagesWithPending[messagesWithPending.length - 1] = {
        ...lastMsg,
        tool_calls: [...(lastMsg.tool_calls || []), ...pendingCalls],
      };
    }
  }

  const inputDisabled = isSending || isStreaming || !connected;
  const hasActiveQuestion = questions.length > 0;

  return (
    <div className="flex flex-col h-full bg-gray-900">
      {/* Connection status */}
      {!connected && (
        <div className="bg-yellow-900/50 border-b border-yellow-700/50 px-4 py-2 text-center text-sm text-yellow-300">
          <span className="animate-pulse">●</span> Reconnecting...
        </div>
      )}

      {/* Error display */}
      {error && (
        <div className="bg-red-900/50 border-b border-red-700/50 px-4 py-2 text-center text-sm text-red-300">
          {error}
        </div>
      )}

      {/* Message list */}
      <MessageList
        messages={messagesWithPending}
        isStreaming={isStreaming || isSending}
      />

      {/* Question prompt (if any) */}
      {hasActiveQuestion && (
        <div className="px-4 pb-2">
          <QuestionPrompt
            question={questions[0]}
            onAnswer={handleAnswerQuestion}
            disabled={inputDisabled}
          />
        </div>
      )}

      {/* Chat input */}
      <div className="border-t border-gray-800 p-4 pb-8">
        <ChatInput
          onSubmit={handleSendMessage}
          disabled={inputDisabled}
          placeholder={
            !connected
              ? 'Connecting...'
              : isStreaming
              ? 'Dex is responding...'
              : isSending
              ? 'Sending...'
              : 'Type a message...'
          }
        />
      </div>
    </div>
  );
}
