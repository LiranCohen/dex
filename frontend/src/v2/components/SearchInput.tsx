import { useState, useRef, useEffect, type ChangeEvent, type KeyboardEvent } from 'react';

interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
  onEscape?: () => void;
  className?: string;
}

export function SearchInput({
  value,
  onChange,
  placeholder = 'Search...',
  autoFocus = false,
  onEscape,
  className = '',
}: SearchInputProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [isFocused, setIsFocused] = useState(false);

  useEffect(() => {
    if (autoFocus) {
      inputRef.current?.focus();
    }
  }, [autoFocus]);

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    onChange(e.target.value);
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      if (value) {
        onChange('');
      } else {
        inputRef.current?.blur();
        onEscape?.();
      }
    }
  };

  const handleClear = () => {
    onChange('');
    inputRef.current?.focus();
  };

  return (
    <div className={`v2-search ${isFocused ? 'v2-search--focused' : ''} ${className}`}>
      <span className="v2-search__icon" aria-hidden="true">/</span>
      <input
        ref={inputRef}
        type="text"
        className="v2-search__input"
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
        placeholder={placeholder}
        aria-label={placeholder}
      />
      {value && (
        <button
          type="button"
          className="v2-search__clear"
          onClick={handleClear}
          aria-label="Clear search"
        >
          ×
        </button>
      )}
    </div>
  );
}
