import { useEffect, useRef, useState, useCallback } from 'react';
import { useAuthStore } from '../stores/auth';
import type { WebSocketEvent } from '../lib/types';

type MessageHandler = (event: WebSocketEvent) => void;

interface UseWebSocketReturn {
  connected: boolean;
  lastMessage: WebSocketEvent | null;
  subscribe: (handler: MessageHandler) => () => void;
}

const WS_RECONNECT_DELAY = 3000;

export function useWebSocket(): UseWebSocketReturn {
  const [connected, setConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<WebSocketEvent | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const handlersRef = useRef<Set<MessageHandler>>(new Set());
  const reconnectTimeoutRef = useRef<number | null>(null);
  const connectRef = useRef<() => void>(() => {});

  const token = useAuthStore((state) => state.token);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  const connect = useCallback(() => {
    if (!isAuthenticated || !token) return;

    // Build WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws?token=${token}`;

    try {
      const ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        setConnected(true);
        console.log('[WebSocket] Connected');
        // Subscribe to all events
        ws.send(JSON.stringify({ action: 'subscribe_all' }));
      };

      ws.onclose = (event) => {
        setConnected(false);
        wsRef.current = null;
        console.log('[WebSocket] Disconnected', event.code, event.reason);

        // Reconnect if still authenticated - use ref to get latest connect function
        if (isAuthenticated) {
          reconnectTimeoutRef.current = window.setTimeout(() => {
            console.log('[WebSocket] Attempting reconnect...');
            connectRef.current();
          }, WS_RECONNECT_DELAY);
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

  return { connected, lastMessage, subscribe };
}
