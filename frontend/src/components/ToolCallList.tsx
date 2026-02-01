import { useState } from 'react';
import type { QuestToolCall } from '../lib/types';
import { ToolCallDisplay } from './ToolCallDisplay';

interface ToolCallListProps {
  toolCalls: QuestToolCall[];
  defaultExpanded?: boolean;
}

export function ToolCallList({ toolCalls, defaultExpanded = false }: ToolCallListProps) {
  const [allExpanded, setAllExpanded] = useState(defaultExpanded);

  if (toolCalls.length === 0) {
    return null;
  }

  const totalDuration = toolCalls.reduce((sum, tc) => sum + tc.duration_ms, 0);
  const errorCount = toolCalls.filter(tc => tc.is_error).length;

  return (
    <div className="space-y-2">
      {/* Summary header */}
      <div className="flex items-center justify-between text-xs text-gray-500 px-1">
        <div className="flex items-center gap-2">
          <span className="text-sm">🔧</span>
          <span>{toolCalls.length} tool {toolCalls.length === 1 ? 'call' : 'calls'}</span>
          {errorCount > 0 && (
            <span className="text-red-400">({errorCount} failed)</span>
          )}
        </div>
        <div className="flex items-center gap-3">
          <span>{totalDuration}ms total</span>
          <button
            onClick={() => setAllExpanded(!allExpanded)}
            className="text-blue-400 hover:text-blue-300 transition-colors"
          >
            {allExpanded ? 'Collapse all' : 'Expand all'}
          </button>
        </div>
      </div>

      {/* Tool calls */}
      <div className="space-y-2">
        {toolCalls.map((toolCall, idx) => (
          <ToolCallDisplay
            key={`${toolCall.tool_name}-${idx}`}
            toolCall={toolCall}
            defaultExpanded={allExpanded}
          />
        ))}
      </div>
    </div>
  );
}
