import { useEffect, useRef, useCallback } from 'react';

interface KeyboardShortcutsProps {
  isOpen: boolean;
  onClose: () => void;
}

const shortcuts = [
  { category: 'Global', items: [
    { keys: 'g h', description: 'Go home (quest list)' },
    { keys: 'g i', description: 'Go to inbox' },
    { keys: 'g o', description: 'Go to all objectives' },
    { keys: '/', description: 'Focus search/filter' },
    { keys: '?', description: 'Show keyboard shortcuts' },
    { keys: 'Esc', description: 'Close modal, cancel, go back' },
  ]},
  { category: 'Lists', items: [
    { keys: 'j / k', description: 'Move selection down / up' },
    { keys: 'Enter', description: 'Open selected item' },
  ]},
  { category: 'Quest Detail', items: [
    { keys: 'g g', description: 'Jump to top' },
    { keys: 'G', description: 'Jump to bottom' },
    { keys: 'y', description: 'Accept proposed objective' },
    { keys: 'n', description: 'Reject proposed objective' },
    { keys: '1-9', description: 'Select question option' },
  ]},
  { category: 'Inbox', items: [
    { keys: 'a', description: 'Approve selected' },
    { keys: 'r', description: 'Reject selected' },
  ]},
];

export function KeyboardShortcuts({ isOpen, onClose }: KeyboardShortcutsProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previousActiveElement = useRef<Element | null>(null);

  // Focus management
  useEffect(() => {
    if (isOpen) {
      previousActiveElement.current = document.activeElement;
      closeButtonRef.current?.focus();
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
      if (previousActiveElement.current instanceof HTMLElement) {
        previousActiveElement.current.focus();
      }
    }

    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  // Keyboard handling with focus trap
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
      return;
    }

    // Focus trap
    if (e.key === 'Tab' && modalRef.current) {
      const focusableElements = modalRef.current.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      );
      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];

      if (e.shiftKey && document.activeElement === firstElement) {
        e.preventDefault();
        lastElement?.focus();
      } else if (!e.shiftKey && document.activeElement === lastElement) {
        e.preventDefault();
        firstElement?.focus();
      }
    }
  }, [onClose]);

  useEffect(() => {
    if (isOpen) {
      document.addEventListener('keydown', handleKeyDown);
      return () => document.removeEventListener('keydown', handleKeyDown);
    }
  }, [isOpen, handleKeyDown]);

  if (!isOpen) return null;

  return (
    <div
      className="v2-modal-overlay"
      onClick={onClose}
      role="presentation"
    >
      <div
        ref={modalRef}
        className="v2-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="shortcuts-title"
      >
        <div className="v2-modal__header">
          <h2 id="shortcuts-title" className="v2-modal__title">Keyboard Shortcuts</h2>
          <button
            ref={closeButtonRef}
            type="button"
            className="v2-modal__close"
            onClick={onClose}
            aria-label="Close keyboard shortcuts"
          >
            ×
          </button>
        </div>
        <div className="v2-modal__content">
          {shortcuts.map((section) => (
            <div key={section.category} className="v2-shortcuts-section">
              <h3 className="v2-shortcuts-section__title">{section.category}</h3>
              <div className="v2-shortcuts-list" role="list">
                {section.items.map((item) => (
                  <div key={item.keys} className="v2-shortcut" role="listitem">
                    <kbd className="v2-shortcut__keys">{item.keys}</kbd>
                    <span className="v2-shortcut__desc">{item.description}</span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
