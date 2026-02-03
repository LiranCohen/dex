type ToolStatus = 'running' | 'complete' | 'error';

interface ToolActivityProps {
  status: ToolStatus;
  tool: string;
  description: string;
}

export function ToolActivity({ status, description }: ToolActivityProps) {
  return (
    <div className={`v2-tool-activity v2-tool-activity--${status}`}>
      {status === 'running' && (
        <div className="v2-tool-activity__spinner" aria-hidden="true" />
      )}
      {status === 'complete' && (
        <span className="v2-tool-activity__icon--complete" aria-hidden="true">·</span>
      )}
      {status === 'error' && (
        <span className="v2-tool-activity__icon--error" aria-hidden="true">✗</span>
      )}
      <span>{description}</span>
    </div>
  );
}
