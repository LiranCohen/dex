/**
 * Connection status indicator component
 */

import { useMeshStore, getHQAddress } from "../stores/mesh";

export function ConnectionStatus() {
  const {
    state,
    netMap,
    disconnect,
    isReconnecting,
    autoReconnect,
    setAutoReconnect,
    stats,
  } = useMeshStore();

  const isConnected = state === "Running";
  const isConnecting = state === "Starting" || isReconnecting;
  const needsAuth = state === "NeedsLogin" || state === "NeedsMachineAuth";

  const statusColor = isConnected
    ? "#10b981" // green
    : isConnecting || needsAuth
      ? "#f59e0b" // yellow
      : "#6b7280"; // gray

  const statusText = isConnected
    ? "Connected"
    : isReconnecting
      ? `Reconnecting${stats.consecutiveFailures > 0 ? ` (${stats.consecutiveFailures})` : ""}...`
      : state === "Starting"
        ? "Connecting..."
        : state === "NeedsLogin"
          ? "Login Required"
          : state === "NeedsMachineAuth"
            ? "Pending Auth"
            : "Disconnected";

  const hqAddress = getHQAddress(netMap);

  return (
    <div className="connection-status">
      <div className="status-indicator">
        <span
          className="status-dot"
          style={{ backgroundColor: statusColor }}
        />
        <span className="status-text">{statusText}</span>
        {isReconnecting && (
          <span className="reconnect-spinner" aria-label="reconnecting" />
        )}
      </div>

      {isConnected && netMap && (
        <div className="connection-details">
          <span className="mesh-ip" title="Your mesh IP">
            {netMap.self.addresses[0]}
          </span>
          {hqAddress && (
            <span className="hq-ip" title="HQ address">
              HQ: {hqAddress}
            </span>
          )}
          <button
            className="disconnect-btn"
            onClick={disconnect}
            title="Disconnect"
          >
            Disconnect
          </button>
        </div>
      )}

      {!isConnected && !isConnecting && (
        <div className="connection-controls">
          <label className="auto-reconnect-toggle" title="Auto-reconnect">
            <input
              type="checkbox"
              checked={autoReconnect}
              onChange={(e) => setAutoReconnect(e.target.checked)}
            />
            <span>Auto-reconnect</span>
          </label>
        </div>
      )}
    </div>
  );
}
