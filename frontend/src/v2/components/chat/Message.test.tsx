import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '../../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { Message } from './Message';

describe('Message', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders message content', () => {
    render(<Message sender="user">Hello, world!</Message>);
    expect(screen.getByText('Hello, world!')).toBeInTheDocument();
  });

  it('displays handle for user messages', () => {
    render(<Message sender="user">Test message</Message>);
    // IRC-style handle: <you>
    expect(screen.getByText('<you>')).toBeInTheDocument();
  });

  it('displays handle for assistant messages', () => {
    render(<Message sender="assistant">Test message</Message>);
    // IRC-style handle: <dex>
    expect(screen.getByText('<dex>')).toBeInTheDocument();
  });

  it('displays timestamp when provided', () => {
    render(
      <Message sender="user" timestamp="2:30 PM">
        Test
      </Message>
    );
    expect(screen.getByText('2:30 PM')).toBeInTheDocument();
  });

  it('hides timestamp during streaming', () => {
    render(
      <Message sender="assistant" timestamp="2:30 PM" isStreaming>
        Streaming...
      </Message>
    );
    expect(screen.queryByText('2:30 PM')).not.toBeInTheDocument();
  });

  describe('message status', () => {
    it('applies error class for error status', () => {
      const { container } = render(
        <Message sender="user" status="error">
          Failed message
        </Message>
      );
      expect(container.querySelector('.v2-message--error')).toBeInTheDocument();
    });

    it('shows error text for error status', () => {
      render(
        <Message sender="user" status="error">
          Failed
        </Message>
      );
      expect(screen.getByText('✗ Failed to send')).toBeInTheDocument();
    });

    it('shows retry button when onRetry is provided', () => {
      render(
        <Message sender="user" status="error" onRetry={() => {}}>
          Failed
        </Message>
      );
      expect(screen.getByRole('button', { name: 'Retry sending message' })).toBeInTheDocument();
    });

    it('calls onRetry when retry button clicked', async () => {
      const user = userEvent.setup();
      const onRetry = vi.fn();
      render(
        <Message sender="user" status="error" onRetry={onRetry}>
          Failed
        </Message>
      );

      await user.click(screen.getByRole('button', { name: 'Retry sending message' }));
      expect(onRetry).toHaveBeenCalledTimes(1);
    });
  });

  describe('streaming state', () => {
    it('applies streaming class', () => {
      const { container } = render(
        <Message sender="assistant" isStreaming>
          Loading...
        </Message>
      );
      expect(container.querySelector('.v2-message--streaming')).toBeInTheDocument();
    });

    it('shows cursor during streaming', () => {
      const { container } = render(
        <Message sender="assistant" isStreaming>
          Loading...
        </Message>
      );
      expect(container.querySelector('.v2-cursor')).toBeInTheDocument();
    });

    it('hides cursor when not streaming', () => {
      const { container } = render(
        <Message sender="assistant">
          Complete message
        </Message>
      );
      expect(container.querySelector('.v2-cursor')).not.toBeInTheDocument();
    });
  });

  describe('tool calls', () => {
    const toolCalls = [
      { id: '1', name: 'Read', status: 'complete' as const, description: 'main.ts' },
      { id: '2', name: 'Write', status: 'running' as const },
      { id: '3', name: 'Bash', status: 'error' as const, description: 'npm test' },
    ];

    it('renders tool calls', () => {
      render(
        <Message sender="assistant" toolCalls={toolCalls}>
          Working on it...
        </Message>
      );

      expect(screen.getByText('Read')).toBeInTheDocument();
      expect(screen.getByText('Write')).toBeInTheDocument();
      expect(screen.getByText('Bash')).toBeInTheDocument();
    });

    it('shows tool descriptions when provided', () => {
      render(
        <Message sender="assistant" toolCalls={toolCalls}>
          Working...
        </Message>
      );

      expect(screen.getByText('main.ts')).toBeInTheDocument();
      expect(screen.getByText('npm test')).toBeInTheDocument();
    });

    it('shows spinner for running tools', () => {
      const { container } = render(
        <Message sender="assistant" toolCalls={[{ id: '1', name: 'Test', status: 'running' }]}>
          Running...
        </Message>
      );

      expect(container.querySelector('.v2-tool-activity__spinner')).toBeInTheDocument();
    });

    it('shows dot icon for complete tools', () => {
      render(
        <Message sender="assistant" toolCalls={[{ id: '1', name: 'Test', status: 'complete' }]}>
          Done
        </Message>
      );

      expect(screen.getByText('·')).toBeInTheDocument();
    });

    it('shows error icon for failed tools', () => {
      render(
        <Message sender="assistant" toolCalls={[{ id: '1', name: 'Test', status: 'error' }]}>
          Failed
        </Message>
      );

      expect(screen.getByText('✗')).toBeInTheDocument();
    });
  });

  describe('copy functionality', () => {
    it('shows copy button on hover for assistant messages', () => {
      const { container } = render(
        <Message sender="assistant">
          Copy me
        </Message>
      );

      const message = container.querySelector('.v2-message')!;
      fireEvent.mouseEnter(message);

      expect(screen.getByLabelText('Copy message')).toBeInTheDocument();
    });

    it('does not show copy button for user messages', () => {
      const { container } = render(
        <Message sender="user">
          My message
        </Message>
      );

      const message = container.querySelector('.v2-message')!;
      fireEvent.mouseEnter(message);

      expect(screen.queryByLabelText('Copy message')).not.toBeInTheDocument();
    });

    it('hides copy button on mouse leave', () => {
      const { container } = render(
        <Message sender="assistant">
          Copy me
        </Message>
      );

      const message = container.querySelector('.v2-message')!;
      fireEvent.mouseEnter(message);
      expect(screen.getByLabelText('Copy message')).toBeInTheDocument();

      fireEvent.mouseLeave(message);
      expect(screen.queryByLabelText('Copy message')).not.toBeInTheDocument();
    });

    it('does not show copy button for error status', () => {
      const { container } = render(
        <Message sender="assistant" status="error">
          Error message
        </Message>
      );

      const message = container.querySelector('.v2-message')!;
      fireEvent.mouseEnter(message);

      expect(screen.queryByLabelText('Copy message')).not.toBeInTheDocument();
    });
  });

  it('applies sender class', () => {
    const { container } = render(<Message sender="user">Test</Message>);
    expect(container.querySelector('.v2-message--user')).toBeInTheDocument();
  });
});
