import { Link } from 'react-router-dom';

export function NotFound() {
  return (
    <div className="app-root">
      <div className="app-error-page">
        <div className="app-error-page__content">
          <div className="app-error-page__code">404</div>
          <h1 className="app-error-page__title">Page not found</h1>
          <p className="app-error-page__message">
            The page you're looking for doesn't exist or has been moved.
          </p>
          <div className="app-error-page__actions">
            <Link to="/" className="app-btn app-btn--primary">
              Go Home
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
