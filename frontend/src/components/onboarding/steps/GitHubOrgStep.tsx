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
  const [isValidated, setIsValidated] = useState(false);

  const handleValidate = async () => {
    if (!orgName.trim()) {
      setValidationError('Organization name is required');
      setIsValidated(false);
      return;
    }

    setIsValidating(true);
    setValidationError(null);
    setIsValidated(false);

    try {
      const validation = await validateOrg(orgName.trim());

      if (!validation.valid) {
        setValidationError(validation.error || 'Invalid organization');
        setIsValidated(false);
      } else {
        setIsValidated(true);
      }
    } catch (err) {
      setValidationError(err instanceof Error ? err.message : 'Validation failed');
      setIsValidated(false);
    } finally {
      setIsValidating(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!orgName.trim()) {
      setValidationError('Organization name is required');
      return;
    }

    // If not validated yet, validate first
    if (!isValidated) {
      setIsValidating(true);
      setValidationError(null);

      try {
        const validation = await validateOrg(orgName.trim());

        if (!validation.valid) {
          setValidationError(validation.error || 'Invalid organization');
          setIsValidating(false);
          return;
        }
        setIsValidated(true);
      } catch (err) {
        setValidationError(err instanceof Error ? err.message : 'Validation failed');
        setIsValidating(false);
        return;
      } finally {
        setIsValidating(false);
      }
    }

    // Now proceed to set the org
    setIsValidating(true);
    try {
      await onSetOrg(orgName.trim());
    } catch (err) {
      setValidationError(err instanceof Error ? err.message : 'Failed to set organization');
      setIsValidated(false);
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

        <div className="bg-yellow-900/30 border border-yellow-600 rounded-lg p-4">
          <p className="text-yellow-300 text-sm">
            <strong>Important:</strong> You must be an <strong>admin</strong> of the organization to create a GitHub App.
            If you're not an admin, ask an organization owner to add you or use a different organization.
          </p>
        </div>

        <div className="bg-blue-900/30 border border-blue-600 rounded-lg p-4">
          <p className="text-blue-300 text-sm mb-2">
            <strong>Don't have an organization?</strong>
          </p>
          <p className="text-gray-400 text-sm mb-3">
            Creating a GitHub organization is free and only takes a minute.
          </p>
          <ExternalLink href="https://github.com/organizations/new?plan=free">
            Create a free organization
          </ExternalLink>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="org-name" className="block text-sm font-medium text-gray-300 mb-2">
            Organization name
          </label>
          <div className="relative">
            <input
              id="org-name"
              type="text"
              value={orgName}
              onChange={(e) => {
                setOrgName(e.target.value);
                setValidationError(null);
                setIsValidated(false);
              }}
              onBlur={() => {
                if (orgName.trim() && !isValidated && !validationError) {
                  handleValidate();
                }
              }}
              placeholder="my-organization"
              className={`w-full bg-gray-700 border rounded-lg px-4 py-3 pr-10 text-white placeholder-gray-400 focus:outline-none focus:ring-1 ${
                validationError
                  ? 'border-red-500 focus:border-red-500 focus:ring-red-500'
                  : isValidated
                  ? 'border-green-500 focus:border-green-500 focus:ring-green-500'
                  : 'border-gray-600 focus:border-blue-500 focus:ring-blue-500'
              }`}
              disabled={isLoading || isValidating}
            />
            {/* Validation status icon */}
            <div className="absolute right-3 top-1/2 -translate-y-1/2">
              {isValidating ? (
                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-400" />
              ) : validationError ? (
                <svg className="w-5 h-5 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              ) : isValidated ? (
                <svg className="w-5 h-5 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              ) : null}
            </div>
          </div>
          <p className="mt-1 text-xs text-gray-500">
            Enter the exact name as it appears on GitHub (e.g., "my-org" not "My Organization")
          </p>
          {isValidated && (
            <p className="mt-1 text-xs text-green-400">
              Organization found and validated
            </p>
          )}
        </div>

        <button
          type="submit"
          disabled={isLoading || isValidating || !orgName.trim() || !!validationError}
          className={`w-full font-medium py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2 ${
            isValidated && !validationError
              ? 'bg-green-600 hover:bg-green-700 disabled:bg-green-800'
              : 'bg-blue-600 hover:bg-blue-700 disabled:bg-blue-800'
          } disabled:cursor-not-allowed text-white`}
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
          ) : isValidated ? (
            <>
              <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              <span>Continue</span>
            </>
          ) : (
            'Validate & Continue'
          )}
        </button>
      </form>
    </StepContainer>
  );
}
