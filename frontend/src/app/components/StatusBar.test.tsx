import { describe, it, expect } from 'vitest';
import { render, screen } from '../../test/test-utils';
import { StatusBar } from './StatusBar';

describe('StatusBar', () => {
  it('renders with active status', () => {
    render(<StatusBar status="active" />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).toHaveClass('app-status-bar--active');
    expect(statusBar).toHaveAttribute('aria-label', 'Status: Active');
  });

  it('renders with pending status', () => {
    render(<StatusBar status="pending" />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).toHaveClass('app-status-bar--pending');
    expect(statusBar).toHaveAttribute('aria-label', 'Status: Pending');
  });

  it('renders with complete status', () => {
    render(<StatusBar status="complete" />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).toHaveClass('app-status-bar--complete');
    expect(statusBar).toHaveAttribute('aria-label', 'Status: Complete');
  });

  it('renders with error status', () => {
    render(<StatusBar status="error" />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).toHaveClass('app-status-bar--error');
    expect(statusBar).toHaveAttribute('aria-label', 'Status: Error');
  });

  it('applies pulse animation when status is active and pulse is true', () => {
    render(<StatusBar status="active" pulse />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).toHaveClass('app-status-bar--pulse');
  });

  it('does not apply pulse animation when status is not active', () => {
    render(<StatusBar status="pending" pulse />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).not.toHaveClass('app-status-bar--pulse');
  });

  it('does not apply pulse animation by default', () => {
    render(<StatusBar status="active" />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).not.toHaveClass('app-status-bar--pulse');
  });

  it('applies custom className', () => {
    render(<StatusBar status="active" className="custom-status" />);
    const statusBar = screen.getByRole('status');
    expect(statusBar).toHaveClass('custom-status');
    expect(statusBar).toHaveClass('app-status-bar');
  });

  it('always has base class', () => {
    render(<StatusBar status="complete" />);
    expect(screen.getByRole('status')).toHaveClass('app-status-bar');
  });
});
