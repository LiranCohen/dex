import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '../../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { QuestionPrompt } from './QuestionPrompt';

describe('QuestionPrompt', () => {
  const defaultProps = {
    question: 'Which framework do you prefer?',
    options: [
      { id: 'react', title: 'React', description: 'A JavaScript library for building user interfaces' },
      { id: 'vue', title: 'Vue', description: 'The Progressive JavaScript Framework' },
      { id: 'svelte', title: 'Svelte', description: 'Cybernetically enhanced web apps' },
    ],
    onSelect: vi.fn(),
  };

  beforeEach(() => {
    defaultProps.onSelect = vi.fn();
  });

  it('renders the question', () => {
    render(<QuestionPrompt {...defaultProps} />);
    expect(screen.getByText('Which framework do you prefer?')).toBeInTheDocument();
  });

  it('renders "Question" label', () => {
    render(<QuestionPrompt {...defaultProps} />);
    expect(screen.getByText('Question')).toBeInTheDocument();
  });

  it('renders all options', () => {
    render(<QuestionPrompt {...defaultProps} />);

    expect(screen.getByText('React')).toBeInTheDocument();
    expect(screen.getByText('Vue')).toBeInTheDocument();
    expect(screen.getByText('Svelte')).toBeInTheDocument();
  });

  it('renders option descriptions', () => {
    render(<QuestionPrompt {...defaultProps} />);

    expect(screen.getByText('A JavaScript library for building user interfaces')).toBeInTheDocument();
    expect(screen.getByText('The Progressive JavaScript Framework')).toBeInTheDocument();
    expect(screen.getByText('Cybernetically enhanced web apps')).toBeInTheDocument();
  });

  it('shows number keys for each option', () => {
    render(<QuestionPrompt {...defaultProps} />);

    expect(screen.getByText('1')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('calls onSelect when option is clicked', async () => {
    const user = userEvent.setup();
    render(<QuestionPrompt {...defaultProps} />);

    await user.click(screen.getByText('Vue'));

    expect(defaultProps.onSelect).toHaveBeenCalledWith('vue');
  });

  it('calls onSelect when option button is clicked', async () => {
    const user = userEvent.setup();
    render(<QuestionPrompt {...defaultProps} />);

    const buttons = screen.getAllByRole('option');
    await user.click(buttons[2]); // Click Svelte

    expect(defaultProps.onSelect).toHaveBeenCalledWith('svelte');
  });

  describe('keyboard shortcuts', () => {
    it('selects option with number key 1', () => {
      render(<QuestionPrompt {...defaultProps} />);

      fireEvent.keyDown(document, { key: '1' });

      expect(defaultProps.onSelect).toHaveBeenCalledWith('react');
    });

    it('selects option with number key 2', () => {
      render(<QuestionPrompt {...defaultProps} />);

      fireEvent.keyDown(document, { key: '2' });

      expect(defaultProps.onSelect).toHaveBeenCalledWith('vue');
    });

    it('selects option with number key 3', () => {
      render(<QuestionPrompt {...defaultProps} />);

      fireEvent.keyDown(document, { key: '3' });

      expect(defaultProps.onSelect).toHaveBeenCalledWith('svelte');
    });

    it('ignores number keys outside option range', () => {
      render(<QuestionPrompt {...defaultProps} />);

      fireEvent.keyDown(document, { key: '5' });

      expect(defaultProps.onSelect).not.toHaveBeenCalled();
    });

    it('ignores number key 0', () => {
      render(<QuestionPrompt {...defaultProps} />);

      fireEvent.keyDown(document, { key: '0' });

      expect(defaultProps.onSelect).not.toHaveBeenCalled();
    });

    it('does not respond to shortcuts when in input', () => {
      render(<QuestionPrompt {...defaultProps} />);

      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();

      const event = new KeyboardEvent('keydown', { key: '1', bubbles: true });
      Object.defineProperty(event, 'target', { value: input });
      document.dispatchEvent(event);

      expect(defaultProps.onSelect).not.toHaveBeenCalled();

      document.body.removeChild(input);
    });

    it('does not respond to shortcuts when in textarea', () => {
      render(<QuestionPrompt {...defaultProps} />);

      const textarea = document.createElement('textarea');
      document.body.appendChild(textarea);
      textarea.focus();

      const event = new KeyboardEvent('keydown', { key: '1', bubbles: true });
      Object.defineProperty(event, 'target', { value: textarea });
      document.dispatchEvent(event);

      expect(defaultProps.onSelect).not.toHaveBeenCalled();

      document.body.removeChild(textarea);
    });

    it('shows keyboard hint when pending', () => {
      render(<QuestionPrompt {...defaultProps} />);
      expect(screen.getByText('Press 1-3 to select')).toBeInTheDocument();
    });
  });

  describe('selected state', () => {
    it('applies selected class to clicked option', async () => {
      const user = userEvent.setup();
      const { container } = render(<QuestionPrompt {...defaultProps} />);

      await user.click(screen.getByText('Vue'));

      const selectedOption = container.querySelector('.app-question__option--selected');
      expect(selectedOption).toBeInTheDocument();
    });

    it('sets aria-selected on selected option', async () => {
      const user = userEvent.setup();
      render(<QuestionPrompt {...defaultProps} />);

      const buttons = screen.getAllByRole('option');
      await user.click(buttons[1]); // Click Vue

      expect(buttons[1]).toHaveAttribute('aria-selected', 'true');
      expect(buttons[0]).toHaveAttribute('aria-selected', 'false');
    });

    it('hides keyboard hint after selection', async () => {
      const user = userEvent.setup();
      render(<QuestionPrompt {...defaultProps} />);

      await user.click(screen.getByText('React'));

      expect(screen.queryByText('Press 1-3 to select')).not.toBeInTheDocument();
    });
  });

  describe('disabled state', () => {
    it('disables all option buttons when disabled', () => {
      render(<QuestionPrompt {...defaultProps} disabled />);

      const buttons = screen.getAllByRole('option');
      buttons.forEach((button) => {
        expect(button).toBeDisabled();
      });
    });

    it('does not call onSelect when disabled', async () => {
      const user = userEvent.setup();
      render(<QuestionPrompt {...defaultProps} disabled />);

      const buttons = screen.getAllByRole('option');
      await user.click(buttons[0]);

      expect(defaultProps.onSelect).not.toHaveBeenCalled();
    });

    it('does not respond to keyboard shortcuts when disabled', () => {
      render(<QuestionPrompt {...defaultProps} disabled />);

      fireEvent.keyDown(document, { key: '1' });

      expect(defaultProps.onSelect).not.toHaveBeenCalled();
    });

    it('hides keyboard hint when disabled', () => {
      render(<QuestionPrompt {...defaultProps} disabled />);
      expect(screen.queryByText('Press 1-3 to select')).not.toBeInTheDocument();
    });
  });

  it('has accessible listbox role', () => {
    render(<QuestionPrompt {...defaultProps} />);
    expect(screen.getByRole('listbox', { name: 'Answer options' })).toBeInTheDocument();
  });

  it('has accessible group role', () => {
    render(<QuestionPrompt {...defaultProps} />);
    expect(screen.getByRole('group')).toBeInTheDocument();
  });

  it('handles options without descriptions', () => {
    const props = {
      ...defaultProps,
      options: [
        { id: 'opt1', title: 'Option 1', description: '' },
        { id: 'opt2', title: 'Option 2', description: '' },
      ],
    };
    render(<QuestionPrompt {...props} />);

    expect(screen.getByText('Option 1')).toBeInTheDocument();
    expect(screen.getByText('Option 2')).toBeInTheDocument();
  });

  it('updates hint text based on option count', () => {
    const props = {
      ...defaultProps,
      options: [
        { id: 'opt1', title: 'Only Option', description: 'The only choice' },
      ],
    };
    render(<QuestionPrompt {...props} />);

    expect(screen.getByText('Press 1-1 to select')).toBeInTheDocument();
  });
});
