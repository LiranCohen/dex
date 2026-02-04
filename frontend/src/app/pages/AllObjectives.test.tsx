import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { AllObjectives } from './AllObjectives';
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

describe('AllObjectives', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('shows skeleton while loading', async () => {
      // Delay the API response to ensure we see loading state
      server.use(
        http.get('/api/v1/tasks', async () => {
          await new Promise((resolve) => setTimeout(resolve, 100));
          return HttpResponse.json({ tasks: [] });
        })
      );

      render(<AllObjectives />);
      // Check for loading indicator
      expect(
        document.querySelectorAll('.app-skeleton').length > 0 ||
        screen.queryByText(/loading/i) !== null
      ).toBe(true);
    });
  });

  describe('task list', () => {
    it('displays tasks after loading', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
        expect(screen.getByText('Pending task')).toBeInTheDocument();
        expect(screen.getByText('Completed task')).toBeInTheDocument();
      });
    });

    it('groups tasks by quest', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Quest: Test Quest')).toBeInTheDocument();
      });
    });

    it('shows task status labels', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('running')).toBeInTheDocument();
        expect(screen.getByText('pending')).toBeInTheDocument();
        expect(screen.getByText('completed')).toBeInTheDocument();
      });
    });

    it('links tasks to their detail pages', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        const taskLink = screen.getByText('Implement feature X').closest('a');
        expect(taskLink).toHaveAttribute('href', '/objectives/task-1');
      });
    });

    it('links quest titles to quest pages', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        const questLink = screen.getByText('Quest: Test Quest');
        expect(questLink).toHaveAttribute('href', '/quests/quest-1');
      });
    });

    it('shows empty state when no tasks', async () => {
      server.use(
        http.get('/api/v1/tasks', () => {
          return HttpResponse.json({ tasks: [], count: 0 });
        })
      );

      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('[ empty ]')).toBeInTheDocument();
      });
    });
  });

  describe('filtering', () => {
    it('shows filter buttons', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Running' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Pending' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Complete' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Failed' })).toBeInTheDocument();
      });
    });

    it('filters to running tasks', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Running' }));

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
        expect(screen.queryByText('Pending task')).not.toBeInTheDocument();
        expect(screen.queryByText('Completed task')).not.toBeInTheDocument();
      });
    });

    it('filters to completed tasks', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Completed task')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Complete' }));

      await waitFor(() => {
        expect(screen.queryByText('Implement feature X')).not.toBeInTheDocument();
        expect(screen.queryByText('Pending task')).not.toBeInTheDocument();
        expect(screen.getByText('Completed task')).toBeInTheDocument();
      });
    });

    it('shows active state on selected filter', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const runningButton = screen.getByRole('button', { name: 'Running' });
      await user.click(runningButton);

      expect(runningButton).toHaveAttribute('aria-pressed', 'true');
    });

    it('shows empty state when filter has no matches', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Failed' }));

      await waitFor(() => {
        expect(screen.getByText('[ empty ]')).toBeInTheDocument();
      });
    });
  });

  describe('search', () => {
    it('shows search input', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Search objectives...')).toBeInTheDocument();
      });
    });

    it('filters tasks by title search', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText('Search objectives...');
      await user.type(searchInput, 'feature X');

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
        expect(screen.queryByText('Pending task')).not.toBeInTheDocument();
        expect(screen.queryByText('Completed task')).not.toBeInTheDocument();
      });
    });

    it('shows empty state for no search matches', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText('Search objectives...');
      await user.type(searchInput, 'nonexistent');

      await waitFor(() => {
        expect(screen.getByText('[ no matching objectives ]')).toBeInTheDocument();
      });
    });

    it('clears search on Escape', async () => {
      const user = userEvent.setup();
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText('Search objectives...');
      await user.type(searchInput, 'feature X');

      // Press Escape
      await user.keyboard('{Escape}');

      expect(searchInput).toHaveValue('');
    });
  });

  describe('/ shortcut', () => {
    it('focuses search input on / key', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText('Search objectives...');
      expect(searchInput).not.toHaveFocus();

      fireEvent.keyDown(document, { key: '/' });

      expect(searchInput).toHaveFocus();
    });

    it('does not focus search when already in input', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      // Create and focus a different input
      const otherInput = document.createElement('input');
      document.body.appendChild(otherInput);
      otherInput.focus();

      // Dispatch '/' from other input
      const event = new KeyboardEvent('keydown', { key: '/', bubbles: true });
      Object.defineProperty(event, 'target', { value: otherInput });
      document.dispatchEvent(event);

      const searchInput = screen.getByPlaceholderText('Search objectives...');
      expect(searchInput).not.toHaveFocus();

      document.body.removeChild(otherInput);
    });
  });

  describe('status indicators', () => {
    it('shows active status bar for running tasks', async () => {
      render(<AllObjectives />);

      // Wait for tasks to load first
      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const runningTask = screen.getByText('Implement feature X').closest('.app-objectives-list-item');
      expect(runningTask?.querySelector('.app-status-bar--active')).toBeInTheDocument();
    });

    it('shows pending status bar for pending tasks', async () => {
      render(<AllObjectives />);

      // Wait for tasks to load first
      await waitFor(() => {
        expect(screen.getByText('Pending task')).toBeInTheDocument();
      });

      const pendingTask = screen.getByText('Pending task').closest('.app-objectives-list-item');
      expect(pendingTask?.querySelector('.app-status-bar--pending')).toBeInTheDocument();
    });

    it('shows complete status bar for completed tasks', async () => {
      render(<AllObjectives />);

      // Wait for tasks to load first
      await waitFor(() => {
        expect(screen.getByText('Completed task')).toBeInTheDocument();
      });

      const completedTask = screen.getByText('Completed task').closest('.app-objectives-list-item');
      expect(completedTask?.querySelector('.app-status-bar--complete')).toBeInTheDocument();
    });
  });

  describe('header', () => {
    it('shows Back link to home', async () => {
      render(<AllObjectives />);

      // Wait for page to load
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'All Objectives' })).toBeInTheDocument();
      });

      const backLink = screen.getByLabelText(/Go back/i);
      expect(backLink).toHaveAttribute('href', '/');
    });

    it('displays inbox count', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        // Multiple badges exist for mobile/desktop
        const badges = screen.getAllByText('2');
        expect(badges.length).toBeGreaterThan(0);
      });
    });
  });

  describe('error handling', () => {
    it('shows toast on load failure', async () => {
      server.use(
        http.get('/api/v1/tasks', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByText('Failed to load objectives')).toBeInTheDocument();
      });
    });
  });

  describe('accessibility', () => {
    it('has labeled filter group', async () => {
      render(<AllObjectives />);

      await waitFor(() => {
        expect(screen.getByRole('group', { name: 'Filter objectives by status' })).toBeInTheDocument();
      });
    });
  });
});
