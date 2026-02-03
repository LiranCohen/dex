import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { Card } from './Card';

describe('Card', () => {
  it('renders children', () => {
    render(<Card>Card content</Card>);
    expect(screen.getByText('Card content')).toBeInTheDocument();
  });

  it('applies medium padding by default', () => {
    render(<Card data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('v2-card--padding-md');
  });

  it('applies small padding', () => {
    render(<Card padding="sm" data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('v2-card--padding-sm');
  });

  it('applies large padding', () => {
    render(<Card padding="lg" data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('v2-card--padding-lg');
  });

  it('applies interactive class when interactive', () => {
    render(<Card interactive data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('v2-card--interactive');
  });

  it('does not apply interactive class by default', () => {
    render(<Card data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).not.toHaveClass('v2-card--interactive');
  });

  it('applies selected class when selected', () => {
    render(<Card selected data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('v2-card--selected');
  });

  it('does not apply selected class by default', () => {
    render(<Card data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).not.toHaveClass('v2-card--selected');
  });

  it('applies custom className', () => {
    render(<Card className="custom-card" data-testid="card">Content</Card>);
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('custom-card');
    expect(card).toHaveClass('v2-card');
  });

  it('handles click events on interactive cards', async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(
      <Card interactive onClick={handleClick} data-testid="card">
        Clickable
      </Card>
    );

    await user.click(screen.getByTestId('card'));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('forwards ref to div element', () => {
    const ref = { current: null as HTMLDivElement | null };
    render(<Card ref={ref}>Ref Test</Card>);
    expect(ref.current).toBeInstanceOf(HTMLDivElement);
  });

  it('combines multiple state classes', () => {
    render(
      <Card interactive selected padding="lg" data-testid="card">
        Content
      </Card>
    );
    const card = screen.getByTestId('card');
    expect(card).toHaveClass('v2-card--interactive');
    expect(card).toHaveClass('v2-card--selected');
    expect(card).toHaveClass('v2-card--padding-lg');
  });

  it('passes through additional div props', () => {
    render(
      <Card role="article" aria-label="Test card" data-testid="card">
        Content
      </Card>
    );
    const card = screen.getByTestId('card');
    expect(card).toHaveAttribute('role', 'article');
    expect(card).toHaveAttribute('aria-label', 'Test card');
  });
});
