import { useState, useEffect, useCallback, useRef } from 'react';

interface QuestionOption {
  id: string;
  title: string;
  description: string;
}

interface QuestionPromptProps {
  question: string;
  options: QuestionOption[];
  onSelect: (optionId: string) => void;
  /** Called when user submits a custom "Other" answer */
  onCustomAnswer?: (answer: string) => void;
  disabled?: boolean;
  /** The ID of the selected answer (when already answered) */
  answeredId?: string;
  /** Custom answer text (when user typed their own response) */
  customAnswer?: string;
  /** Timestamp when the question was asked */
  timestamp?: string;
  /** Iteration or context reference (e.g., "Step 3 of analysis") */
  contextRef?: string;
}

export function QuestionPrompt({
  question,
  options,
  onSelect,
  onCustomAnswer,
  disabled = false,
  answeredId,
  customAnswer,
  timestamp,
  contextRef,
}: QuestionPromptProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const customInputRef = useRef<HTMLInputElement>(null);
  const [selectedId, setSelectedId] = useState<string | null>(answeredId || null);
  const [showCustomInput, setShowCustomInput] = useState(false);
  const [customInputValue, setCustomInputValue] = useState('');
  const isAnswered = answeredId != null || selectedId != null;

  // Check if the answer is a custom text (not matching any predefined option)
  const effectiveSelectedId = answeredId || selectedId;
  const selectedOption = effectiveSelectedId != null ? options.find(opt => opt.id === effectiveSelectedId) : null;
  const isCustomAnswer = isAnswered && customAnswer && (!selectedOption || selectedOption.title !== customAnswer);

  const handleSelect = useCallback((optionId: string) => {
    if (disabled || isAnswered) return;
    setSelectedId(optionId);
    onSelect(optionId);
  }, [disabled, isAnswered, onSelect]);

  const handleCustomSubmit = useCallback(() => {
    if (disabled || isAnswered || !onCustomAnswer) return;
    const trimmed = customInputValue.trim();
    if (!trimmed) return;
    setSelectedId('custom');
    onCustomAnswer(trimmed);
  }, [disabled, isAnswered, onCustomAnswer, customInputValue]);

  const handleOtherClick = useCallback(() => {
    setShowCustomInput(true);
    // Focus the input after it renders
    setTimeout(() => customInputRef.current?.focus(), 0);
  }, []);

  // Number key shortcuts (1-9)
  // Only responds if this component has focus or is the first unanswered question
  useEffect(() => {
    // Don't register shortcuts if already answered or disabled
    if (disabled || selectedId || isAnswered) return;
    // Don't register if no options
    if (options.length === 0) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Use event target as the authoritative source - it's where the event originated
      const target = e.target;

      // Check if user is typing in any text input field
      const isTypingInInput = (el: EventTarget | null): boolean => {
        if (!el || !(el instanceof Element)) return false;
        return (
          el instanceof HTMLInputElement ||
          el instanceof HTMLTextAreaElement ||
          el.getAttribute('contenteditable') === 'true' ||
          el.closest('[contenteditable="true"]') !== null
        );
      };

      if (isTypingInInput(target)) {
        return;
      }

      // Check if this component has focus or contains the focused element
      const hasFocus = containerRef.current?.contains(document.activeElement);

      // If no focus, only respond if this is the first unanswered question in the DOM
      if (!hasFocus) {
        const allUnansweredQuestions = document.querySelectorAll('.app-question:not(.app-question--answered)');
        const isFirst = allUnansweredQuestions.length > 0 && allUnansweredQuestions[0] === containerRef.current;
        if (!isFirst) {
          return;
        }
      }

      // Only handle number keys 1-9
      const keyNum = parseInt(e.key, 10);
      if (!isNaN(keyNum) && keyNum >= 1 && keyNum <= Math.min(options.length, 9)) {
        e.preventDefault();
        handleSelect(options[keyNum - 1].id);
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [disabled, selectedId, isAnswered, options, handleSelect]);

  return (
    <div ref={containerRef} className={`app-question ${isAnswered ? 'app-question--answered' : ''}`} role="group" aria-labelledby="question-text">
      <div className="app-question__header">
        <div className="app-question__label">
          {isAnswered ? 'Answered' : 'Question'}
        </div>
        {(timestamp || contextRef) && (
          <div className="app-question__context">
            {contextRef && <span className="app-question__context-ref">{contextRef}</span>}
            {timestamp && <time className="app-question__context-time">{timestamp}</time>}
          </div>
        )}
      </div>
      <p id="question-text" className="app-question__text">{question}</p>

      <div className="app-question__options" role="listbox" aria-label="Answer options" aria-describedby={!disabled && !isAnswered && options.length > 0 ? "question-hint" : undefined}>
        {options.map((option, index) => {
          const isSelected = effectiveSelectedId === option.id;
          const isNotSelected = isAnswered && !isSelected;

          return (
            <button
              key={option.id}
              type="button"
              className={`app-question__option ${isSelected ? 'app-question__option--selected' : ''} ${isNotSelected ? 'app-question__option--dimmed' : ''}`}
              onClick={() => handleSelect(option.id)}
              disabled={disabled || isAnswered}
              role="option"
              aria-selected={isSelected}
            >
              <span className="app-question__option-key" aria-hidden="true">
                {isSelected ? '✓' : index + 1}
              </span>
              <div>
                <div className="app-question__option-title">{option.title}</div>
                {option.description && (
                  <div className="app-question__option-desc">{option.description}</div>
                )}
              </div>
            </button>
          );
        })}

        {/* Other option with custom input */}
        {!disabled && !isAnswered && onCustomAnswer && (
          <>
            {!showCustomInput ? (
              <button
                type="button"
                className="app-question__option app-question__option--other"
                onClick={handleOtherClick}
              >
                <span className="app-question__option-key" aria-hidden="true">...</span>
                <div>
                  <div className="app-question__option-title">Other</div>
                  <div className="app-question__option-desc">Type your own answer</div>
                </div>
              </button>
            ) : (
              <div className="app-question__custom-input-container">
                <input
                  ref={customInputRef}
                  type="text"
                  className="app-question__custom-input"
                  placeholder="Type your answer..."
                  value={customInputValue}
                  onChange={(e) => setCustomInputValue(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && customInputValue.trim()) {
                      e.preventDefault();
                      handleCustomSubmit();
                    } else if (e.key === 'Escape') {
                      setShowCustomInput(false);
                      setCustomInputValue('');
                    }
                  }}
                />
                <button
                  type="button"
                  className="app-btn app-btn--primary app-question__custom-submit"
                  onClick={handleCustomSubmit}
                  disabled={!customInputValue.trim()}
                >
                  Submit
                </button>
              </div>
            )}
          </>
        )}
      </div>

      {/* Custom answer display when user typed their own response */}
      {isCustomAnswer && (
        <div className="app-question__custom-answer">
          <span className="app-question__custom-label">Your answer:</span>
          <p className="app-question__custom-text">{customAnswer}</p>
        </div>
      )}

      {!disabled && !isAnswered && options.length > 0 && (
        <p id="question-hint" className="app-question__hint">
          Press 1-{Math.min(options.length, 9)} to select
        </p>
      )}
    </div>
  );
}
