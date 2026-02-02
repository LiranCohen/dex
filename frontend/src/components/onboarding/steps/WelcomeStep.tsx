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
      <div className="space-y-4 mb-6">
        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">What is Dex?</h3>
          <p className="text-gray-400 text-sm">
            Dex is an AI development assistant that helps you build software. It can create tasks,
            write code, make commits, and open pull requests on your behalf.
          </p>
        </div>

        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">What you'll need:</h3>
          <ul className="space-y-2 text-sm text-gray-400">
            <li className="flex items-start gap-2">
              <span className="text-blue-400 mt-0.5">1.</span>
              <span><strong>A passkey</strong> - for secure authentication</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400 mt-0.5">2.</span>
              <span><strong>A GitHub organization</strong> - where Dex will create repositories</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400 mt-0.5">3.</span>
              <span><strong>An Anthropic API key</strong> - to power the AI assistant</span>
            </li>
          </ul>
        </div>
      </div>

      <button
        onClick={onContinue}
        disabled={isLoading}
        className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-blue-800 disabled:cursor-not-allowed text-white font-medium py-3 px-4 rounded-lg transition-colors"
      >
        {isLoading ? 'Loading...' : 'Get Started'}
      </button>
    </StepContainer>
  );
}
