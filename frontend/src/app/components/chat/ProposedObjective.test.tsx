import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '../../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { ProposedObjective } from './ProposedObjective';

describe('ProposedObjective', () => {
  const defaultProps = {
    title: 'Implement Feature X',
    description: 'Add the new feature with tests',
    checklist: [
      { id: '1', text: 'Write implementation' },
      { id: '2', text: 'Add unit tests' },
      { id: '3', text: 'Update documentation', isOptional: true },
    ],
    onAccept: vi.fn().mockResolvedValue(undefined),
    onReject: vi.fn().mockResolvedValue(undefined),
  };

  beforeEach(() => {
    defaultProps.onAccept = vi.fn().mockResolvedValue(undefined);
    defaultProps.onReject = vi.fn().mockResolvedValue(undefined);
  });

  it('renders title', () => {
    render(<ProposedObjective {...defaultProps} />);
    expect(screen.getByText('Implement Feature X')).toBeInTheDocument();
  });

  it('renders description', () => {
    render(<ProposedObjective {...defaultProps} />);
    expect(screen.getByText('Add the new feature with tests')).toBeInTheDocument();
  });

  it('renders must have items', () => {
    render(<ProposedObjective {...defaultProps} />);
    expect(screen.getByText('Write implementation')).toBeInTheDocument();
    expect(screen.getByText('Add unit tests')).toBeInTheDocument();
  });

  it('renders optional items separately', () => {
    render(<ProposedObjective {...defaultProps} />);
    expect(screen.getByText('Update documentation')).toBeInTheDocument();
    expect(screen.getByText('Optional')).toBeInTheDocument();
  });

  it('shows "Proposed" label when pending', () => {
    render(<ProposedObjective {...defaultProps} />);
    expect(screen.getByText('Proposed')).toBeInTheDocument();
  });

  it('has accessible article role', () => {
    render(<ProposedObjective {...defaultProps} />);
    expect(screen.getByRole('article', { name: /Proposed objective: Implement Feature X/i })).toBeInTheDocument();
  });

  describe('buttons', () => {
    it('shows Accept and Reject buttons when pending', () => {
      render(<ProposedObjective {...defaultProps} />);
      expect(screen.getByRole('button', { name: /Accept/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Reject/i })).toBeInTheDocument();
    });

    it('calls onAccept when Accept is clicked', async () => {
      const user = userEvent.setup();
      render(<ProposedObjective {...defaultProps} />);

      await user.click(screen.getByRole('button', { name: /Accept/i }));

      expect(defaultProps.onAccept).toHaveBeenCalledTimes(1);
    });

    it('calls onReject when Reject is clicked', async () => {
      const user = userEvent.setup();
      render(<ProposedObjective {...defaultProps} />);

      await user.click(screen.getByRole('button', { name: /Reject/i }));

      expect(defaultProps.onReject).toHaveBeenCalledTimes(1);
    });

    it('shows accepting state when status is accepting', () => {
      render(<ProposedObjective {...defaultProps} status="accepting" />);
      expect(screen.getByText('Accepting...')).toBeInTheDocument();
    });

    it('shows rejecting state when status is rejecting', () => {
      render(<ProposedObjective {...defaultProps} status="rejecting" />);
      expect(screen.getByText('Rejecting...')).toBeInTheDocument();
    });

    it('shows accepted state when status is accepted', () => {
      render(<ProposedObjective {...defaultProps} status="accepted" />);
      expect(screen.getByText('✓ Accepted')).toBeInTheDocument();
    });

    it('shows rejected state when status is rejected', () => {
      render(<ProposedObjective {...defaultProps} status="rejected" />);
      expect(screen.getByText('✗ Rejected')).toBeInTheDocument();
    });

    it('hides buttons when not pending', () => {
      render(<ProposedObjective {...defaultProps} status="accepted" />);
      expect(screen.queryByRole('button', { name: /Accept/i })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /Reject/i })).not.toBeInTheDocument();
    });

    it('calls onAccept with selected optional indices', async () => {
      const user = userEvent.setup();
      render(<ProposedObjective {...defaultProps} />);

      await user.click(screen.getByRole('button', { name: /Accept/i }));

      expect(defaultProps.onAccept).toHaveBeenCalledWith([0]); // First optional item selected by default
    });
  });

  describe('keyboard shortcuts', () => {
    it('accepts on Y key', async () => {
      render(<ProposedObjective {...defaultProps} />);

      fireEvent.keyDown(document, { key: 'y' });

      await waitFor(() => {
        expect(defaultProps.onAccept).toHaveBeenCalledTimes(1);
      });
    });

    it('accepts on uppercase Y key', async () => {
      render(<ProposedObjective {...defaultProps} />);

      fireEvent.keyDown(document, { key: 'Y' });

      await waitFor(() => {
        expect(defaultProps.onAccept).toHaveBeenCalledTimes(1);
      });
    });

    it('rejects on N key', async () => {
      render(<ProposedObjective {...defaultProps} />);

      fireEvent.keyDown(document, { key: 'n' });

      await waitFor(() => {
        expect(defaultProps.onReject).toHaveBeenCalledTimes(1);
      });
    });

    it('rejects on uppercase N key', async () => {
      render(<ProposedObjective {...defaultProps} />);

      fireEvent.keyDown(document, { key: 'N' });

      await waitFor(() => {
        expect(defaultProps.onReject).toHaveBeenCalledTimes(1);
      });
    });

    it('does not respond to shortcuts when in input', () => {
      render(<ProposedObjective {...defaultProps} />);

      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();

      const event = new KeyboardEvent('keydown', { key: 'y', bubbles: true });
      Object.defineProperty(event, 'target', { value: input });
      document.dispatchEvent(event);

      expect(defaultProps.onAccept).not.toHaveBeenCalled();

      document.body.removeChild(input);
    });

    it('shows keyboard hint for pending proposals', () => {
      render(<ProposedObjective {...defaultProps} />);
      expect(screen.getByText(/Press Y to accept, N to reject/)).toBeInTheDocument();
    });

    it('hides keyboard hint when not pending', () => {
      render(<ProposedObjective {...defaultProps} status="accepted" />);
      expect(screen.queryByText(/Press Y to accept, N to reject/)).not.toBeInTheDocument();
    });
  });

  describe('status prop', () => {
    it('respects accepted status', () => {
      render(<ProposedObjective {...defaultProps} status="accepted" />);
      expect(screen.getByText('✓ Accepted')).toBeInTheDocument();
    });

    it('respects rejected status', () => {
      render(<ProposedObjective {...defaultProps} status="rejected" />);
      expect(screen.getByText('✗ Rejected')).toBeInTheDocument();
    });
  });

  describe('styling', () => {
    it('applies accepted class', () => {
      const { container } = render(<ProposedObjective {...defaultProps} status="accepted" />);
      expect(container.querySelector('.app-proposed--accepted')).toBeInTheDocument();
    });

    it('applies rejected class', () => {
      const { container } = render(<ProposedObjective {...defaultProps} status="rejected" />);
      expect(container.querySelector('.app-proposed--rejected')).toBeInTheDocument();
    });

    it('applies loading class during action', () => {
      const { container } = render(<ProposedObjective {...defaultProps} status="accepting" />);
      expect(container.querySelector('.app-proposed--loading')).toBeInTheDocument();
    });
  });

  it('handles checklist without optional items', () => {
    const props = {
      ...defaultProps,
      checklist: [
        { id: '1', text: 'Item 1' },
        { id: '2', text: 'Item 2' },
      ],
    };
    render(<ProposedObjective {...props} />);

    expect(screen.queryByText('Optional')).not.toBeInTheDocument();
    expect(screen.getByText('Item 1')).toBeInTheDocument();
  });
});
