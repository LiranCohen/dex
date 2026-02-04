import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { EmptyState } from './EmptyState';

describe('EmptyState', () => {
  it('renders default message', () => {
    render(<EmptyState />);
    expect(screen.getByText('[ Nothing here yet ]')).toBeInTheDocument();
  });

  it('renders custom message', () => {
    render(<EmptyState message="No results found" />);
    expect(screen.getByText('[ No results found ]')).toBeInTheDocument();
  });

  it('renders hint when provided', () => {
    render(<EmptyState hint="Try adjusting your search" />);
    expect(screen.getByText('Try adjusting your search')).toBeInTheDocument();
  });

  it('does not render hint when not provided', () => {
    const { container } = render(<EmptyState />);
    const hint = container.querySelector('.app-empty-state__hint');
    expect(hint).not.toBeInTheDocument();
  });

  it('renders action button when provided', () => {
    const handleAction = vi.fn();
    render(<EmptyState action={{ label: 'Create New', onClick: handleAction }} />);
    expect(screen.getByRole('button', { name: 'Create New' })).toBeInTheDocument();
  });

  it('does not render action button when not provided', () => {
    render(<EmptyState />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('calls onClick when action button is clicked', async () => {
    const user = userEvent.setup();
    const handleAction = vi.fn();
    render(<EmptyState action={{ label: 'Create New', onClick: handleAction }} />);

    await user.click(screen.getByRole('button', { name: 'Create New' }));
    expect(handleAction).toHaveBeenCalledTimes(1);
  });

  it('action button has primary variant styling', () => {
    const handleAction = vi.fn();
    render(<EmptyState action={{ label: 'Action', onClick: handleAction }} />);
    const button = screen.getByRole('button');
    expect(button).toHaveClass('app-btn--primary');
  });

  it('has status role', () => {
    render(<EmptyState />);
    expect(screen.getByRole('status')).toBeInTheDocument();
  });

  it('has empty state class', () => {
    const { container } = render(<EmptyState />);
    expect(container.querySelector('.app-empty-state')).toBeInTheDocument();
  });

  it('renders all elements together', () => {
    const handleAction = vi.fn();
    render(
      <EmptyState
        message="No quests found"
        hint="Start a new quest to get going"
        action={{ label: 'Start Quest', onClick: handleAction }}
      />
    );

    expect(screen.getByText('[ No quests found ]')).toBeInTheDocument();
    expect(screen.getByText('Start a new quest to get going')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start Quest' })).toBeInTheDocument();
  });
});
