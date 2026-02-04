import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { Inbox } from './Inbox';
import { server } from '../../test/mocks/server';
import { http, HttpResponse } from 'msw';

// Mock useWebSocket
vi.mock('../../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    connected: true,
    connectionState: 'connected',
    connectionQuality: 'excellent',
    latency: 50,
    reconnectAttempts: 0,
    subscribe: vi.fn(() => vi.fn()),
    subscribeToChannel: vi.fn(() => vi.fn()),
    subscribedChannels: new Set(),
    lastMessage: null,
    reconnect: vi.fn(),
  }),
}));

describe('Inbox', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('shows skeleton while loading', () => {
      render(<Inbox />);
      expect(document.querySelectorAll('.app-skeleton').length).toBeGreaterThan(0);
    });
  });

  describe('approval list', () => {
    it('displays pending approvals after loading', async () => {
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
        expect(screen.getByText('Approve PR')).toBeInTheDocument();
      });
    });

    it('shows approval type labels', async () => {
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText(/APPROVAL · Commit/)).toBeInTheDocument();
        expect(screen.getByText(/APPROVAL · Pull Request/)).toBeInTheDocument();
      });
    });

    it('shows approval descriptions', async () => {
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('feat: add new feature')).toBeInTheDocument();
        expect(screen.getByText('Create pull request for feature')).toBeInTheDocument();
      });
    });

    it('shows empty state when no pending approvals', async () => {
      server.use(
        http.get('/api/v1/approvals', () => {
          return HttpResponse.json({ approvals: [], count: 0 });
        })
      );

      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Nothing needs attention')).toBeInTheDocument();
      });
    });

    it('links to objective when task_id is present', async () => {
      render(<Inbox />);

      await waitFor(() => {
        const link = screen.getAllByText('View Objective →')[0];
        expect(link).toHaveAttribute('href', '/objectives/task-1');
      });
    });
  });

  describe('approve action', () => {
    it('shows Approve button for each approval', async () => {
      render(<Inbox />);

      await waitFor(() => {
        const approveButtons = screen.getAllByRole('button', { name: 'Approve' });
        expect(approveButtons).toHaveLength(2);
      });
    });

    it('removes approval from list on approve', async () => {
      const user = userEvent.setup();
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
      });

      const approveButtons = screen.getAllByRole('button', { name: 'Approve' });
      await user.click(approveButtons[0]);

      await waitFor(() => {
        expect(screen.queryByText('Approve commit')).not.toBeInTheDocument();
      });
    });

    it('shows success toast on approve', async () => {
      const user = userEvent.setup();
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getAllByRole('button', { name: 'Approve' })).toHaveLength(2);
      });

      await user.click(screen.getAllByRole('button', { name: 'Approve' })[0]);

      await waitFor(() => {
        expect(screen.getByText('Approved successfully')).toBeInTheDocument();
      });
    });

    it('shows error toast on approve failure', async () => {
      server.use(
        http.post('/api/v1/approvals/:id/approve', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      const user = userEvent.setup();
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getAllByRole('button', { name: 'Approve' })).toHaveLength(2);
      });

      await user.click(screen.getAllByRole('button', { name: 'Approve' })[0]);

      await waitFor(() => {
        expect(screen.getByText('Failed to approve')).toBeInTheDocument();
      });
    });
  });

  describe('reject action', () => {
    it('shows Reject button for each approval', async () => {
      render(<Inbox />);

      await waitFor(() => {
        const rejectButtons = screen.getAllByRole('button', { name: 'Reject' });
        expect(rejectButtons).toHaveLength(2);
      });
    });

    it('removes approval from list on reject', async () => {
      const user = userEvent.setup();
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
      });

      const rejectButtons = screen.getAllByRole('button', { name: 'Reject' });
      await user.click(rejectButtons[0]);

      await waitFor(() => {
        expect(screen.queryByText('Approve commit')).not.toBeInTheDocument();
      });
    });

    it('shows info toast on reject', async () => {
      const user = userEvent.setup();
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getAllByRole('button', { name: 'Reject' })).toHaveLength(2);
      });

      await user.click(screen.getAllByRole('button', { name: 'Reject' })[0]);

      await waitFor(() => {
        expect(screen.getByText('Rejected')).toBeInTheDocument();
      });
    });
  });

  describe('keyboard shortcuts', () => {
    it('selects items with j/k keys', async () => {
      const user = userEvent.setup();
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
      });

      // Press j to select
      await user.keyboard('j');

      await waitFor(() => {
        expect(document.querySelector('.app-card--selected')).toBeInTheDocument();
      });
    });

    it('approves selected item with a key', async () => {
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
      });

      // First select an item
      fireEvent.keyDown(window, { key: 'j' });

      await waitFor(
        () => {
          expect(document.querySelector('.app-card--selected')).toBeInTheDocument();
        },
        { timeout: 3000 }
      );

      // Then press 'a' to approve
      fireEvent.keyDown(window, { key: 'a' });

      await waitFor(
        () => {
          expect(screen.queryByText('Approve commit')).not.toBeInTheDocument();
        },
        { timeout: 3000 }
      );
    });

    it('rejects selected item with r key', async () => {
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
      });

      // First select an item
      fireEvent.keyDown(window, { key: 'j' });

      await waitFor(() => {
        expect(document.querySelector('.app-card--selected')).toBeInTheDocument();
      });

      // Then press 'r' to reject
      fireEvent.keyDown(window, { key: 'r' });

      await waitFor(() => {
        expect(screen.queryByText('Approve commit')).not.toBeInTheDocument();
      });
    });

    it('does not respond to shortcuts when in input', async () => {
      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Approve commit')).toBeInTheDocument();
      });

      // First select an item
      fireEvent.keyDown(window, { key: 'j' });

      await waitFor(() => {
        expect(document.querySelector('.app-card--selected')).toBeInTheDocument();
      });

      // Create and focus an input
      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();

      // Dispatch 'a' key from input
      const event = new KeyboardEvent('keydown', { key: 'a', bubbles: true });
      Object.defineProperty(event, 'target', { value: input });
      window.dispatchEvent(event);

      // Approval should still be there
      expect(screen.getByText('Approve commit')).toBeInTheDocument();

      document.body.removeChild(input);
    });
  });

  describe('header', () => {
    it('shows Back link to home', async () => {
      render(<Inbox />);

      // Wait for data to load - use the page title which is unique
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Inbox' })).toBeInTheDocument();
      });

      // Find the back link by its aria-label
      const backLink = screen.getByLabelText(/Go back/i);
      expect(backLink).toHaveAttribute('href', '/');
    });

    it('displays inbox count', async () => {
      render(<Inbox />);

      await waitFor(() => {
        // Filters to only pending, so should show count of pending approvals
        // Multiple badges exist for mobile/desktop
        const badges = screen.getAllByText('2');
        expect(badges.length).toBeGreaterThan(0);
      });
    });
  });

  describe('error handling', () => {
    it('shows toast on load failure', async () => {
      server.use(
        http.get('/api/v1/approvals', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      render(<Inbox />);

      await waitFor(() => {
        expect(screen.getByText('Failed to load inbox')).toBeInTheDocument();
      });
    });
  });
});
