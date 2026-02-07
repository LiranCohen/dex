import { useEffect, useCallback } from 'react';
import { useOnboarding } from './hooks/useOnboarding';
import { StepIndicator } from './shared/StepIndicator';
import { WelcomeStep } from './steps/WelcomeStep';
import { PasskeyStep } from './steps/PasskeyStep';
import { AnthropicStep } from './steps/AnthropicStep';
import { CompleteStep } from './steps/CompleteStep';
import '../../app/styles/app.css';

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
    advanceWelcome,
    completePasskey,
    setAnthropicKey,
    completeSetup,
  } = useOnboarding();

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
      <div className="app-root app-onboarding-root">
        <div className="app-onboarding-loading">
          <div className="app-spinner" />
          <p className="app-onboarding-loading-text">Checking setup status...</p>
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
    <div className="app-root app-onboarding-root">
      {/* Step indicator */}
      {status?.steps && status.steps.length > 0 && (
        <StepIndicator steps={status.steps} currentStep={currentStep} />
      )}

      {/* Current step content */}
      {renderStep()}

      {/* Footer */}
      <div className="app-onboarding-footer">
        <p>Dex - AI Development Assistant</p>
      </div>
    </div>
  );
}
