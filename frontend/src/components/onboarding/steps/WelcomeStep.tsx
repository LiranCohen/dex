import { useEffect, useState } from 'react';
import { StepContainer } from '../shared/StepContainer';
import { api } from '../../../lib/api';

interface DexProfileData {
  traits: string[];
  greeting_style: string;
  catchphrase: string;
  has_avatar: boolean;
  onboarding_messages?: {
    welcome?: string;
    tagline?: string;
    [key: string]: unknown;
  };
}

interface WelcomeStepProps {
  onContinue: () => void;
  isLoading: boolean;
}

export function WelcomeStep({ onContinue, isLoading }: WelcomeStepProps) {
  const [dexProfile, setDexProfile] = useState<DexProfileData | null>(null);

  useEffect(() => {
    api.get<DexProfileData>('/setup/dex-profile')
      .then(setDexProfile)
      .catch(() => { /* No profile available — use default welcome */ });
  }, []);

  // If we have a Dex profile, show the personalized welcome
  if (dexProfile) {
    const welcomeMsg = dexProfile.onboarding_messages?.welcome || `Hey there! I'm your Poindexter.`;
    const tagline = dexProfile.onboarding_messages?.tagline;

    return (
      <StepContainer
        title="Meet your Poindexter"
        description={tagline || 'Your AI-powered development assistant'}
      >
        <div className="app-onboarding-content">
          {/* Avatar + greeting */}
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
            <div style={{
              width: '56px',
              height: '56px',
              borderRadius: '50%',
              overflow: 'hidden',
              flexShrink: 0,
              background: 'var(--color-surface-2, #262626)',
            }}>
              {dexProfile.has_avatar ? (
                <img
                  src="/api/v1/setup/dex-avatar"
                  alt="Poindexter"
                  style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                />
              ) : (
                <svg viewBox="0 0 56 56" fill="currentColor" style={{ width: '100%', height: '100%', color: 'var(--color-text-tertiary, #666)' }}>
                  <circle cx="28" cy="21" r="11" />
                  <ellipse cx="28" cy="50" rx="20" ry="14" />
                </svg>
              )}
            </div>
            <div className="app-onboarding-box" style={{ flex: 1 }}>
              <p className="app-onboarding-box__text">{welcomeMsg}</p>
            </div>
          </div>

          <div className="app-onboarding-box">
            <h3 className="app-onboarding-box__title">What you'll need:</h3>
            <ul className="app-onboarding-list">
              <li className="app-onboarding-list__item">
                <span className="app-onboarding-list__marker">1.</span>
                <span><strong>A passkey</strong> — for secure authentication</span>
              </li>
              <li className="app-onboarding-list__item">
                <span className="app-onboarding-list__marker">2.</span>
                <span><strong>An Anthropic API key</strong> — to power the AI assistant</span>
              </li>
            </ul>
          </div>
        </div>

        <button
          onClick={onContinue}
          disabled={isLoading}
          className="app-onboarding-btn"
        >
          {isLoading ? 'Loading...' : 'Get Started'}
        </button>
      </StepContainer>
    );
  }

  // Default welcome (no Dex profile available)
  return (
    <StepContainer
      title="Welcome to Dex"
      description="Your AI-powered development assistant"
    >
      <div className="app-onboarding-content">
        <div className="app-onboarding-box">
          <h3 className="app-onboarding-box__title">What is Dex?</h3>
          <p className="app-onboarding-box__text">
            Dex is an AI development assistant that helps you build software. It can create tasks,
            write code, make commits, and open pull requests on your behalf.
          </p>
        </div>

        <div className="app-onboarding-box">
          <h3 className="app-onboarding-box__title">What you'll need:</h3>
          <ul className="app-onboarding-list">
            <li className="app-onboarding-list__item">
              <span className="app-onboarding-list__marker">1.</span>
              <span><strong>A passkey</strong> — for secure authentication</span>
            </li>
            <li className="app-onboarding-list__item">
              <span className="app-onboarding-list__marker">2.</span>
              <span><strong>An Anthropic API key</strong> — to power the AI assistant</span>
            </li>
          </ul>
        </div>
      </div>

      <button
        onClick={onContinue}
        disabled={isLoading}
        className="app-onboarding-btn"
      >
        {isLoading ? 'Loading...' : 'Get Started'}
      </button>
    </StepContainer>
  );
}
