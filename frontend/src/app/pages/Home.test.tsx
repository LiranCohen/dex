import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { Home } from './Home';
import { server } from '../../test/mocks/server';
import { http, HttpResponse } from 'msw';

// Mock useWebSocket
vi.mock('../../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    connected: true,
    subscribe: vi.fn(() => vi.fn()),
    lastMessage: null,
  }),
}));

// Mock useNavigate
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('Home', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('shows skeleton while loading', async () => {
      // Delay the API response to ensure we see loading state
      server.use(
        http.get('/api/v1/quests', async () => {
          await new Promise((resolve) => setTimeout(resolve, 100));
          return HttpResponse.json({ quests: [] });
        })
      );

      render(<Home />);
      // Check for loading indicator
      expect(
        document.querySelectorAll('.app-skeleton').length > 0 ||
        screen.queryByText(/loading/i) !== null
      ).toBe(true);
    });
  });

  describe('quest list', () => {
    it('displays quests after loading', async () => {
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByText('Test Quest')).toBeInTheDocument();
      });
    });

    it('displays quest progress', async () => {
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByText(/1\/3 objectives/)).toBeInTheDocument();
      });
    });

    it('separates active and completed quests', async () => {
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByText('Test Quest')).toBeInTheDocument();
        expect(screen.getByText('Completed Quest')).toBeInTheDocument();
        expect(screen.getByText('Completed')).toBeInTheDocument(); // Divider text
      });
    });

    it('shows empty state when no quests', async () => {
      server.use(
        http.get('/api/v1/projects/:projectId/quests', () => {
          return HttpResponse.json([]);
        })
      );

      render(<Home />);

      await waitFor(() => {
        expect(screen.getByText('No quests yet')).toBeInTheDocument();
        expect(screen.getByText('Start by creating a new quest')).toBeInTheDocument();
      });
    });

    it('links quests to their detail pages', async () => {
      render(<Home />);

      await waitFor(() => {
        const questLink = screen.getByText('Test Quest').closest('a');
        expect(questLink).toHaveAttribute('href', '/quests/quest-1');
      });
    });
  });

  describe('new quest', () => {
    it('shows New Quest button', async () => {
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /New Quest/i })).toBeInTheDocument();
      });
    });

    it('creates new quest and navigates on click', async () => {
      const user = userEvent.setup();
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /New Quest/i })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /New Quest/i }));

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith(expect.stringMatching(/\/quests\/quest-/));
      });
    });

    it('shows error toast on create failure', async () => {
      server.use(
        http.post('/api/v1/projects/:projectId/quests', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      const user = userEvent.setup();
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /New Quest/i })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /New Quest/i }));

      await waitFor(() => {
        expect(screen.getByText('Failed to create quest')).toBeInTheDocument();
      });
    });
  });

  describe('header', () => {
    it('displays inbox count from approvals', async () => {
      render(<Home />);

      await waitFor(() => {
        // Two pending approvals in mock data - multiple badges exist for mobile/desktop
        const badges = screen.getAllByText('2');
        expect(badges.length).toBeGreaterThan(0);
      });
    });
  });

  describe('status indicators', () => {
    it('shows active status for running quests', async () => {
      render(<Home />);

      // Wait for quest to load first
      await waitFor(() => {
        expect(screen.getByText('Test Quest')).toBeInTheDocument();
      });

      const questCard = screen.getByText('Test Quest').closest('.app-quest-card');
      expect(questCard?.querySelector('.app-status-bar--active')).toBeInTheDocument();
    });

    it('shows complete status for completed quests', async () => {
      render(<Home />);

      // Wait for quest to load first
      await waitFor(() => {
        expect(screen.getByText('Completed Quest')).toBeInTheDocument();
      });

      const questCard = screen.getByText('Completed Quest').closest('.app-quest-card');
      expect(questCard?.querySelector('.app-status-bar--complete')).toBeInTheDocument();
    });
  });

  describe('keyboard navigation', () => {
    it('highlights selected quest with j/k navigation', async () => {
      const user = userEvent.setup();
      render(<Home />);

      await waitFor(() => {
        expect(screen.getByText('Test Quest')).toBeInTheDocument();
      });

      // Press j to select first item
      await user.keyboard('j');

      await waitFor(() => {
        const selectedCard = document.querySelector('.app-card--selected');
        expect(selectedCard).toBeInTheDocument();
      });
    });
  });

  describe('error handling', () => {
    it('shows toast on load failure', async () => {
      server.use(
        http.get('/api/v1/projects/:projectId/quests', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      render(<Home />);

      await waitFor(() => {
        expect(screen.getByText('Failed to load quests')).toBeInTheDocument();
      });
    });
  });
});
