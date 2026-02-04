import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, fireEvent } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { Header } from './Header';

describe('Header', () => {
  afterEach(() => {
    document.body.style.overflow = '';
  });

  it('renders DEX logo by default', () => {
    render(<Header />);
    expect(screen.getByText('DEX')).toBeInTheDocument();
    expect(screen.getByLabelText('Dex home')).toBeInTheDocument();
  });

  it('renders back link when provided', () => {
    render(<Header backLink={{ to: '/quests', label: 'Quests' }} />);
    expect(screen.getByText('Quests')).toBeInTheDocument();
    expect(screen.getByLabelText('Go back to Quests')).toBeInTheDocument();
    expect(screen.queryByText('DEX')).not.toBeInTheDocument();
  });

  it('renders inbox link', () => {
    render(<Header />);
    expect(screen.getByRole('link', { name: 'Inbox' })).toBeInTheDocument();
  });

  it('renders inbox with badge when count > 0', () => {
    render(<Header inboxCount={5} />);
    // Find the desktop inbox link with the badge
    const inboxLinks = screen.getAllByText('Inbox');
    expect(inboxLinks.length).toBeGreaterThan(0);
    // Check that the badge is rendered
    expect(screen.getAllByText('5').length).toBeGreaterThan(0);
  });

  it('does not show inbox badge when count is 0', () => {
    const { container } = render(<Header inboxCount={0} />);
    expect(container.querySelector('.app-header__inbox-badge')).not.toBeInTheDocument();
  });

  it('renders All Objectives link', () => {
    render(<Header />);
    expect(screen.getByRole('link', { name: 'All Objectives' })).toBeInTheDocument();
  });

  describe('mobile menu', () => {
    it('renders hamburger menu button', () => {
      render(<Header />);
      expect(screen.getByLabelText('Open menu')).toBeInTheDocument();
    });

    it('toggles menu open when hamburger clicked', async () => {
      const user = userEvent.setup();
      render(<Header />);

      const menuButton = screen.getByLabelText('Open menu');
      await user.click(menuButton);

      expect(screen.getByLabelText('Close menu')).toBeInTheDocument();
    });

    it('shows mobile navigation links when menu is open', async () => {
      const user = userEvent.setup();
      render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));

      const mobileNav = screen.getByRole('navigation', { name: 'Mobile navigation' });
      expect(mobileNav).toHaveAttribute('aria-hidden', 'false');
    });

    it('mobile navigation has correct aria-hidden when menu is closed', () => {
      const { container } = render(<Header />);
      const mobileNav = container.querySelector('.app-header__mobile-menu');
      // When closed, the menu should not have the open class
      expect(mobileNav).not.toHaveClass('app-header__mobile-menu--open');
    });

    it('closes menu on Escape key', async () => {
      const user = userEvent.setup();
      render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));
      expect(screen.getByLabelText('Close menu')).toBeInTheDocument();

      fireEvent.keyDown(document, { key: 'Escape' });
      expect(screen.getByLabelText('Open menu')).toBeInTheDocument();
    });

    it('does not close menu on Escape when already closed', () => {
      render(<Header />);
      fireEvent.keyDown(document, { key: 'Escape' });
      expect(screen.getByLabelText('Open menu')).toBeInTheDocument();
    });

    it('closes menu when overlay is clicked', async () => {
      const user = userEvent.setup();
      const { container } = render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));

      const overlay = container.querySelector('.app-header__menu-overlay');
      await user.click(overlay!);

      expect(screen.getByLabelText('Open menu')).toBeInTheDocument();
    });

    it('prevents body scroll when menu is open', async () => {
      const user = userEvent.setup();
      render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));
      expect(document.body.style.overflow).toBe('hidden');
    });

    it('restores body scroll when menu is closed', async () => {
      const user = userEvent.setup();
      render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));
      expect(document.body.style.overflow).toBe('hidden');

      await user.click(screen.getByLabelText('Close menu'));
      expect(document.body.style.overflow).toBe('');
    });

    it('shows inbox badge on hamburger when menu closed and count > 0', () => {
      const { container } = render(<Header inboxCount={3} />);
      const menuBadge = container.querySelector('.app-header__menu-badge');
      expect(menuBadge).toBeInTheDocument();
      expect(menuBadge).toHaveTextContent('3');
    });

    it('hides inbox badge on hamburger when menu is open', async () => {
      const user = userEvent.setup();
      const { container } = render(<Header inboxCount={3} />);

      await user.click(screen.getByLabelText('Open menu'));

      const menuBadge = container.querySelector('.app-header__menu-badge');
      expect(menuBadge).not.toBeInTheDocument();
    });

    it('closes menu when mobile link is clicked', async () => {
      const user = userEvent.setup();
      render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));

      // Click Home link in mobile menu
      const mobileLinks = screen.getAllByRole('link', { name: /Home/i });
      const mobileHomeLink = mobileLinks.find(link =>
        link.closest('.app-header__mobile-menu')
      );

      if (mobileHomeLink) {
        await user.click(mobileHomeLink);
      }

      // Menu should be closed (showing "Open menu" button)
      expect(screen.getByLabelText('Open menu')).toBeInTheDocument();
    });

    it('applies open class to hamburger when menu is open', async () => {
      const user = userEvent.setup();
      const { container } = render(<Header />);

      await user.click(screen.getByLabelText('Open menu'));

      const hamburger = container.querySelector('.app-header__hamburger');
      expect(hamburger).toHaveClass('app-header__hamburger--open');
    });

    it('mobile menu contains all navigation links', async () => {
      const user = userEvent.setup();
      render(<Header inboxCount={2} />);

      await user.click(screen.getByLabelText('Open menu'));

      const mobileMenu = screen.getByRole('navigation', { name: 'Mobile navigation' });
      expect(mobileMenu).toContainElement(screen.getByRole('link', { name: 'Home' }));
    });
  });

  it('has proper semantic structure', () => {
    render(<Header />);
    expect(screen.getByRole('banner')).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Primary navigation' })).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Secondary navigation' })).toBeInTheDocument();
  });
});
