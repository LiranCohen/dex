import { useState } from 'react';

interface CollapsibleContentProps {
  content: string;
  maxLength?: number;
  isJson?: boolean;
}

export function CollapsibleContent({
  content,
  maxLength = 200,
  isJson = false,
}: CollapsibleContentProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  // Try to format as JSON if requested
  let displayContent = content;
  let isJsonContent = isJson;

  if (isJson || content.startsWith('{') || content.startsWith('[')) {
    try {
      const parsed = JSON.parse(content);
      displayContent = JSON.stringify(parsed, null, 2);
      isJsonContent = true;
    } catch {
      // Not valid JSON, use original
    }
  }

  const needsCollapse = displayContent.length > maxLength;
  const truncatedContent = needsCollapse && !isExpanded
    ? displayContent.slice(0, maxLength) + '...'
    : displayContent;

  return (
    <div className="relative">
      <pre
        className={`text-xs font-mono whitespace-pre-wrap break-words bg-gray-900/50 rounded p-2 overflow-x-auto ${
          isJsonContent ? 'text-gray-300' : 'text-gray-400'
        }`}
      >
        {truncatedContent}
      </pre>
      {needsCollapse && (
        <button
          onClick={() => setIsExpanded(!isExpanded)}
          className="text-xs text-blue-400 hover:text-blue-300 mt-1 transition-colors"
        >
          {isExpanded ? 'Show less' : 'Show more'}
        </button>
      )}
    </div>
  );
}
