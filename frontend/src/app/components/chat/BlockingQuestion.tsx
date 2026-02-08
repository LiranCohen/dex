import { useState, useCallback, useRef, useEffect } from 'react';
import type { PendingQuestion, PendingQuestionOption } from '../../../lib/types';

interface BlockingQuestionProps {
  question: PendingQuestion;
  onAnswer: (answer: string, selectedIndices: number[], isCustom: boolean) => void;
  disabled?: boolean;
}

export function BlockingQuestion({
  question,
  onAnswer,
  disabled = false,
}: BlockingQuestionProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const customInputRef = useRef<HTMLInputElement>(null);
  const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set());
  const [showCustomInput, setShowCustomInput] = useState(false);
  const [customInputValue, setCustomInputValue] = useState('');

  const options = question.options || [];
  const allowMultiple = question.allow_multiple ?? false;
  const allowCustom = question.allow_custom ?? true;
  const recommendedIndex = question.recommended_index;

  const handleSelectOption = useCallback((index: number) => {
    if (disabled) return;

    setSelectedIndices((prev) => {
      const next = new Set(prev);
      if (allowMultiple) {
        // Toggle selection for multi-select
        if (next.has(index)) {
          next.delete(index);
        } else {
          next.add(index);
        }
      } else {
        // Single select - replace selection
        next.clear();
        next.add(index);
      }
      return next;
    });
  }, [disabled, allowMultiple]);

  const handleSubmit = useCallback(() => {
    if (disabled) return;

    const indices = Array.from(selectedIndices).sort((a, b) => a - b);
    if (indices.length === 0) return;

    // Build answer text from selected options
    const answerLabels = indices.map((i) => options[i]?.label || '').filter(Boolean);
    const answer = answerLabels.join(', ');

    onAnswer(answer, indices, false);
  }, [disabled, selectedIndices, options, onAnswer]);

  const handleCustomSubmit = useCallback(() => {
    if (disabled) return;
    const trimmed = customInputValue.trim();
    if (!trimmed) return;

    onAnswer(trimmed, [], true);
  }, [disabled, customInputValue, onAnswer]);

  const handleOtherClick = useCallback(() => {
    setShowCustomInput(true);
    setTimeout(() => customInputRef.current?.focus(), 0);
  }, []);

  // Number key shortcuts (1-9)
  useEffect(() => {
    if (disabled) return;
    if (options.length === 0) return;

    const handleKeyDown = (e: KeyboardEvent) => {
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

      // Only handle number keys 1-9
      const keyNum = parseInt(e.key, 10);
      if (!isNaN(keyNum) && keyNum >= 1 && keyNum <= Math.min(options.length, 9)) {
        e.preventDefault();
        handleSelectOption(keyNum - 1);
      }

      // Enter to submit (only for single-select with selection)
      if (e.key === 'Enter' && !allowMultiple && selectedIndices.size > 0) {
        e.preventDefault();
        handleSubmit();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [disabled, options.length, allowMultiple, selectedIndices, handleSelectOption, handleSubmit]);

  return (
    <div
      ref={containerRef}
      className="app-blocking-question"
      role="group"
      aria-labelledby="blocking-question-text"
    >
      <div className="app-blocking-question__header">
        <div className="app-blocking-question__label">
          {question.header || 'Question'}
        </div>
      </div>

      <p id="blocking-question-text" className="app-blocking-question__text">
        {question.question}
      </p>

      <div
        className="app-blocking-question__options"
        role="listbox"
        aria-label="Answer options"
        aria-multiselectable={allowMultiple}
      >
        {options.map((option: PendingQuestionOption, index: number) => {
          const isSelected = selectedIndices.has(index);
          const isRecommended = recommendedIndex === index;

          return (
            <button
              key={index}
              type="button"
              className={`app-blocking-question__option ${isSelected ? 'app-blocking-question__option--selected' : ''} ${isRecommended ? 'app-blocking-question__option--recommended' : ''}`}
              onClick={() => handleSelectOption(index)}
              disabled={disabled}
              role="option"
              aria-selected={isSelected}
            >
              <span className="app-blocking-question__option-key" aria-hidden="true">
                {isSelected ? '✓' : index + 1}
              </span>
              <div className="app-blocking-question__option-content">
                <div className="app-blocking-question__option-label">
                  {option.label}
                  {isRecommended && (
                    <span className="app-blocking-question__recommended-badge">
                      Recommended
                    </span>
                  )}
                </div>
                {option.description && (
                  <div className="app-blocking-question__option-desc">
                    {option.description}
                  </div>
                )}
              </div>
            </button>
          );
        })}

        {/* Custom answer option */}
        {allowCustom && !disabled && (
          <>
            {!showCustomInput ? (
              <button
                type="button"
                className="app-blocking-question__option app-blocking-question__option--other"
                onClick={handleOtherClick}
              >
                <span className="app-blocking-question__option-key" aria-hidden="true">
                  ...
                </span>
                <div className="app-blocking-question__option-content">
                  <div className="app-blocking-question__option-label">Other</div>
                  <div className="app-blocking-question__option-desc">
                    Type your own answer
                  </div>
                </div>
              </button>
            ) : (
              <div className="app-blocking-question__custom-input-container">
                <input
                  ref={customInputRef}
                  type="text"
                  className="app-blocking-question__custom-input"
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
                  className="app-btn app-btn--primary app-blocking-question__custom-submit"
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

      {/* Submit button for multi-select or single-select with selection */}
      {options.length > 0 && !disabled && (
        <div className="app-blocking-question__actions">
          {allowMultiple && (
            <p className="app-blocking-question__hint">
              Select one or more options, then click Submit
            </p>
          )}
          <button
            type="button"
            className="app-btn app-btn--primary app-blocking-question__submit"
            onClick={handleSubmit}
            disabled={selectedIndices.size === 0}
          >
            Submit
          </button>
        </div>
      )}

      {!disabled && options.length > 0 && !allowMultiple && (
        <p className="app-blocking-question__hint">
          Press 1-{Math.min(options.length, 9)} to select, Enter to submit
        </p>
      )}
    </div>
  );
}
