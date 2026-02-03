import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { ReactNode } from 'react';
import { useKeyboardNavigation } from './useKeyboardNavigation';

// Mock useNavigate
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

function wrapper({ children }: { children: ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

function dispatchKey(key: string, options: Partial<KeyboardEventInit> = {}) {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, ...options });
  window.dispatchEvent(event);
}

describe('useKeyboardNavigation', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockNavigate.mockClear();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  describe('global shortcuts', () => {
    it('navigates to home with g+h', () => {
      renderHook(() => useKeyboardNavigation(), { wrapper });

      act(() => {
        dispatchKey('g');
        dispatchKey('h');
      });

      expect(mockNavigate).toHaveBeenCalledWith('/v2');
    });

    it('navigates to inbox with g+i', () => {
      renderHook(() => useKeyboardNavigation(), { wrapper });

      act(() => {
        dispatchKey('g');
        dispatchKey('i');
      });

      expect(mockNavigate).toHaveBeenCalledWith('/v2/inbox');
    });

    it('navigates to objectives with g+o', () => {
      renderHook(() => useKeyboardNavigation(), { wrapper });

      act(() => {
        dispatchKey('g');
        dispatchKey('o');
      });

      expect(mockNavigate).toHaveBeenCalledWith('/v2/objectives');
    });

    it('scrolls to top with g+g', () => {
      renderHook(() => useKeyboardNavigation(), { wrapper });

      act(() => {
        dispatchKey('g');
        dispatchKey('g');
      });

      expect(window.scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' });
    });

    it('scrolls to bottom with Shift+G', () => {
      renderHook(() => useKeyboardNavigation(), { wrapper });

      act(() => {
        dispatchKey('G', { shiftKey: true });
      });

      expect(window.scrollTo).toHaveBeenCalledWith({ top: document.body.scrollHeight, behavior: 'smooth' });
    });

    it('clears g combo after timeout', () => {
      renderHook(() => useKeyboardNavigation(), { wrapper });

      act(() => {
        dispatchKey('g');
        vi.advanceTimersByTime(1000);
        dispatchKey('h');
      });

      expect(mockNavigate).not.toHaveBeenCalled();
    });

    it('calls onSearch when / is pressed', () => {
      const onSearch = vi.fn();
      renderHook(() => useKeyboardNavigation({ onSearch }), { wrapper });

      act(() => {
        dispatchKey('/');
      });

      expect(onSearch).toHaveBeenCalledTimes(1);
    });

    it('calls onHelp when ? is pressed', () => {
      const onHelp = vi.fn();
      renderHook(() => useKeyboardNavigation({ onHelp }), { wrapper });

      act(() => {
        dispatchKey('?');
      });

      expect(onHelp).toHaveBeenCalledTimes(1);
    });

    it('clears selection on Escape', () => {
      const items = [{ id: '1' }, { id: '2' }];
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j'); // Select first item
      });

      expect(result.current.selectedIndex).toBe(0);

      act(() => {
        dispatchKey('Escape');
      });

      expect(result.current.selectedIndex).toBe(-1);
      expect(result.current.isNavigating).toBe(false);
    });
  });

  describe('list navigation with j/k', () => {
    const items = [
      { id: '1', onClick: vi.fn() },
      { id: '2', onClick: vi.fn() },
      { id: '3', onClick: vi.fn() },
    ];

    beforeEach(() => {
      items.forEach((item) => item.onClick.mockClear());
    });

    it('moves down with j', () => {
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j');
      });

      expect(result.current.selectedIndex).toBe(0);
      expect(result.current.isNavigating).toBe(true);

      act(() => {
        dispatchKey('j');
      });

      expect(result.current.selectedIndex).toBe(1);
    });

    it('moves up with k', () => {
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j'); // Go to 0
        dispatchKey('j'); // Go to 1
        dispatchKey('k'); // Back to 0
      });

      expect(result.current.selectedIndex).toBe(0);
    });

    it('does not go below 0 with k', () => {
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j'); // Go to 0
        dispatchKey('k'); // Try to go to -1
      });

      expect(result.current.selectedIndex).toBe(0);
    });

    it('does not exceed items length with j', () => {
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j'); // 0
        dispatchKey('j'); // 1
        dispatchKey('j'); // 2
        dispatchKey('j'); // Still 2
      });

      expect(result.current.selectedIndex).toBe(2);
    });

    it('activates selected item with Enter', () => {
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j'); // Select first item
      });

      expect(result.current.isNavigating).toBe(true);

      act(() => {
        dispatchKey('Enter');
      });

      expect(items[0].onClick).toHaveBeenCalledTimes(1);
    });

    it('does not activate item if not navigating', () => {
      renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('Enter');
      });

      expect(items[0].onClick).not.toHaveBeenCalled();
    });

    it('resets selection when items change', () => {
      const { result, rerender } = renderHook(
        ({ items }) => useKeyboardNavigation({ items }),
        { wrapper, initialProps: { items } }
      );

      act(() => {
        dispatchKey('j'); // Select first item
      });

      expect(result.current.selectedIndex).toBe(0);

      // Change items
      rerender({ items: [{ id: 'new1', onClick: vi.fn() }, { id: 'new2', onClick: vi.fn() }] });

      expect(result.current.selectedIndex).toBe(-1);
      expect(result.current.isNavigating).toBe(false);
    });

    it('does nothing when items array is empty', () => {
      const { result } = renderHook(() => useKeyboardNavigation({ items: [] }), { wrapper });

      act(() => {
        dispatchKey('j');
      });

      expect(result.current.selectedIndex).toBe(-1);
    });
  });

  describe('input detection', () => {
    it('ignores shortcuts when focused on input', () => {
      const onSearch = vi.fn();
      renderHook(() => useKeyboardNavigation({ onSearch }), { wrapper });

      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();

      act(() => {
        const event = new KeyboardEvent('keydown', { key: '/', bubbles: true });
        Object.defineProperty(event, 'target', { value: input, writable: false });
        window.dispatchEvent(event);
      });

      expect(onSearch).not.toHaveBeenCalled();

      document.body.removeChild(input);
    });

    it('ignores shortcuts when focused on textarea', () => {
      const onSearch = vi.fn();
      renderHook(() => useKeyboardNavigation({ onSearch }), { wrapper });

      const textarea = document.createElement('textarea');
      document.body.appendChild(textarea);
      textarea.focus();

      act(() => {
        const event = new KeyboardEvent('keydown', { key: '/', bubbles: true });
        Object.defineProperty(event, 'target', { value: textarea, writable: false });
        window.dispatchEvent(event);
      });

      expect(onSearch).not.toHaveBeenCalled();

      document.body.removeChild(textarea);
    });

    it('allows Escape to blur input and enable navigation', () => {
      const { result } = renderHook(() => useKeyboardNavigation(), { wrapper });

      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();

      act(() => {
        const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
        Object.defineProperty(event, 'target', { value: input, writable: false });
        window.dispatchEvent(event);
      });

      expect(result.current.isNavigating).toBe(true);

      document.body.removeChild(input);
    });
  });

  describe('enabled option', () => {
    it('does nothing when enabled is false', () => {
      const onSearch = vi.fn();
      renderHook(() => useKeyboardNavigation({ onSearch, enabled: false }), { wrapper });

      act(() => {
        dispatchKey('/');
      });

      expect(onSearch).not.toHaveBeenCalled();
    });

    it('works when enabled is true', () => {
      const onSearch = vi.fn();
      renderHook(() => useKeyboardNavigation({ onSearch, enabled: true }), { wrapper });

      act(() => {
        dispatchKey('/');
      });

      expect(onSearch).toHaveBeenCalledTimes(1);
    });

    it('defaults enabled to true', () => {
      const onSearch = vi.fn();
      renderHook(() => useKeyboardNavigation({ onSearch }), { wrapper });

      act(() => {
        dispatchKey('/');
      });

      expect(onSearch).toHaveBeenCalledTimes(1);
    });
  });

  describe('clearSelection method', () => {
    it('clears selection when called', () => {
      const items = [{ id: '1' }, { id: '2' }];
      const { result } = renderHook(() => useKeyboardNavigation({ items }), { wrapper });

      act(() => {
        dispatchKey('j');
      });

      expect(result.current.selectedIndex).toBe(0);

      act(() => {
        result.current.clearSelection();
      });

      expect(result.current.selectedIndex).toBe(-1);
      expect(result.current.isNavigating).toBe(false);
    });
  });
});
