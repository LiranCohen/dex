import { Component, type ReactNode, type ErrorInfo } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('ErrorBoundary caught an error:', error, errorInfo);
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null });
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <div className="app-error-page">
          <div className="app-error-page__content">
            <div className="app-error-page__icon" aria-hidden="true">!</div>
            <h1 className="app-error-page__title">Something went wrong</h1>
            <p className="app-error-page__message">
              {this.state.error?.message || 'An unexpected error occurred.'}
            </p>
            <div className="app-error-page__actions">
              <button
                type="button"
                className="app-btn app-btn--primary"
                onClick={this.handleRetry}
              >
                Try Again
              </button>
              <button
                type="button"
                className="app-btn app-btn--secondary"
                onClick={() => window.location.href = '/'}
              >
                Go Home
              </button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
