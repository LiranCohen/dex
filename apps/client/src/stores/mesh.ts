/**
 * Mesh connection state store using Zustand
 *
 * Provides robust connection management with:
 * - Auto-reconnection on failure
 * - Connection health monitoring
 * - Exponential backoff for retries
 */

import { create } from "zustand";
import {
  createMeshClient,
  DEFAULT_RETRY_CONFIG,
  calculateBackoff,
  type RetryConfig,
} from "../mesh/client";
import type { MeshClient, MeshState, NetMap, MeshConfig } from "../mesh/types";

// States that indicate a recoverable failure
const RECOVERABLE_STATES: MeshState[] = ["NoState", "Stopped"];

// States that indicate the connection is healthy
const HEALTHY_STATES: MeshState[] = ["Running"];

// Auto-reconnect configuration
const AUTO_RECONNECT_CONFIG: RetryConfig = {
  maxAttempts: Infinity, // Keep trying
  initialDelayMs: 2000,
  maxDelayMs: 60000,
  backoffMultiplier: 1.5,
};

interface ConnectionStats {
  connectAttempts: number;
  lastConnectAttempt: number | null;
  lastConnectedAt: number | null;
  lastDisconnectedAt: number | null;
  consecutiveFailures: number;
}

interface MeshStore {
  // Connection state
  state: MeshState;
  netMap: NetMap | null;
  error: string | null;
  authUrl: string | null;

  // Client instance
  client: MeshClient | null;

  // Connection management
  config: MeshConfig | null;
  autoReconnect: boolean;
  isReconnecting: boolean;
  stats: ConnectionStats;

  // Actions
  connect: (config: MeshConfig, autoReconnect?: boolean) => Promise<void>;
  disconnect: () => void;
  login: () => void;
  clearError: () => void;
  setAutoReconnect: (enabled: boolean) => void;
}

// Track reconnection timeout for cleanup
let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;

/**
 * Schedule a reconnection attempt
 */
function scheduleReconnect(
  get: () => MeshStore,
  set: (partial: Partial<MeshStore>) => void
): void {
  const { config, autoReconnect, isReconnecting, stats } = get();

  if (!config || !autoReconnect || isReconnecting) {
    return;
  }

  // Clear any existing timeout
  if (reconnectTimeout) {
    clearTimeout(reconnectTimeout);
  }

  const delay = calculateBackoff(stats.consecutiveFailures, AUTO_RECONNECT_CONFIG);
  console.log(
    `[mesh] scheduling reconnect in ${Math.round(delay)}ms (attempt ${stats.consecutiveFailures + 1})`
  );

  set({ isReconnecting: true });

  reconnectTimeout = setTimeout(async () => {
    const currentState = get();
    if (!currentState.autoReconnect || !currentState.config) {
      set({ isReconnecting: false });
      return;
    }

    console.log("[mesh] attempting reconnection...");
    try {
      await get().connect(currentState.config, true);
    } catch {
      // connect() handles its own errors
    }
    set({ isReconnecting: false });
  }, delay);
}

export const useMeshStore = create<MeshStore>((set, get) => ({
  state: "NoState",
  netMap: null,
  error: null,
  authUrl: null,
  client: null,
  config: null,
  autoReconnect: true,
  isReconnecting: false,
  stats: {
    connectAttempts: 0,
    lastConnectAttempt: null,
    lastConnectedAt: null,
    lastDisconnectedAt: null,
    consecutiveFailures: 0,
  },

  connect: async (config: MeshConfig, autoReconnect = true) => {
    // Clear any pending reconnect
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout);
      reconnectTimeout = null;
    }

    const prevStats = get().stats;
    set({
      state: "Starting",
      error: null,
      config,
      autoReconnect,
      isReconnecting: false,
      stats: {
        ...prevStats,
        connectAttempts: prevStats.connectAttempts + 1,
        lastConnectAttempt: Date.now(),
      },
    });

    try {
      const client = await createMeshClient(config, DEFAULT_RETRY_CONFIG);

      // Track if we've ever reached Running state
      let hasConnected = false;

      client.run({
        notifyState: (state: MeshState) => {
          console.log("[mesh] state:", state);
          const current = get();

          // Track connection success
          if (HEALTHY_STATES.includes(state) && !hasConnected) {
            hasConnected = true;
            set({
              state,
              stats: {
                ...current.stats,
                lastConnectedAt: Date.now(),
                consecutiveFailures: 0, // Reset on success
              },
            });
            return;
          }

          // Handle disconnection/failure
          if (RECOVERABLE_STATES.includes(state) && hasConnected) {
            console.log("[mesh] connection lost, state:", state);
            set({
              state,
              stats: {
                ...current.stats,
                lastDisconnectedAt: Date.now(),
                consecutiveFailures: current.stats.consecutiveFailures + 1,
              },
            });

            // Trigger auto-reconnect
            if (current.autoReconnect) {
              scheduleReconnect(get, set);
            }
            return;
          }

          set({ state });
        },

        notifyNetMap: (netMapJson: string) => {
          try {
            const netMap = JSON.parse(netMapJson) as NetMap;
            console.log("[mesh] netmap updated, peers:", netMap.peers.length);
            set({ netMap });
          } catch (e) {
            console.error("[mesh] failed to parse netmap:", e);
          }
        },

        notifyBrowseToURL: (url: string) => {
          console.log("[mesh] auth URL:", url);
          set({ authUrl: url });
          window.open(url, "_blank");
        },

        notifyPanicRecover: (err: string) => {
          console.error("[mesh] panic recovered:", err);
          const current = get();
          set({
            error: `Mesh client error: ${err}`,
            stats: {
              ...current.stats,
              consecutiveFailures: current.stats.consecutiveFailures + 1,
            },
          });

          // Trigger auto-reconnect on panic
          if (current.autoReconnect) {
            scheduleReconnect(get, set);
          }
        },
      });

      set({ client });
    } catch (e) {
      const error = e instanceof Error ? e.message : String(e);
      console.error("[mesh] connect failed:", error);

      const current = get();
      set({
        state: "NoState",
        error,
        stats: {
          ...current.stats,
          consecutiveFailures: current.stats.consecutiveFailures + 1,
        },
      });

      // Trigger auto-reconnect on connection failure
      if (current.autoReconnect) {
        scheduleReconnect(get, set);
      }
    }
  },

  disconnect: () => {
    // Clear any pending reconnect
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout);
      reconnectTimeout = null;
    }

    const { client, stats } = get();
    if (client) {
      client.logout();
    }
    set({
      state: "NoState",
      netMap: null,
      client: null,
      authUrl: null,
      autoReconnect: false, // Disable auto-reconnect on explicit disconnect
      isReconnecting: false,
      stats: {
        ...stats,
        lastDisconnectedAt: Date.now(),
      },
    });
  },

  login: () => {
    const { client } = get();
    if (client) {
      client.login();
    }
  },

  clearError: () => {
    set({ error: null });
  },

  setAutoReconnect: (enabled: boolean) => {
    set({ autoReconnect: enabled });
    if (!enabled && reconnectTimeout) {
      clearTimeout(reconnectTimeout);
      reconnectTimeout = null;
      set({ isReconnecting: false });
    }
  },
}));

/**
 * Find the HQ peer in the netmap
 */
export function findHQPeer(netMap: NetMap | null): NetMap["peers"][0] | null {
  if (!netMap) return null;

  // Look for peer with "hq" in hostname or tagged as HQ
  return (
    netMap.peers.find(
      (p) =>
        p.name.toLowerCase().includes("hq") ||
        p.name.toLowerCase().includes("poindexter")
    ) || null
  );
}

/**
 * Get HQ's mesh address
 */
export function getHQAddress(netMap: NetMap | null): string | null {
  const hq = findHQPeer(netMap);
  if (!hq || !hq.addresses.length) return null;
  return hq.addresses[0];
}
