import { Button } from './Button';

interface ObjectiveActionsProps {
  status: string;
  actionLoading: string | null;
  isBlocked?: boolean;
  onStart: () => void;
  onPause: () => void;
  onResume: () => void;
  onCancel: () => void;
}

export function ObjectiveActions({
  status,
  actionLoading,
  isBlocked = false,
  onStart,
  onPause,
  onResume,
  onCancel,
}: ObjectiveActionsProps) {
  // Can only start if status allows it AND not blocked by dependencies
  const canStart = (status === 'ready' || status === 'pending') && !isBlocked;
  const canPause = status === 'running';
  const canResume = status === 'paused';
  const canCancel = status === 'running' || status === 'paused';

  return (
    <div className="app-objective-header__actions">
      {canStart && (
        <Button
          variant="primary"
          onClick={onStart}
          loading={actionLoading === 'start'}
          disabled={!!actionLoading}
        >
          Start
        </Button>
      )}
      {canPause && (
        <Button
          variant="secondary"
          onClick={onPause}
          loading={actionLoading === 'pause'}
          disabled={!!actionLoading}
        >
          Pause
        </Button>
      )}
      {canResume && (
        <Button
          variant="primary"
          onClick={onResume}
          loading={actionLoading === 'resume'}
          disabled={!!actionLoading}
        >
          Resume
        </Button>
      )}
      {canCancel && (
        <Button
          variant="ghost"
          onClick={onCancel}
          disabled={!!actionLoading}
        >
          Cancel
        </Button>
      )}
    </div>
  );
}
