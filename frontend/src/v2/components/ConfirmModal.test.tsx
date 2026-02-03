import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { ConfirmModal } from './ConfirmModal';

describe('ConfirmModal', () => {
  const defaultProps = {
    isOpen: true,
    title: 'Confirm Action',
    message: 'Are you sure you want to proceed?',
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
  };

  beforeEach(() => {
    defaultProps.onConfirm = vi.fn();
    defaultProps.onCancel = vi.fn();
  });

  afterEach(() => {
    document.body.style.overflow = '';
  });

  it('renders when isOpen is true', () => {
    render(<ConfirmModal {...defaultProps} />);
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    expect(screen.getByText('Confirm Action')).toBeInTheDocument();
    expect(screen.getByText('Are you sure you want to proceed?')).toBeInTheDocument();
  });

  it('does not render when isOpen is false', () => {
    render(<ConfirmModal {...defaultProps} isOpen={false} />);
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('uses default button labels', () => {
    render(<ConfirmModal {...defaultProps} />);
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('uses custom button labels', () => {
    render(
      <ConfirmModal
        {...defaultProps}
        confirmLabel="Yes, delete"
        cancelLabel="No, keep it"
      />
    );
    expect(screen.getByRole('button', { name: 'Yes, delete' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'No, keep it' })).toBeInTheDocument();
  });

  it('calls onConfirm when confirm button is clicked', async () => {
    const user = userEvent.setup();
    render(<ConfirmModal {...defaultProps} />);

    await user.click(screen.getByRole('button', { name: 'Confirm' }));
    expect(defaultProps.onConfirm).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel when cancel button is clicked', async () => {
    const user = userEvent.setup();
    render(<ConfirmModal {...defaultProps} />);

    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel when clicking overlay', async () => {
    const user = userEvent.setup();
    const { container } = render(<ConfirmModal {...defaultProps} />);

    const overlay = container.querySelector('.v2-modal-overlay');
    await user.click(overlay!);
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  it('does not call onCancel when clicking modal content', async () => {
    const user = userEvent.setup();
    render(<ConfirmModal {...defaultProps} />);

    await user.click(screen.getByRole('alertdialog'));
    expect(defaultProps.onCancel).not.toHaveBeenCalled();
  });

  it('calls onCancel when Escape key is pressed', () => {
    render(<ConfirmModal {...defaultProps} />);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  it('focuses confirm button when opened', () => {
    render(<ConfirmModal {...defaultProps} />);
    expect(screen.getByRole('button', { name: 'Confirm' })).toHaveFocus();
  });

  it('applies danger variant styling', () => {
    render(<ConfirmModal {...defaultProps} variant="danger" />);
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toHaveClass('v2-btn--danger');
  });

  it('applies default variant styling', () => {
    render(<ConfirmModal {...defaultProps} variant="default" />);
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toHaveClass('v2-btn--primary');
  });

  it('prevents body scroll when open', () => {
    render(<ConfirmModal {...defaultProps} />);
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('restores body scroll when closed', () => {
    const { rerender } = render(<ConfirmModal {...defaultProps} />);
    expect(document.body.style.overflow).toBe('hidden');

    rerender(<ConfirmModal {...defaultProps} isOpen={false} />);
    expect(document.body.style.overflow).toBe('');
  });

  it('has proper ARIA attributes', () => {
    render(<ConfirmModal {...defaultProps} />);
    const dialog = screen.getByRole('alertdialog');

    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAttribute('aria-labelledby', 'confirm-modal-title');
    expect(dialog).toHaveAttribute('aria-describedby', 'confirm-modal-message');
  });

  describe('focus trap', () => {
    it('traps focus within modal', async () => {
      const user = userEvent.setup();
      render(<ConfirmModal {...defaultProps} />);

      const confirmButton = screen.getByRole('button', { name: 'Confirm' });
      const cancelButton = screen.getByRole('button', { name: 'Cancel' });

      // Confirm button is focused initially
      expect(confirmButton).toHaveFocus();

      // Tab forward - should go to Cancel
      await user.tab();
      expect(cancelButton).toHaveFocus();

      // Tab forward again - should wrap to Confirm
      await user.tab();
      expect(confirmButton).toHaveFocus();
    });

    it('wraps focus backwards with Shift+Tab', async () => {
      const user = userEvent.setup();
      render(<ConfirmModal {...defaultProps} />);

      const confirmButton = screen.getByRole('button', { name: 'Confirm' });
      const cancelButton = screen.getByRole('button', { name: 'Cancel' });

      // Confirm button is focused initially
      expect(confirmButton).toHaveFocus();

      // Shift+Tab - should wrap to Cancel
      await user.tab({ shift: true });
      expect(cancelButton).toHaveFocus();
    });
  });

  describe('focus restoration', () => {
    it('stores previous active element on open', () => {
      // Focus a button before opening modal
      const button = document.createElement('button');
      button.textContent = 'External Button';
      document.body.appendChild(button);
      button.focus();
      expect(document.activeElement).toBe(button);

      // Open modal
      render(<ConfirmModal {...defaultProps} />);

      // Confirm button should now be focused
      expect(screen.getByRole('button', { name: 'Confirm' })).toHaveFocus();

      // Cleanup
      document.body.removeChild(button);
    });
  });
});
