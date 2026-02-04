import { useEffect, useCallback, useState } from 'react';
import { useNavigate } from 'react-router-dom';

interface KeyboardNavigationOptions {
  onSearch?: () => void;
  onHelp?: () => void;
  items?: { id: string; onClick?: () => void }[];
  enabled?: boolean;
}

export function useKeyboardNavigation({
  onSearch,
  onHelp,
  items = [],
  enabled = true,
}: KeyboardNavigationOptions = {}) {
  const navigate = useNavigate();
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const [isNavigating, setIsNavigating] = useState(false);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!enabled) return;

      // Don't handle if we're in an input/textarea
      const target = e.target as HTMLElement;
      if (
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable
      ) {
        // Allow Escape to exit
        if (e.key === 'Escape') {
          target.blur();
          setIsNavigating(true);
        }
        return;
      }

      // Global shortcuts
      switch (e.key) {
        case 'g':
          // Wait for next key
          const handleNextKey = (e2: KeyboardEvent) => {
            window.removeEventListener('keydown', handleNextKey);
            switch (e2.key) {
              case 'h':
                e2.preventDefault();
                navigate('/');
                break;
              case 'i':
                e2.preventDefault();
                navigate('/inbox');
                break;
              case 'o':
                e2.preventDefault();
                navigate('/objectives');
                break;
              case 'g':
                // gg - scroll to top (handled by page)
                e2.preventDefault();
                window.scrollTo({ top: 0, behavior: 'smooth' });
                break;
            }
          };
          window.addEventListener('keydown', handleNextKey, { once: true });
          // Clear listener after timeout
          setTimeout(() => {
            window.removeEventListener('keydown', handleNextKey);
          }, 1000);
          break;

        case 'G':
          // Shift+G - scroll to bottom
          e.preventDefault();
          window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
          break;

        case '/':
          e.preventDefault();
          onSearch?.();
          break;

        case '?':
          e.preventDefault();
          onHelp?.();
          break;

        case 'Escape':
          e.preventDefault();
          setSelectedIndex(-1);
          setIsNavigating(false);
          break;

        case 'j':
          // Move down in list
          if (items.length > 0) {
            e.preventDefault();
            setIsNavigating(true);
            setSelectedIndex((prev) =>
              prev < items.length - 1 ? prev + 1 : prev
            );
          }
          break;

        case 'k':
          // Move up in list
          if (items.length > 0) {
            e.preventDefault();
            setIsNavigating(true);
            setSelectedIndex((prev) => (prev > 0 ? prev - 1 : 0));
          }
          break;

        case 'Enter':
          // Activate selected item
          if (isNavigating && selectedIndex >= 0 && items[selectedIndex]?.onClick) {
            e.preventDefault();
            items[selectedIndex].onClick?.();
          }
          break;
      }
    },
    [enabled, navigate, onSearch, onHelp, items, isNavigating, selectedIndex]
  );

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  // Reset selection when items change
  useEffect(() => {
    setSelectedIndex(-1);
    setIsNavigating(false);
  }, [items.length]);

  return {
    selectedIndex: isNavigating ? selectedIndex : -1,
    isNavigating,
    clearSelection: () => {
      setSelectedIndex(-1);
      setIsNavigating(false);
    },
  };
}
