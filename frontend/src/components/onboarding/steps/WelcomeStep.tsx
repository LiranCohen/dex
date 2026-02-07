import { StepContainer } from '../shared/StepContainer';

interface WelcomeStepProps {
  onContinue: () => void;
  isLoading: boolean;
}

export function WelcomeStep({ onContinue, isLoading }: WelcomeStepProps) {
  return (
    <StepContainer
      title="Welcome to Dex"
      description="Your AI-powered development assistant"
    >
      <div className="app-onboarding-content">
        <div className="app-onboarding-box">
          <h3 className="app-onboarding-box__title">What is Dex?</h3>
          <p className="app-onboarding-box__text">
            Dex is an AI development assistant that helps you build software. It can create tasks,
            write code, make commits, and open pull requests on your behalf.
          </p>
        </div>

        <div className="app-onboarding-box">
          <h3 className="app-onboarding-box__title">What you'll need:</h3>
          <ul className="app-onboarding-list">
            <li className="app-onboarding-list__item">
              <span className="app-onboarding-list__marker">1.</span>
              <span><strong>A passkey</strong> - for secure authentication</span>
            </li>
            <li className="app-onboarding-list__item">
              <span className="app-onboarding-list__marker">2.</span>
              <span><strong>An Anthropic API key</strong> - to power the AI assistant</span>
            </li>
          </ul>
        </div>
      </div>

      <button
        onClick={onContinue}
        disabled={isLoading}
        className="app-onboarding-btn"
      >
        {isLoading ? 'Loading...' : 'Get Started'}
      </button>
    </StepContainer>
  );
}
