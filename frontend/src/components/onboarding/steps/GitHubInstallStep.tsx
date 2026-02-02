import { useState, useEffect } from 'react';
import { api } from '../../../lib/api';
import { StepContainer } from '../shared/StepContainer';

interface GitHubInstallStepProps {
  orgName: string;
  appSlug?: string;
  onComplete: () => void;
  error: string | null;
}

interface GitHubAppStatus {
  app_configured: boolean;
  app_slug?: string;
  install_url?: string;
  installations: number;
  auth_method: string;
}

export function GitHubInstallStep({ orgName, appSlug: initialSlug, onComplete, error }: GitHubInstallStepProps) {
  const [isLoading] = useState(false);
  const [appStatus, setAppStatus] = useState<GitHubAppStatus | null>(null);

  useEffect(() => {
    // Check URL params for install callback
    const params = new URLSearchParams(window.location.search);
    const githubInstalled = params.get('github_installed');

    if (githubInstalled === 'true') {
      // Clean up URL
      window.history.replaceState({}, '', window.location.pathname);
      // Complete the step
      onComplete();
      return;
    }

    // Fetch app status to get install URL
    const fetchStatus = async () => {
      try {
        const status = await api.get<GitHubAppStatus>('/setup/github/app/status');
        setAppStatus(status);
      } catch (err) {
        console.error('Failed to fetch GitHub App status:', err);
      }
    };

    fetchStatus();
  }, [onComplete]);

  const handleInstall = () => {
    const slug = appStatus?.app_slug || initialSlug;
    if (slug) {
      // Redirect to GitHub to install the app
      const installUrl = appStatus?.install_url || `https://github.com/apps/${slug}/installations/new`;
      window.location.href = installUrl;
    }
  };

  const slug = appStatus?.app_slug || initialSlug;

  return (
    <StepContainer
      title={`Install Dex on ${orgName}`}
      description="Install the GitHub App on your organization to give Dex access to create and manage repositories."
      error={error}
    >
      <div className="space-y-4 mb-6">
        <div className="bg-gray-700/50 rounded-lg p-4">
          <div className="flex items-center gap-3 mb-3">
            <div className="w-10 h-10 bg-gray-600 rounded-lg flex items-center justify-center">
              <svg className="w-6 h-6 text-gray-300" fill="currentColor" viewBox="0 0 24 24">
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
              </svg>
            </div>
            <div>
              <p className="font-medium text-gray-200">{slug || 'dex-app'}</p>
              <p className="text-sm text-gray-400">Ready to install on: {orgName}</p>
            </div>
          </div>
        </div>

        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">Permissions granted:</h3>
          <ul className="space-y-1 text-sm text-gray-400">
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>Repository administration (create/delete repos)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>Contents (read/write code)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>Issues (create/update issues)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400">&#x2713;</span>
              <span>Pull requests (create PRs)</span>
            </li>
          </ul>
        </div>

        <div className="bg-blue-900/30 border border-blue-600 rounded-lg p-4">
          <p className="text-blue-300 text-sm">
            <strong>Note:</strong> You'll be redirected to GitHub to complete the installation.
            Make sure to select "{orgName}" as the target organization.
          </p>
        </div>
      </div>

      <button
        onClick={handleInstall}
        disabled={isLoading || !slug}
        className="w-full bg-green-600 hover:bg-green-700 disabled:bg-green-800 disabled:cursor-not-allowed text-white font-medium py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
      >
        {isLoading ? (
          <>
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
            <span>Redirecting...</span>
          </>
        ) : (
          <>
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
            <span>Install on GitHub</span>
          </>
        )}
      </button>
    </StepContainer>
  );
}
