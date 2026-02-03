import { useState, useEffect, useCallback } from 'react';

interface QuestionOption {
  id: string;
  title: string;
  description: string;
}

interface QuestionPromptProps {
  question: string;
  options: QuestionOption[];
  onSelect: (optionId: string) => void;
  disabled?: boolean;
}

export function QuestionPrompt({
  question,
  options,
  onSelect,
  disabled = false,
}: QuestionPromptProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const handleSelect = useCallback((optionId: string) => {
    if (disabled) return;
    setSelectedId(optionId);
    onSelect(optionId);
  }, [disabled, onSelect]);

  // Number key shortcuts (1-9)
  useEffect(() => {
    if (disabled || selectedId) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Ignore if user is typing in an input
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }

      const keyNum = parseInt(e.key, 10);
      if (!isNaN(keyNum) && keyNum >= 1 && keyNum <= options.length) {
        e.preventDefault();
        handleSelect(options[keyNum - 1].id);
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [disabled, selectedId, options, handleSelect]);

  return (
    <div className="v2-question" role="group" aria-labelledby="question-text">
      <div className="v2-question__label">Question</div>
      <p id="question-text" className="v2-question__text">{question}</p>

      <div className="v2-question__options" role="listbox" aria-label="Answer options">
        {options.map((option, index) => (
          <button
            key={option.id}
            type="button"
            className={`v2-question__option ${selectedId === option.id ? 'v2-question__option--selected' : ''}`}
            onClick={() => handleSelect(option.id)}
            disabled={disabled}
            role="option"
            aria-selected={selectedId === option.id}
          >
            <span className="v2-question__option-key" aria-hidden="true">
              {index + 1}
            </span>
            <div>
              <div className="v2-question__option-title">{option.title}</div>
              {option.description && (
                <div className="v2-question__option-desc">{option.description}</div>
              )}
            </div>
          </button>
        ))}
      </div>

      {!disabled && !selectedId && options.length > 0 && (
        <p className="v2-question__hint">
          Press 1-{Math.min(options.length, 9)} to select
        </p>
      )}
    </div>
  );
}
