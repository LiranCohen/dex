import { useState } from 'react';
import { StepContainer } from '../shared/StepContainer';
import { ExternalLink } from '../shared/ExternalLink';

interface AnthropicStepProps {
  onSetKey: (key: string) => Promise<void>;
  error: string | null;
  isLoading: boolean;
}

export function AnthropicStep({ onSetKey, error, isLoading }: AnthropicStepProps) {
  const [apiKey, setApiKey] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!apiKey.trim()) {
      setValidationError('API key is required');
      return;
    }

    if (!apiKey.startsWith('sk-ant')) {
      setValidationError('Invalid API key format. It should start with "sk-ant".');
      return;
    }

    setValidationError(null);

    try {
      await onSetKey(apiKey.trim());
    } catch (err) {
      setValidationError(err instanceof Error ? err.message : 'Failed to save API key');
    }
  };

  const displayError = validationError || error;

  return (
    <StepContainer
      title="Anthropic API Key"
      description="Dex uses Claude to power its AI capabilities."
      error={displayError}
    >
      <div className="space-y-4 mb-6">
        {/* Direct link to API keys */}
        <div className="bg-blue-900/30 border border-blue-600 rounded-lg p-4">
          <p className="text-blue-300 text-sm mb-3">
            <strong>Have an Anthropic account?</strong> Go directly to your API keys:
          </p>
          <a
            href="https://console.anthropic.com/settings/keys"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 bg-blue-600 hover:bg-blue-700 text-white font-medium py-2 px-4 rounded-lg transition-colors text-sm"
          >
            Open API Keys Dashboard
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
          </a>
        </div>

        {/* Instructions for new users */}
        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">New to Anthropic? Here's how to get started:</h3>
          <ol className="space-y-2 text-sm text-gray-400 list-decimal list-inside">
            <li>
              Go to{' '}
              <ExternalLink href="https://console.anthropic.com/signup">
                console.anthropic.com/signup
              </ExternalLink>
            </li>
            <li>Create an account with your email or Google</li>
            <li>Complete the verification process</li>
            <li>
              Add billing info at{' '}
              <ExternalLink href="https://console.anthropic.com/settings/billing">
                Settings → Billing
              </ExternalLink>
            </li>
            <li>
              Go to{' '}
              <ExternalLink href="https://console.anthropic.com/settings/keys">
                Settings → API Keys
              </ExternalLink>
            </li>
            <li>Click "Create Key" and copy it</li>
          </ol>
        </div>

        <div className="bg-yellow-900/30 border border-yellow-600 rounded-lg p-4">
          <p className="text-yellow-300 text-sm">
            <strong>Important:</strong> Your API key is stored securely and never shared.
            Make sure you have billing set up on your Anthropic account.
          </p>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="api-key" className="block text-sm font-medium text-gray-300 mb-2">
            API Key
          </label>
          <input
            id="api-key"
            type="password"
            value={apiKey}
            onChange={(e) => {
              setApiKey(e.target.value);
              setValidationError(null);
            }}
            placeholder="sk-ant-..."
            className="w-full bg-gray-700 border border-gray-600 rounded-lg px-4 py-3 text-white placeholder-gray-400 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 font-mono"
            disabled={isLoading}
          />
        </div>

        <button
          type="submit"
          disabled={isLoading || !apiKey.trim()}
          className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-blue-800 disabled:cursor-not-allowed text-white font-medium py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
        >
          {isLoading ? (
            <>
              <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
              <span>Validating...</span>
            </>
          ) : (
            'Continue'
          )}
        </button>
      </form>
    </StepContainer>
  );
}
