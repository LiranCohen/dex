import { useState } from 'react';
import { StepContainer } from '../shared/StepContainer';
import { ExternalLink } from '../shared/ExternalLink';

interface GitHubOrgStepProps {
  onSetOrg: (orgName: string) => Promise<unknown>;
  validateOrg: (orgName: string) => Promise<{ valid: boolean; error?: string; org_type?: string }>;
  error: string | null;
  isLoading: boolean;
}

export function GitHubOrgStep({ onSetOrg, validateOrg, error, isLoading }: GitHubOrgStepProps) {
  const [orgName, setOrgName] = useState('');
  const [isValidating, setIsValidating] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!orgName.trim()) {
      setValidationError('Organization name is required');
      return;
    }

    setIsValidating(true);
    setValidationError(null);

    try {
      // First validate the org
      const validation = await validateOrg(orgName.trim());

      if (!validation.valid) {
        setValidationError(validation.error || 'Invalid organization');
        setIsValidating(false);
        return;
      }

      // If valid, set it
      await onSetOrg(orgName.trim());
    } catch (err) {
      setValidationError(err instanceof Error ? err.message : 'Failed to set organization');
    } finally {
      setIsValidating(false);
    }
  };

  const displayError = validationError || error;

  return (
    <StepContainer
      title="GitHub Organization"
      description="Dex needs a GitHub organization to create and manage repositories."
      error={displayError}
    >
      <div className="space-y-4 mb-6">
        <div className="bg-gray-700/50 rounded-lg p-4">
          <h3 className="font-medium text-gray-200 mb-2">Why an organization?</h3>
          <p className="text-gray-400 text-sm">
            GitHub Apps can only create repositories in organizations, not personal accounts.
            This is a GitHub API limitation.
          </p>
        </div>

        <div className="bg-blue-900/30 border border-blue-600 rounded-lg p-4">
          <p className="text-blue-300 text-sm mb-2">
            <strong>Don't have an organization?</strong>
          </p>
          <p className="text-gray-400 text-sm mb-3">
            Creating a GitHub organization is free and only takes a minute.
          </p>
          <ExternalLink href="https://github.com/organizations/plan">
            Create a free organization
          </ExternalLink>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="org-name" className="block text-sm font-medium text-gray-300 mb-2">
            Organization name
          </label>
          <input
            id="org-name"
            type="text"
            value={orgName}
            onChange={(e) => {
              setOrgName(e.target.value);
              setValidationError(null);
            }}
            placeholder="my-organization"
            className="w-full bg-gray-700 border border-gray-600 rounded-lg px-4 py-3 text-white placeholder-gray-400 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            disabled={isLoading || isValidating}
          />
          <p className="mt-1 text-xs text-gray-500">
            Enter the exact name as it appears on GitHub (e.g., "my-org" not "My Organization")
          </p>
        </div>

        <button
          type="submit"
          disabled={isLoading || isValidating || !orgName.trim()}
          className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-blue-800 disabled:cursor-not-allowed text-white font-medium py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
        >
          {isValidating ? (
            <>
              <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
              <span>Validating...</span>
            </>
          ) : isLoading ? (
            <>
              <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
              <span>Saving...</span>
            </>
          ) : (
            'Continue'
          )}
        </button>
      </form>
    </StepContainer>
  );
}
