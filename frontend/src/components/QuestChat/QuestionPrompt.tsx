import { useState, useCallback, memo } from 'react';

interface Question {
  question: string;
  options?: string[];
}

interface QuestionPromptProps {
  question: Question;
  onAnswer: (answer: string) => void;
  disabled?: boolean;
}

export const QuestionPrompt = memo(function QuestionPrompt({
  question,
  onAnswer,
  disabled = false,
}: QuestionPromptProps) {
  const [selectedOption, setSelectedOption] = useState<string | null>(null);
  const [customInput, setCustomInput] = useState('');
  const [showCustomInput, setShowCustomInput] = useState(false);

  const handleOptionClick = useCallback((option: string) => {
    if (disabled) return;

    if (option === '__other__') {
      setShowCustomInput(true);
      setSelectedOption(null);
    } else {
      setSelectedOption(option);
      setShowCustomInput(false);
      onAnswer(option);
    }
  }, [disabled, onAnswer]);

  const handleCustomSubmit = useCallback(() => {
    const trimmed = customInput.trim();
    if (!trimmed || disabled) return;

    setSelectedOption(trimmed);
    onAnswer(trimmed);
  }, [customInput, disabled, onAnswer]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleCustomSubmit();
    }
    if (e.key === 'Escape') {
      setShowCustomInput(false);
      setCustomInput('');
    }
  }, [handleCustomSubmit]);

  const hasOptions = question.options && question.options.length > 0;
  const isAnswered = selectedOption !== null;

  return (
    <div
      className={`
        bg-gradient-to-br from-purple-900/30 to-blue-900/30
        border border-purple-500/30 rounded-lg p-4 my-4
        ${isAnswered ? 'opacity-60' : ''}
      `}
    >
      {/* Question text */}
      <p className="text-gray-200 font-medium mb-4">{question.question}</p>

      {/* Options */}
      {hasOptions && (
        <div className="flex flex-wrap gap-2 mb-3">
          {question.options!.map((option, idx) => (
            <button
              key={idx}
              onClick={() => handleOptionClick(option)}
              disabled={disabled || isAnswered}
              className={`
                px-4 py-2 rounded-lg text-sm font-medium
                transition-all duration-150
                ${selectedOption === option
                  ? 'bg-purple-600 text-white border-purple-500'
                  : 'bg-gray-800 text-gray-300 border-gray-700 hover:bg-gray-700 hover:border-gray-600'
                }
                border
                ${disabled || isAnswered ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}
              `}
            >
              {option}
            </button>
          ))}

          {/* "Other" option */}
          <button
            onClick={() => handleOptionClick('__other__')}
            disabled={disabled || isAnswered}
            className={`
              px-4 py-2 rounded-lg text-sm font-medium
              transition-all duration-150
              ${showCustomInput
                ? 'bg-blue-600 text-white border-blue-500'
                : 'bg-gray-800/50 text-gray-400 border-gray-700 hover:bg-gray-700 hover:border-gray-600'
              }
              border
              ${disabled || isAnswered ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}
            `}
          >
            Other...
          </button>
        </div>
      )}

      {/* Custom input (shown when "Other" is clicked or no options provided) */}
      {(showCustomInput || !hasOptions) && !isAnswered && (
        <div className="flex gap-2">
          <input
            type="text"
            value={customInput}
            onChange={(e) => setCustomInput(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={disabled}
            placeholder="Type your answer..."
            autoFocus
            className={`
              flex-1 px-3 py-2
              bg-gray-800 border border-gray-700 rounded-lg
              text-gray-100 placeholder-gray-500
              focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent
              disabled:opacity-50
            `}
          />
          <button
            onClick={handleCustomSubmit}
            disabled={disabled || !customInput.trim()}
            className={`
              px-4 py-2 rounded-lg font-medium
              transition-colors
              ${disabled || !customInput.trim()
                ? 'bg-gray-700 text-gray-500 cursor-not-allowed'
                : 'bg-purple-600 text-white hover:bg-purple-500'
              }
            `}
          >
            Send
          </button>
        </div>
      )}

      {/* Selected answer display */}
      {isAnswered && (
        <div className="flex items-center gap-2 text-sm text-green-400 mt-2">
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
          </svg>
          <span>Answered: {selectedOption}</span>
        </div>
      )}
    </div>
  );
});
