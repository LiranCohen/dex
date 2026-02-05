/**
 * Mesh client wrapper for tsconnect WASM module
 */

import type {
  MeshClient,
  MeshConfig,
  StateStorage,
} from "./types";

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

let wasmInitialized = false;
let initPromise: Promise<void> | null = null;

/**
 * Initialize the WASM module
 * This must be called before creating a mesh client
 */
export async function initWasm(): Promise<void> {
  if (wasmInitialized) return;
  if (initPromise) return initPromise;

  initPromise = (async () => {
    // Load wasm_exec.js (Go's WASM support)
    if (!window.Go) {
      await loadScript("/wasm_exec.js");
    }

    const go = new window.Go();

    // Load the WASM module
    const result = await WebAssembly.instantiateStreaming(
      fetch("/main.wasm"),
      go.importObject
    );

    // Run the Go program (this sets up newIPN global)
    go.run(result.instance);

    // Wait a tick for newIPN to be available
    await new Promise((resolve) => setTimeout(resolve, 100));

    if (!window.newIPN) {
      throw new Error("WASM module did not expose newIPN function");
    }

    wasmInitialized = true;
  })();

  return initPromise;
}

/**
 * Create a mesh client instance
 */
export async function createMeshClient(config: MeshConfig): Promise<MeshClient> {
  await initWasm();

  const wasmConfig: WasmConfig = {
    controlURL: config.controlURL,
    authKey: config.authKey,
    hostname: config.hostname || generateHostname(),
    stateStorage: config.stateStorage || createLocalStorageState(),
  };

  return window.newIPN(wasmConfig);
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
