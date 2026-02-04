import { useState, useEffect } from 'react';
import { Routes, Route, Navigate, useNavigate } from 'react-router-dom';
import { useAuthStore } from './stores/auth';
import { api } from './lib/api';
import { OnboardingFlow } from './components/onboarding';
import { DexApp } from './app/App';

// Setup status type
interface SetupStatus {
  passkey_registered: boolean;
  github_token_set: boolean;
  github_app_set: boolean;
  anthropic_key_set: boolean;
  setup_complete: boolean;
  workspace_ready: boolean;
  workspace_path?: string;
  workspace_github_ready: boolean;
  workspace_github_url?: string;
  workspace_error?: string;
}

// WebAuthn helper to convert base64url to ArrayBuffer
function base64urlToBuffer(base64url: string): ArrayBuffer {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const binary = atob(base64 + padding);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

// WebAuthn helper to convert ArrayBuffer to base64url
function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const base64 = btoa(binary);
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

function LoginPage() {
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isCheckingStatus, setIsCheckingStatus] = useState(true);
  const [isConfigured, setIsConfigured] = useState(false);

  const setToken = useAuthStore((state) => state.setToken);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const navigate = useNavigate();

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  // Check if passkeys are configured
  useEffect(() => {
    const checkStatus = async () => {
      try {
        const status = await api.get<{ configured: boolean }>('/auth/passkey/status');
        setIsConfigured(status.configured);
      } catch (err) {
        console.error('Failed to check passkey status:', err);
        setError('Failed to connect to server');
      } finally {
        setIsCheckingStatus(false);
      }
    };
    checkStatus();
  }, []);

  // Handle passkey registration (first time setup)
  const handleRegister = async () => {
    setError(null);
    setIsLoading(true);

    try {
      // 1. Begin registration - get options from server
      // go-webauthn wraps the options in a "publicKey" field
      const beginResponse = await api.post<{
        session_id: string;
        user_id: string;
        options: { publicKey: PublicKeyCredentialCreationOptions };
      }>('/auth/passkey/register/begin');

      // 2. Convert base64url fields to ArrayBuffer for WebAuthn API
      const options = beginResponse.options.publicKey;
      const publicKeyOptions: PublicKeyCredentialCreationOptions = {
        ...options,
        challenge: base64urlToBuffer(options.challenge as unknown as string),
        user: {
          ...options.user,
          id: base64urlToBuffer(options.user.id as unknown as string),
        },
        excludeCredentials: options.excludeCredentials?.map((cred) => ({
          ...cred,
          id: base64urlToBuffer(cred.id as unknown as string),
        })),
      };

      // 3. Create credential using WebAuthn API
      const credential = await navigator.credentials.create({
        publicKey: publicKeyOptions,
      }) as PublicKeyCredential;

      if (!credential) {
        throw new Error('Failed to create credential');
      }

      const attestationResponse = credential.response as AuthenticatorAttestationResponse;

      // 4. Send credential to server to complete registration
      // session_id and user_id go in query params, credential in body
      const finishResponse = await api.post<{ token: string; user_id: string }>(
        `/auth/passkey/register/finish?session_id=${encodeURIComponent(beginResponse.session_id)}&user_id=${encodeURIComponent(beginResponse.user_id)}`,
        {
          id: credential.id,
          rawId: bufferToBase64url(credential.rawId),
          type: credential.type,
          response: {
            attestationObject: bufferToBase64url(attestationResponse.attestationObject),
            clientDataJSON: bufferToBase64url(attestationResponse.clientDataJSON),
          },
        }
      );

      // 5. Store JWT and navigate
      setToken(finishResponse.token, finishResponse.user_id);
      navigate('/', { replace: true });
    } catch (err: unknown) {
      let message = 'Registration failed';
      if (err instanceof Error) {
        message = err.message;
      } else if (err && typeof err === 'object' && 'message' in err) {
        message = String((err as { message: unknown }).message);
      }
      console.error('Registration error:', err);
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  // Handle passkey login
  const handleLogin = async () => {
    setError(null);
    setIsLoading(true);

    try {
      // 1. Begin login - get options from server
      // go-webauthn wraps the options in a "publicKey" field
      const beginResponse = await api.post<{
        session_id: string;
        user_id: string;
        options: { publicKey: PublicKeyCredentialRequestOptions };
      }>('/auth/passkey/login/begin');

      // 2. Convert base64url fields to ArrayBuffer for WebAuthn API
      const options = beginResponse.options.publicKey;
      const publicKeyOptions: PublicKeyCredentialRequestOptions = {
        ...options,
        challenge: base64urlToBuffer(options.challenge as unknown as string),
        allowCredentials: options.allowCredentials?.map((cred) => ({
          ...cred,
          id: base64urlToBuffer(cred.id as unknown as string),
        })),
      };

      // 3. Get credential using WebAuthn API
      const credential = await navigator.credentials.get({
        publicKey: publicKeyOptions,
      }) as PublicKeyCredential;

      if (!credential) {
        throw new Error('Failed to get credential');
      }

      const assertionResponse = credential.response as AuthenticatorAssertionResponse;

      // 4. Send assertion to server to complete login
      // session_id and user_id go in query params, credential in body
      const finishResponse = await api.post<{ token: string; user_id: string }>(
        `/auth/passkey/login/finish?session_id=${encodeURIComponent(beginResponse.session_id)}&user_id=${encodeURIComponent(beginResponse.user_id)}`,
        {
          id: credential.id,
          rawId: bufferToBase64url(credential.rawId),
          type: credential.type,
          response: {
            authenticatorData: bufferToBase64url(assertionResponse.authenticatorData),
            clientDataJSON: bufferToBase64url(assertionResponse.clientDataJSON),
            signature: bufferToBase64url(assertionResponse.signature),
            userHandle: assertionResponse.userHandle
              ? bufferToBase64url(assertionResponse.userHandle)
              : null,
          },
        }
      );

      // 5. Store JWT and navigate
      setToken(finishResponse.token, finishResponse.user_id);
      navigate('/', { replace: true });
    } catch (err: unknown) {
      let message = 'Authentication failed';
      if (err instanceof Error) {
        message = err.message;
      } else if (err && typeof err === 'object' && 'message' in err) {
        message = String((err as { message: unknown }).message);
      }
      console.error('Login error:', err);
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  if (isCheckingStatus) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-4xl font-bold mb-2">Poindexter</h1>
          <p className="text-gray-400">Your AI Orchestration Genius</p>
        </div>

        <div className="bg-gray-800 rounded-lg p-6">
          {error && (
            <div className="bg-red-900/50 border border-red-500 rounded-lg p-3 mb-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          {isConfigured ? (
            // Login with existing passkey
            <div className="space-y-4">
              <div className="text-center mb-6">
                <svg
                  className="w-16 h-16 mx-auto text-blue-500 mb-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={1.5}
                    d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
                  />
                </svg>
                <p className="text-gray-300">
                  Use your passkey to sign in
                </p>
              </div>

              <button
                onClick={handleLogin}
                disabled={isLoading}
                className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isLoading ? (
                  <>
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
                    Authenticating...
                  </>
                ) : (
                  <>
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"
                      />
                    </svg>
                    Sign in with Passkey
                  </>
                )}
              </button>
            </div>
          ) : (
            // First time setup - register passkey
            <div className="space-y-4">
              <div className="text-center mb-6">
                <svg
                  className="w-16 h-16 mx-auto text-green-500 mb-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={1.5}
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                  />
                </svg>
                <h2 className="text-xl font-semibold mb-2">Welcome to Poindexter</h2>
                <p className="text-gray-400 text-sm">
                  Set up a passkey to secure your account. You'll use Face ID, Touch ID, or your device's security to sign in.
                </p>
              </div>

              <button
                onClick={handleRegister}
                disabled={isLoading}
                className="w-full bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-semibold py-3 px-4 rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isLoading ? (
                  <>
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white" />
                    Setting up...
                  </>
                ) : (
                  <>
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"
                      />
                    </svg>
                    Set up Passkey
                  </>
                )}
              </button>

              <p className="text-xs text-gray-500 text-center">
                Passkeys are more secure than passwords and easier to use.
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// Protected route wrapper that also handles onboarding
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const [isLoading, setIsLoading] = useState(true);
  const [showOnboarding, setShowOnboarding] = useState(false);

  useEffect(() => {
    if (isAuthenticated) {
      checkSetupStatus();
    }
  }, [isAuthenticated]);

  const checkSetupStatus = async () => {
    setIsLoading(true);
    try {
      const status = await api.get<SetupStatus>('/setup/status');

      // Show onboarding if setup is not complete
      // Check for either GitHub auth method (app or token)
      const hasGitHubAuth = status.github_token_set || status.github_app_set;
      if (!status.setup_complete && (!hasGitHubAuth || !status.anthropic_key_set)) {
        setShowOnboarding(true);
      }
    } catch (err) {
      console.error('Failed to check setup status:', err);
      // Don't block user if setup check fails
    } finally {
      setIsLoading(false);
    }
  };

  const handleOnboardingComplete = () => {
    setShowOnboarding(false);
    checkSetupStatus();
  };

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4" />
          <p className="text-gray-400">Loading...</p>
        </div>
      </div>
    );
  }

  if (showOnboarding) {
    return <OnboardingFlow onComplete={handleOnboardingComplete} />;
  }

  return <>{children}</>;
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <DexApp />
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}

export default App;
