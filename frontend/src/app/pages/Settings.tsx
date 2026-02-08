import { useState, useEffect, useCallback } from 'react';
import { Header, useToast } from '../components';
import { fetchDevices, createDeviceEnrollmentKey, type Device, type EnrollmentKeyResponse } from '../../lib/api';

type SettingsTab = 'devices';

export function Settings() {
  const [activeTab, setActiveTab] = useState<SettingsTab>('devices');

  return (
    <div className="app-root">
      <Header inboxCount={0} />
      <main className="app-content">
        <div className="app-home-header">
          <h1 className="app-page-title">Settings</h1>
        </div>

        {/* Tab navigation */}
        <div className="settings-tabs">
          <button
            type="button"
            className={`settings-tab ${activeTab === 'devices' ? 'settings-tab--active' : ''}`}
            onClick={() => setActiveTab('devices')}
          >
            Devices
          </button>
        </div>

        {/* Tab content */}
        <div className="settings-content">
          {activeTab === 'devices' && <DevicesSection />}
        </div>
      </main>
    </div>
  );
}

function DevicesSection() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddDialog, setShowAddDialog] = useState(false);
  const { showToast } = useToast();

  const loadDevices = useCallback(async () => {
    try {
      const response = await fetchDevices();
      setDevices(response.devices || []);
    } catch (err) {
      console.error('Failed to load devices:', err);
      showToast('Failed to load devices', 'error');
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadDevices();
  }, [loadDevices]);

  // Separate client devices from other peers
  const clientDevices = devices.filter(d => d.is_client);
  const otherPeers = devices.filter(d => !d.is_client);

  if (loading) {
    return (
      <div className="settings-section">
        <p className="settings-loading">Loading devices...</p>
      </div>
    );
  }

  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div>
          <h2 className="settings-section-title">Connected Devices</h2>
          <p className="settings-section-desc">
            Devices connected to your mesh network for direct HQ access.
          </p>
        </div>
        <button
          type="button"
          className="app-btn app-btn--primary"
          onClick={() => setShowAddDialog(true)}
        >
          + Add Device
        </button>
      </div>

      {clientDevices.length === 0 && otherPeers.length === 0 ? (
        <div className="settings-empty">
          <p>No devices connected</p>
          <p className="settings-empty-hint">
            Add a device to access HQ directly from your laptop or desktop.
          </p>
        </div>
      ) : (
        <>
          {clientDevices.length > 0 && (
            <div className="devices-list">
              <h3 className="devices-list-title">Client Devices</h3>
              {clientDevices.map((device) => (
                <DeviceCard key={device.hostname} device={device} />
              ))}
            </div>
          )}

          {otherPeers.length > 0 && (
            <div className="devices-list">
              <h3 className="devices-list-title">Other Peers</h3>
              {otherPeers.map((device) => (
                <DeviceCard key={device.hostname} device={device} />
              ))}
            </div>
          )}
        </>
      )}

      {showAddDialog && (
        <AddDeviceDialog
          onClose={() => setShowAddDialog(false)}
          onDeviceAdded={loadDevices}
        />
      )}
    </div>
  );
}

interface DeviceCardProps {
  device: Device;
}

function DeviceCard({ device }: DeviceCardProps) {
  return (
    <div className={`device-card ${device.online ? 'device-card--online' : 'device-card--offline'}`}>
      <div className="device-card-status">
        <span className={`device-status-dot ${device.online ? 'online' : 'offline'}`} />
        <span className="device-status-text">
          {device.online ? 'Online' : 'Offline'}
        </span>
      </div>
      <div className="device-card-info">
        <h4 className="device-hostname">{device.hostname}</h4>
        <p className="device-ip">{device.mesh_ip || 'No IP assigned'}</p>
        {device.direct && device.online && (
          <span className="device-badge">Direct</span>
        )}
        {device.is_client && (
          <span className="device-badge device-badge--client">Client</span>
        )}
      </div>
      {device.last_seen && !device.online && (
        <p className="device-last-seen">
          Last seen: {new Date(device.last_seen).toLocaleString()}
        </p>
      )}
    </div>
  );
}

interface AddDeviceDialogProps {
  onClose: () => void;
  onDeviceAdded: () => void;
}

function AddDeviceDialog({ onClose, onDeviceAdded }: AddDeviceDialogProps) {
  const [hostname, setHostname] = useState('');
  const [enrollmentKey, setEnrollmentKey] = useState<EnrollmentKeyResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);
  const { showToast } = useToast();

  const handleGenerateKey = async () => {
    setLoading(true);
    try {
      const response = await createDeviceEnrollmentKey(hostname || undefined);
      setEnrollmentKey(response);
    } catch (err) {
      console.error('Failed to create enrollment key:', err);
      showToast('Failed to create enrollment key', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleCopy = async () => {
    if (enrollmentKey) {
      try {
        await navigator.clipboard.writeText(enrollmentKey.install_command);
        setCopied(true);
        showToast('Copied to clipboard', 'success');
        setTimeout(() => setCopied(false), 2000);
      } catch {
        showToast('Failed to copy', 'error');
      }
    }
  };

  const handleClose = () => {
    if (enrollmentKey) {
      onDeviceAdded();
    }
    onClose();
  };

  return (
    <div className="dialog-overlay" onClick={handleClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog-header">
          <h2 className="dialog-title">Add Device</h2>
          <button type="button" className="dialog-close" onClick={handleClose}>
            &times;
          </button>
        </div>

        <div className="dialog-content">
          {!enrollmentKey ? (
            <>
              <p className="dialog-desc">
                Generate an enrollment key to connect a device (laptop, desktop) to your mesh network.
              </p>

              <div className="dialog-field">
                <label htmlFor="hostname" className="dialog-label">
                  Device hostname (optional)
                </label>
                <input
                  id="hostname"
                  type="text"
                  className="dialog-input"
                  placeholder="e.g., macbook, desktop"
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                />
                <p className="dialog-hint">
                  Leave blank for auto-detection
                </p>
              </div>

              <button
                type="button"
                className="app-btn app-btn--primary dialog-btn-full"
                onClick={handleGenerateKey}
                disabled={loading}
              >
                {loading ? 'Generating...' : 'Generate Key'}
              </button>
            </>
          ) : (
            <>
              <p className="dialog-desc dialog-desc--success">
                Enrollment key generated! Run this command on your device:
              </p>

              <div className="dialog-code-block">
                <code>{enrollmentKey.install_command}</code>
                <button
                  type="button"
                  className="dialog-copy-btn"
                  onClick={handleCopy}
                >
                  {copied ? 'Copied!' : 'Copy'}
                </button>
              </div>

              <div className="dialog-info">
                <p><strong>Device:</strong> {enrollmentKey.hostname}</p>
                <p><strong>Expires:</strong> {new Date(enrollmentKey.expires_at).toLocaleString()}</p>
              </div>

              <p className="dialog-hint">
                This key can only be used once and expires in 1 hour.
              </p>

              <button
                type="button"
                className="app-btn app-btn--secondary dialog-btn-full"
                onClick={handleClose}
              >
                Done
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
