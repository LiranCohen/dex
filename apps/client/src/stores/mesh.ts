/**
 * Mesh connection state store using Zustand
 */

import { create } from "zustand";
import { createMeshClient } from "../mesh/client";
import type { MeshClient, MeshState, NetMap, MeshConfig } from "../mesh/types";

interface MeshStore {
  // Connection state
  state: MeshState;
  netMap: NetMap | null;
  error: string | null;
  authUrl: string | null;

  // Client instance
  client: MeshClient | null;

  // Actions
  connect: (config: MeshConfig) => Promise<void>;
  disconnect: () => void;
  login: () => void;
  clearError: () => void;
}

export const useMeshStore = create<MeshStore>((set, get) => ({
  state: "NoState",
  netMap: null,
  error: null,
  authUrl: null,
  client: null,

  connect: async (config: MeshConfig) => {
    try {
      set({ state: "Starting", error: null });

      const client = await createMeshClient(config);

      client.run({
        notifyState: (state: MeshState) => {
          console.log("[mesh] state:", state);
          set({ state });
        },

        notifyNetMap: (netMapJson: string) => {
          try {
            const netMap = JSON.parse(netMapJson) as NetMap;
            console.log("[mesh] netmap:", netMap);
            set({ netMap });
          } catch (e) {
            console.error("[mesh] failed to parse netmap:", e);
          }
        },

        notifyBrowseToURL: (url: string) => {
          console.log("[mesh] auth URL:", url);
          set({ authUrl: url });
          // Open in system browser (or new tab in web)
          window.open(url, "_blank");
        },

        notifyPanicRecover: (err: string) => {
          console.error("[mesh] panic recovered:", err);
          set({ error: `Mesh client error: ${err}` });
        },
      });

      set({ client });
    } catch (e) {
      const error = e instanceof Error ? e.message : String(e);
      console.error("[mesh] connect failed:", error);
      set({ state: "NoState", error });
    }
  },

  disconnect: () => {
    const { client } = get();
    if (client) {
      client.logout();
    }
    set({
      state: "NoState",
      netMap: null,
      client: null,
      authUrl: null,
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
