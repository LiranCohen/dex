import { memo, useState, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';

interface MarkdownContentProps {
  content: string;
  isStreaming?: boolean;
  searchQuery?: string;
}

// Highlight search matches in text
function highlightText(text: string, query: string): React.ReactNode {
  if (!query.trim()) return text;

  const parts = text.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi'));
  return parts.map((part, i) =>
    part.toLowerCase() === query.toLowerCase() ? (
      <mark key={i} className="v2-search-highlight">{part}</mark>
    ) : (
      part
    )
  );
}

// Recursively process children to highlight text
function highlightChildren(children: React.ReactNode, query: string): React.ReactNode {
  if (!query.trim()) return children;

  if (typeof children === 'string') {
    return highlightText(children, query);
  }

  if (Array.isArray(children)) {
    return children.map((child, i) => {
      if (typeof child === 'string') {
        return <span key={i}>{highlightText(child, query)}</span>;
      }
      return child;
    });
  }

  return children;
}

interface CodeBlockProps {
  language: string | undefined;
  content: string;
  isStreaming?: boolean;
}

// Memoized code block component with v2 styling
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

  return (
    <div className="v2-code-block">
      {/* Language label and copy button */}
      <div className="v2-code-block__header">
        <span className="v2-code-block__lang">{language || 'text'}</span>
        <button
          type="button"
          onClick={handleCopy}
          disabled={isStreaming}
          className={`v2-code-block__copy ${copied ? 'v2-code-block__copy--copied' : ''}`}
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
          borderBottomLeftRadius: 'var(--radius-sm)',
          borderBottomRightRadius: 'var(--radius-sm)',
          fontSize: '0.8125rem',
          background: 'var(--color-bg-tertiary)',
        }}
        codeTagProps={{
          style: {
            fontFamily: 'var(--font-mono)',
          },
        }}
      >
        {content}
      </SyntaxHighlighter>
    </div>
  );
});

// Inline code component with v2 styling
function InlineCode({ children }: { children: React.ReactNode }) {
  return (
    <code className="v2-inline-code">
      {children}
    </code>
  );
}

// Internal markdown renderer with v2 styling
const MarkdownRenderer = memo(function MarkdownRenderer({
  content,
  isStreaming = false,
  searchQuery = ''
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
              className="v2-md-link"
              {...props}
            >
              {highlightChildren(children, searchQuery)}
            </a>
          );
        },

        // Paragraphs
        p({ children, ...props }) {
          return (
            <p className="v2-md-p" {...props}>
              {highlightChildren(children, searchQuery)}
            </p>
          );
        },

        // Headings
        h1({ children, ...props }) {
          return <h1 className="v2-md-h1" {...props}>{highlightChildren(children, searchQuery)}</h1>;
        },
        h2({ children, ...props }) {
          return <h2 className="v2-md-h2" {...props}>{highlightChildren(children, searchQuery)}</h2>;
        },
        h3({ children, ...props }) {
          return <h3 className="v2-md-h3" {...props}>{highlightChildren(children, searchQuery)}</h3>;
        },

        // Lists
        ul({ children, ...props }) {
          return <ul className="v2-md-ul" {...props}>{children}</ul>;
        },
        ol({ children, ...props }) {
          return <ol className="v2-md-ol" {...props}>{children}</ol>;
        },
        li({ children, ...props }) {
          return <li className="v2-md-li" {...props}>{highlightChildren(children, searchQuery)}</li>;
        },

        // Blockquotes
        blockquote({ children, ...props }) {
          return (
            <blockquote className="v2-md-blockquote" {...props}>
              {children}
            </blockquote>
          );
        },

        // Tables
        table({ children, ...props }) {
          return (
            <div className="v2-md-table-wrapper">
              <table className="v2-md-table" {...props}>
                {children}
              </table>
            </div>
          );
        },
        thead({ children, ...props }) {
          return <thead className="v2-md-thead" {...props}>{children}</thead>;
        },
        th({ children, ...props }) {
          return <th className="v2-md-th" {...props}>{highlightChildren(children, searchQuery)}</th>;
        },
        td({ children, ...props }) {
          return <td className="v2-md-td" {...props}>{highlightChildren(children, searchQuery)}</td>;
        },

        // Horizontal rules
        hr({ ...props }) {
          return <hr className="v2-md-hr" {...props} />;
        },

        // Strong and emphasis
        strong({ children, ...props }) {
          return <strong className="v2-md-strong" {...props}>{highlightChildren(children, searchQuery)}</strong>;
        },
        em({ children, ...props }) {
          return <em className="v2-md-em" {...props}>{highlightChildren(children, searchQuery)}</em>;
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
});

// Main export
export const MarkdownContent = memo(function MarkdownContent({
  content,
  isStreaming = false,
  searchQuery = ''
}: MarkdownContentProps) {
  return (
    <div className="v2-markdown">
      <MarkdownRenderer content={content} isStreaming={isStreaming} searchQuery={searchQuery} />
    </div>
  );
});
