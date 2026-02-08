import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams } from 'react-router-dom';
import {
  Header,
  ChatInput,
  KeyboardShortcuts,
  LoadingState,
  useToast,
  QuestObjectivesList,
  MessageList,
  ConnectionStatusBanner,
  SearchInput,
  BlockingQuestion,
  type AcceptedDraft,
  type MessageErrorInfo,
  type ErrorType,
} from '../components';
import type { SearchInputRef } from '../components/SearchInput';

import { fetchQuest, fetchQuestTasks, sendQuestMessage, createObjective, createObjectivesBatch, fetchApprovals, cancelQuestSession, answerQuestQuestion, isApiError } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';
import type { Quest, QuestMessage, Task, Approval, WebSocketEvent, ObjectiveDraft, PendingQuestion } from '../../lib/types';

// Type guards for WebSocket payloads
function isQuestMessage(obj: unknown): obj is QuestMessage {
  return typeof obj === 'object' && obj !== null &&
    'id' in obj && 'role' in obj && 'content' in obj;
}

function isString(val: unknown): val is string {
  return typeof val === 'string';
}

function isBoolean(val: unknown): val is boolean {
  return typeof val === 'boolean';
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
  const [acceptedDrafts, setAcceptedDrafts] = useState<Map<string, AcceptedDraft>>(new Map());
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [commandHistory, setCommandHistory] = useState<string[]>([]);
  const [failedMessages, setFailedMessages] = useState<Set<string>>(new Set());
  const [messageErrors, setMessageErrors] = useState<Map<string, MessageErrorInfo>>(new Map());
  const [billingWarning, setBillingWarning] = useState<string | null>(null);
  const [acceptingDrafts, setAcceptingDrafts] = useState<Set<string>>(new Set());
  const [acceptingAll, setAcceptingAll] = useState(false);
  // Track recently rejected drafts for undo (key -> { draft, timer })
  const [rejectedDrafts, setRejectedDrafts] = useState<Map<string, ObjectiveDraft>>(new Map());
  // Queue messages typed while disconnected
  const [queuedMessages, setQueuedMessages] = useState<string[]>([]);
  // Queue draft acceptances while disconnected
  const [queuedDraftAccepts, setQueuedDraftAccepts] = useState<Array<{
    draftKey: string;
    draft: ObjectiveDraft;
    selectedOptionalIndices: number[];
  }>>([]);
  // Search state
  const [searchQuery, setSearchQuery] = useState('');
  const [showSearch, setShowSearch] = useState(false);
  const searchInputRef = useRef<SearchInputRef>(null);
  // Blocking tool question (from ask_question tool)
  const [blockingQuestion, setBlockingQuestion] = useState<PendingQuestion | null>(null);
  const [answeringQuestion, setAnsweringQuestion] = useState(false);
  const { subscribe, subscribeToChannel, connected: isConnected, connectionState, connectionQuality, latency, reconnectAttempts, reconnect } = useWebSocket();

  // Subscribe to quest-specific channel for targeted updates
  useEffect(() => {
    if (!id) return;
    return subscribeToChannel(`quest:${id}`);
  }, [id, subscribeToChannel]);
  const { showToast } = useToast();

  // Refs for synchronous access to latest state (avoids race conditions)
  const acceptedDraftsRef = useRef<Map<string, AcceptedDraft>>(acceptedDrafts);
  const toolCleanupTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const rejectedDraftTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const loadDataDebounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastLoadTimestamp = useRef<number>(0);
  const isMountedRef = useRef(true);
  // Track in-flight draft acceptance requests synchronously to prevent duplicates
  const inFlightAcceptsRef = useRef<Set<string>>(new Set());

  // Keep ref in sync with state
  useEffect(() => {
    acceptedDraftsRef.current = acceptedDrafts;
  }, [acceptedDrafts]);

  // Track mount state and cleanup timers on unmount
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
      if (loadDataDebounceTimer.current) {
        clearTimeout(loadDataDebounceTimer.current);
      }
      // Clear all tool cleanup timers
      toolCleanupTimers.current.forEach((timer) => clearTimeout(timer));
      toolCleanupTimers.current.clear();
      // Clear all rejected draft timers
      rejectedDraftTimers.current.forEach((timer) => clearTimeout(timer));
      rejectedDraftTimers.current.clear();
    };
  }, []);

  // Keyboard navigation
  useKeyboardNavigation({
    onHelp: () => setShowShortcuts(true),
    enabled: true,
  });

  // "/" keyboard shortcut to toggle search
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }
      if (e.key === '/') {
        e.preventDefault();
        setShowSearch(true);
        // Focus search input after state update
        setTimeout(() => {
          searchInputRef.current?.focus();
        }, 0);
      }
      // Escape to close search
      if (e.key === 'Escape' && showSearch) {
        setShowSearch(false);
        setSearchQuery('');
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [showSearch]);

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

    // Track this load's timestamp to prevent stale data
    const loadTimestamp = Date.now();
    lastLoadTimestamp.current = loadTimestamp;

    try {
      const [questData, tasksData, approvalsData] = await Promise.all([
        fetchQuest(id),
        fetchQuestTasks(id),
        fetchApprovals(),
      ]);

      // Don't update state if a newer load has started
      if (lastLoadTimestamp.current !== loadTimestamp) {
        return;
      }

      setQuest(questData.quest);
      setMessages(questData.messages || []);
      setTasks(tasksData || []);
      setApprovalCount((approvalsData.approvals || []).filter((a: Approval) => a.status === 'pending').length);

      // Note: Pending drafts and questions now come from WebSocket events via tools
      // (propose_objective broadcasts quest.objective_draft, ask_question broadcasts quest.question)
    } catch (err) {
      console.error('Failed to load quest:', err);
      showToast('Failed to load quest', 'error');
    } finally {
      setLoading(false);
    }
  }, [id, showToast]);

  // Debounced loadData for WebSocket events - prevents rapid reloads
  const debouncedLoadData = useCallback(() => {
    if (loadDataDebounceTimer.current) {
      clearTimeout(loadDataDebounceTimer.current);
    }
    loadDataDebounceTimer.current = setTimeout(() => {
      loadData();
      loadDataDebounceTimer.current = null;
    }, 500); // 500ms debounce
  }, [loadData]);

  // Define handleSend before useEffects that use it
  const handleSend = useCallback(async (content: string) => {
    if (!id || sending) return;

    // If disconnected, queue the message for later
    if (!isConnected) {
      setQueuedMessages((prev) => [...prev, content]);
      showToast('Message queued - will send when reconnected', 'info');
      return;
    }

    setSending(true);
    setPendingDrafts(new Map());

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
      console.error('Failed to send message:', { messageId: userMsg.id, error: err });
      setSending(false);

      // Mark message as failed
      setFailedMessages((prev) => new Set(prev).add(userMsg.id));

      // Determine error type and create detailed error info
      let errorInfo: MessageErrorInfo;

      if (isApiError(err)) {
        const errorType = (err.errorType || 'unknown') as ErrorType;
        const retryable = err.retryable !== false && errorType !== 'billing_error';

        errorInfo = {
          type: errorType,
          message: err.message,
          retryable,
          details: err.data ? JSON.stringify(err.data) : undefined,
        };

        // Show specific toast messages
        if (errorType === 'billing_error') {
          setBillingWarning(err.message);
          showToast('Credit balance too low', 'error');
        } else if (errorType === 'rate_limit') {
          showToast('Rate limit exceeded - please wait and try again', 'error');
        } else {
          showToast('Failed to send message', 'error');
        }
      } else if (err instanceof TypeError && err.message.includes('fetch')) {
        // Network error
        errorInfo = {
          type: 'network',
          message: 'Unable to reach the server. Check your connection.',
          retryable: true,
        };
        showToast('Network error - check your connection', 'error');
      } else {
        // Unknown error
        errorInfo = {
          type: 'unknown',
          message: err instanceof Error ? err.message : 'An unexpected error occurred',
          retryable: true,
        };
        showToast('Failed to send message', 'error');
      }

      // Store error info for the message
      setMessageErrors((prev) => {
        const next = new Map(prev);
        next.set(userMsg.id, errorInfo);
        return next;
      });
    }
  }, [id, sending, isConnected, showToast]);

  // Define handleAcceptDraft before useEffects that use it
  const handleAcceptDraft = useCallback(async (draftKey: string, draft: ObjectiveDraft, selectedOptionalIndices: number[]) => {
    if (!id) return;

    // Synchronous deduplication check using ref (prevents race conditions)
    if (inFlightAcceptsRef.current.has(draftKey) || acceptedDraftsRef.current.has(draft.draft_id)) {
      return;
    }

    // If disconnected, queue the draft acceptance for later
    if (!isConnected) {
      // Remove from pending (optimistic UI)
      setPendingDrafts((prev) => {
        const next = new Map(prev);
        next.delete(draftKey);
        return next;
      });
      // Add to queue
      setQueuedDraftAccepts((prev) => [...prev, { draftKey, draft, selectedOptionalIndices }]);
      showToast('Objective queued - will create when reconnected', 'info');
      return;
    }

    // Mark as in-flight synchronously BEFORE any async work
    inFlightAcceptsRef.current.add(draftKey);

    // Mark draft as being accepted (for UI state)
    setAcceptingDrafts((prev) => new Set(prev).add(draftKey));

    // Optimistic UI: remove from pending immediately
    setPendingDrafts((prev) => {
      const next = new Map(prev);
      next.delete(draftKey);
      return next;
    });

    try {
      // Use the selected optional indices from the user
      const result = await createObjective(id, draft, selectedOptionalIndices);

      // Move to accepted
      setAcceptedDrafts((prev) => {
        const next = new Map(prev);
        next.set(draft.draft_id, { draft, taskId: result.task?.ID });
        return next;
      });

      showToast('Objective created', 'success');
      // No explicit loadData() - WebSocket task.created event will trigger debouncedLoadData()
    } catch (err) {
      console.error('Failed to create objective:', err);
      showToast('Failed to create objective', 'error');
      // Rollback: restore to pending drafts on error
      setPendingDrafts((prev) => {
        const next = new Map(prev);
        next.set(draftKey, draft);
        return next;
      });
    } finally {
      // Clear in-flight tracking
      inFlightAcceptsRef.current.delete(draftKey);
      setAcceptingDrafts((prev) => {
        const next = new Set(prev);
        next.delete(draftKey);
        return next;
      });
    }
  }, [id, isConnected, showToast]);

  // Clear pending debounce timer when loadData dependency changes
  useEffect(() => {
    return () => {
      if (loadDataDebounceTimer.current) {
        clearTimeout(loadDataDebounceTimer.current);
        loadDataDebounceTimer.current = null;
      }
    };
  }, [loadData]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Flush queued messages when reconnected
  useEffect(() => {
    if (isConnected && queuedMessages.length > 0 && !sending) {
      // Take the first queued message and send it
      const [firstMessage, ...rest] = queuedMessages;
      setQueuedMessages(rest);

      // Small delay to ensure connection is stable
      const timer = setTimeout(() => {
        if (isMountedRef.current && firstMessage) {
          handleSend(firstMessage);
        }
      }, 500);

      return () => clearTimeout(timer);
    }
  }, [isConnected, queuedMessages, sending, handleSend]);

  // Flush queued draft acceptances when reconnected
  useEffect(() => {
    if (isConnected && queuedDraftAccepts.length > 0) {
      // Take the first queued draft and process it
      const [firstDraft, ...rest] = queuedDraftAccepts;
      setQueuedDraftAccepts(rest);

      // Small delay to ensure connection is stable
      const timer = setTimeout(() => {
        if (isMountedRef.current && firstDraft) {
          handleAcceptDraft(firstDraft.draftKey, firstDraft.draft, firstDraft.selectedOptionalIndices);
        }
      }, 500);

      return () => clearTimeout(timer);
    }
  }, [isConnected, queuedDraftAccepts, handleAcceptDraft]);

  // WebSocket events
  useEffect(() => {
    const unsubscribe = subscribe((event: WebSocketEvent) => {
      // Skip state updates if component is unmounted
      if (!id || !isMountedRef.current) return;

      // Event data is flat (quest_id, message, content, etc. are top-level properties)
      // Cast to Record for property access since WebSocketEvent type doesn't include all fields
      const eventData = event as unknown as Record<string, unknown>;

      switch (event.type) {
        case 'quest.message':
          if (eventData.quest_id === id && isQuestMessage(eventData.message)) {
            const msg = eventData.message;
            if (msg.role === 'assistant') {
              setStreamingContent('');
              setMessages((prev) => [...prev, msg]);
              setSending(false);
              // Note: Drafts and questions are now handled via tool events
              // (quest.objective_draft and quest.question)
            }
          }
          break;

        case 'quest.objective_draft':
          // Handle objective drafts from propose_objective tool
          if (eventData.quest_id === id) {
            const draft = eventData.draft as ObjectiveDraft | undefined;
            if (draft && draft.draft_id && draft.title) {
              // Add to pending drafts if not already accepted
              const currentAccepted = acceptedDraftsRef.current;
              if (!currentAccepted.has(draft.draft_id)) {
                setPendingDrafts((prev) => {
                  const next = new Map(prev);
                  next.set(draft.draft_id, draft);
                  return next;
                });
              }
            }
          }
          break;

        case 'quest.content_delta':
          if (eventData.quest_id === id && isString(eventData.content)) {
            setStreamingContent(eventData.content);
          }
          break;

        case 'quest.tool_call':
          if (eventData.quest_id === id && isString(eventData.tool_name)) {
            const toolName = eventData.tool_name;
            // Use call_id for pairing if available, fall back to tool_name for backwards compat
            const callId = isString(eventData.call_id) ? eventData.call_id : toolName;
            // Clear streaming content when tools start - we're in tool execution phase
            setStreamingContent('');
            // Cancel any pending cleanup timer for this tool
            const existingTimer = toolCleanupTimers.current.get(callId);
            if (existingTimer) {
              clearTimeout(existingTimer);
              toolCleanupTimers.current.delete(callId);
            }
            // Only update if tool is not already in running state
            setActiveTools((prev) => {
              const existing = prev.get(callId);
              if (existing?.status === 'running') {
                return prev; // No change needed
              }
              const next = new Map(prev);
              next.set(callId, { tool: toolName, status: 'running' });
              return next;
            });
          }
          break;

        case 'quest.tool_result':
          if (eventData.quest_id === id && isString(eventData.tool_name)) {
            const toolName = eventData.tool_name;
            // Use call_id for pairing if available, fall back to tool_name for backwards compat
            const callId = isString(eventData.call_id) ? eventData.call_id : toolName;
            const isError = isBoolean(eventData.is_error) ? eventData.is_error : false;
            const newStatus = isError ? 'error' : 'complete';
            // Only update if status actually changed
            setActiveTools((prev) => {
              const existing = prev.get(callId);
              if (existing?.status === newStatus) {
                return prev; // No change needed
              }
              const next = new Map(prev);
              next.set(callId, { tool: toolName, status: newStatus });
              return next;
            });
            // Clear completed/errored tools after showing result briefly
            // Use ref to track timers and prevent race conditions
            const existingTimer = toolCleanupTimers.current.get(callId);
            if (existingTimer) {
              clearTimeout(existingTimer);
            }
            const timer = setTimeout(() => {
              // Skip if unmounted
              if (!isMountedRef.current) return;
              setActiveTools((prev) => {
                const next = new Map(prev);
                const tool = next.get(callId);
                if (tool && tool.status !== 'running') {
                  next.delete(callId);
                }
                return next;
              });
              toolCleanupTimers.current.delete(callId);
            }, 3000);
            toolCleanupTimers.current.set(callId, timer);
          }
          break;

        case 'quest.question':
          // Blocking tool question from ask_question tool
          if (eventData.quest_id === id) {
            // Build PendingQuestion from event data
            const question: PendingQuestion = {
              call_id: isString(eventData.call_id) ? eventData.call_id : '',
              question: isString(eventData.question) ? eventData.question : '',
              header: isString(eventData.header) ? eventData.header : undefined,
              options: Array.isArray(eventData.options) ? eventData.options.map((opt: unknown) => {
                if (typeof opt === 'object' && opt !== null) {
                  const o = opt as Record<string, unknown>;
                  return {
                    label: typeof o.label === 'string' ? o.label : '',
                    description: typeof o.description === 'string' ? o.description : undefined,
                  };
                }
                return { label: String(opt) };
              }) : undefined,
              allow_multiple: isBoolean(eventData.allow_multiple) ? eventData.allow_multiple : undefined,
              allow_custom: isBoolean(eventData.allow_custom) ? eventData.allow_custom : undefined,
              recommended_index: typeof eventData.recommended_index === 'number' ? eventData.recommended_index : undefined,
            };
            if (question.call_id && question.question) {
              setBlockingQuestion(question);
            }
          }
          break;

        case 'task.created':
        case 'task.updated': {
          // Only reload if the task belongs to this quest or if we can't determine
          const task = eventData.task as { QuestID?: string | null } | undefined;
          const taskQuestId = task?.QuestID;
          // Reload if: task belongs to this quest, or payload doesn't include quest info (backwards compat)
          if (taskQuestId === id || taskQuestId === undefined) {
            debouncedLoadData();
          }
          break;
        }
      }
    });

    return unsubscribe;
  }, [id, subscribe, loadData, debouncedLoadData]);

  const handleRetry = (msg: QuestMessage) => {
    // Check connection before retry
    if (!isConnected) {
      showToast('Cannot retry while disconnected', 'error');
      return;
    }

    // Remove from failed set
    setFailedMessages((prev) => {
      const next = new Set(prev);
      next.delete(msg.id);
      return next;
    });
    // Remove error info
    setMessageErrors((prev) => {
      const next = new Map(prev);
      next.delete(msg.id);
      return next;
    });
    // Remove the failed message
    setMessages((prev) => prev.filter((m) => m.id !== msg.id));
    // Resend
    handleSend(msg.content);
  };

  const handleAcceptAll = async () => {
    if (!id || pendingDrafts.size === 0 || acceptingAll) return;

    setAcceptingAll(true);

    // Capture drafts before clearing for potential rollback
    const draftsEntries = Array.from(pendingDrafts.entries());
    const draftsArray = draftsEntries.map(([, draft]) => ({
      draft,
      selectedOptional: (draft.checklist?.optional || []).map((_, i) => i),
    }));

    // Optimistic UI: clear pending immediately
    setPendingDrafts(new Map());

    try {
      await createObjectivesBatch(id, draftsArray);

      // Move all to accepted - use draft_id as key for persistence
      setAcceptedDrafts((prev) => {
        const next = new Map(prev);
        draftsEntries.forEach(([, draft]) => {
          next.set(draft.draft_id, { draft });
        });
        return next;
      });

      showToast(`Created ${draftsArray.length} objectives`, 'success');
      // No explicit loadData() - WebSocket task.created events will trigger debouncedLoadData()
    } catch (err) {
      console.error('Failed to create objectives:', err);
      showToast('Failed to create objectives', 'error');
      // Rollback: restore pending drafts on error
      setPendingDrafts((prev) => {
        const next = new Map(prev);
        draftsEntries.forEach(([key, draft]) => {
          next.set(key, draft);
        });
        return next;
      });
    } finally {
      setAcceptingAll(false);
    }
  };

  const handleRejectAll = () => {
    if (pendingDrafts.size === 0) return;

    // Move all pending drafts to rejected (for undo)
    const draftsEntries = Array.from(pendingDrafts.entries());

    // Clear pending
    setPendingDrafts(new Map());

    // Add all to rejected for undo
    setRejectedDrafts((prev) => {
      const next = new Map(prev);
      draftsEntries.forEach(([key, draft]) => {
        next.set(key, draft);
      });
      return next;
    });

    // Set timers for each rejected draft
    draftsEntries.forEach(([draftKey]) => {
      // Clear any existing timer for this draft
      const existingTimer = rejectedDraftTimers.current.get(draftKey);
      if (existingTimer) {
        clearTimeout(existingTimer);
      }

      // Set timer to permanently remove after 15 seconds
      const timer = setTimeout(() => {
        if (!isMountedRef.current) return;
        setRejectedDrafts((prev) => {
          const next = new Map(prev);
          next.delete(draftKey);
          return next;
        });
        rejectedDraftTimers.current.delete(draftKey);
      }, 15000);
      rejectedDraftTimers.current.set(draftKey, timer);
    });

    showToast(`Rejected ${draftsEntries.length} objectives`, 'info');
  };

  const handleRejectDraft = (draftKey: string) => {
    // Get the draft before removing
    const draft = pendingDrafts.get(draftKey);
    if (!draft) return;

    // Remove from pending
    setPendingDrafts((prev) => {
      const next = new Map(prev);
      next.delete(draftKey);
      return next;
    });

    // Add to rejected for undo
    setRejectedDrafts((prev) => {
      const next = new Map(prev);
      next.set(draftKey, draft);
      return next;
    });

    // Clear any existing timer for this draft
    const existingTimer = rejectedDraftTimers.current.get(draftKey);
    if (existingTimer) {
      clearTimeout(existingTimer);
    }

    // Set timer to permanently remove after 15 seconds
    const timer = setTimeout(() => {
      if (!isMountedRef.current) return;
      setRejectedDrafts((prev) => {
        const next = new Map(prev);
        next.delete(draftKey);
        return next;
      });
      rejectedDraftTimers.current.delete(draftKey);
    }, 15000);
    rejectedDraftTimers.current.set(draftKey, timer);
  };

  const handleUndoReject = (draftKey: string) => {
    const draft = rejectedDrafts.get(draftKey);
    if (!draft) return;

    // Clear the expiration timer
    const timer = rejectedDraftTimers.current.get(draftKey);
    if (timer) {
      clearTimeout(timer);
      rejectedDraftTimers.current.delete(draftKey);
    }

    // Remove from rejected
    setRejectedDrafts((prev) => {
      const next = new Map(prev);
      next.delete(draftKey);
      return next;
    });

    // Restore to pending
    setPendingDrafts((prev) => {
      const next = new Map(prev);
      next.set(draftKey, draft);
      return next;
    });

    showToast('Objective restored', 'info');
  };

  // Handle answering a blocking tool question (from ask_question tool)
  const handleAnswerBlockingQuestion = useCallback(async (answer: string, selectedIndices: number[], isCustom: boolean) => {
    if (!id || !blockingQuestion || answeringQuestion) return;

    setAnsweringQuestion(true);
    try {
      await answerQuestQuestion(id, {
        answer,
        selected_indices: selectedIndices.length > 0 ? selectedIndices : undefined,
        is_custom: isCustom,
      });
      // Clear the blocking question after successful answer
      setBlockingQuestion(null);
    } catch (err) {
      console.error('Failed to answer question:', err);
      showToast('Failed to submit answer', 'error');
    } finally {
      setAnsweringQuestion(false);
    }
  }, [id, blockingQuestion, answeringQuestion, showToast]);

  // Filter messages by search query
  const filteredMessages = useMemo(() => {
    if (!searchQuery.trim()) return messages;
    const query = searchQuery.toLowerCase();
    return messages.filter((msg) => msg.content.toLowerCase().includes(query));
  }, [messages, searchQuery]);

  // Calculate search match count for display
  const searchMatchCount = searchQuery.trim() ? filteredMessages.length : 0;

  // Export conversation as markdown
  const handleExport = useCallback(() => {
    if (!quest) return;

    const timestamp = new Date().toISOString().split('T')[0];
    const filename = `${quest.title?.replace(/[^a-z0-9]/gi, '-').toLowerCase() || 'quest'}-${timestamp}.md`;

    // Build markdown content
    const lines: string[] = [];
    lines.push(`# ${quest.title || 'Untitled Quest'}`);
    lines.push('');
    lines.push(`**Exported:** ${new Date().toLocaleString()}`);
    lines.push('');

    // Objectives section
    if (tasks.length > 0) {
      lines.push('## Objectives');
      lines.push('');
      tasks.forEach((task) => {
        const statusIcon = task.Status === 'completed' ? '✓' : task.Status === 'running' ? '⟳' : '○';
        lines.push(`- ${statusIcon} **${task.Title}** (${task.Status})`);
        if (task.Description) {
          lines.push(`  ${task.Description}`);
        }
      });
      lines.push('');
    }

    // Conversation section
    lines.push('## Conversation');
    lines.push('');
    messages.forEach((msg) => {
      const time = new Date(msg.created_at).toLocaleString();
      const role = msg.role === 'user' ? 'You' : 'Dex';
      lines.push(`### ${role} (${time})`);
      lines.push('');
      lines.push(msg.content);
      lines.push('');
      lines.push('---');
      lines.push('');
    });

    // Create and download the file
    const content = lines.join('\n');
    const blob = new Blob([content], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    showToast('Conversation exported', 'success');
  }, [quest, tasks, messages, showToast]);

  if (loading) {
    return (
      <div className="app-root">
        <Header backLink={{ to: '/', label: 'Back' }} inboxCount={0} />
        <main className="app-content">
          <LoadingState message="Loading conversation..." size="large" />
        </main>
      </div>
    );
  }

  if (!quest) {
    return (
      <div className="app-root">
        <Header backLink={{ to: '/', label: 'Back' }} inboxCount={0} />
        <main className="app-content">
          <p className="app-empty-hint">Quest not found</p>
        </main>
      </div>
    );
  }

  return (
    <div className="app-root app-quest-layout">
      <Header backLink={{ to: '/', label: 'Back' }} inboxCount={approvalCount} />

      <main className="app-quest-main">
        {/* Connection status banner */}
        <ConnectionStatusBanner
          connectionState={connectionState}
          connectionQuality={connectionQuality}
          latency={latency}
          reconnectAttempts={reconnectAttempts}
          onReconnect={reconnect}
        />

        {/* Billing warning banner */}
        {billingWarning && (
          <div className="app-warning-banner">
            <span className="app-warning-icon">!</span>
            <span className="app-warning-text">{billingWarning}</span>
            <a
              href="https://console.anthropic.com"
              target="_blank"
              rel="noopener noreferrer"
              className="app-warning-link"
            >
              Add credits
            </a>
            <button
              className="app-warning-dismiss"
              onClick={() => setBillingWarning(null)}
              aria-label="Dismiss warning"
            >
              x
            </button>
          </div>
        )}

        {/* Quest header */}
        <div className="app-quest-header">
          <div className="app-quest-header__top">
            <h1 className="app-page-title">
              {quest.title || 'Untitled Quest'}
            </h1>
            <div className="app-quest-header__actions">
              <button
                className={`app-btn app-btn--ghost app-quest-search-toggle ${showSearch ? 'app-quest-search-toggle--active' : ''}`}
                onClick={() => {
                  setShowSearch(!showSearch);
                  if (!showSearch) {
                    setTimeout(() => searchInputRef.current?.focus(), 0);
                  } else {
                    setSearchQuery('');
                  }
                }}
                aria-label="Toggle search"
                title="Search messages (/)"
              >
                /
              </button>
              <button
                className="app-btn app-btn--ghost"
                onClick={handleExport}
                disabled={messages.length === 0}
                aria-label="Export conversation"
                title="Export as markdown"
              >
                Export
              </button>
            </div>
          </div>
          {showSearch && (
            <div className="app-quest-search">
              <SearchInput
                ref={searchInputRef}
                value={searchQuery}
                onChange={setSearchQuery}
                placeholder="Search messages..."
                onEscape={() => {
                  setShowSearch(false);
                  setSearchQuery('');
                }}
              />
              {searchQuery.trim() && (
                <span className="app-quest-search__count">
                  {searchMatchCount} {searchMatchCount === 1 ? 'match' : 'matches'}
                </span>
              )}
            </div>
          )}
          <QuestObjectivesList tasks={tasks} />
        </div>

        {/* Conversation */}
        <MessageList
          messages={filteredMessages}
          searchQuery={searchQuery}
          failedMessages={failedMessages}
          messageErrors={messageErrors}
          activeTools={activeTools}
          streamingContent={streamingContent}
          sending={sending}
          pendingDrafts={pendingDrafts}
          acceptedDrafts={acceptedDrafts}
          rejectedDrafts={rejectedDrafts}
          acceptingDrafts={acceptingDrafts}
          acceptingAll={acceptingAll}
          onRetry={handleRetry}
          onCopy={handleCopyMessage}
          onAcceptDraft={handleAcceptDraft}
          onRejectDraft={handleRejectDraft}
          onUndoReject={handleUndoReject}
          onAcceptAll={pendingDrafts.size > 1 ? handleAcceptAll : undefined}
          onRejectAll={pendingDrafts.size > 1 ? handleRejectAll : undefined}
        />

        {/* Blocking tool question (from ask_question tool) */}
        {blockingQuestion && (
          <div className="app-quest-blocking-question">
            <BlockingQuestion
              question={blockingQuestion}
              onAnswer={handleAnswerBlockingQuestion}
              disabled={answeringQuestion}
            />
          </div>
        )}

        {/* Input */}
        <ChatInput
          onSend={handleSend}
          onStop={handleStop}
          disabled={sending && !isConnected}
          isGenerating={sending}
          isConnected={isConnected}
          isReconnecting={connectionState === 'reconnecting'}
          placeholder="Type a message..."
          commandHistory={commandHistory}
        />
      </main>

      {/* Keyboard shortcuts modal */}
      <KeyboardShortcuts isOpen={showShortcuts} onClose={() => setShowShortcuts(false)} />
    </div>
  );
}
