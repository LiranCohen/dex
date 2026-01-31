import { Component } from 'react';
import type { ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: string | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null, errorInfo: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error, errorInfo: null };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('ErrorBoundary caught an error:', error, errorInfo);
    this.setState({
      error,
      errorInfo: errorInfo.componentStack || null,
    });
  }

  handleReload = () => {
    window.location.reload();
  };

  handleReset = () => {
    this.setState({ hasError: false, error: null, errorInfo: null });
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
          <div className="bg-gray-800 rounded-lg p-6 max-w-2xl w-full">
            <div className="flex items-center gap-3 mb-4">
              <svg
                className="w-8 h-8 text-red-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
              <h1 className="text-xl font-bold text-red-400">Something went wrong</h1>
            </div>

            {this.state.error && (
              <div className="mb-4">
                <h2 className="text-sm font-semibold text-gray-400 mb-2">Error</h2>
                <div className="bg-red-900/30 border border-red-700 rounded p-3">
                  <code className="text-red-300 text-sm font-mono break-all">
                    {typeof this.state.error.message === 'string'
                      ? this.state.error.message
                      : JSON.stringify(this.state.error.message)}
                  </code>
                </div>
              </div>
            )}

            {this.state.errorInfo && (
              <div className="mb-4">
                <h2 className="text-sm font-semibold text-gray-400 mb-2">Component Stack</h2>
                <div className="bg-gray-900 rounded p-3 max-h-48 overflow-auto">
                  <pre className="text-gray-400 text-xs font-mono whitespace-pre-wrap">
                    {this.state.errorInfo}
                  </pre>
                </div>
              </div>
            )}

            <div className="flex gap-3">
              <button
                onClick={this.handleReset}
                className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded transition-colors"
              >
                Try Again
              </button>
              <button
                onClick={this.handleReload}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded transition-colors"
              >
                Reload Page
              </button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
