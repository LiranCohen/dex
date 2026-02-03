import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Header, StatusBar, Message, ToolActivity, ProposedObjective, QuestionPrompt, ChatInput, ScrollIndicator, KeyboardShortcuts, SkeletonMessage, useToast } from '../components';
import { fetchQuest, fetchQuestTasks, sendQuestMessage, createObjective, fetchApprovals, cancelQuestSession } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';
import type { Quest, QuestMessage, Task, Approval, WebSocketEvent, ObjectiveDraft } from '../../lib/types';
import { parseObjectiveDrafts, parseQuestions, type QuestQuestion } from '../../components/QuestChat';
import ReactMarkdown from 'react-markdown';

function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
}

function getTaskStatus(status: string): 'active' | 'pending' | 'complete' | 'error' {
  switch (status) {
    case 'running':
      return 'active';
    case 'completed':
      return 'complete';
    case 'failed':
    case 'cancelled':
      return 'error';
    default:
      return 'pending';
  }
}

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
  const [showScrollIndicator, setShowScrollIndicator] = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [commandHistory, setCommandHistory] = useState<string[]>([]);
  const [failedMessages, setFailedMessages] = useState<Set<string>>(new Set());
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const { subscribe, connected: isConnected } = useWebSocket();
  const { showToast } = useToast();

  // Keyboard navigation
  useKeyboardNavigation({
    onHelp: () => setShowShortcuts(true),
    enabled: true,
  });

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
    const isAtBottom = checkIfAtBottom();
    if (isAtBottom) {
      setShowScrollIndicator(false);
    }
  }, [checkIfAtBottom]);

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

  useEffect(() => {
    // Auto-scroll if at bottom, otherwise show indicator
    if (checkIfAtBottom()) {
      scrollToBottom();
    } else if (messages.length > 0) {
      setShowScrollIndicator(true);
    }
  }, [messages, streamingContent, checkIfAtBottom]);

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

              // Parse drafts and questions
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

        case 'quest.tool_call':
          if (payload?.quest_id === id) {
            const toolName = payload.tool_name as string;
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
            // Clear after a moment
            setTimeout(() => {
              setActiveTools((prev) => {
                const next = new Map(prev);
                next.delete(toolName);
                return next;
              });
            }, 2000);
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
      await sendQuestMessage(id, content);
    } catch (err) {
      console.error('Failed to send message:', err);
      setSending(false);
      // Mark message as failed
      setFailedMessages((prev) => new Set(prev).add(userMsg.id));
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
      await createObjective(id, draft, selectedOptional);
      setPendingDrafts((prev) => {
        const next = new Map(prev);
        next.delete(draftKey);
        return next;
      });
      showToast('Objective created', 'success');
      loadData();
    } catch (err) {
      console.error('Failed to create objective:', err);
      showToast('Failed to create objective', 'error');
    }
  };

  const handleRejectDraft = (draftKey: string) => {
    setPendingDrafts((prev) => {
      const next = new Map(prev);
      next.delete(draftKey);
      return next;
    });
  };

  const handleAnswerQuestion = async (answer: string) => {
    setPendingQuestions([]);
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
        {/* Quest header */}
        <div className="v2-quest-header">
          <h1 className="v2-page-title">
            {quest.title || 'Untitled Quest'}
          </h1>

          {/* Objectives list */}
          {tasks.length > 0 && (
            <div className="v2-objectives-list">
              <div className="v2-label">Objectives</div>
              <div className="v2-objectives-list__items">
                {tasks.map((task) => (
                  <Link
                    key={task.ID}
                    to={`/v2/objectives/${task.ID}`}
                    className="v2-objective-link"
                  >
                    <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
                    <span className="v2-objective-link__title">{task.Title}</span>
                    <span className="v2-label">{task.Status}</span>
                  </Link>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Conversation */}
        <div
          ref={messagesContainerRef}
          onScroll={handleScroll}
          className="v2-quest-messages"
        >
          {messages.length === 0 && !sending ? (
            <div className="v2-quest-empty">
              <p>Start by describing what you want to accomplish.</p>
            </div>
          ) : (
            <div className="v2-quest-messages__list">
              {messages.map((msg) => {
                const isFailed = failedMessages.has(msg.id);
                return (
                  <Message
                    key={msg.id}
                    sender={msg.role === 'user' ? 'user' : 'assistant'}
                    timestamp={formatTime(msg.created_at)}
                    status={isFailed ? 'error' : undefined}
                    onRetry={isFailed ? () => handleRetry(msg) : undefined}
                    onCopy={msg.role === 'assistant' ? () => handleCopyMessage(msg.content) : undefined}
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
                    onAccept={() => handleAcceptDraft(key, draft)}
                    onReject={() => handleRejectDraft(key)}
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
                    handleAnswerQuestion(answer);
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
          )}

          {/* Scroll indicator */}
          <ScrollIndicator visible={showScrollIndicator} onClick={scrollToBottom} />
        </div>

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
