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
        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">How to get an API key:</h3>
          <ol className="space-y-2 text-sm text-gray-400 list-decimal list-inside">
            <li>
              Go to the{' '}
              <ExternalLink href="https://console.anthropic.com/">
                Anthropic Console
              </ExternalLink>
            </li>
            <li>Sign in or create an account</li>
            <li>Navigate to API Keys</li>
            <li>Create a new API key</li>
            <li>Copy and paste it below</li>
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
