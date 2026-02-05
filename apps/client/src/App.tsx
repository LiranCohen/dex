import { useState, useEffect } from "react";
import { useMeshStore } from "./stores/mesh";
import { ConnectionStatus } from "./components/ConnectionStatus";
import { ConnectDialog } from "./components/ConnectDialog";
import { Dashboard } from "./components/Dashboard";

function App() {
  const { state, connect } = useMeshStore();
  const [showConnectDialog, setShowConnectDialog] = useState(false);

  // Show connect dialog if not connected
  useEffect(() => {
    if (state === "NoState" || state === "NeedsLogin") {
      setShowConnectDialog(true);
    } else {
      setShowConnectDialog(false);
    }
  }, [state]);

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
            {state === "Starting" && <p>Connecting to mesh network...</p>}
            {state === "NeedsMachineAuth" && (
              <p>Waiting for machine authorization...</p>
            )}
          </div>
        )}
      </main>

      {showConnectDialog && (
        <ConnectDialog
          onConnect={(config) => {
            connect(config);
            setShowConnectDialog(false);
          }}
          onCancel={() => setShowConnectDialog(false)}
        />
      )}
    </div>
  );
}

export default App;
