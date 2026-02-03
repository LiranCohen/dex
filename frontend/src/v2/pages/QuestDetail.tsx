import { useState, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import {
  Header,
  ChatInput,
  KeyboardShortcuts,
  SkeletonMessage,
  useToast,
  QuestObjectivesList,
  MessageList,
  type AnsweredQuestion,
  type AcceptedDraft,
} from '../components';
import { fetchQuest, fetchQuestTasks, sendQuestMessage, createObjective, createObjectivesBatch, fetchApprovals, cancelQuestSession, isApiError } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';
import type { Quest, QuestMessage, Task, Approval, WebSocketEvent, ObjectiveDraft } from '../../lib/types';
import { parseObjectiveDrafts, parseQuestions, type QuestQuestion } from '../../components/QuestChat';

export function QuestDetail() {
  const { id } = useParams<{ id: string }>();
  const [quest, setQuest] = useState<Quest | null>(null);
  const [messages, setMessages] = useState<QuestMessage[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [approvalCount, setApprovalCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [streamingContent, setStreamingContent] = useState<string>('');
  const [activeTools, setActiveTools] = useState<Map<string, { tool: string; status: 'running' | 'complete' | 'error' }>>(new Map());
  const [pendingDrafts, setPendingDrafts] = useState<Map<string, ObjectiveDraft>>(new Map());
  const [pendingQuestions, setPendingQuestions] = useState<QuestQuestion[]>([]);
  const [answeredQuestions, setAnsweredQuestions] = useState<AnsweredQuestion[]>([]);
  const [acceptedDrafts, setAcceptedDrafts] = useState<Map<string, AcceptedDraft>>(new Map());
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [commandHistory, setCommandHistory] = useState<string[]>([]);
  const [failedMessages, setFailedMessages] = useState<Set<string>>(new Set());
  const [billingWarning, setBillingWarning] = useState<string | null>(null);
  const { subscribe, connected: isConnected } = useWebSocket();
  const { showToast } = useToast();

  // Keyboard navigation
  useKeyboardNavigation({
    onHelp: () => setShowShortcuts(true),
    enabled: true,
  });

  const handleStop = async () => {
    if (!id) return;
    try {
      await cancelQuestSession(id);
      setSending(false);
      showToast('Session stopped', 'info');
    } catch (err) {
      console.error('Failed to stop session:', err);
      showToast('Failed to stop session', 'error');
    }
  };

  const handleCopyMessage = (content: string) => {
    navigator.clipboard.writeText(content)
      .then(() => showToast('Copied to clipboard', 'success'))
      .catch((err) => {
        console.error('Failed to copy:', err);
        showToast('Failed to copy', 'error');
      });
  };

  const loadData = useCallback(async () => {
    if (!id) return;
    try {
      const [questData, tasksData, approvalsData] = await Promise.all([
        fetchQuest(id),
        fetchQuestTasks(id),
        fetchApprovals(),
      ]);
      setQuest(questData.quest);
      setMessages(questData.messages || []);
      setTasks(tasksData || []);
      setApprovalCount((approvalsData.approvals || []).filter((a: Approval) => a.status === 'pending').length);

      // Parse any pending drafts/questions from last assistant message
      const lastAssistantMsg = [...(questData.messages || [])].reverse().find((m: QuestMessage) => m.role === 'assistant');
      if (lastAssistantMsg) {
        const drafts = parseObjectiveDrafts(lastAssistantMsg.content);
        const questions = parseQuestions(lastAssistantMsg.content);
        if (drafts.length > 0) {
          const draftsMap = new Map<string, ObjectiveDraft>();
          drafts.forEach((d, i) => draftsMap.set(`draft-${i}`, d));
          setPendingDrafts(draftsMap);
        }
        if (questions.length > 0) {
          setPendingQuestions(questions);
        }
      }
    } catch (err) {
      console.error('Failed to load quest:', err);
      showToast('Failed to load quest', 'error');
    } finally {
      setLoading(false);
    }
  }, [id, showToast]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // WebSocket events
  useEffect(() => {
    const unsubscribe = subscribe((event: WebSocketEvent) => {
      if (!id) return;

      const payload = event.payload as Record<string, unknown>;

      switch (event.type) {
        case 'quest.message':
          if (payload?.quest_id === id) {
            const msg = payload.message as QuestMessage;
            if (msg.role === 'assistant') {
              setStreamingContent('');
              setMessages((prev) => [...prev, msg]);
              setSending(false);

              // Clear pending questions (answered ones are kept)
              setPendingQuestions([]);

              // Parse drafts and questions from new message
              const drafts = parseObjectiveDrafts(msg.content);
              const questions = parseQuestions(msg.content);
              if (drafts.length > 0) {
                const draftsMap = new Map<string, ObjectiveDraft>();
                drafts.forEach((d, i) => draftsMap.set(`draft-${i}`, d));
                setPendingDrafts(draftsMap);
              }
              if (questions.length > 0) {
                setPendingQuestions(questions);
              }
            }
          }
          break;

        case 'quest.content_delta':
          if (payload?.quest_id === id) {
            const content = payload.content as string;
            setStreamingContent(content);
          }
          break;

        case 'quest.tool_call':
          if (payload?.quest_id === id) {
            const toolName = payload.tool_name as string;
            // Clear streaming content when tools start - we're in tool execution phase
            setStreamingContent('');
            setActiveTools((prev) => new Map(prev).set(toolName, { tool: toolName, status: 'running' }));
          }
          break;

        case 'quest.tool_result':
          if (payload?.quest_id === id) {
            const toolName = payload.tool_name as string;
            const isError = payload.is_error as boolean;
            setActiveTools((prev) => {
              const next = new Map(prev);
              next.set(toolName, { tool: toolName, status: isError ? 'error' : 'complete' });
              return next;
            });
            // Clear completed/errored tools after showing result briefly
            // But only after assistant message arrives (detected by 'sending' becoming false)
            const checkAndClear = () => {
              setActiveTools((prev) => {
                // Only clear if not currently generating (message has arrived)
                const next = new Map(prev);
                const tool = next.get(toolName);
                if (tool && tool.status !== 'running') {
                  next.delete(toolName);
                }
                return next;
              });
            };
            // Clear after 3 seconds, or when new message arrives (whichever first)
            setTimeout(checkAndClear, 3000);
          }
          break;

        case 'task.created':
        case 'task.updated':
          loadData();
          break;
      }
    });

    return unsubscribe;
  }, [id, subscribe, loadData]);

  const handleSend = async (content: string) => {
    if (!id || sending) return;

    setSending(true);
    setPendingDrafts(new Map());
    setPendingQuestions([]);

    // Track command history
    setCommandHistory((prev) => {
      const newHistory = [content, ...prev.filter((c) => c !== content)].slice(0, 50);
      return newHistory;
    });

    // Optimistically add user message
    const userMsg: QuestMessage = {
      id: `temp-${Date.now()}`,
      quest_id: id,
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, userMsg]);

    try {
      const result = await sendQuestMessage(id, content);
      // On success, clear any billing warning
      setBillingWarning(null);
      return result;
    } catch (err) {
      console.error('Failed to send message:', err);
      setSending(false);
      // Mark message as failed
      setFailedMessages((prev) => new Set(prev).add(userMsg.id));

      // Check for billing/rate limit errors
      if (isApiError(err)) {
        if (err.errorType === 'billing_error') {
          setBillingWarning(err.message);
          showToast('Credit balance too low', 'error');
          return;
        }
        if (err.errorType === 'rate_limit') {
          showToast('Rate limit exceeded - please wait and try again', 'error');
          return;
        }
      }

      showToast('Failed to send message', 'error');
    }
  };

  const handleRetry = (msg: QuestMessage) => {
    // Remove from failed set and resend
    setFailedMessages((prev) => {
      const next = new Set(prev);
      next.delete(msg.id);
      return next;
    });
    // Remove the failed message
    setMessages((prev) => prev.filter((m) => m.id !== msg.id));
    // Resend
    handleSend(msg.content);
  };

  const handleAcceptDraft = async (draftKey: string, draft: ObjectiveDraft) => {
    if (!id) return;
    try {
      // Accept all optional items by default
      const selectedOptional = (draft.checklist.optional || []).map((_, i) => i);
      const result = await createObjective(id, draft, selectedOptional);

      // Move from pending to accepted
      setPendingDrafts((prev) => {
        const next = new Map(prev);
        next.delete(draftKey);
        return next;
      });
      setAcceptedDrafts((prev) => {
        const next = new Map(prev);
        next.set(draftKey, { draft, taskId: result.task?.ID });
        return next;
      });

      showToast('Objective created', 'success');
      loadData();
    } catch (err) {
      console.error('Failed to create objective:', err);
      showToast('Failed to create objective', 'error');
    }
  };

  const handleAcceptAll = async () => {
    if (!id || pendingDrafts.size === 0) return;
    try {
      // Convert all pending drafts to batch format
      const draftsEntries = Array.from(pendingDrafts.entries());
      const draftsArray = draftsEntries.map(([, draft]) => ({
        draft,
        selectedOptional: (draft.checklist.optional || []).map((_, i) => i),
      }));

      await createObjectivesBatch(id, draftsArray);

      // Move all from pending to accepted
      setAcceptedDrafts((prev) => {
        const next = new Map(prev);
        draftsEntries.forEach(([key, draft]) => {
          next.set(key, { draft });
        });
        return next;
      });
      setPendingDrafts(new Map());

      showToast(`Created ${draftsArray.length} objectives`, 'success');
      loadData();
    } catch (err) {
      console.error('Failed to create objectives:', err);
      showToast('Failed to create objectives', 'error');
    }
  };

  const handleRejectDraft = (draftKey: string) => {
    setPendingDrafts((prev) => {
      const next = new Map(prev);
      next.delete(draftKey);
      return next;
    });
  };

  const handleAnswerQuestion = async (answer: string, optionId: string) => {
    // Move question to answered list
    if (pendingQuestions.length > 0) {
      const question = pendingQuestions[0];
      setAnsweredQuestions((prev) => [...prev, { question, answerId: optionId, answer }]);
      setPendingQuestions((prev) => prev.slice(1));
    }
    await handleSend(answer);
  };

  if (loading) {
    return (
      <div className="v2-root">
        <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={0} />
        <main className="v2-content">
          <SkeletonMessage />
          <SkeletonMessage />
          <SkeletonMessage />
        </main>
      </div>
    );
  }

  if (!quest) {
    return (
      <div className="v2-root">
        <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={0} />
        <main className="v2-content">
          <p className="v2-empty-hint">Quest not found</p>
        </main>
      </div>
    );
  }

  return (
    <div className="v2-root v2-quest-layout">
      <Header backLink={{ to: '/v2', label: 'Back' }} inboxCount={approvalCount} />

      <main className="v2-quest-main">
        {/* Billing warning banner */}
        {billingWarning && (
          <div className="v2-warning-banner">
            <span className="v2-warning-icon">!</span>
            <span className="v2-warning-text">{billingWarning}</span>
            <a
              href="https://console.anthropic.com"
              target="_blank"
              rel="noopener noreferrer"
              className="v2-warning-link"
            >
              Add credits
            </a>
            <button
              className="v2-warning-dismiss"
              onClick={() => setBillingWarning(null)}
              aria-label="Dismiss warning"
            >
              x
            </button>
          </div>
        )}

        {/* Quest header */}
        <div className="v2-quest-header">
          <h1 className="v2-page-title">
            {quest.title || 'Untitled Quest'}
          </h1>
          <QuestObjectivesList tasks={tasks} />
        </div>

        {/* Conversation */}
        <MessageList
          messages={messages}
          failedMessages={failedMessages}
          activeTools={activeTools}
          streamingContent={streamingContent}
          sending={sending}
          pendingDrafts={pendingDrafts}
          pendingQuestions={pendingQuestions}
          answeredQuestions={answeredQuestions}
          acceptedDrafts={acceptedDrafts}
          onRetry={handleRetry}
          onCopy={handleCopyMessage}
          onAcceptDraft={handleAcceptDraft}
          onRejectDraft={handleRejectDraft}
          onAnswerQuestion={handleAnswerQuestion}
          onAcceptAll={pendingDrafts.size > 1 ? handleAcceptAll : undefined}
        />

        {/* Input */}
        <ChatInput
          onSend={handleSend}
          onStop={handleStop}
          disabled={sending && !isConnected}
          isGenerating={sending}
          isConnected={isConnected}
          placeholder="Type a message..."
          commandHistory={commandHistory}
        />
      </main>

      {/* Keyboard shortcuts modal */}
      <KeyboardShortcuts isOpen={showShortcuts} onClose={() => setShowShortcuts(false)} />
    </div>
  );
}
