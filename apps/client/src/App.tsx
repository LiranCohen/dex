import { useState, useEffect } from "react";
import { useMeshStore } from "./stores/mesh";
import { ConnectionStatus } from "./components/ConnectionStatus";
import { ConnectDialog } from "./components/ConnectDialog";
import { Dashboard } from "./components/Dashboard";

function App() {
  const { state, connect, isReconnecting, config } = useMeshStore();
  const [showConnectDialog, setShowConnectDialog] = useState(false);

  // Show connect dialog only if not connected AND not auto-reconnecting
  useEffect(() => {
    const needsConnection = state === "NoState" || state === "NeedsLogin";
    const hasExistingConfig = config !== null;

    // Don't show dialog if we're reconnecting or have a config (auto-reconnect will handle it)
    if (needsConnection && !isReconnecting && !hasExistingConfig) {
      setShowConnectDialog(true);
    } else {
      setShowConnectDialog(false);
    }
  }, [state, isReconnecting, config]);

  const isConnecting = state === "Starting" || isReconnecting;

  return (
    <div className="app">
      <header className="app-header">
        <h1>Dex</h1>
        <ConnectionStatus />
      </header>

      <main className="app-main">
        {state === "Running" ? (
          <Dashboard />
        ) : (
          <div className="connecting-message">
            {isConnecting && <p>Connecting to mesh network...</p>}
            {state === "NeedsMachineAuth" && (
              <p>Waiting for machine authorization...</p>
            )}
            {state === "NoState" && isReconnecting && (
              <p>Connection lost. Reconnecting...</p>
            )}
          </div>
        )}
      </main>

      {showConnectDialog && (
        <ConnectDialog
          onConnect={(meshConfig) => {
            connect(meshConfig);
            setShowConnectDialog(false);
          }}
          onCancel={() => setShowConnectDialog(false)}
        />
      )}
    </div>
  );
}

export default App;
