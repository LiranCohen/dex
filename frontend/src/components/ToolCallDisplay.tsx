import { useState } from 'react';
import type { QuestToolCall } from '../lib/types';

interface ToolCallDisplayProps {
  toolCall: QuestToolCall;
  defaultExpanded?: boolean;
}

// Map tool names to icons
const toolIcons: Record<string, string> = {
  read_file: '📄',
  list_files: '📁',
  glob: '🔍',
  grep: '🔎',
  git_status: '📊',
  git_diff: '📝',
  git_log: '📜',
  web_search: '🌐',
  web_fetch: '🔗',
};

// Detect language from file path or content
function detectLanguage(toolName: string, input: Record<string, unknown>, content: string): string | null {
  // Check for file path
  const path = input.path as string | undefined;
  if (path) {
    const ext = path.split('.').pop()?.toLowerCase();
    const langMap: Record<string, string> = {
      ts: 'typescript',
      tsx: 'typescript',
      js: 'javascript',
      jsx: 'javascript',
      go: 'go',
      py: 'python',
      rs: 'rust',
      json: 'json',
      yaml: 'yaml',
      yml: 'yaml',
      md: 'markdown',
      css: 'css',
      html: 'html',
      sql: 'sql',
      sh: 'bash',
      bash: 'bash',
    };
    if (ext && langMap[ext]) {
      return langMap[ext];
    }
  }

  // Check for common patterns
  if (content.startsWith('{') || content.startsWith('[')) {
    try {
      JSON.parse(content);
      return 'json';
    } catch {
      // Not JSON
    }
  }

  if (toolName === 'git_diff') {
    return 'diff';
  }

  return null;
}

// Summarize tool output for collapsed view
function summarizeOutput(toolName: string, input: Record<string, unknown>, output: string, isError: boolean): string {
  if (isError) {
    return 'Error: ' + (output.length > 50 ? output.slice(0, 50) + '...' : output);
  }

  switch (toolName) {
    case 'read_file': {
      const path = input.path as string || 'file';
      const lines = output.split('\n').length;
      return `Read ${path} (${lines} lines)`;
    }
    case 'list_files': {
      const files = output.split('\n').filter(Boolean);
      return `Found ${files.length} items`;
    }
    case 'glob': {
      const matches = output.split('\n').filter(Boolean);
      if (matches.length === 0 || output === 'No files matched the pattern') {
        return 'No matches';
      }
      return `${matches.length} matches`;
    }
    case 'grep': {
      if (output === 'No matches found') {
        return 'No matches';
      }
      const lines = output.split('\n').filter(Boolean);
      return `${lines.length} matches`;
    }
    case 'git_status': {
      if (output === 'Working directory clean') {
        return 'Clean';
      }
      const changes = output.split('\n').filter(Boolean);
      return `${changes.length} changes`;
    }
    case 'git_diff': {
      if (output === 'No changes') {
        return 'No changes';
      }
      const lines = output.split('\n');
      const additions = lines.filter(l => l.startsWith('+')).length;
      const deletions = lines.filter(l => l.startsWith('-')).length;
      return `+${additions}/-${deletions} lines`;
    }
    case 'git_log': {
      const commits = output.split('\n').filter(Boolean);
      return `${commits.length} commits`;
    }
    case 'web_search':
    case 'web_fetch':
      return output.length > 60 ? output.slice(0, 60) + '...' : output;
    default:
      return output.length > 60 ? output.slice(0, 60) + '...' : output;
  }
}

export function ToolCallDisplay({ toolCall, defaultExpanded = false }: ToolCallDisplayProps) {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  const icon = toolIcons[toolCall.tool_name] || '🔧';
  const summary = summarizeOutput(toolCall.tool_name, toolCall.input, toolCall.output, toolCall.is_error);
  const language = detectLanguage(toolCall.tool_name, toolCall.input, toolCall.output);

  return (
    <div className={`border rounded-lg overflow-hidden ${toolCall.is_error ? 'border-red-500/50' : 'border-gray-700'}`}>
      {/* Header - always visible */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full flex items-center justify-between px-3 py-2 bg-gray-800/50 hover:bg-gray-800 transition-colors text-left"
      >
        <div className="flex items-center gap-2">
          <span className="text-sm">{icon}</span>
          <span className="text-sm font-mono text-gray-300">{toolCall.tool_name}</span>
          {toolCall.is_error ? (
            <span className="text-red-400 text-xs">✗</span>
          ) : (
            <span className="text-green-400 text-xs">✓</span>
          )}
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-500">{toolCall.duration_ms}ms</span>
          <span className="text-gray-500">{isExpanded ? '▲' : '▼'}</span>
        </div>
      </button>

      {/* Summary - visible when collapsed */}
      {!isExpanded && (
        <div className="px-3 py-1.5 bg-gray-900/50 border-t border-gray-700/50">
          <p className={`text-xs ${toolCall.is_error ? 'text-red-400' : 'text-gray-400'} truncate`}>
            {summary}
          </p>
        </div>
      )}

      {/* Expanded content */}
      {isExpanded && (
        <div className="border-t border-gray-700/50">
          {/* Input section */}
          <div className="px-3 py-2 bg-gray-900/30">
            <p className="text-xs text-gray-500 mb-1">Input:</p>
            <pre className="text-xs font-mono text-gray-400 bg-gray-900/50 rounded p-2 overflow-x-auto">
              {JSON.stringify(toolCall.input, null, 2)}
            </pre>
          </div>

          {/* Output section */}
          <div className="px-3 py-2 bg-gray-900/50">
            <p className="text-xs text-gray-500 mb-1">Output:</p>
            <pre
              className={`text-xs font-mono whitespace-pre-wrap break-words rounded p-2 overflow-x-auto max-h-96 overflow-y-auto ${
                toolCall.is_error
                  ? 'text-red-400 bg-red-900/20'
                  : language
                  ? 'text-gray-300 bg-gray-900/50'
                  : 'text-gray-400 bg-gray-900/50'
              }`}
            >
              {toolCall.output || '(empty)'}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
}
