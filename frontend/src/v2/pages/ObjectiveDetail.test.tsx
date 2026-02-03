import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { ObjectiveDetail } from './ObjectiveDetail';
import { server } from '../../test/mocks/server';
import { http, HttpResponse } from 'msw';
import { mockTask, mockPendingTask } from '../../test/mocks/data';

// Mock useWebSocket
vi.mock('../../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    connected: true,
    subscribe: vi.fn(() => vi.fn()),
    lastMessage: null,
  }),
}));

// Mock useParams
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useParams: () => ({ id: 'task-1' }),
  };
});

describe('ObjectiveDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('shows skeleton while loading', () => {
      render(<ObjectiveDetail />);
      expect(document.querySelectorAll('.v2-skeleton').length).toBeGreaterThan(0);
    });
  });

  describe('task info', () => {
    it('displays task title', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });
    });

    it('displays task status', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('RUNNING')).toBeInTheDocument();
      });
    });

    it('displays task description', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('A detailed description of the task')).toBeInTheDocument();
      });
    });

    it('shows not found for invalid task', async () => {
      server.use(
        http.get('/api/v1/tasks/:id', () => {
          return new HttpResponse(null, { status: 404 });
        })
      );

      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('Objective not found')).toBeInTheDocument();
      });
    });
  });

  describe('checklist', () => {
    it('displays checklist items', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('Create login endpoint')).toBeInTheDocument();
        expect(screen.getByText('Add JWT token generation')).toBeInTheDocument();
        expect(screen.getByText('Implement auth middleware')).toBeInTheDocument();
      });
    });

    it('shows completed status for done items', async () => {
      render(<ObjectiveDetail />);

      // Wait for checklist to load
      await waitFor(() => {
        expect(screen.getByText('Create login endpoint')).toBeInTheDocument();
      });

      // Check that done items have the complete icon (✓)
      const doneItem = screen.getByText('Create login endpoint').closest('.v2-checklist-item');
      expect(doneItem).toBeInTheDocument();
      expect(doneItem?.querySelector('.v2-checklist-item__icon--complete')).toBeInTheDocument();
    });

    it('shows pending status for incomplete items', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('Implement auth middleware')).toBeInTheDocument();
      });

      const pendingItem = screen.getByText('Implement auth middleware').closest('.v2-checklist-item');
      expect(pendingItem?.querySelector('.v2-checklist-item__icon--pending')).toBeInTheDocument();
    });

    it('shows empty state when no checklist', async () => {
      server.use(
        http.get('/api/v1/tasks/:id/checklist', () => {
          return HttpResponse.json({ checklist: null, items: [], summary: { total: 0, done: 0, failed: 0, all_done: false } });
        })
      );

      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('// no checklist items')).toBeInTheDocument();
      });
    });
  });

  describe('activity', () => {
    it('displays activity log', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('Task started')).toBeInTheDocument();
        expect(screen.getByText('Read file main.ts')).toBeInTheDocument();
      });
    });

    it('shows empty state when no activity', async () => {
      server.use(
        http.get('/api/v1/tasks/:id/activity', () => {
          return HttpResponse.json({ activity: [], summary: { total_iterations: 0, total_tokens: 0 } });
        })
      );

      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('// no activity yet')).toBeInTheDocument();
      });
    });
  });

  describe('task actions', () => {
    it('shows Pause button for running tasks', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
      });
    });

    it('shows Cancel button for running tasks', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
      });
    });

    it('calls pause API on Pause click', async () => {
      const user = userEvent.setup();
      let pauseCalled = false;

      server.use(
        http.post('/api/v1/tasks/:id/pause', () => {
          pauseCalled = true;
          return HttpResponse.json({ success: true });
        })
      );

      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Pause' }));

      await waitFor(() => {
        expect(pauseCalled).toBe(true);
      });
    });

    it('shows success toast after pause', async () => {
      const user = userEvent.setup();
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Pause' }));

      await waitFor(() => {
        expect(screen.getByText('Objective paused')).toBeInTheDocument();
      });
    });

    describe('for paused tasks', () => {
      beforeEach(() => {
        server.use(
          http.get('/api/v1/tasks/:id', () => {
            return HttpResponse.json({ ...mockTask, Status: 'paused' });
          })
        );
      });

      it('shows Resume button', async () => {
        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
        });
      });

      it('calls resume API on Resume click', async () => {
        const user = userEvent.setup();
        let resumeCalled = false;

        server.use(
          http.post('/api/v1/tasks/:id/resume', () => {
            resumeCalled = true;
            return HttpResponse.json({ success: true });
          })
        );

        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Resume' }));

        await waitFor(() => {
          expect(resumeCalled).toBe(true);
        });
      });
    });

    describe('for pending tasks', () => {
      beforeEach(() => {
        server.use(
          http.get('/api/v1/tasks/:id', () => {
            return HttpResponse.json(mockPendingTask);
          })
        );
      });

      it('shows Start button', async () => {
        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument();
        });
      });

      it('calls start API on Start click', async () => {
        const user = userEvent.setup();
        let startCalled = false;

        server.use(
          http.post('/api/v1/tasks/:id/start', () => {
            startCalled = true;
            return HttpResponse.json({ success: true });
          })
        );

        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Start' }));

        await waitFor(() => {
          expect(startCalled).toBe(true);
        });
      });
    });

    describe('cancel confirmation', () => {
      it('shows confirmation modal on Cancel click', async () => {
        const user = userEvent.setup();
        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Cancel' }));

        await waitFor(() => {
          // Use heading role to find modal title specifically
          expect(screen.getByRole('heading', { name: 'Cancel Objective' })).toBeInTheDocument();
          expect(screen.getByText(/Are you sure you want to cancel/)).toBeInTheDocument();
        });
      });

      it('calls cancel API when confirmed', async () => {
        const user = userEvent.setup();
        let cancelCalled = false;

        server.use(
          http.post('/api/v1/tasks/:id/cancel', () => {
            cancelCalled = true;
            return HttpResponse.json({ success: true });
          })
        );

        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Cancel' }));

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Cancel Objective' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Cancel Objective' }));

        await waitFor(() => {
          expect(cancelCalled).toBe(true);
        });
      });

      it('closes modal when cancelled', async () => {
        const user = userEvent.setup();
        render(<ObjectiveDetail />);

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Cancel' }));

        await waitFor(() => {
          expect(screen.getByRole('button', { name: 'Keep Running' })).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Keep Running' }));

        await waitFor(() => {
          expect(screen.queryByText('Cancel Objective')).not.toBeInTheDocument();
        });
      });
    });
  });

  describe('status indicators', () => {
    it('shows active status bar for running tasks', async () => {
      render(<ObjectiveDetail />);

      // Wait for task to load
      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      expect(document.querySelector('.v2-status-bar--active')).toBeInTheDocument();
    });

    it('shows pulsing animation for running tasks', async () => {
      render(<ObjectiveDetail />);

      // Wait for task to load
      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      expect(document.querySelector('.v2-status-bar--pulse')).toBeInTheDocument();
    });
  });

  describe('header', () => {
    it('shows back link to quest when task has QuestID', async () => {
      render(<ObjectiveDetail />);

      // Wait for task to load
      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const backLink = screen.getByLabelText(/Go back/i);
      expect(backLink).toHaveAttribute('href', '/v2/quests/quest-1');
    });

    it('shows back link to home when task has no QuestID', async () => {
      server.use(
        http.get('/api/v1/tasks/:id', () => {
          return HttpResponse.json({ ...mockTask, QuestID: null });
        })
      );

      render(<ObjectiveDetail />);

      // Wait for task to load
      await waitFor(() => {
        expect(screen.getByText('Implement feature X')).toBeInTheDocument();
      });

      const backLink = screen.getByLabelText(/Go back/i);
      expect(backLink).toHaveAttribute('href', '/v2');
    });

    it('displays inbox count', async () => {
      render(<ObjectiveDetail />);

      await waitFor(() => {
        // Multiple badges exist for mobile/desktop
        const badges = screen.getAllByText('2');
        expect(badges.length).toBeGreaterThan(0);
      });
    });
  });

  describe('error handling', () => {
    it('shows toast on pause failure', async () => {
      server.use(
        http.post('/api/v1/tasks/:id/pause', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      const user = userEvent.setup();
      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Pause' }));

      await waitFor(() => {
        expect(screen.getByText('Failed to pause objective')).toBeInTheDocument();
      });
    });

    it('shows toast on load failure', async () => {
      server.use(
        http.get('/api/v1/tasks/:id', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      render(<ObjectiveDetail />);

      await waitFor(() => {
        expect(screen.getByText('Failed to load objective')).toBeInTheDocument();
      });
    });
  });
});
