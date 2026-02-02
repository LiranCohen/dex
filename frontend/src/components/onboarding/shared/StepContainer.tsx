import type { ReactNode } from 'react';

interface StepContainerProps {
  title: string;
  description?: string;
  children: ReactNode;
  error?: string | null;
}

export function StepContainer({ title, description, children, error }: StepContainerProps) {
  return (
    <div className="w-full max-w-md">
      <div className="bg-gray-800 rounded-lg p-6 shadow-xl">
        <h2 className="text-xl font-semibold mb-2">{title}</h2>
        {description && (
          <p className="text-gray-400 text-sm mb-6">{description}</p>
        )}

        {error && (
          <div className="bg-red-900/30 border border-red-500 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {children}
      </div>
    </div>
  );
}
