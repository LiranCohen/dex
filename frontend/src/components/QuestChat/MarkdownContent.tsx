import { memo, useState, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';

interface MarkdownContentProps {
  content: string;
  isStreaming?: boolean;
}

interface CodeBlockProps {
  language: string | undefined;
  content: string;
  isStreaming?: boolean;
}

// Memoized code block component
const CodeBlock = memo(function CodeBlock({ language, content, isStreaming }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    if (isStreaming) return;

    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }, [content, isStreaming]);

  // Log warning for large code blocks
  if (content.length > 10000) {
    console.log(`[MarkdownContent] Large code block: ${content.length} chars`);
  }

  return (
    <div className="relative group my-2">
      {/* Language label and copy button */}
      <div className="flex items-center justify-between bg-gray-800 text-gray-400 text-xs px-3 py-1 rounded-t border-b border-gray-700">
        <span>{language || 'text'}</span>
        <button
          onClick={handleCopy}
          disabled={isStreaming}
          className={`px-2 py-0.5 rounded transition-colors ${
            isStreaming
              ? 'text-gray-600 cursor-not-allowed'
              : copied
              ? 'text-green-400'
              : 'text-gray-400 hover:text-white hover:bg-gray-700'
          }`}
          title={isStreaming ? 'Wait for completion' : 'Copy code'}
        >
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>

      <SyntaxHighlighter
        style={oneDark}
        language={language || 'text'}
        PreTag="div"
        customStyle={{
          margin: 0,
          borderTopLeftRadius: 0,
          borderTopRightRadius: 0,
          fontSize: '0.8125rem',
        }}
        codeTagProps={{
          style: {
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
          },
        }}
      >
        {content}
      </SyntaxHighlighter>
    </div>
  );
});

// Inline code component
function InlineCode({ children }: { children: React.ReactNode }) {
  return (
    <code className="bg-gray-800 text-gray-200 px-1.5 py-0.5 rounded text-sm font-mono">
      {children}
    </code>
  );
}

// Internal markdown renderer
const MarkdownRenderer = memo(function MarkdownRenderer({
  content,
  isStreaming = false
}: MarkdownContentProps) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        // Code blocks
        code({ className, children, ...props }) {
          const match = /language-(\w+)/.exec(className || '');
          const isMultiline = String(children).includes('\n');

          if (isMultiline || match) {
            return (
              <CodeBlock
                language={match?.[1]}
                content={String(children).replace(/\n$/, '')}
                isStreaming={isStreaming}
              />
            );
          }

          return <InlineCode {...props}>{children}</InlineCode>;
        },

        // Links
        a({ href, children, ...props }) {
          return (
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-400 hover:text-blue-300 underline"
              {...props}
            >
              {children}
            </a>
          );
        },

        // Paragraphs
        p({ children, ...props }) {
          return (
            <p className="mb-2 last:mb-0" {...props}>
              {children}
            </p>
          );
        },

        // Headings
        h1({ children, ...props }) {
          return <h1 className="text-xl font-bold mb-2 mt-4 first:mt-0" {...props}>{children}</h1>;
        },
        h2({ children, ...props }) {
          return <h2 className="text-lg font-bold mb-2 mt-3 first:mt-0" {...props}>{children}</h2>;
        },
        h3({ children, ...props }) {
          return <h3 className="text-base font-bold mb-2 mt-3 first:mt-0" {...props}>{children}</h3>;
        },

        // Lists
        ul({ children, ...props }) {
          return <ul className="list-disc list-inside mb-2 space-y-1" {...props}>{children}</ul>;
        },
        ol({ children, ...props }) {
          return <ol className="list-decimal list-inside mb-2 space-y-1" {...props}>{children}</ol>;
        },
        li({ children, ...props }) {
          return <li className="text-gray-300" {...props}>{children}</li>;
        },

        // Blockquotes
        blockquote({ children, ...props }) {
          return (
            <blockquote
              className="border-l-4 border-gray-600 pl-4 my-2 text-gray-400 italic"
              {...props}
            >
              {children}
            </blockquote>
          );
        },

        // Tables
        table({ children, ...props }) {
          return (
            <div className="overflow-x-auto my-2">
              <table className="min-w-full border-collapse border border-gray-700" {...props}>
                {children}
              </table>
            </div>
          );
        },
        thead({ children, ...props }) {
          return <thead className="bg-gray-800" {...props}>{children}</thead>;
        },
        th({ children, ...props }) {
          return <th className="border border-gray-700 px-3 py-2 text-left font-medium" {...props}>{children}</th>;
        },
        td({ children, ...props }) {
          return <td className="border border-gray-700 px-3 py-2" {...props}>{children}</td>;
        },

        // Horizontal rules
        hr({ ...props }) {
          return <hr className="border-gray-700 my-4" {...props} />;
        },

        // Strong and emphasis
        strong({ children, ...props }) {
          return <strong className="font-bold text-white" {...props}>{children}</strong>;
        },
        em({ children, ...props }) {
          return <em className="italic text-gray-300" {...props}>{children}</em>;
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
});

// Main export with wrapper styling
export const MarkdownContent = memo(function MarkdownContent(props: MarkdownContentProps) {
  return (
    <div className="text-gray-300 leading-relaxed">
      <MarkdownRenderer {...props} />
    </div>
  );
});
