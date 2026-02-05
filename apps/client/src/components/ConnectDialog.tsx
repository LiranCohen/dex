/**
 * Connect dialog for entering mesh configuration
 */

import { useState } from "react";
import type { MeshConfig } from "../mesh/types";

interface ConnectDialogProps {
  onConnect: (config: MeshConfig) => void;
  onCancel: () => void;
}

export function ConnectDialog({ onConnect, onCancel }: ConnectDialogProps) {
  const [controlURL, setControlURL] = useState("https://central.dex.dev");
  const [authKey, setAuthKey] = useState("");
  const [hostname, setHostname] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onConnect({
      controlURL,
      authKey: authKey || undefined,
      hostname: hostname || undefined,
    });
  };

  return (
    <div className="dialog-overlay">
      <div className="dialog">
        <h2>Connect to Dex Network</h2>

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="authKey">Auth Key (optional)</label>
            <input
              id="authKey"
              type="password"
              value={authKey}
              onChange={(e) => setAuthKey(e.target.value)}
              placeholder="dexkey-auth-..."
              autoComplete="off"
            />
            <p className="form-hint">
              Pre-authentication key for automatic login.
              Leave empty to use browser-based login.
            </p>
          </div>

          <button
            type="button"
            className="advanced-toggle"
            onClick={() => setShowAdvanced(!showAdvanced)}
          >
            {showAdvanced ? "Hide" : "Show"} Advanced Options
          </button>

          {showAdvanced && (
            <>
              <div className="form-group">
                <label htmlFor="controlURL">Central URL</label>
                <input
                  id="controlURL"
                  type="url"
                  value={controlURL}
                  onChange={(e) => setControlURL(e.target.value)}
                  placeholder="https://central.dex.dev"
                />
              </div>

              <div className="form-group">
                <label htmlFor="hostname">Device Name (optional)</label>
                <input
                  id="hostname"
                  type="text"
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  placeholder="Auto-generated if empty"
                />
              </div>
            </>
          )}

          <div className="dialog-actions">
            <button type="button" className="btn-secondary" onClick={onCancel}>
              Cancel
            </button>
            <button type="submit" className="btn-primary">
              Connect
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
