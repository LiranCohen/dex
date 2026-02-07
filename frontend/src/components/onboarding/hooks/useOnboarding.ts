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
  workspace_url?: string;

  // Status flags
  passkey_registered: boolean;
  anthropic_key_set: boolean;
  setup_complete: boolean;
  access_method?: string;
  permanent_url?: string;
  workspace_ready: boolean;
  workspace_path?: string;
  workspace_error?: string;
}

export interface OnboardingState {
  status: SetupStatus | null;
  currentStep: string;
  isLoading: boolean;
  error: string | null;
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

  // Fetch status on mount
  useEffect(() => {
    let cancelled = false;

    const loadInitialStatus = async () => {
      try {
        const data = await api.get<SetupStatus>('/setup/status');
        if (!cancelled) {
          setState({
            status: data,
            currentStep: data.current_step || 'welcome',
            isLoading: false,
            error: null,
          });
        }
      } catch (err) {
        if (!cancelled) {
          const message = err instanceof Error ? err.message : 'Failed to fetch setup status';
          setState(prev => ({
            ...prev,
            isLoading: false,
            error: message,
          }));
        }
      }
    };

    loadInitialStatus();

    return () => {
      cancelled = true;
    };
  }, []);

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

  return {
    ...state,
    fetchStatus,
    setError,
    advanceWelcome,
    completePasskey,
    setAnthropicKey,
    completeSetup,
  };
}
