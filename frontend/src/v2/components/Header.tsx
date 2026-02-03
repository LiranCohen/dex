import { useState, useEffect, useCallback } from 'react';
import { Link, useLocation } from 'react-router-dom';

interface HeaderProps {
  backLink?: { to: string; label: string };
  inboxCount?: number;
}

export function Header({ backLink, inboxCount = 0 }: HeaderProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();

  // Close menu on route change
  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  // Close menu on escape
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape' && menuOpen) {
      setMenuOpen(false);
    }
  }, [menuOpen]);

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  // Prevent body scroll when menu is open
  useEffect(() => {
    if (menuOpen) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [menuOpen]);

  return (
    <header className="v2-header" role="banner">
      <nav className="v2-header__left" aria-label="Primary navigation">
        {backLink ? (
          <Link to={backLink.to} className="v2-header__back" aria-label={`Go back to ${backLink.label}`}>
            <span aria-hidden="true">←</span>
            <span>{backLink.label}</span>
          </Link>
        ) : (
          <Link to="/v2" className="v2-header__logo" aria-label="Dex home">
            DEX
          </Link>
        )}
      </nav>

      {/* Desktop navigation */}
      <nav className="v2-header__right v2-header__right--desktop" aria-label="Secondary navigation">
        <Link
          to="/v2/inbox"
          className="v2-header__inbox"
          aria-label={inboxCount > 0 ? `Inbox with ${inboxCount} pending items` : 'Inbox'}
        >
          {inboxCount > 0 && (
            <span className="v2-header__inbox-badge" aria-hidden="true">{inboxCount}</span>
          )}
          <span>Inbox</span>
        </Link>
        <Link to="/v2/objectives" className="v2-header__nav-link">
          All Objectives
        </Link>
      </nav>

      {/* Mobile hamburger button */}
      <button
        type="button"
        className="v2-header__menu-toggle"
        onClick={() => setMenuOpen(!menuOpen)}
        aria-label={menuOpen ? 'Close menu' : 'Open menu'}
        aria-expanded={menuOpen}
      >
        <span className={`v2-header__hamburger ${menuOpen ? 'v2-header__hamburger--open' : ''}`}>
          <span />
          <span />
          <span />
        </span>
        {inboxCount > 0 && !menuOpen && (
          <span className="v2-header__menu-badge" aria-hidden="true">{inboxCount}</span>
        )}
      </button>

      {/* Mobile menu overlay */}
      {menuOpen && (
        <div
          className="v2-header__menu-overlay"
          onClick={() => setMenuOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* Mobile menu */}
      <nav
        className={`v2-header__mobile-menu ${menuOpen ? 'v2-header__mobile-menu--open' : ''}`}
        aria-label="Mobile navigation"
        aria-hidden={!menuOpen}
      >
        <Link
          to="/v2"
          className="v2-header__mobile-link"
          onClick={() => setMenuOpen(false)}
        >
          Home
        </Link>
        <Link
          to="/v2/inbox"
          className="v2-header__mobile-link"
          onClick={() => setMenuOpen(false)}
        >
          Inbox
          {inboxCount > 0 && (
            <span className="v2-header__inbox-badge">{inboxCount}</span>
          )}
        </Link>
        <Link
          to="/v2/objectives"
          className="v2-header__mobile-link"
          onClick={() => setMenuOpen(false)}
        >
          All Objectives
        </Link>
      </nav>
    </header>
  );
}
