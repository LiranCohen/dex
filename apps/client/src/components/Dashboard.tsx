/**
 * Main dashboard shown when connected to mesh
 */

import { useMeshStore, findHQPeer, getHQAddress } from "../stores/mesh";
import { useState, useEffect } from "react";

export function Dashboard() {
  const { netMap, client } = useMeshStore();
  const [hqStatus, setHqStatus] = useState<"checking" | "online" | "offline">(
    "checking"
  );
  const [hqData, setHqData] = useState<Record<string, unknown> | null>(null);

  const hqAddress = getHQAddress(netMap);
  const hqPeer = findHQPeer(netMap);

  // Check HQ connectivity
  useEffect(() => {
    if (!client || !hqAddress) {
      setHqStatus("offline");
      return;
    }

    const checkHQ = async () => {
      try {
        setHqStatus("checking");
        const response = await client.fetch(
          `http://${hqAddress}:8080/api/v1/system/status`
        );
        if (response.status === 200) {
          const text = await response.text();
          setHqData(JSON.parse(text));
          setHqStatus("online");
        } else {
          setHqStatus("offline");
        }
      } catch (e) {
        console.error("[dashboard] HQ check failed:", e);
        setHqStatus("offline");
      }
    };

    checkHQ();
    const interval = setInterval(checkHQ, 30000); // Check every 30s
    return () => clearInterval(interval);
  }, [client, hqAddress]);

  return (
    <div className="dashboard">
      <section className="dashboard-section">
        <h2>Network Status</h2>
        <div className="info-grid">
          <div className="info-item">
            <label>Your Address</label>
            <span>{netMap?.self.addresses[0] || "N/A"}</span>
          </div>
          <div className="info-item">
            <label>Device Name</label>
            <span>{netMap?.self.name || "N/A"}</span>
          </div>
          <div className="info-item">
            <label>Peers Online</label>
            <span>
              {netMap?.peers.filter((p) => p.online).length || 0} /{" "}
              {netMap?.peers.length || 0}
            </span>
          </div>
        </div>
      </section>

      <section className="dashboard-section">
        <h2>HQ Status</h2>
        <div className="hq-status">
          <div className="status-indicator">
            <span
              className="status-dot"
              style={{
                backgroundColor:
                  hqStatus === "online"
                    ? "#10b981"
                    : hqStatus === "checking"
                      ? "#f59e0b"
                      : "#ef4444",
              }}
            />
            <span>
              {hqStatus === "online"
                ? "HQ Online"
                : hqStatus === "checking"
                  ? "Checking..."
                  : "HQ Offline"}
            </span>
          </div>
          {hqPeer && (
            <div className="info-grid">
              <div className="info-item">
                <label>HQ Name</label>
                <span>{hqPeer.name}</span>
              </div>
              <div className="info-item">
                <label>HQ Address</label>
                <span>{hqAddress}</span>
              </div>
              {hqData && (
                <div className="info-item">
                  <label>HQ Version</label>
                  <span>{(hqData as { version?: string }).version || "N/A"}</span>
                </div>
              )}
            </div>
          )}
          {!hqPeer && (
            <p className="warning">
              No HQ found on the network. Make sure HQ is running and connected
              to the mesh.
            </p>
          )}
        </div>
      </section>

      <section className="dashboard-section">
        <h2>Peers</h2>
        {netMap && netMap.peers.length > 0 ? (
          <div className="peers-list">
            {netMap.peers.map((peer) => (
              <div
                key={peer.nodeKey}
                className={`peer-item ${peer.online ? "online" : "offline"}`}
              >
                <span className="peer-name">{peer.name}</span>
                <span className="peer-address">{peer.addresses[0]}</span>
                <span
                  className={`peer-status ${peer.online ? "online" : "offline"}`}
                >
                  {peer.online ? "Online" : "Offline"}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="no-peers">No other peers on the network.</p>
        )}
      </section>
    </div>
  );
}
