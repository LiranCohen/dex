import { useEffect, useRef, useState, useCallback } from 'react';
import { Centrifuge, Subscription, type PublicationContext } from 'centrifuge';
import { useAuthStore } from '../stores/auth';
import type { WebSocketEvent } from '../lib/types';

type MessageHandler = (event: WebSocketEvent) => void;

export type ConnectionState = 'connected' | 'disconnected' | 'reconnecting' | 'failed';

interface UseWebSocketReturn {
  connected: boolean;
  connectionState: ConnectionState;
  reconnectAttempts: number;
  lastMessage: WebSocketEvent | null;
  subscribe: (handler: MessageHandler) => () => void;
  reconnect: () => void;
}

const MAX_RECONNECT_ATTEMPTS = 5;

export function useWebSocket(): UseWebSocketReturn {
  const [connected, setConnected] = useState(false);
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [reconnectAttempts, setReconnectAttempts] = useState(0);
  const [lastMessage, setLastMessage] = useState<WebSocketEvent | null>(null);
  const centrifugeRef = useRef<Centrifuge | null>(null);
  const subscriptionRef = useRef<Subscription | null>(null);
  const handlersRef = useRef<Set<MessageHandler>>(new Set());
  const reconnectAttemptsRef = useRef(0);

  const token = useAuthStore((state) => state.token);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  const connect = useCallback((isManualReconnect = false) => {
    if (!isAuthenticated || !token) return;

    // If manual reconnect, reset attempts
    if (isManualReconnect) {
      reconnectAttemptsRef.current = 0;
      setReconnectAttempts(0);
    }

    // Clean up existing connection
    if (centrifugeRef.current) {
      centrifugeRef.current.disconnect();
      centrifugeRef.current = null;
    }

    setConnectionState('reconnecting');

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
      });

      centrifuge.on('connecting', () => {
        console.log('[Centrifuge] Connecting...');
        setConnectionState('reconnecting');
      });

      centrifuge.on('disconnected', (ctx) => {
        console.log('[Centrifuge] Disconnected:', ctx.reason);
        setConnected(false);

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
      const sub = centrifuge.newSubscription('global');

      sub.on('publication', (ctx: PublicationContext) => {
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
      });

      sub.on('subscribed', () => {
        console.log('[Centrifuge] Subscribed to global channel');
      });

      sub.on('error', (ctx) => {
        console.error('[Centrifuge] Subscription error:', ctx.error);
      });

      // Subscribe and connect
      sub.subscribe();
      subscriptionRef.current = sub;

      centrifuge.connect();
      centrifugeRef.current = centrifuge;
    } catch (err) {
      console.error('[Centrifuge] Failed to connect:', err);
      setConnectionState('failed');
    }
  }, [isAuthenticated, token]);

  // Connect/disconnect based on auth state
  useEffect(() => {
    if (isAuthenticated && token) {
      connect();
    }

    return () => {
      if (subscriptionRef.current) {
        subscriptionRef.current.unsubscribe();
        subscriptionRef.current = null;
      }
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

  return { connected, connectionState, reconnectAttempts, lastMessage, subscribe, reconnect };
}
