import { useState } from 'react';
import { StepContainer } from '../shared/StepContainer';

interface GitHubAppStepProps {
  orgName: string;
  getManifest: () => Promise<{ manifest: Record<string, unknown>; manifest_url: string }>;
  error: string | null;
}

export function GitHubAppStep({ orgName, getManifest, error }: GitHubAppStepProps) {
  const [isLoading, setIsLoading] = useState(false);

  const handleCreateApp = async () => {
    setIsLoading(true);

    try {
      // Get the manifest from the server
      const { manifest } = await getManifest();

      // Create a form to POST to GitHub's manifest URL
      const form = document.createElement('form');
      form.method = 'POST';
      form.action = 'https://github.com/settings/apps/new';
      form.target = '_self';

      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = 'manifest';
      input.value = JSON.stringify(manifest);
      form.appendChild(input);

      document.body.appendChild(form);
      form.submit();
    } catch (err) {
      console.error('Failed to create GitHub App:', err);
      setIsLoading(false);
    }
  };

  return (
    <StepContainer
      title="Create GitHub App"
      description={`Create a GitHub App for your organization "${orgName}".`}
      error={error}
    >
      <div className="space-y-4 mb-6">
        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">What is a GitHub App?</h3>
          <p className="text-gray-400 text-sm">
            A GitHub App is a first-class integration that acts on its own behalf.
            Dex will use this app to create repositories and manage code.
          </p>
        </div>

        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">Permissions requested:</h3>
          <ul className="space-y-1 text-sm text-gray-400">
            <li className="flex items-start gap-2">
              <span className="text-blue-400">&#x2022;</span>
              <span>Repository administration (create/delete repos)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">&#x2022;</span>
              <span>Contents (read/write code)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">&#x2022;</span>
              <span>Issues (create/update issues)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">&#x2022;</span>
              <span>Pull requests (create PRs)</span>
            </li>
          </ul>
        </div>

        <div className="bg-yellow-900/30 border border-yellow-600 rounded-lg p-4">
          <p className="text-yellow-300 text-sm">
            <strong>Note:</strong> You'll be redirected to GitHub to create and authorize the app.
            After creation, you'll be sent back here automatically.
          </p>
        </div>
      </div>

      <button
        onClick={handleCreateApp}
        disabled={isLoading}
        className="w-full bg-gray-700 hover:bg-gray-600 disabled:bg-gray-800 disabled:cursor-not-allowed text-white font-medium py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2 border border-gray-600"
      >
        {isLoading ? (
          <>
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
            <span>Redirecting to GitHub...</span>
          </>
        ) : (
          <>
            <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
            </svg>
            <span>Create GitHub App</span>
          </>
        )}
      </button>
    </StepContainer>
  );
}
