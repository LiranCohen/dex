import { memo, useState } from 'react';
import type { QuestToolCall } from '../../lib/types';

interface ToolActivityProps {
  toolCall: QuestToolCall;
  status: 'running' | 'complete' | 'error';
}

// Map tool names to icons and display names
const toolInfo: Record<string, { icon: string; label: string }> = {
  read_file: { icon: '📄', label: 'Read file' },
  list_files: { icon: '📁', label: 'List files' },
  glob: { icon: '🔍', label: 'Find files' },
  grep: { icon: '🔎', label: 'Search' },
  git_status: { icon: '📊', label: 'Git status' },
  git_diff: { icon: '📝', label: 'Git diff' },
  git_log: { icon: '📜', label: 'Git log' },
  web_search: { icon: '🌐', label: 'Web search' },
  web_fetch: { icon: '🔗', label: 'Fetch URL' },
  list_runtimes: { icon: '⚙️', label: 'List runtimes' },
  list_objectives: { icon: '📋', label: 'List objectives' },
  get_objective_details: { icon: '📋', label: 'Objective details' },
  cancel_objective: { icon: '🚫', label: 'Cancel objective' },
};

// Get display summary based on tool type
function getToolSummary(toolCall: QuestToolCall): { running: string; complete: string } {
  const input = toolCall.input;

  switch (toolCall.tool_name) {
    case 'read_file':
      return {
        running: `Reading: \`${input.path}\`...`,
        complete: getReadFileSummary(toolCall),
      };

    case 'list_files':
      return {
        running: `Listing: \`${input.path || '.'}\`...`,
        complete: getListFilesSummary(toolCall),
      };

    case 'glob':
      return {
        running: `Finding files: \`${input.pattern}\`...`,
        complete: getGlobSummary(toolCall),
      };

    case 'grep':
      return {
        running: `Searching for: \`${input.pattern}\`...`,
        complete: getGrepSummary(toolCall),
      };

    case 'git_status':
      return {
        running: 'Checking git status...',
        complete: getGitStatusSummary(toolCall),
      };

    case 'git_diff':
      return {
        running: 'Getting changes...',
        complete: getGitDiffSummary(toolCall),
      };

    case 'git_log':
      return {
        running: 'Getting commit history...',
        complete: getGitLogSummary(toolCall),
      };

    case 'web_search':
      return {
        running: `Searching: "${input.query}"...`,
        complete: getWebSearchSummary(toolCall),
      };

    case 'web_fetch':
      return {
        running: `Reading: ${input.url}...`,
        complete: getWebFetchSummary(toolCall),
      };

    case 'list_objectives':
      return {
        running: 'Checking objectives...',
        complete: getObjectivesSummary(toolCall),
      };

    case 'get_objective_details':
      return {
        running: 'Loading objective...',
        complete: 'Objective loaded',
      };

    case 'cancel_objective':
      return {
        running: 'Cancelling...',
        complete: toolCall.is_error ? 'Failed to cancel' : 'Cancelled',
      };

    default:
      return {
        running: `Running ${toolCall.tool_name}...`,
        complete: toolCall.is_error ? 'Error' : 'Complete',
      };
  }
}

function getReadFileSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Failed to read file';
  const lines = tc.output.split('\n').length;
  return `Read ${lines} lines`;
}

function getListFilesSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Failed to list';
  const items = tc.output.split('\n').filter(Boolean).length;
  return `${items} items`;
}

function getGlobSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Error';
  if (tc.output === 'No files matched the pattern') return 'No matches';
  const matches = tc.output.split('\n').filter(Boolean).length;
  return `${matches} matches`;
}

function getGrepSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Error';
  if (tc.output === 'No matches found') return 'No matches';
  const lines = tc.output.split('\n').filter(Boolean).length;
  return `${lines} matches`;
}

function getGitStatusSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Error';
  if (tc.output === 'Working directory clean') return 'Clean';
  const changes = tc.output.split('\n').filter(Boolean).length;
  return `${changes} changes`;
}

function getGitDiffSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Error';
  if (tc.output === 'No changes') return 'No changes';
  const lines = tc.output.split('\n');
  const additions = lines.filter(l => l.startsWith('+') && !l.startsWith('+++')).length;
  const deletions = lines.filter(l => l.startsWith('-') && !l.startsWith('---')).length;
  return `+${additions}/-${deletions}`;
}

function getGitLogSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Error';
  const commits = tc.output.split('\n').filter(l => l.startsWith('commit ')).length || 1;
  return `${commits} commits`;
}

function getWebSearchSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Search failed';
  // Try to count results
  const resultMatches = tc.output.match(/\d+\s+results?/i);
  if (resultMatches) return resultMatches[0];
  return 'Results found';
}

function getWebFetchSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Failed to fetch';
  const length = tc.output.length;
  if (length > 10000) return `${Math.round(length / 1000)}KB fetched`;
  return 'Page loaded';
}

function getObjectivesSummary(tc: QuestToolCall): string {
  if (tc.is_error) return 'Error';
  try {
    const data = JSON.parse(tc.output);
    if (Array.isArray(data)) return `${data.length} objectives`;
  } catch {
    // Ignore parse errors - output may not be JSON
  }
  return 'Loaded';
}

export const ToolActivity = memo(function ToolActivity({
  toolCall,
  status,
}: ToolActivityProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const info = toolInfo[toolCall.tool_name] || { icon: '🔧', label: toolCall.tool_name };
  const summary = getToolSummary(toolCall);

  const isRunning = status === 'running';
  const isError = toolCall.is_error || status === 'error';

  return (
    <div
      className={`
        border rounded-lg overflow-hidden text-sm
        ${isError ? 'border-red-500/40 bg-red-950/20' : 'border-gray-700/50 bg-gray-800/30'}
      `}
    >
      {/* Header */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        disabled={isRunning}
        className={`
          w-full flex items-center justify-between px-3 py-2
          hover:bg-gray-800/50 transition-colors text-left
          ${isRunning ? 'cursor-default' : 'cursor-pointer'}
        `}
      >
        <div className="flex items-center gap-2">
          <span>{info.icon}</span>
          <span className="text-gray-400">{info.label}</span>
          {isRunning ? (
            <span className="w-1.5 h-1.5 bg-blue-400 rounded-full animate-pulse" />
          ) : isError ? (
            <span className="text-red-400 text-xs">✗</span>
          ) : (
            <span className="text-green-400 text-xs">✓</span>
          )}
        </div>

        <div className="flex items-center gap-2 text-xs text-gray-500">
          {!isRunning && toolCall.duration_ms > 0 && (
            <span>{toolCall.duration_ms}ms</span>
          )}
          {!isRunning && (
            <span className="text-gray-600">{isExpanded ? '▲' : '▼'}</span>
          )}
        </div>
      </button>

      {/* Summary */}
      <div className="px-3 py-1.5 border-t border-gray-700/30 bg-gray-900/30">
        <p className={`text-xs ${isError ? 'text-red-400' : 'text-gray-400'}`}>
          {isRunning ? summary.running : summary.complete}
        </p>
      </div>

      {/* Expanded output */}
      {isExpanded && !isRunning && (
        <div className="border-t border-gray-700/30">
          {/* Input */}
          <div className="px-3 py-2 bg-gray-900/20">
            <p className="text-xs text-gray-600 mb-1">Input:</p>
            <pre className="text-xs font-mono text-gray-400 bg-gray-900/50 rounded p-2 overflow-x-auto">
              {JSON.stringify(toolCall.input, null, 2)}
            </pre>
          </div>

          {/* Output */}
          <div className="px-3 py-2 bg-gray-900/40">
            <p className="text-xs text-gray-600 mb-1">Output:</p>
            <pre
              className={`
                text-xs font-mono whitespace-pre-wrap break-words
                rounded p-2 overflow-x-auto max-h-64 overflow-y-auto
                ${isError ? 'text-red-400 bg-red-950/30' : 'text-gray-400 bg-gray-900/50'}
              `}
            >
              {toolCall.output || '(empty)'}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
});
