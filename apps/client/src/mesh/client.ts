/**
 * Mesh client wrapper for tsconnect WASM module
 *
 * Provides a robust connection layer with:
 * - Proper WASM initialization with polling (no arbitrary timeouts)
 * - Retry logic with exponential backoff
 * - Connection state management
 */

import type { MeshClient, MeshConfig, StateStorage } from "./types";

// Global reference to the Go WASM instance
declare global {
  interface Window {
    Go: new () => GoInstance;
    newIPN: (config: WasmConfig) => MeshClient;
  }
}

interface GoInstance {
  importObject: WebAssembly.Imports;
  run: (instance: WebAssembly.Instance) => Promise<void>;
}

interface WasmConfig {
  controlURL?: string;
  authKey?: string;
  hostname?: string;
  stateStorage?: StateStorage;
}

// Configuration for retry behavior
export interface RetryConfig {
  maxAttempts: number;
  initialDelayMs: number;
  maxDelayMs: number;
  backoffMultiplier: number;
}

export const DEFAULT_RETRY_CONFIG: RetryConfig = {
  maxAttempts: 5,
  initialDelayMs: 1000,
  maxDelayMs: 30000,
  backoffMultiplier: 2,
};

// WASM initialization constants
const WASM_INIT_POLL_INTERVAL_MS = 50;
const WASM_INIT_TIMEOUT_MS = 10000;

let wasmInitialized = false;
let initPromise: Promise<void> | null = null;

/**
 * Wait for a condition to be true with polling
 */
async function waitFor(
  condition: () => boolean,
  timeoutMs: number,
  pollIntervalMs: number,
  errorMessage: string
): Promise<void> {
  const startTime = Date.now();

  while (!condition()) {
    if (Date.now() - startTime > timeoutMs) {
      throw new Error(errorMessage);
    }
    await new Promise((resolve) => setTimeout(resolve, pollIntervalMs));
  }
}

/**
 * Sleep for a given duration
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Calculate delay for retry attempt using exponential backoff
 */
export function calculateBackoff(
  attempt: number,
  config: RetryConfig
): number {
  const delay =
    config.initialDelayMs * Math.pow(config.backoffMultiplier, attempt);
  // Add jitter (±20%) to prevent thundering herd
  const jitter = delay * 0.2 * (Math.random() * 2 - 1);
  return Math.min(delay + jitter, config.maxDelayMs);
}

/**
 * Initialize the WASM module
 * This must be called before creating a mesh client
 */
export async function initWasm(): Promise<void> {
  if (wasmInitialized) return;
  if (initPromise) return initPromise;

  initPromise = (async () => {
    console.log("[mesh] initializing WASM module...");

    // Load wasm_exec.js (Go's WASM support)
    if (!window.Go) {
      console.log("[mesh] loading wasm_exec.js...");
      await loadScript("/wasm_exec.js");
    }

    const go = new window.Go();

    // Load the WASM module
    console.log("[mesh] loading main.wasm...");
    const result = await WebAssembly.instantiateStreaming(
      fetch("/main.wasm"),
      go.importObject
    );

    // Run the Go program (this sets up newIPN global)
    // Note: go.run() returns a promise but doesn't wait for initialization
    console.log("[mesh] starting Go runtime...");
    go.run(result.instance);

    // Poll for newIPN to be available instead of arbitrary timeout
    console.log("[mesh] waiting for newIPN to be available...");
    await waitFor(
      () => typeof window.newIPN === "function",
      WASM_INIT_TIMEOUT_MS,
      WASM_INIT_POLL_INTERVAL_MS,
      `WASM module did not expose newIPN function within ${WASM_INIT_TIMEOUT_MS}ms`
    );

    wasmInitialized = true;
    console.log("[mesh] WASM module initialized successfully");
  })();

  return initPromise;
}

/**
 * Reset WASM initialization state (for testing or recovery)
 */
export function resetWasmInit(): void {
  wasmInitialized = false;
  initPromise = null;
}

/**
 * Check if WASM is initialized
 */
export function isWasmInitialized(): boolean {
  return wasmInitialized;
}

/**
 * Create a mesh client instance with retry logic
 */
export async function createMeshClient(
  config: MeshConfig,
  retryConfig: RetryConfig = DEFAULT_RETRY_CONFIG
): Promise<MeshClient> {
  let lastError: Error | null = null;

  for (let attempt = 0; attempt < retryConfig.maxAttempts; attempt++) {
    try {
      if (attempt > 0) {
        const delay = calculateBackoff(attempt - 1, retryConfig);
        console.log(
          `[mesh] retry attempt ${attempt + 1}/${retryConfig.maxAttempts} after ${Math.round(delay)}ms`
        );
        await sleep(delay);
      }

      await initWasm();

      const wasmConfig: WasmConfig = {
        controlURL: config.controlURL,
        authKey: config.authKey,
        hostname: config.hostname || generateHostname(),
        stateStorage: config.stateStorage || createLocalStorageState(),
      };

      const client = window.newIPN(wasmConfig);
      console.log("[mesh] client created successfully");
      return client;
    } catch (e) {
      lastError = e instanceof Error ? e : new Error(String(e));
      console.error(
        `[mesh] attempt ${attempt + 1}/${retryConfig.maxAttempts} failed:`,
        lastError.message
      );

      // If WASM init failed, reset so we can retry
      if (!wasmInitialized) {
        resetWasmInit();
      }
    }
  }

  throw new Error(
    `Failed to create mesh client after ${retryConfig.maxAttempts} attempts: ${lastError?.message}`
  );
}

/**
 * Generate a random hostname for the client
 */
function generateHostname(): string {
  const adjectives = ["swift", "bright", "calm", "bold", "keen"];
  const nouns = ["falcon", "raven", "wolf", "bear", "hawk"];
  const adj = adjectives[Math.floor(Math.random() * adjectives.length)];
  const noun = nouns[Math.floor(Math.random() * nouns.length)];
  const num = Math.floor(Math.random() * 1000);
  return `${adj}-${noun}-${num}`;
}

/**
 * Create a localStorage-based state storage
 */
function createLocalStorageState(): StateStorage {
  return {
    getState: (key: string) => localStorage.getItem(`dex-mesh-${key}`) || "",
    setState: (key: string, value: string) =>
      localStorage.setItem(`dex-mesh-${key}`, value),
  };
}

/**
 * Load a script dynamically
 */
function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = src;
    script.onload = () => resolve();
    script.onerror = reject;
    document.head.appendChild(script);
  });
}
