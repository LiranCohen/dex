import { describe, it, expect, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useWebSocket } from './useWebSocket';

// Mock the auth store to return unauthenticated state
vi.mock('../stores/auth', () => ({
  useAuthStore: vi.fn((selector) => {
    const state = {
      token: null,
      isAuthenticated: false,
    };
    return selector(state);
  }),
}));

describe('useWebSocket', () => {
  it('returns expected interface', () => {
    const { result } = renderHook(() => useWebSocket());

    expect(result.current).toHaveProperty('connected');
    expect(result.current).toHaveProperty('lastMessage');
    expect(result.current).toHaveProperty('subscribe');
    expect(typeof result.current.subscribe).toBe('function');
  });

  it('starts disconnected', () => {
    const { result } = renderHook(() => useWebSocket());

    expect(result.current.connected).toBe(false);
  });

  it('has null lastMessage initially', () => {
    const { result } = renderHook(() => useWebSocket());

    expect(result.current.lastMessage).toBe(null);
  });

  it('subscribe returns unsubscribe function', () => {
    const { result } = renderHook(() => useWebSocket());
    const handler = vi.fn();

    const unsubscribe = result.current.subscribe(handler);

    expect(typeof unsubscribe).toBe('function');
  });

  it('does not connect when not authenticated', () => {
    // With our mock, isAuthenticated is false
    const { result } = renderHook(() => useWebSocket());

    // Should remain disconnected
    expect(result.current.connected).toBe(false);
  });
});
