import { useEffect, useRef, useState, useCallback } from 'react';
import { Centrifuge, Subscription, type PublicationContext } from 'centrifuge';
import { useAuthStore } from '../stores/auth';
import type { WebSocketEvent } from '../lib/types';

type MessageHandler = (event: WebSocketEvent) => void;

export type ConnectionState = 'connected' | 'disconnected' | 'reconnecting' | 'failed';
export type ConnectionQuality = 'excellent' | 'good' | 'poor' | 'disconnected';

interface UseWebSocketReturn {
  connected: boolean;
  connectionState: ConnectionState;
  connectionQuality: ConnectionQuality;
  latency: number | null;
  reconnectAttempts: number;
  lastMessage: WebSocketEvent | null;
  subscribe: (handler: MessageHandler) => () => void;
  subscribeToChannel: (channel: string) => () => void;
  subscribedChannels: Set<string>;
  reconnect: () => void;
}

const MAX_RECONNECT_ATTEMPTS = 5;
const PING_INTERVAL = 30000; // 30 seconds

// Calculate connection quality from latency
function getConnectionQuality(latency: number | null, connected: boolean): ConnectionQuality {
  if (!connected) return 'disconnected';
  if (latency === null) return 'good'; // No measurement yet
  if (latency < 100) return 'excellent';
  if (latency < 300) return 'good';
  return 'poor';
}

export function useWebSocket(): UseWebSocketReturn {
  const [connected, setConnected] = useState(false);
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [reconnectAttempts, setReconnectAttempts] = useState(0);
  const [lastMessage, setLastMessage] = useState<WebSocketEvent | null>(null);
  const [latency, setLatency] = useState<number | null>(null);
  const [subscribedChannels, setSubscribedChannels] = useState<Set<string>>(new Set());

  const centrifugeRef = useRef<Centrifuge | null>(null);
  const subscriptionsRef = useRef<Map<string, Subscription>>(new Map());
  const handlersRef = useRef<Set<MessageHandler>>(new Set());
  const reconnectAttemptsRef = useRef(0);
  const pingIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const token = useAuthStore((state) => state.token);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  const connectionQuality = getConnectionQuality(latency, connected);

  // Measure latency via ping RPC
  const measureLatency = useCallback(async () => {
    if (!centrifugeRef.current || connectionState !== 'connected') return;

    try {
      const start = performance.now();
      await centrifugeRef.current.rpc('ping', {});
      const elapsed = performance.now() - start;
      setLatency(Math.round(elapsed));
    } catch (err) {
      console.warn('[Centrifuge] Ping failed:', err);
    }
  }, [connectionState]);

  // Handle publication from any subscription
  const handlePublication = useCallback((ctx: PublicationContext) => {
    try {
      const data = ctx.data as WebSocketEvent;
      console.log('[Centrifuge] Received:', data.type, data);
      setLastMessage(data);

      // Notify all subscribers
      const handlerCount = handlersRef.current.size;
      console.log(`[Centrifuge] Notifying ${handlerCount} handlers`);
      handlersRef.current.forEach((handler) => {
        try {
          handler(data);
        } catch (err) {
          console.error('[Centrifuge] Handler error:', err);
        }
      });
    } catch (err) {
      console.error('[Centrifuge] Failed to process message:', err);
    }
  }, []);

  // Subscribe to a specific channel dynamically
  const subscribeToChannel = useCallback((channel: string) => {
    const centrifuge = centrifugeRef.current;
    if (!centrifuge) {
      console.warn('[Centrifuge] Cannot subscribe - not connected');
      return () => {};
    }

    // Check if already subscribed
    if (subscriptionsRef.current.has(channel)) {
      console.log(`[Centrifuge] Already subscribed to ${channel}`);
      return () => {
        // Return unsubscribe for the existing subscription
        const sub = subscriptionsRef.current.get(channel);
        if (sub) {
          sub.unsubscribe();
          subscriptionsRef.current.delete(channel);
          setSubscribedChannels(new Set(subscriptionsRef.current.keys()));
        }
      };
    }

    console.log(`[Centrifuge] Subscribing to ${channel}`);
    const sub = centrifuge.newSubscription(channel, {
      // Enable recovery to receive missed messages on reconnect
      recover: true,
    });

    sub.on('publication', handlePublication);

    sub.on('subscribed', (ctx) => {
      console.log(`[Centrifuge] Subscribed to ${channel}`, ctx.wasRecovering ? '(recovered)' : '');
      if (ctx.wasRecovering && ctx.recovered) {
        console.log(`[Centrifuge] Recovered ${ctx.recoveredPublications?.length || 0} missed messages`);
      }
    });

    sub.on('error', (ctx) => {
      console.error(`[Centrifuge] Subscription error on ${channel}:`, ctx.error);
    });

    sub.subscribe();
    subscriptionsRef.current.set(channel, sub);
    setSubscribedChannels(new Set(subscriptionsRef.current.keys()));

    // Return unsubscribe function
    return () => {
      console.log(`[Centrifuge] Unsubscribing from ${channel}`);
      sub.unsubscribe();
      subscriptionsRef.current.delete(channel);
      setSubscribedChannels(new Set(subscriptionsRef.current.keys()));
    };
  }, [handlePublication]);

  const connect = useCallback((isManualReconnect = false) => {
    if (!isAuthenticated || !token) return;

    // If manual reconnect, reset attempts
    if (isManualReconnect) {
      reconnectAttemptsRef.current = 0;
      setReconnectAttempts(0);
    }

    // Clean up existing connection and subscriptions
    if (centrifugeRef.current) {
      subscriptionsRef.current.forEach((sub) => sub.unsubscribe());
      subscriptionsRef.current.clear();
      centrifugeRef.current.disconnect();
      centrifugeRef.current = null;
    }

    // Clear ping interval
    if (pingIntervalRef.current) {
      clearInterval(pingIntervalRef.current);
      pingIntervalRef.current = null;
    }

    setConnectionState('reconnecting');
    setSubscribedChannels(new Set());

    // Build WebSocket URL for Centrifuge
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/realtime`;

    try {
      // Create Centrifuge client with auth token
      const centrifuge = new Centrifuge(wsUrl, {
        token: token,
        // Centrifuge will handle reconnection automatically
        minReconnectDelay: 1000,
        maxReconnectDelay: 10000,
      });

      // Handle connection state changes
      centrifuge.on('connected', () => {
        console.log('[Centrifuge] Connected');
        setConnected(true);
        setConnectionState('connected');
        reconnectAttemptsRef.current = 0;
        setReconnectAttempts(0);

        // Start periodic latency measurement
        measureLatency(); // Initial measurement
        pingIntervalRef.current = setInterval(measureLatency, PING_INTERVAL);
      });

      centrifuge.on('connecting', () => {
        console.log('[Centrifuge] Connecting...');
        setConnectionState('reconnecting');
      });

      centrifuge.on('disconnected', (ctx) => {
        console.log('[Centrifuge] Disconnected:', ctx.reason);
        setConnected(false);
        setLatency(null);

        // Clear ping interval on disconnect
        if (pingIntervalRef.current) {
          clearInterval(pingIntervalRef.current);
          pingIntervalRef.current = null;
        }

        if (isAuthenticated) {
          reconnectAttemptsRef.current += 1;
          setReconnectAttempts(reconnectAttemptsRef.current);

          if (reconnectAttemptsRef.current >= MAX_RECONNECT_ATTEMPTS) {
            setConnectionState('failed');
            console.log('[Centrifuge] Max reconnect attempts reached');
          } else {
            setConnectionState('reconnecting');
          }
        } else {
          setConnectionState('disconnected');
        }
      });

      centrifuge.on('error', (ctx) => {
        console.error('[Centrifuge] Error:', ctx.error);
      });

      // Subscribe to the global channel to receive all events
      const globalSub = centrifuge.newSubscription('global', {
        recover: true,
      });

      globalSub.on('publication', handlePublication);

      globalSub.on('subscribed', (ctx) => {
        console.log('[Centrifuge] Subscribed to global channel', ctx.wasRecovering ? '(recovered)' : '');
      });

      globalSub.on('error', (ctx) => {
        console.error('[Centrifuge] Global subscription error:', ctx.error);
      });

      // Subscribe and connect
      globalSub.subscribe();
      subscriptionsRef.current.set('global', globalSub);
      setSubscribedChannels(new Set(['global']));

      centrifuge.connect();
      centrifugeRef.current = centrifuge;
    } catch (err) {
      console.error('[Centrifuge] Failed to connect:', err);
      setConnectionState('failed');
    }
  }, [isAuthenticated, token, handlePublication, measureLatency]);

  // Connect/disconnect based on auth state
  useEffect(() => {
    if (isAuthenticated && token) {
      connect();
    }

    return () => {
      // Clear ping interval
      if (pingIntervalRef.current) {
        clearInterval(pingIntervalRef.current);
        pingIntervalRef.current = null;
      }
      // Unsubscribe from all channels
      subscriptionsRef.current.forEach((sub) => sub.unsubscribe());
      subscriptionsRef.current.clear();
      if (centrifugeRef.current) {
        centrifugeRef.current.disconnect();
        centrifugeRef.current = null;
      }
    };
  }, [isAuthenticated, token, connect]);

  // Subscribe function for components to receive messages
  const subscribe = useCallback((handler: MessageHandler) => {
    handlersRef.current.add(handler);

    // Return unsubscribe function
    return () => {
      handlersRef.current.delete(handler);
    };
  }, []);

  // Manual reconnect function
  const reconnect = useCallback(() => {
    connect(true);
  }, [connect]);

  return {
    connected,
    connectionState,
    connectionQuality,
    latency,
    reconnectAttempts,
    lastMessage,
    subscribe,
    subscribeToChannel,
    subscribedChannels,
    reconnect,
  };
}
