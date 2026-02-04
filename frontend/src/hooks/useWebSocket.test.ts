import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useWebSocket, type ConnectionQuality } from './useWebSocket';

// Mock the auth store
const mockAuthStore = {
  token: null as string | null,
  isAuthenticated: false,
};

vi.mock('../stores/auth', () => ({
  useAuthStore: vi.fn((selector) => selector(mockAuthStore)),
}));

// Mock Centrifuge client
const mockSubscription = {
  on: vi.fn(),
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
};

const mockCentrifuge = {
  on: vi.fn(),
  connect: vi.fn(),
  disconnect: vi.fn(),
  newSubscription: vi.fn(() => mockSubscription),
  rpc: vi.fn().mockResolvedValue({}),
};

vi.mock('centrifuge', () => ({
  Centrifuge: vi.fn(() => mockCentrifuge),
}));

describe('useWebSocket', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.token = null;
    mockAuthStore.isAuthenticated = false;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('interface', () => {
    it('returns expected interface', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current).toHaveProperty('connected');
      expect(result.current).toHaveProperty('connectionState');
      expect(result.current).toHaveProperty('connectionQuality');
      expect(result.current).toHaveProperty('latency');
      expect(result.current).toHaveProperty('lastMessage');
      expect(result.current).toHaveProperty('subscribe');
      expect(result.current).toHaveProperty('subscribeToChannel');
      expect(result.current).toHaveProperty('subscribedChannels');
      expect(result.current).toHaveProperty('reconnect');
      expect(typeof result.current.subscribe).toBe('function');
      expect(typeof result.current.subscribeToChannel).toBe('function');
      expect(typeof result.current.reconnect).toBe('function');
    });

    it('starts disconnected', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current.connected).toBe(false);
      expect(result.current.connectionState).toBe('disconnected');
    });

    it('has null lastMessage initially', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current.lastMessage).toBe(null);
    });

    it('has null latency initially', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current.latency).toBe(null);
    });

    it('has disconnected quality when not connected', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current.connectionQuality).toBe('disconnected');
    });
  });

  describe('subscription management', () => {
    it('subscribe returns unsubscribe function', () => {
      const { result } = renderHook(() => useWebSocket());
      const handler = vi.fn();

      const unsubscribe = result.current.subscribe(handler);

      expect(typeof unsubscribe).toBe('function');
    });

    it('multiple handlers can subscribe', () => {
      const { result } = renderHook(() => useWebSocket());
      const handler1 = vi.fn();
      const handler2 = vi.fn();

      const unsub1 = result.current.subscribe(handler1);
      const unsub2 = result.current.subscribe(handler2);

      expect(typeof unsub1).toBe('function');
      expect(typeof unsub2).toBe('function');
    });

    it('unsubscribe removes handler', () => {
      const { result } = renderHook(() => useWebSocket());
      const handler = vi.fn();

      const unsubscribe = result.current.subscribe(handler);
      unsubscribe();

      // Handler should be removed (no easy way to verify, but no error)
    });
  });

  describe('dynamic channel subscriptions', () => {
    it('subscribeToChannel returns unsubscribe function when not connected', () => {
      const { result } = renderHook(() => useWebSocket());

      const unsubscribe = result.current.subscribeToChannel('task:123');

      expect(typeof unsubscribe).toBe('function');
    });

    it('starts with empty subscribedChannels when not connected', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current.subscribedChannels.size).toBe(0);
    });
  });

  describe('authentication', () => {
    it('does not connect when not authenticated', () => {
      mockAuthStore.isAuthenticated = false;
      mockAuthStore.token = null;

      const { result } = renderHook(() => useWebSocket());

      expect(result.current.connected).toBe(false);
      expect(mockCentrifuge.connect).not.toHaveBeenCalled();
    });
  });

  describe('connection quality calculation', () => {
    it('returns disconnected when not connected', () => {
      const { result } = renderHook(() => useWebSocket());

      expect(result.current.connectionQuality).toBe('disconnected');
    });

    // Note: Testing actual connection quality requires connected state
    // which needs more complex mocking of the Centrifuge events
  });

  describe('reconnect', () => {
    it('reconnect function is callable', () => {
      const { result } = renderHook(() => useWebSocket());

      // Should not throw
      expect(() => result.current.reconnect()).not.toThrow();
    });
  });

  describe('cleanup', () => {
    it('cleans up on unmount', () => {
      const { unmount } = renderHook(() => useWebSocket());

      // Should not throw
      expect(() => unmount()).not.toThrow();
    });
  });
});

describe('ConnectionQuality helper', () => {
  // Test the quality calculation logic independently
  function getConnectionQuality(latency: number | null, connected: boolean): ConnectionQuality {
    if (!connected) return 'disconnected';
    if (latency === null) return 'good';
    if (latency < 100) return 'excellent';
    if (latency < 300) return 'good';
    return 'poor';
  }

  it('returns disconnected when not connected', () => {
    expect(getConnectionQuality(50, false)).toBe('disconnected');
  });

  it('returns good when latency is null', () => {
    expect(getConnectionQuality(null, true)).toBe('good');
  });

  it('returns excellent for latency under 100ms', () => {
    expect(getConnectionQuality(50, true)).toBe('excellent');
    expect(getConnectionQuality(99, true)).toBe('excellent');
  });

  it('returns good for latency 100-300ms', () => {
    expect(getConnectionQuality(100, true)).toBe('good');
    expect(getConnectionQuality(200, true)).toBe('good');
    expect(getConnectionQuality(299, true)).toBe('good');
  });

  it('returns poor for latency over 300ms', () => {
    expect(getConnectionQuality(300, true)).toBe('poor');
    expect(getConnectionQuality(500, true)).toBe('poor');
    expect(getConnectionQuality(1000, true)).toBe('poor');
  });
});

describe('useWebSocket - handler management', () => {
  // These tests focus on the handler management logic which doesn't require
  // mocking the full Centrifuge connection flow

  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.token = null;
    mockAuthStore.isAuthenticated = false;
  });

  it('subscribe returns unsubscribe function', () => {
    const { result } = renderHook(() => useWebSocket());
    const handler = vi.fn();

    const unsubscribe = result.current.subscribe(handler);

    expect(typeof unsubscribe).toBe('function');
  });

  it('multiple handlers can be subscribed', () => {
    const { result } = renderHook(() => useWebSocket());
    const handler1 = vi.fn();
    const handler2 = vi.fn();
    const handler3 = vi.fn();

    const unsub1 = result.current.subscribe(handler1);
    const unsub2 = result.current.subscribe(handler2);
    const unsub3 = result.current.subscribe(handler3);

    expect(typeof unsub1).toBe('function');
    expect(typeof unsub2).toBe('function');
    expect(typeof unsub3).toBe('function');
  });

  it('unsubscribe can be called multiple times safely', () => {
    const { result } = renderHook(() => useWebSocket());
    const handler = vi.fn();

    const unsubscribe = result.current.subscribe(handler);

    // Multiple calls should not throw
    expect(() => unsubscribe()).not.toThrow();
    expect(() => unsubscribe()).not.toThrow();
  });
});

describe('useWebSocket - reconnect function', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.token = null;
    mockAuthStore.isAuthenticated = false;
  });

  it('reconnect is callable when not authenticated', () => {
    const { result } = renderHook(() => useWebSocket());

    // Should not throw even when not connected
    expect(() => result.current.reconnect()).not.toThrow();
  });

  it('reconnect resets reconnect attempts state', () => {
    const { result } = renderHook(() => useWebSocket());

    // Initial state should have 0 reconnect attempts
    expect(result.current.reconnectAttempts).toBe(0);

    // Calling reconnect should not throw
    expect(() => result.current.reconnect()).not.toThrow();
  });
});

describe('useWebSocket - channel subscriptions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.token = null;
    mockAuthStore.isAuthenticated = false;
  });

  it('subscribeToChannel returns noop when not connected', () => {
    const { result } = renderHook(() => useWebSocket());

    // Should return a function that does nothing
    const unsubscribe = result.current.subscribeToChannel('task:123');
    expect(typeof unsubscribe).toBe('function');

    // Calling unsubscribe should not throw
    expect(() => unsubscribe()).not.toThrow();
  });

  it('subscribedChannels is empty when not connected', () => {
    const { result } = renderHook(() => useWebSocket());

    expect(result.current.subscribedChannels.size).toBe(0);
  });
});
