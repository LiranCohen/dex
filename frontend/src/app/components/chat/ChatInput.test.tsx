import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '../../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { ChatInput } from './ChatInput';

// Mock react-textarea-autosize since it has complex resize behavior
vi.mock('react-textarea-autosize', () => ({
  default: ({ ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => (
    <textarea {...props} />
  ),
}));

describe('ChatInput', () => {
  const defaultProps = {
    onSend: vi.fn(),
  };

  beforeEach(() => {
    defaultProps.onSend = vi.fn();
  });

  it('renders textarea with default placeholder', () => {
    render(<ChatInput {...defaultProps} />);
    expect(screen.getByPlaceholderText('Type a message...')).toBeInTheDocument();
  });

  it('renders with custom placeholder', () => {
    render(<ChatInput {...defaultProps} placeholder="Ask me anything..." />);
    expect(screen.getByPlaceholderText('Ask me anything...')).toBeInTheDocument();
  });

  it('shows disconnected placeholder when not connected', () => {
    render(<ChatInput {...defaultProps} isConnected={false} />);
    expect(screen.getByPlaceholderText('Connection lost...')).toBeInTheDocument();
  });

  it('shows "..." placeholder when generating', () => {
    render(<ChatInput {...defaultProps} isGenerating />);
    expect(screen.getByPlaceholderText('...')).toBeInTheDocument();
  });

  it('calls onSend when Enter is pressed', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} />);

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, 'Hello world');
    await user.keyboard('{Enter}');

    expect(defaultProps.onSend).toHaveBeenCalledWith('Hello world');
  });

  it('does not send on Shift+Enter (allows newline)', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} />);

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, 'Line 1{Shift>}{Enter}{/Shift}Line 2');

    expect(defaultProps.onSend).not.toHaveBeenCalled();
  });

  it('clears input after sending', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} />);

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, 'Hello');
    await user.keyboard('{Enter}');

    expect(textarea).toHaveValue('');
  });

  it('does not send empty messages', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} />);

    const textarea = screen.getByRole('textbox');
    await user.click(textarea);
    await user.keyboard('{Enter}');

    expect(defaultProps.onSend).not.toHaveBeenCalled();
  });

  it('does not send whitespace-only messages', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} />);

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, '   ');
    await user.keyboard('{Enter}');

    expect(defaultProps.onSend).not.toHaveBeenCalled();
  });

  it('does not send when disabled', () => {
    render(<ChatInput {...defaultProps} disabled />);

    const textarea = screen.getByRole('textbox');
    expect(textarea).toBeDisabled();
  });

  it('does not send when generating', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} isGenerating />);

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, 'Hello');
    await user.keyboard('{Enter}');

    expect(defaultProps.onSend).not.toHaveBeenCalled();
  });

  it('still accepts input when disconnected for queuing', async () => {
    // When disconnected, users can still type and send messages (they get queued)
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} isConnected={false} />);

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, 'Hello');
    await user.keyboard('{Enter}');

    // Message is sent (will be queued by parent component)
    expect(defaultProps.onSend).toHaveBeenCalledWith('Hello');
  });

  describe('command history', () => {
    it('shows most recent command on ArrowUp when input is empty', async () => {
      const user = userEvent.setup();
      // History is stored oldest first, so 'third command' is most recent
      const history = ['first command', 'second command', 'third command'];
      render(<ChatInput {...defaultProps} commandHistory={history} />);

      const textarea = screen.getByRole('textbox');
      await user.click(textarea);
      await user.keyboard('{ArrowUp}');

      // Most recent command appears first
      expect(textarea).toHaveValue('third command');
    });

    it('navigates down through history with ArrowDown', async () => {
      const user = userEvent.setup();
      const history = ['first command', 'second command'];
      render(<ChatInput {...defaultProps} commandHistory={history} />);

      const textarea = screen.getByRole('textbox');
      await user.click(textarea);

      // Go up to most recent (second command)
      await user.keyboard('{ArrowUp}');
      expect(textarea).toHaveValue('second command');

      // Go down (back to empty since we only went up once)
      await user.keyboard('{ArrowDown}');
      expect(textarea).toHaveValue('');
    });

    it('clears to empty when navigating past newest in history', async () => {
      const user = userEvent.setup();
      const history = ['old command'];
      render(<ChatInput {...defaultProps} commandHistory={history} />);

      const textarea = screen.getByRole('textbox');
      await user.click(textarea);

      await user.keyboard('{ArrowUp}');
      expect(textarea).toHaveValue('old command');

      await user.keyboard('{ArrowDown}');
      expect(textarea).toHaveValue('');
    });

    it('only navigates history when input is empty', async () => {
      const user = userEvent.setup();
      const history = ['old command'];
      render(<ChatInput {...defaultProps} commandHistory={history} />);

      const textarea = screen.getByRole('textbox');
      await user.type(textarea, 'current text');
      await user.keyboard('{ArrowUp}');

      // Should not change because there's text in the input
      expect(textarea).toHaveValue('current text');
    });
  });

  describe('stop button', () => {
    it('shows stop button when generating', () => {
      const onStop = vi.fn();
      render(<ChatInput {...defaultProps} isGenerating onStop={onStop} />);

      expect(screen.getByRole('button', { name: 'Stop generating' })).toBeInTheDocument();
    });

    it('calls onStop when stop button is clicked', async () => {
      const user = userEvent.setup();
      const onStop = vi.fn();
      render(<ChatInput {...defaultProps} isGenerating onStop={onStop} />);

      await user.click(screen.getByRole('button', { name: 'Stop generating' }));
      expect(onStop).toHaveBeenCalledTimes(1);
    });

    it('shows send button when not generating', () => {
      render(<ChatInput {...defaultProps} />);
      expect(screen.getByRole('button', { name: 'Send message' })).toBeInTheDocument();
    });
  });

  describe('send button', () => {
    it('is disabled when input is empty', () => {
      render(<ChatInput {...defaultProps} />);
      expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
    });

    it('is enabled when input has content', async () => {
      const user = userEvent.setup();
      render(<ChatInput {...defaultProps} />);

      const textarea = screen.getByRole('textbox');
      await user.type(textarea, 'Hello');

      expect(screen.getByRole('button', { name: 'Send message' })).not.toBeDisabled();
    });

    it('sends message when clicked', async () => {
      const user = userEvent.setup();
      render(<ChatInput {...defaultProps} />);

      const textarea = screen.getByRole('textbox');
      await user.type(textarea, 'Hello');
      await user.click(screen.getByRole('button', { name: 'Send message' }));

      expect(defaultProps.onSend).toHaveBeenCalledWith('Hello');
    });
  });

  it('blurs on Escape key', async () => {
    const user = userEvent.setup();
    render(<ChatInput {...defaultProps} />);

    const textarea = screen.getByRole('textbox');
    await user.click(textarea);
    expect(textarea).toHaveFocus();

    await user.keyboard('{Escape}');
    expect(textarea).not.toHaveFocus();
  });

  it('applies disconnected class when not connected', () => {
    const { container } = render(<ChatInput {...defaultProps} isConnected={false} />);
    expect(container.querySelector('.app-chat-input--disconnected')).toBeInTheDocument();
  });
});
