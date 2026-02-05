/**
 * Mesh client types - matches the tsconnect WASM API
 */

export type MeshState =
  | "NoState"
  | "InUseOtherUser"
  | "NeedsLogin"
  | "NeedsMachineAuth"
  | "Stopped"
  | "Starting"
  | "Running";

export interface NetMapNode {
  name: string;
  addresses: string[];
  machineKey: string;
  nodeKey: string;
}

export interface NetMapSelfNode extends NetMapNode {
  machineStatus: string;
}

export interface NetMapPeerNode extends NetMapNode {
  online?: boolean;
  tailscaleSSHEnabled: boolean;
}

export interface NetMap {
  self: NetMapSelfNode;
  peers: NetMapPeerNode[];
  lockedOut: boolean;
}

export interface MeshCallbacks {
  notifyState: (state: MeshState) => void;
  notifyNetMap: (netMapJson: string) => void;
  notifyBrowseToURL: (url: string) => void;
  notifyPanicRecover: (err: string) => void;
}

export interface MeshConfig {
  controlURL: string;
  authKey?: string;
  hostname?: string;
  stateStorage?: StateStorage;
}

export interface StateStorage {
  getState: (key: string) => string;
  setState: (key: string, value: string) => void;
}

export interface MeshClient {
  run: (callbacks: MeshCallbacks) => void;
  login: () => void;
  logout: () => void;
  fetch: (url: string) => Promise<FetchResponse>;
  ssh: (host: string, user: string, termConfig: TermConfig) => SSHSession;
}

export interface FetchResponse {
  status: number;
  statusText: string;
  text: () => Promise<string>;
}

export interface TermConfig {
  writeFn: (data: string) => void;
  writeErrorFn: (data: string) => void;
  setReadFn: (fn: (data: string) => void) => void;
  rows: number;
  cols: number;
  timeoutSeconds?: number;
  onConnectionProgress: (message: string) => void;
  onConnected: () => void;
  onDone: () => void;
}

export interface SSHSession {
  close: () => boolean;
  resize: (rows: number, cols: number) => boolean;
}
