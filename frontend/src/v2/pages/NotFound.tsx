import { Link } from 'react-router-dom';

export function NotFound() {
  return (
    <div className="v2-root">
      <div className="v2-error-page">
        <div className="v2-error-page__content">
          <div className="v2-error-page__code">404</div>
          <h1 className="v2-error-page__title">Page not found</h1>
          <p className="v2-error-page__message">
            The page you're looking for doesn't exist or has been moved.
          </p>
          <div className="v2-error-page__actions">
            <Link to="/v2" className="v2-btn v2-btn--primary">
              Go Home
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
