import { useState, useRef, useCallback, useEffect } from 'react';
import TextareaAutosize from 'react-textarea-autosize';

interface ChatInputProps {
  onSubmit: (content: string) => void;
  disabled?: boolean;
  placeholder?: string;
  autoFocus?: boolean;
}

export function ChatInput({
  onSubmit,
  disabled = false,
  placeholder = "Type a message...",
  autoFocus = true
}: ChatInputProps) {
  const [value, setValue] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Auto-focus when enabled and not disabled
  useEffect(() => {
    if (autoFocus && !disabled && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [autoFocus, disabled]);

  // Focus when becoming enabled after being disabled
  useEffect(() => {
    if (!disabled && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [disabled]);

  const handleSubmit = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed || disabled) return;

    onSubmit(trimmed);
    setValue('');
  }, [value, disabled, onSubmit]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter to submit (without Shift)
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }

    // Escape to clear
    if (e.key === 'Escape') {
      e.preventDefault();
      setValue('');
    }
  }, [handleSubmit]);

  const handleChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setValue(e.target.value);
  }, []);

  return (
    <div className="relative">
      <TextareaAutosize
        ref={textareaRef}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        placeholder={placeholder}
        minRows={1}
        maxRows={8}
        className={`
          w-full px-4 py-3 pr-12
          bg-gray-800 border border-gray-700 rounded-lg
          text-gray-100 placeholder-gray-500
          resize-none
          focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
          disabled:opacity-50 disabled:cursor-not-allowed
          transition-colors
        `}
      />

      {/* Submit button */}
      <button
        onClick={handleSubmit}
        disabled={disabled || !value.trim()}
        className={`
          absolute right-2 bottom-2
          p-2 rounded-lg
          transition-colors
          ${disabled || !value.trim()
            ? 'text-gray-600 cursor-not-allowed'
            : 'text-blue-400 hover:text-blue-300 hover:bg-gray-700'
          }
        `}
        title="Send (Enter)"
      >
        <svg
          className="w-5 h-5"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"
          />
        </svg>
      </button>

      {/* Hint text */}
      <div className="absolute -bottom-5 left-0 text-xs text-gray-600">
        Enter to send, Shift+Enter for newline
      </div>
    </div>
  );
}
