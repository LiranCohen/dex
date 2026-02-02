import { useState, useEffect, useCallback } from 'react';
import { api } from '../../../lib/api';

export interface Step {
  id: string;
  name: string;
  status: 'pending' | 'current' | 'complete';
  skippable: boolean;
}

export interface SetupStatus {
  current_step: string;
  steps: Step[];
  github_org?: string;
  github_app_slug?: string;
  workspace_url?: string;

  // Legacy fields for backward compatibility
  passkey_registered: boolean;
  github_token_set: boolean;
  github_app_set: boolean;
  github_auth_method: string;
  anthropic_key_set: boolean;
  setup_complete: boolean;
  access_method?: string;
  permanent_url?: string;
  workspace_ready: boolean;
  workspace_path?: string;
  workspace_github_ready: boolean;
  workspace_github_url?: string;
  workspace_error?: string;
}

export interface OnboardingState {
  status: SetupStatus | null;
  currentStep: string;
  isLoading: boolean;
  error: string | null;
}

interface ValidationResult {
  valid: boolean;
  error?: string;
  org_id?: number;
  org_login?: string;
  org_name?: string;
  org_type?: string;
}

export function useOnboarding() {
  const [state, setState] = useState<OnboardingState>({
    status: null,
    currentStep: 'loading',
    isLoading: true,
    error: null,
  });

  const fetchStatus = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      const data = await api.get<SetupStatus>('/setup/status');
      setState({
        status: data,
        currentStep: data.current_step || 'welcome',
        isLoading: false,
        error: null,
      });
      return data;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch setup status';
      setState(prev => ({
        ...prev,
        isLoading: false,
        error: message,
      }));
      return null;
    }
  }, []);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  const setError = useCallback((error: string | null) => {
    setState(prev => ({ ...prev, error }));
  }, []);

  const advanceWelcome = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      await api.post('/setup/steps/welcome', {});
      await fetchStatus();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to advance';
      setState(prev => ({ ...prev, isLoading: false, error: message }));
    }
  }, [fetchStatus]);

  const completePasskey = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      await api.post('/setup/steps/passkey', {});
      await fetchStatus();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to complete passkey step';
      setState(prev => ({ ...prev, isLoading: false, error: message }));
    }
  }, [fetchStatus]);

  const setGitHubOrg = useCallback(async (orgName: string) => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      const response = await api.post<{ success: boolean; org_login: string }>('/setup/steps/github-org', { org_name: orgName });
      if (response.success) {
        await fetchStatus();
      }
      return response;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to set GitHub organization';
      setState(prev => ({ ...prev, isLoading: false, error: message }));
      throw err;
    }
  }, [fetchStatus]);

  const validateGitHubOrg = useCallback(async (orgName: string): Promise<ValidationResult> => {
    try {
      return await api.post<ValidationResult>('/setup/validate/github-org', { org_name: orgName });
    } catch (err) {
      return {
        valid: false,
        error: err instanceof Error ? err.message : 'Validation failed',
      };
    }
  }, []);

  const completeGitHubInstall = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      await api.post('/setup/steps/github-install', {});
      await fetchStatus();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to complete GitHub install step';
      setState(prev => ({ ...prev, isLoading: false, error: message }));
    }
  }, [fetchStatus]);

  const setAnthropicKey = useCallback(async (key: string) => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      await api.post('/setup/steps/anthropic', { key });
      await fetchStatus();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save Anthropic API key';
      setState(prev => ({ ...prev, isLoading: false, error: message }));
      throw err;
    }
  }, [fetchStatus]);

  const completeSetup = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, isLoading: true, error: null }));
      await api.post('/setup/complete', {});
      await fetchStatus();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to complete setup';
      setState(prev => ({ ...prev, isLoading: false, error: message }));
      throw err;
    }
  }, [fetchStatus]);

  const getGitHubAppManifest = useCallback(async () => {
    return api.get<{
      manifest: Record<string, unknown>;
      manifest_url: string;
    }>('/setup/github/app/manifest');
  }, []);

  return {
    ...state,
    fetchStatus,
    setError,
    advanceWelcome,
    completePasskey,
    setGitHubOrg,
    validateGitHubOrg,
    completeGitHubInstall,
    setAnthropicKey,
    completeSetup,
    getGitHubAppManifest,
  };
}
