import { useEffect, useState } from 'react';
import { StepContainer } from '../shared/StepContainer';

interface CompleteStepProps {
  onComplete: () => Promise<void>;
  workspaceUrl?: string;
  error: string | null;
}

export function CompleteStep({ onComplete, workspaceUrl, error }: CompleteStepProps) {
  const [isCompleting, setIsCompleting] = useState(true);
  const [completed, setCompleted] = useState(false);

  useEffect(() => {
    const complete = async () => {
      try {
        await onComplete();
        setCompleted(true);
      } catch (err) {
        console.error('Failed to complete setup:', err);
      } finally {
        setIsCompleting(false);
      }
    };

    complete();
  }, [onComplete]);

  if (isCompleting) {
    return (
      <StepContainer
        title="Completing Setup"
        description="Creating your workspace and finalizing configuration..."
        error={error}
      >
        <div className="flex flex-col items-center py-8">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500 mb-4" />
          <p className="text-gray-400">This may take a moment...</p>
        </div>
      </StepContainer>
    );
  }

  return (
    <StepContainer
      title="Setup Complete!"
      description="You're all set up and ready to use Dex."
      error={error}
    >
      <div className="space-y-4 mb-6">
        <div className="flex justify-center py-4">
          <div className="w-20 h-20 bg-green-500/20 rounded-full flex items-center justify-center">
            <svg className="w-10 h-10 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
        </div>

        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">What's next?</h3>
          <ul className="space-y-2 text-sm text-gray-400">
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>Your workspace repository has been created</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>Dex is connected to your GitHub organization</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>AI assistant is ready to help you build</span>
            </li>
          </ul>
        </div>

        {workspaceUrl && (
          <div className="app-onboarding-workspace">
            <p className="app-onboarding-workspace-label">
              <strong>Your workspace:</strong>
            </p>
            <code className="app-onboarding-workspace-url">{workspaceUrl}</code>
            <button
              type="button"
              className="app-btn app-btn--primary app-onboarding-workspace-btn"
              onClick={() => window.location.href = workspaceUrl}
            >
              Go to Workspace
            </button>
          </div>
        )}
      </div>

      {completed && (
        <div className="text-center text-gray-400 text-sm">
          <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-green-500 mx-auto mb-2" />
          Redirecting to dashboard...
        </div>
      )}
    </StepContainer>
  );
}
