import type { ConnectionState } from '../../hooks/useWebSocket';

interface ConnectionStatusBannerProps {
  connectionState: ConnectionState;
  reconnectAttempts: number;
  onReconnect: () => void;
}

export function ConnectionStatusBanner({
  connectionState,
  reconnectAttempts,
  onReconnect,
}: ConnectionStatusBannerProps) {
  // Don't show banner when connected
  if (connectionState === 'connected') {
    return null;
  }

  const getStatusInfo = () => {
    switch (connectionState) {
      case 'reconnecting':
        return {
          icon: '🔄',
          message: `Reconnecting to server${reconnectAttempts > 0 ? ` (attempt ${reconnectAttempts}/5)` : ''}...`,
          showButton: false,
          variant: 'warning' as const,
        };
      case 'failed':
        return {
          icon: '🔌',
          message: 'Connection lost. Real-time updates are unavailable.',
          showButton: true,
          variant: 'error' as const,
        };
      case 'disconnected':
      default:
        return {
          icon: '📡',
          message: 'Disconnected from server.',
          showButton: true,
          variant: 'warning' as const,
        };
    }
  };

  const status = getStatusInfo();

  return (
    <div
      className={`v2-connection-banner v2-connection-banner--${status.variant}`}
      role="alert"
      aria-live="polite"
    >
      <span className="v2-connection-banner__icon" aria-hidden="true">
        {status.icon}
      </span>
      <span className="v2-connection-banner__message">
        {status.message}
      </span>
      {status.showButton && (
        <button
          type="button"
          className="v2-connection-banner__btn"
          onClick={onReconnect}
        >
          Reconnect
        </button>
      )}
      {connectionState === 'reconnecting' && (
        <div className="v2-connection-banner__spinner" aria-hidden="true" />
      )}
    </div>
  );
}
