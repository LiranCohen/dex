import type { ReactNode } from 'react';

interface StepContainerProps {
  title: string;
  description?: string;
  children: ReactNode;
  error?: string | null;
}

export function StepContainer({ title, description, children, error }: StepContainerProps) {
  return (
    <div className="app-onboarding-container">
      <div className="app-card app-onboarding-card">
        <h2 className="app-onboarding-title">{title}</h2>
        {description && (
          <p className="app-onboarding-description">{description}</p>
        )}

        {error && (
          <div className="app-onboarding-error">
            <p>{error}</p>
          </div>
        )}

        {children}
      </div>
    </div>
  );
}
