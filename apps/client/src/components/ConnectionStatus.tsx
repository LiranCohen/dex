/**
 * Connection status indicator component
 */

import { useMeshStore, getHQAddress } from "../stores/mesh";

export function ConnectionStatus() {
  const { state, netMap, disconnect } = useMeshStore();

  const statusColor =
    state === "Running"
      ? "#10b981" // green
      : state === "Starting" || state === "NeedsMachineAuth"
        ? "#f59e0b" // yellow
        : "#6b7280"; // gray

  const statusText =
    state === "Running"
      ? "Connected"
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
      </div>

      {state === "Running" && netMap && (
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
    </div>
  );
}
