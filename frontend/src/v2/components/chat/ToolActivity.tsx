import { memo, useState } from 'react';

type ToolStatus = 'running' | 'complete' | 'error';

interface ToolActivityProps {
  status: ToolStatus;
  tool: string;
  description?: string;
  input?: Record<string, unknown>;
  output?: string;
  isError?: boolean;
  durationMs?: number;
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

// Get display summary based on tool type and output
function getToolSummary(tool: string, input: Record<string, unknown> | undefined, output: string | undefined, isError: boolean): { running: string; complete: string } {
  const getPath = () => (input?.path as string) || '.';
  const getPattern = () => (input?.pattern as string) || '';
  const getQuery = () => (input?.query as string) || '';
  const getUrl = () => (input?.url as string) || '';
  const countLines = (s: string) => s.split('\n').filter(Boolean).length;

  switch (tool) {
    case 'read_file':
      return {
        running: `Reading: \`${getPath()}\`...`,
        complete: isError ? 'Failed to read file' : `Read ${countLines(output || '')} lines`,
      };

    case 'list_files':
      return {
        running: `Listing: \`${getPath()}\`...`,
        complete: isError ? 'Failed to list' : `${countLines(output || '')} items`,
      };

    case 'glob':
      return {
        running: `Finding files: \`${getPattern()}\`...`,
        complete: isError ? 'Error' : output === 'No files matched the pattern' ? 'No matches' : `${countLines(output || '')} matches`,
      };

    case 'grep':
      return {
        running: `Searching for: \`${getPattern()}\`...`,
        complete: isError ? 'Error' : output === 'No matches found' ? 'No matches' : `${countLines(output || '')} matches`,
      };

    case 'git_status':
      return {
        running: 'Checking git status...',
        complete: isError ? 'Error' : output === 'Working directory clean' ? 'Clean' : `${countLines(output || '')} changes`,
      };

    case 'git_diff': {
      if (isError) return { running: 'Getting changes...', complete: 'Error' };
      if (!output || output === 'No changes') return { running: 'Getting changes...', complete: 'No changes' };
      const lines = (output || '').split('\n');
      const additions = lines.filter(l => l.startsWith('+') && !l.startsWith('+++')).length;
      const deletions = lines.filter(l => l.startsWith('-') && !l.startsWith('---')).length;
      return {
        running: 'Getting changes...',
        complete: `+${additions}/-${deletions}`,
      };
    }

    case 'git_log': {
      if (isError) return { running: 'Getting commit history...', complete: 'Error' };
      const commits = (output || '').split('\n').filter(l => l.startsWith('commit ')).length || 1;
      return {
        running: 'Getting commit history...',
        complete: `${commits} commits`,
      };
    }

    case 'web_search': {
      if (isError) return { running: `Searching: "${getQuery()}"...`, complete: 'Search failed' };
      const resultMatches = (output || '').match(/\d+\s+results?/i);
      return {
        running: `Searching: "${getQuery()}"...`,
        complete: resultMatches ? resultMatches[0] : 'Results found',
      };
    }

    case 'web_fetch': {
      if (isError) return { running: `Reading: ${getUrl()}...`, complete: 'Failed to fetch' };
      const length = (output || '').length;
      return {
        running: `Reading: ${getUrl()}...`,
        complete: length > 10000 ? `${Math.round(length / 1000)}KB fetched` : 'Page loaded',
      };
    }

    case 'list_objectives':
      return {
        running: 'Checking objectives...',
        complete: isError ? 'Error' : 'Loaded',
      };

    case 'get_objective_details':
      return {
        running: 'Loading objective...',
        complete: 'Objective loaded',
      };

    case 'cancel_objective':
      return {
        running: 'Cancelling...',
        complete: isError ? 'Failed to cancel' : 'Cancelled',
      };

    default:
      return {
        running: `Running ${tool}...`,
        complete: isError ? 'Error' : 'Complete',
      };
  }
}

export const ToolActivity = memo(function ToolActivity({
  status,
  tool,
  description,
  input,
  output,
  isError = false,
  durationMs,
}: ToolActivityProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const info = toolInfo[tool] || { icon: '🔧', label: tool };
  const summary = getToolSummary(tool, input, output, isError);
  const isRunning = status === 'running';
  const hasError = isError || status === 'error';
  const hasDetails = input || output;

  // Use provided description or generated summary
  const displayText = description || (isRunning ? summary.running : summary.complete);

  return (
    <div className={`v2-tool-activity v2-tool-activity--${status} ${hasError ? 'v2-tool-activity--has-error' : ''}`}>
      {/* Header row */}
      <button
        type="button"
        onClick={() => !isRunning && hasDetails && setIsExpanded(!isExpanded)}
        disabled={isRunning || !hasDetails}
        className="v2-tool-activity__header"
      >
        <div className="v2-tool-activity__left">
          <span className="v2-tool-activity__icon">{info.icon}</span>
          <span className="v2-tool-activity__label">{info.label}</span>
          {isRunning ? (
            <span className="v2-tool-activity__pulse" />
          ) : hasError ? (
            <span className="v2-tool-activity__status v2-tool-activity__status--error">✗</span>
          ) : (
            <span className="v2-tool-activity__status v2-tool-activity__status--success">✓</span>
          )}
        </div>

        <div className="v2-tool-activity__right">
          {!isRunning && durationMs != null && durationMs > 0 && (
            <span className="v2-tool-activity__duration">{durationMs}ms</span>
          )}
          {!isRunning && hasDetails && (
            <span className="v2-tool-activity__expand">{isExpanded ? '▲' : '▼'}</span>
          )}
        </div>
      </button>

      {/* Summary row */}
      <div className="v2-tool-activity__summary">
        <span className={hasError ? 'v2-tool-activity__summary--error' : ''}>
          {displayText}
        </span>
      </div>

      {/* Expanded details */}
      {isExpanded && !isRunning && hasDetails && (
        <div className="v2-tool-activity__details">
          {input && (
            <div className="v2-tool-activity__section">
              <span className="v2-tool-activity__section-label">Input:</span>
              <pre className="v2-tool-activity__code">
                {JSON.stringify(input, null, 2)}
              </pre>
            </div>
          )}
          {output && (
            <div className="v2-tool-activity__section">
              <span className="v2-tool-activity__section-label">Output:</span>
              <pre className={`v2-tool-activity__code v2-tool-activity__output ${hasError ? 'v2-tool-activity__output--error' : ''}`}>
                {output || '(empty)'}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
});
