import { useState, useEffect } from 'react';
import { api } from '../lib/api';

interface SetupStatus {
  passkey_registered: boolean;
  github_token_set: boolean;
  anthropic_key_set: boolean;
  setup_complete: boolean;
  access_method?: string;
  permanent_url?: string;
}

interface OnboardingProps {
  onComplete: () => void;
}

type OnboardingStep = 'loading' | 'mobile_warning' | 'github_token' | 'anthropic_key' | 'complete';

export function Onboarding({ onComplete }: OnboardingProps) {
  const [_status, setStatus] = useState<SetupStatus | null>(null);
  const [step, setStep] = useState<OnboardingStep>('loading');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  // Form state
  const [githubToken, setGithubToken] = useState('');
  const [anthropicKey, setAnthropicKey] = useState('');

  // Detect mobile device
  const isMobile = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);

  // Fetch setup status on mount
  useEffect(() => {
    fetchStatus();
  }, []);

  const fetchStatus = async () => {
    try {
      const data = await api.get<SetupStatus>('/setup/status');
      setStatus(data);

      // Determine which step we should be on
      if (data.setup_complete) {
        onComplete();
      } else if (!data.passkey_registered && !isMobile) {
        // On desktop without passkey - show mobile warning
        setStep('mobile_warning');
      } else if (!data.github_token_set) {
        setStep('github_token');
      } else if (!data.anthropic_key_set) {
        setStep('anthropic_key');
      } else {
        // All set, complete setup
        await completeSetup();
      }
    } catch (err) {
      console.error('Failed to fetch setup status:', err);
      setError('Failed to check setup status');
      setStep('github_token'); // Default to first step
    }
  };

  const handleContinueOnDesktop = () => {
    setStep('github_token');
  };

  const handleGitHubSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      await api.post('/setup/github-token', { token: githubToken });
      setStep('anthropic_key');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save GitHub token';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  const handleAnthropicSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      await api.post('/setup/anthropic-key', { key: anthropicKey });
      await completeSetup();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save Anthropic API key';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  const completeSetup = async () => {
    try {
      await api.post('/setup/complete', {});
      setStep('complete');
      setTimeout(onComplete, 1500);
    } catch (err) {
      console.error('Failed to complete setup:', err);
      // Still proceed if completion fails
      onComplete();
    }
  };

  if (step === 'loading') {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Checking setup status...</p>
        </div>
      </div>
    );
  }

  if (step === 'complete') {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
        <div className="w-full max-w-md text-center">
          <div className="text-6xl mb-6">&#x1F389;</div>
          <h1 className="text-3xl font-bold mb-4">Setup Complete!</h1>
          <p className="text-gray-400 mb-6">You're all set up. Redirecting to dashboard...</p>
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500 mx-auto" />
        </div>
      </div>
    );
  }

  if (step === 'mobile_warning') {
    const currentUrl = window.location.href;
    const qrCodeUrl = `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(currentUrl)}`;

    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
        <div className="w-full max-w-md">
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-center mb-6">
              <svg className="w-16 h-16 mx-auto text-yellow-400 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={1.5}
                  d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z"
                />
              </svg>
              <h2 className="text-xl font-semibold mb-2">Register Passkey on Mobile</h2>
              <p className="text-gray-400 text-sm">
                For the best security experience, we recommend registering your passkey on your phone.
              </p>
            </div>

            <div className="bg-gray-700 rounded-lg p-4 mb-6">
              <p className="font-medium text-gray-300 mb-3 text-center">Why use mobile?</p>
              <ul className="space-y-2 text-sm text-gray-400">
                <li className="flex items-start gap-2">
                  <span className="text-green-400 mt-0.5">&#x2713;</span>
                  <span>Your private key stays securely on your phone</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-green-400 mt-0.5">&#x2713;</span>
                  <span>Log in from any device by scanning a QR code</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-green-400 mt-0.5">&#x2713;</span>
                  <span>Face ID / Touch ID for instant authentication</span>
                </li>
              </ul>
            </div>

            <div className="text-center mb-6">
              <p className="text-sm text-gray-400 mb-3">Scan to open on your phone:</p>
              <div className="inline-block bg-white p-2 rounded-lg">
                <img
                  src={qrCodeUrl}
                  alt="QR Code"
                  className="w-48 h-48"
                />
              </div>
            </div>

            <button
              onClick={handleContinueOnDesktop}
              className="w-full bg-gray-700 hover:bg-gray-600 text-gray-300 font-medium py-3 px-4 rounded-lg transition-colors"
            >
              Continue on Desktop Anyway
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        {/* Progress indicator */}
        <div className="flex justify-center gap-2 mb-8">
          <div className={`w-3 h-3 rounded-full ${step === 'github_token' ? 'bg-blue-500' : 'bg-gray-600'}`} />
          <div className={`w-3 h-3 rounded-full ${step === 'anthropic_key' ? 'bg-blue-500' : 'bg-gray-600'}`} />
        </div>

        {step === 'github_token' && (
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-center mb-6">
              <svg className="w-16 h-16 mx-auto text-gray-400 mb-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
              </svg>
              <h2 className="text-xl font-semibold mb-2">Connect to GitHub</h2>
              <p className="text-gray-400 text-sm">
                Poindexter needs a GitHub token to manage code and create pull requests.
              </p>
            </div>

            <div className="bg-gray-700 rounded-lg p-4 mb-6 text-sm">
              <p className="font-medium text-gray-300 mb-2">How to create a token:</p>
              <ol className="list-decimal list-inside space-y-1 text-gray-400">
                <li>
                  Go to{' '}
                  <a
                    href="https://github.com/settings/tokens"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-400 hover:underline"
                  >
                    github.com/settings/tokens
                  </a>
                </li>
                <li>Click "Generate new token (classic)"</li>
                <li>Select scopes: <code className="bg-gray-800 px-1 rounded">repo</code>, <code className="bg-gray-800 px-1 rounded">workflow</code></li>
                <li>Copy the token</li>
              </ol>
            </div>

            {error && (
              <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
                <p className="text-red-400 text-sm">{error}</p>
              </div>
            )}

            <form onSubmit={handleGitHubSubmit}>
              <label htmlFor="github-token" className="block text-sm font-medium text-gray-300 mb-2">
                GitHub Token
              </label>
              <input
                id="github-token"
                type="password"
                value={githubToken}
                onChange={(e) => setGithubToken(e.target.value)}
                placeholder="ghp_xxxxxxxxxxxx"
                className="w-full bg-gray-700 border border-gray-600 rounded-lg px-4 py-3 text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent mb-4"
                disabled={isLoading}
                autoComplete="off"
              />

              <button
                type="submit"
                disabled={isLoading || !githubToken.trim()}
                className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isLoading ? (
                  <>
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
                    Validating...
                  </>
                ) : (
                  'Continue'
                )}
              </button>
            </form>
          </div>
        )}

        {step === 'anthropic_key' && (
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-center mb-6">
              <svg className="w-16 h-16 mx-auto text-orange-400 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={1.5}
                  d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
                />
              </svg>
              <h2 className="text-xl font-semibold mb-2">Connect to Anthropic</h2>
              <p className="text-gray-400 text-sm">
                Poindexter uses Claude AI to help with code and tasks.
              </p>
            </div>

            <div className="bg-gray-700 rounded-lg p-4 mb-6 text-sm">
              <p className="font-medium text-gray-300 mb-2">How to get your API key:</p>
              <ol className="list-decimal list-inside space-y-1 text-gray-400">
                <li>
                  Go to{' '}
                  <a
                    href="https://console.anthropic.com"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-400 hover:underline"
                  >
                    console.anthropic.com
                  </a>
                </li>
                <li>Navigate to Settings &rarr; API Keys</li>
                <li>Click "Create Key"</li>
                <li>Copy the key</li>
              </ol>
            </div>

            {error && (
              <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
                <p className="text-red-400 text-sm">{error}</p>
              </div>
            )}

            <form onSubmit={handleAnthropicSubmit}>
              <label htmlFor="anthropic-key" className="block text-sm font-medium text-gray-300 mb-2">
                Anthropic API Key
              </label>
              <input
                id="anthropic-key"
                type="password"
                value={anthropicKey}
                onChange={(e) => setAnthropicKey(e.target.value)}
                placeholder="sk-ant-api03-xxxxxxxxxxxx"
                className="w-full bg-gray-700 border border-gray-600 rounded-lg px-4 py-3 text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent mb-4"
                disabled={isLoading}
                autoComplete="off"
              />

              <button
                type="submit"
                disabled={isLoading || !anthropicKey.trim()}
                className="w-full bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isLoading ? (
                  <>
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
                    Validating...
                  </>
                ) : (
                  'Finish Setup'
                )}
              </button>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}

export default Onboarding;
