import type { Step } from '../hooks/useOnboarding';

interface StepIndicatorProps {
  steps: Step[];
  currentStep: string;
}

export function StepIndicator({ steps, currentStep }: StepIndicatorProps) {
  return (
    <div className="flex items-center justify-center mb-8">
      {steps.map((step, index) => {
        const isComplete = step.status === 'complete';
        const isCurrent = step.id === currentStep;
        const isPending = step.status === 'pending';

        return (
          <div key={step.id} className="flex items-center">
            {/* Step dot */}
            <div className="relative">
              <div
                className={`w-3 h-3 rounded-full transition-colors ${
                  isComplete
                    ? 'bg-green-500'
                    : isCurrent
                    ? 'bg-blue-500'
                    : 'bg-gray-600'
                }`}
              />
              {isCurrent && (
                <div className="absolute -inset-1 rounded-full border-2 border-blue-500 animate-pulse" />
              )}
            </div>

            {/* Connector line */}
            {index < steps.length - 1 && (
              <div
                className={`w-8 h-0.5 mx-1 transition-colors ${
                  isComplete ? 'bg-green-500' : isPending ? 'bg-gray-600' : 'bg-gray-600'
                }`}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
