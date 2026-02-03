import { useState, useRef, useEffect, useCallback, type KeyboardEvent } from 'react';
import TextareaAutosize from 'react-textarea-autosize';

interface ChatInputProps {
  onSend: (message: string) => void;
  onStop?: () => void;
  disabled?: boolean;
  isGenerating?: boolean;
  isConnected?: boolean;
  placeholder?: string;
  commandHistory?: string[];
}

export function ChatInput({
  onSend,
  onStop,
  disabled = false,
  isGenerating = false,
  isConnected = true,
  placeholder = 'Type a message...',
  commandHistory = [],
}: ChatInputProps) {
  const [value, setValue] = useState('');
  const [historyIndex, setHistoryIndex] = useState(-1);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter to send (without shift)
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
      return;
    }

    // Arrow up for command history (when input is empty or at start)
    if (e.key === 'ArrowUp' && value === '' && commandHistory.length > 0) {
      e.preventDefault();
      const newIndex = historyIndex < commandHistory.length - 1 ? historyIndex + 1 : historyIndex;
      setHistoryIndex(newIndex);
      setValue(commandHistory[commandHistory.length - 1 - newIndex] || '');
      return;
    }

    // Arrow down to go forward in history
    if (e.key === 'ArrowDown' && historyIndex >= 0) {
      e.preventDefault();
      const newIndex = historyIndex > 0 ? historyIndex - 1 : -1;
      setHistoryIndex(newIndex);
      if (newIndex === -1) {
        setValue('');
      } else {
        setValue(commandHistory[commandHistory.length - 1 - newIndex] || '');
      }
      return;
    }

    // Escape to blur and return to keyboard nav
    if (e.key === 'Escape') {
      textareaRef.current?.blur();
      return;
    }
  }, [value, historyIndex, commandHistory]);

  const handleSend = useCallback(() => {
    const trimmed = value.trim();
    if (trimmed && !disabled && !isGenerating) {
      onSend(trimmed);
      setValue('');
      setHistoryIndex(-1);
    }
  }, [value, disabled, isGenerating, onSend]);

  const handleStop = useCallback(() => {
    if (onStop && isGenerating) {
      onStop();
    }
  }, [onStop, isGenerating]);

  // Focus on mount
  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  // Re-focus when generation completes
  useEffect(() => {
    if (!isGenerating && !disabled) {
      textareaRef.current?.focus();
    }
  }, [isGenerating, disabled]);

  // Determine placeholder text
  const getPlaceholder = () => {
    if (!isConnected) return 'Reconnecting...';
    if (isGenerating) return '...';
    return placeholder;
  };

  const canSend = value.trim() && !disabled && !isGenerating && isConnected;
  const showStop = isGenerating && onStop;

  return (
    <div className="v2-chat-input-wrapper">
      <div
        ref={containerRef}
        className={`v2-chat-input ${!isConnected ? 'v2-chat-input--disconnected' : ''}`}
      >
        <span className="v2-chat-input__cursor">▌</span>
        <TextareaAutosize
          ref={textareaRef}
          className="v2-chat-input__field"
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            setHistoryIndex(-1);
          }}
          onKeyDown={handleKeyDown}
          placeholder={getPlaceholder()}
          disabled={disabled || !isConnected}
          minRows={1}
          maxRows={6}
        />
        {showStop ? (
          <button
            type="button"
            className="v2-btn v2-btn--secondary v2-chat-input__stop"
            onClick={handleStop}
            aria-label="Stop generating"
          >
            Stop
          </button>
        ) : (
          <button
            type="button"
            className="v2-chat-input__send"
            onClick={handleSend}
            disabled={!canSend}
            aria-label="Send message"
          >
            <svg className="v2-chat-input__send-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
            </svg>
          </button>
        )}
      </div>
      <div className="v2-chat-input__hint">
        Enter to send · Shift+Enter for newline · ↑ for history
      </div>
    </div>
  );
}
