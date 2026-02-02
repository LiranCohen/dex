import { useEffect, useCallback } from 'react';
import { useOnboarding } from './hooks/useOnboarding';
import { StepIndicator } from './shared/StepIndicator';
import { WelcomeStep } from './steps/WelcomeStep';
import { PasskeyStep } from './steps/PasskeyStep';
import { GitHubOrgStep } from './steps/GitHubOrgStep';
import { GitHubAppStep } from './steps/GitHubAppStep';
import { GitHubInstallStep } from './steps/GitHubInstallStep';
import { AnthropicStep } from './steps/AnthropicStep';
import { CompleteStep } from './steps/CompleteStep';

interface OnboardingFlowProps {
  onComplete: () => void;
}

export function OnboardingFlow({ onComplete }: OnboardingFlowProps) {
  const {
    status,
    currentStep,
    isLoading,
    error,
    setError,
    fetchStatus,
    advanceWelcome,
    completePasskey,
    setGitHubOrg,
    validateGitHubOrg,
    completeGitHubInstall,
    setAnthropicKey,
    completeSetup,
    getGitHubAppManifest,
  } = useOnboarding();

  // Check URL params for GitHub callbacks on mount
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const githubAppCreated = params.get('github_app');
    const githubInstalled = params.get('github_installed');

    if (githubAppCreated === 'created' || githubInstalled === 'true') {
      // Clean up URL params
      window.history.replaceState({}, '', window.location.pathname);
      // Refresh status to get updated step
      fetchStatus();
    }
  }, [fetchStatus]);

  // Handle completion
  useEffect(() => {
    if (status?.setup_complete) {
      const timer = setTimeout(onComplete, 1500);
      return () => clearTimeout(timer);
    }
  }, [status?.setup_complete, onComplete]);

  const handleCompleteSetup = useCallback(async () => {
    await completeSetup();
  }, [completeSetup]);

  // Render loading state
  if (isLoading && !status) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Checking setup status...</p>
        </div>
      </div>
    );
  }

  // Render current step
  const renderStep = () => {
    switch (currentStep) {
      case 'welcome':
        return (
          <WelcomeStep
            onContinue={advanceWelcome}
            isLoading={isLoading}
          />
        );

      case 'passkey':
        return (
          <PasskeyStep
            onComplete={completePasskey}
            error={error}
            setError={setError}
          />
        );

      case 'github_org':
        return (
          <GitHubOrgStep
            onSetOrg={setGitHubOrg}
            validateOrg={validateGitHubOrg}
            error={error}
            isLoading={isLoading}
          />
        );

      case 'github_app':
        return (
          <GitHubAppStep
            orgName={status?.github_org || ''}
            getManifest={getGitHubAppManifest}
            error={error}
          />
        );

      case 'github_install':
        return (
          <GitHubInstallStep
            orgName={status?.github_org || ''}
            appSlug={status?.github_app_slug}
            onComplete={completeGitHubInstall}
            error={error}
          />
        );

      case 'anthropic':
        return (
          <AnthropicStep
            onSetKey={setAnthropicKey}
            error={error}
            isLoading={isLoading}
          />
        );

      case 'complete':
        return (
          <CompleteStep
            onComplete={handleCompleteSetup}
            workspaceUrl={status?.workspace_url}
            error={error}
          />
        );

      default:
        return (
          <WelcomeStep
            onContinue={advanceWelcome}
            isLoading={isLoading}
          />
        );
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 text-white flex flex-col items-center justify-center p-4">
      {/* Step indicator */}
      {status?.steps && status.steps.length > 0 && (
        <StepIndicator steps={status.steps} currentStep={currentStep} />
      )}

      {/* Current step content */}
      {renderStep()}

      {/* Footer */}
      <div className="mt-8 text-center text-sm text-gray-500">
        <p>Dex - AI Development Assistant</p>
      </div>
    </div>
  );
}
