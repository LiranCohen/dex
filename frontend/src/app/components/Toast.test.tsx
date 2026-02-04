import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '../../test/test-utils';
import { useToast } from './Toast';

// Test component that exposes showToast
function TestComponent() {
  const { showToast } = useToast();

  return (
    <div>
      <button onClick={() => showToast('Info message')}>Show Info</button>
      <button onClick={() => showToast('Success message', 'success')}>Show Success</button>
      <button onClick={() => showToast('Error message', 'error')}>Show Error</button>
    </div>
  );
}

describe('Toast', () => {
  it('throws error when useToast is used outside ToastProvider', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => {
      render(<TestComponent />, { withToast: false });
    }).toThrow('useToast must be used within a ToastProvider');

    consoleError.mockRestore();
  });

  it('shows toast when showToast is called', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));

    expect(screen.getByText('Info message')).toBeInTheDocument();
  });

  it('shows success toast', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Success'));

    const toast = screen.getByRole('alert');
    expect(toast).toHaveClass('app-toast--success');
    expect(screen.getByText('Success message')).toBeInTheDocument();
  });

  it('shows error toast', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Error'));

    const toast = screen.getByRole('alert');
    expect(toast).toHaveClass('app-toast--error');
    expect(screen.getByText('Error message')).toBeInTheDocument();
  });

  it('shows info toast by default', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));

    const toast = screen.getByRole('alert');
    expect(toast).toHaveClass('app-toast--info');
  });

  it('shows multiple toasts at once', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));
    fireEvent.click(screen.getByText('Show Success'));
    fireEvent.click(screen.getByText('Show Error'));

    expect(screen.getByText('Info message')).toBeInTheDocument();
    expect(screen.getByText('Success message')).toBeInTheDocument();
    expect(screen.getByText('Error message')).toBeInTheDocument();
  });

  it('renders toast container with notifications region', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));

    expect(screen.getByRole('region', { name: 'Notifications' })).toBeInTheDocument();
  });

  it('does not render toast container when no toasts', () => {
    render(<TestComponent />);
    expect(screen.queryByRole('region', { name: 'Notifications' })).not.toBeInTheDocument();
  });

  it('displays correct icon for success toast', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Success'));

    expect(screen.getByText('✓')).toBeInTheDocument();
  });

  it('displays correct icon for error toast', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Error'));

    expect(screen.getByText('✗')).toBeInTheDocument();
  });

  it('displays correct icon for info toast', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));

    expect(screen.getByText('·')).toBeInTheDocument();
  });

  it('has dismiss button', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));

    expect(screen.getByLabelText('Dismiss notification')).toBeInTheDocument();
  });

  it('has correct aria attributes on toast', () => {
    render(<TestComponent />);

    fireEvent.click(screen.getByText('Show Info'));

    const toast = screen.getByRole('alert');
    expect(toast).toHaveAttribute('aria-live', 'polite');
  });
});
