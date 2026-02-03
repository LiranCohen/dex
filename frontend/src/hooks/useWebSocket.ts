import { useEffect, useRef, useState, useCallback } from 'react';
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

const WS_RECONNECT_DELAY = 3000;
const MAX_RECONNECT_ATTEMPTS = 5;

export function useWebSocket(): UseWebSocketReturn {
  const [connected, setConnected] = useState(false);
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [reconnectAttempts, setReconnectAttempts] = useState(0);
  const [lastMessage, setLastMessage] = useState<WebSocketEvent | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const handlersRef = useRef<Set<MessageHandler>>(new Set());
  const reconnectTimeoutRef = useRef<number | null>(null);
  const connectRef = useRef<() => void>(() => {});
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

    setConnectionState('reconnecting');

    // Build WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws?token=${token}`;

    try {
      const ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        setConnected(true);
        setConnectionState('connected');
        reconnectAttemptsRef.current = 0;
        setReconnectAttempts(0);
        console.log('[WebSocket] Connected');
        // Subscribe to all events
        ws.send(JSON.stringify({ action: 'subscribe_all' }));
      };

      ws.onclose = (event) => {
        setConnected(false);
        wsRef.current = null;
        console.log('[WebSocket] Disconnected', event.code, event.reason);

        // Reconnect if still authenticated and under max attempts
        if (isAuthenticated) {
          reconnectAttemptsRef.current += 1;
          setReconnectAttempts(reconnectAttemptsRef.current);

          if (reconnectAttemptsRef.current >= MAX_RECONNECT_ATTEMPTS) {
            setConnectionState('failed');
            console.log('[WebSocket] Max reconnect attempts reached');
            return;
          }

          setConnectionState('reconnecting');
          const delay = WS_RECONNECT_DELAY * Math.min(reconnectAttemptsRef.current, 3); // Exponential backoff up to 3x
          reconnectTimeoutRef.current = window.setTimeout(() => {
            console.log(`[WebSocket] Attempting reconnect (${reconnectAttemptsRef.current}/${MAX_RECONNECT_ATTEMPTS})...`);
            connectRef.current();
          }, delay);
        } else {
          setConnectionState('disconnected');
        }
      };

      ws.onerror = (error) => {
        console.error('[WebSocket] Error:', error);
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as WebSocketEvent;
          setLastMessage(data);

          // Notify all subscribers
          handlersRef.current.forEach((handler) => {
            try {
              handler(data);
            } catch (err) {
              console.error('[WebSocket] Handler error:', err);
            }
          });
        } catch (err) {
          console.error('[WebSocket] Failed to parse message:', err);
        }
      };

      wsRef.current = ws;
    } catch (err) {
      console.error('[WebSocket] Failed to connect:', err);
      setConnectionState('failed');
    }
  }, [isAuthenticated, token]);

  // Keep connectRef updated with latest connect function
  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  // Connect/disconnect based on auth state
  useEffect(() => {
    if (isAuthenticated && token) {
      connect();
    }

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
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
    // Clear any pending reconnect timeout
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    // Close existing connection if any
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    // Connect with manual flag to reset attempts
    connect(true);
  }, [connect]);

  return { connected, connectionState, reconnectAttempts, lastMessage, subscribe, reconnect };
}
