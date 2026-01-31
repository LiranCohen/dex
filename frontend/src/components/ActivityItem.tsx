import type { Activity, ToolCallContent, ToolResultContent, HatTransitionContent } from '../lib/types';
import { CollapsibleContent } from './CollapsibleContent';

interface ActivityItemProps {
  activity: Activity;
}

// Icon components
function UserIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
    </svg>
  );
}

function BotIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
    </svg>
  );
}

function WrenchIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
    </svg>
  );
}

function TerminalIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
    </svg>
  );
}

function HatIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

// Style configurations per event type
const eventStyles: Record<string, { border: string; bg: string; icon: React.ReactNode; label: string }> = {
  user_message: {
    border: 'border-l-blue-500',
    bg: 'bg-blue-900/10',
    icon: <UserIcon />,
    label: 'User',
  },
  assistant_response: {
    border: 'border-l-green-500',
    bg: 'bg-green-900/10',
    icon: <BotIcon />,
    label: 'Assistant',
  },
  tool_call: {
    border: 'border-l-purple-500',
    bg: 'bg-purple-900/10',
    icon: <WrenchIcon />,
    label: 'Tool Call',
  },
  tool_result: {
    border: 'border-l-orange-500',
    bg: 'bg-orange-900/10',
    icon: <TerminalIcon />,
    label: 'Tool Result',
  },
  completion_signal: {
    border: 'border-l-emerald-500',
    bg: 'bg-emerald-900/10',
    icon: <CheckIcon />,
    label: 'Completed',
  },
  hat_transition: {
    border: 'border-l-yellow-500',
    bg: 'bg-yellow-900/10',
    icon: <HatIcon />,
    label: 'Hat Transition',
  },
};

function formatTime(dateString: string): string {
  return new Date(dateString).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function parseToolCallContent(content: string): ToolCallContent | null {
  try {
    return JSON.parse(content) as ToolCallContent;
  } catch {
    return null;
  }
}

function parseToolResultContent(content: string): ToolResultContent | null {
  try {
    return JSON.parse(content) as ToolResultContent;
  } catch {
    return null;
  }
}

function parseHatTransitionContent(content: string): HatTransitionContent | null {
  try {
    return JSON.parse(content) as HatTransitionContent;
  } catch {
    return null;
  }
}

export function ActivityItem({ activity }: ActivityItemProps) {
  const style = eventStyles[activity.event_type] || eventStyles.user_message;

  const renderContent = () => {
    switch (activity.event_type) {
      case 'user_message':
        return (
          <div className="text-sm text-gray-300">
            {activity.content || '(empty message)'}
          </div>
        );

      case 'assistant_response':
        return (
          <div className="space-y-2">
            {activity.content && (
              <CollapsibleContent content={activity.content} maxLength={300} />
            )}
            {(activity.tokens_input || activity.tokens_output) && (
              <div className="flex gap-2 text-xs">
                {activity.tokens_input && (
                  <span className="bg-gray-700 text-gray-300 px-2 py-0.5 rounded">
                    In: {activity.tokens_input.toLocaleString()}
                  </span>
                )}
                {activity.tokens_output && (
                  <span className="bg-gray-700 text-gray-300 px-2 py-0.5 rounded">
                    Out: {activity.tokens_output.toLocaleString()}
                  </span>
                )}
              </div>
            )}
          </div>
        );

      case 'tool_call': {
        const toolCall = parseToolCallContent(activity.content || '{}');
        return (
          <div className="space-y-2">
            {toolCall && (
              <>
                <span className="bg-purple-900 text-purple-200 px-2 py-0.5 rounded text-sm font-medium">
                  {toolCall.name}
                </span>
                {toolCall.input && Object.keys(toolCall.input).length > 0 && (
                  <CollapsibleContent
                    content={JSON.stringify(toolCall.input)}
                    isJson
                    maxLength={200}
                  />
                )}
              </>
            )}
            {!toolCall && activity.content && (
              <CollapsibleContent content={activity.content} maxLength={200} />
            )}
          </div>
        );
      }

      case 'tool_result': {
        const toolResult = parseToolResultContent(activity.content || '{}');
        const isError = toolResult?.result?.IsError ?? false;
        return (
          <div className={`space-y-2 ${isError ? 'border-l-red-500' : ''}`}>
            {toolResult && (
              <>
                <span className={`px-2 py-0.5 rounded text-sm font-medium ${
                  isError ? 'bg-red-900 text-red-200' : 'bg-orange-900 text-orange-200'
                }`}>
                  {toolResult.name} {isError && '(error)'}
                </span>
                {toolResult.result?.Output && (
                  <CollapsibleContent
                    content={toolResult.result.Output}
                    maxLength={200}
                  />
                )}
              </>
            )}
            {!toolResult && activity.content && (
              <CollapsibleContent content={activity.content} maxLength={200} />
            )}
          </div>
        );
      }

      case 'completion_signal':
        return (
          <div className="text-sm text-emerald-400">
            {activity.content || 'Session completed'}
          </div>
        );

      case 'hat_transition': {
        const transition = parseHatTransitionContent(activity.content || '{}');
        return (
          <div className="text-sm text-yellow-300 flex items-center gap-2">
            {transition ? (
              <>
                <span className="bg-yellow-900/50 px-2 py-0.5 rounded">{transition.from_hat}</span>
                <span className="text-gray-500">-&gt;</span>
                <span className="bg-yellow-900/50 px-2 py-0.5 rounded">{transition.to_hat}</span>
              </>
            ) : (
              activity.content || 'Hat changed'
            )}
          </div>
        );
      }

      default:
        return activity.content ? (
          <CollapsibleContent content={activity.content} maxLength={200} />
        ) : null;
    }
  };

  return (
    <div className={`border-l-2 ${style.border} ${style.bg} rounded-r p-3`}>
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2 text-gray-400">
          {style.icon}
          <span className="text-xs font-medium">{style.label}</span>
          {activity.iteration > 0 && (
            <span className="text-xs text-gray-500">
              (iteration {activity.iteration})
            </span>
          )}
        </div>
        <span className="text-xs text-gray-500">{formatTime(activity.created_at)}</span>
      </div>
      {renderContent()}
    </div>
  );
}
