import { useState } from 'react';
import { api } from '../../../lib/api';
import { useAuthStore } from '../../../stores/auth';
import { StepContainer } from '../shared/StepContainer';

interface PasskeyStepProps {
  onComplete: () => void;
  error: string | null;
  setError: (error: string | null) => void;
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

export function PasskeyStep({ onComplete, error, setError }: PasskeyStepProps) {
  const [isLoading, setIsLoading] = useState(false);
  const setToken = useAuthStore((state) => state.setToken);

  // Detect mobile device
  const isMobile = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);

  const handleRegisterPasskey = async () => {
    setError(null);
    setIsLoading(true);

    try {
      // 1. Begin registration - get options from server
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

      // 5. Store JWT and proceed to next step
      setToken(finishResponse.token, finishResponse.user_id);
      onComplete();
    } catch (err: unknown) {
      let message = 'Passkey registration failed';
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

  return (
    <StepContainer
      title="Create Your Passkey"
      description="Passkeys provide secure, passwordless authentication using your device's biometrics."
      error={error}
    >
      <div className="app-onboarding-content">
        <div className="app-onboarding-box">
          <h3 className="app-onboarding-box__title">What is a passkey?</h3>
          <p className="app-onboarding-box__text">
            A passkey is a modern, secure alternative to passwords. It uses your device's
            biometrics (Face ID, Touch ID, or Windows Hello) to authenticate you.
          </p>
        </div>

        {!isMobile && (
          <div className="app-onboarding-box app-onboarding-box--warning">
            <p className="app-onboarding-box__text app-onboarding-box__text--warning">
              <strong>Tip:</strong> For the best experience, register your passkey on a mobile device.
              You can then use it to authenticate from any device.
            </p>
          </div>
        )}

        <ul className="app-onboarding-list">
          <li className="app-onboarding-list__item">
            <span className="app-onboarding-list__marker app-onboarding-list__marker--success">&#x2713;</span>
            <span>Secure - your private key never leaves your device</span>
          </li>
          <li className="app-onboarding-list__item">
            <span className="app-onboarding-list__marker app-onboarding-list__marker--success">&#x2713;</span>
            <span>Fast - authenticate instantly with biometrics</span>
          </li>
          <li className="app-onboarding-list__item">
            <span className="app-onboarding-list__marker app-onboarding-list__marker--success">&#x2713;</span>
            <span>Phishing-resistant - bound to this domain only</span>
          </li>
        </ul>
      </div>

      <button
        onClick={handleRegisterPasskey}
        disabled={isLoading}
        className="app-onboarding-btn"
      >
        {isLoading ? (
          <>
            <div className="app-onboarding-btn__spinner" />
            <span>Registering...</span>
          </>
        ) : (
          <>
            <svg className="app-onboarding-btn__icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"
              />
            </svg>
            <span>Register Passkey</span>
          </>
        )}
      </button>
    </StepContainer>
  );
}
