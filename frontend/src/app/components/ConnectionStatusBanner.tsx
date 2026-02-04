import type { ConnectionState, ConnectionQuality } from '../../hooks/useWebSocket';

interface ConnectionStatusBannerProps {
  connectionState: ConnectionState;
  connectionQuality?: ConnectionQuality;
  latency?: number | null;
  reconnectAttempts: number;
  onReconnect: () => void;
}

function QualityIndicator({ quality, latency }: { quality: ConnectionQuality; latency?: number | null }) {
  const getQualityInfo = () => {
    switch (quality) {
      case 'excellent':
        return { bars: 3, color: 'var(--color-success)', label: 'Excellent connection' };
      case 'good':
        return { bars: 2, color: 'var(--color-warning)', label: 'Good connection' };
      case 'poor':
        return { bars: 1, color: 'var(--color-error)', label: 'Poor connection' };
      default:
        return { bars: 0, color: 'var(--color-muted)', label: 'Disconnected' };
    }
  };

  const info = getQualityInfo();
  const latencyText = latency !== null && latency !== undefined ? `${latency}ms` : '';

  return (
    <div
      className="app-connection-quality"
      title={`${info.label}${latencyText ? ` (${latencyText})` : ''}`}
      aria-label={info.label}
    >
      <div className="app-connection-quality__bars">
        {[1, 2, 3].map((bar) => (
          <div
            key={bar}
            className={`app-connection-quality__bar ${bar <= info.bars ? 'app-connection-quality__bar--active' : ''}`}
            style={{ backgroundColor: bar <= info.bars ? info.color : undefined }}
          />
        ))}
      </div>
      {latencyText && (
        <span className="app-connection-quality__latency">{latencyText}</span>
      )}
    </div>
  );
}

export function ConnectionStatusBanner({
  connectionState,
  connectionQuality,
  latency,
  reconnectAttempts,
  onReconnect,
}: ConnectionStatusBannerProps) {
  // Show quality indicator when connected
  if (connectionState === 'connected') {
    // Only show quality indicator if we have quality data and it's not excellent
    if (connectionQuality && connectionQuality !== 'excellent') {
      return (
        <div
          className="app-connection-banner app-connection-banner--info"
          role="status"
          aria-live="polite"
        >
          <QualityIndicator quality={connectionQuality} latency={latency} />
          <span className="app-connection-banner__message">
            {connectionQuality === 'poor' ? 'Connection is slow' : 'Connected'}
          </span>
        </div>
      );
    }
    return null;
  }

  const getStatusInfo = () => {
    switch (connectionState) {
      case 'reconnecting':
        return {
          icon: null,
          message: `Reconnecting to server${reconnectAttempts > 0 ? ` (attempt ${reconnectAttempts}/5)` : ''}...`,
          showButton: false,
          variant: 'warning' as const,
        };
      case 'failed':
        return {
          icon: null,
          message: 'Connection lost. Real-time updates are unavailable.',
          showButton: true,
          variant: 'error' as const,
        };
      case 'disconnected':
      default:
        return {
          icon: null,
          message: 'Disconnected from server.',
          showButton: true,
          variant: 'warning' as const,
        };
    }
  };

  const status = getStatusInfo();

  return (
    <div
      className={`app-connection-banner app-connection-banner--${status.variant}`}
      role="alert"
      aria-live="polite"
    >
      <QualityIndicator quality="disconnected" />
      <span className="app-connection-banner__message">
        {status.message}
      </span>
      {status.showButton && (
        <button
          type="button"
          className="app-connection-banner__btn"
          onClick={onReconnect}
        >
          Reconnect
        </button>
      )}
      {connectionState === 'reconnecting' && (
        <div className="app-connection-banner__spinner" aria-hidden="true" />
      )}
    </div>
  );
}
