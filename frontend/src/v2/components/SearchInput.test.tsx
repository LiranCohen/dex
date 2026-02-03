import { describe, it, expect, vi } from 'vitest';
import { createRef } from 'react';
import { render, screen, fireEvent } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';
import { SearchInput, type SearchInputRef } from './SearchInput';

describe('SearchInput', () => {
  it('renders with placeholder', () => {
    render(<SearchInput value="" onChange={() => {}} />);
    expect(screen.getByPlaceholderText('Search...')).toBeInTheDocument();
  });

  it('renders with custom placeholder', () => {
    render(<SearchInput value="" onChange={() => {}} placeholder="Find quests..." />);
    expect(screen.getByPlaceholderText('Find quests...')).toBeInTheDocument();
  });

  it('displays the current value', () => {
    render(<SearchInput value="test query" onChange={() => {}} />);
    expect(screen.getByDisplayValue('test query')).toBeInTheDocument();
  });

  it('calls onChange when user types', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<SearchInput value="" onChange={handleChange} />);

    const input = screen.getByRole('textbox');
    await user.type(input, 'hello');

    expect(handleChange).toHaveBeenCalledWith('h');
    expect(handleChange).toHaveBeenCalledWith('e');
    expect(handleChange).toHaveBeenCalledWith('l');
    expect(handleChange).toHaveBeenCalledWith('l');
    expect(handleChange).toHaveBeenCalledWith('o');
  });

  it('auto-focuses when autoFocus is true', () => {
    render(<SearchInput value="" onChange={() => {}} autoFocus />);
    expect(screen.getByRole('textbox')).toHaveFocus();
  });

  it('does not auto-focus by default', () => {
    render(<SearchInput value="" onChange={() => {}} />);
    expect(screen.getByRole('textbox')).not.toHaveFocus();
  });

  it('shows clear button when value is not empty', () => {
    render(<SearchInput value="query" onChange={() => {}} />);
    expect(screen.getByLabelText('Clear search')).toBeInTheDocument();
  });

  it('hides clear button when value is empty', () => {
    render(<SearchInput value="" onChange={() => {}} />);
    expect(screen.queryByLabelText('Clear search')).not.toBeInTheDocument();
  });

  it('clears value and focuses input when clear button clicked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<SearchInput value="query" onChange={handleChange} />);

    await user.click(screen.getByLabelText('Clear search'));
    expect(handleChange).toHaveBeenCalledWith('');
  });

  it('clears value on Escape when input has content', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const handleEscape = vi.fn();
    render(<SearchInput value="query" onChange={handleChange} onEscape={handleEscape} />);

    const input = screen.getByRole('textbox');
    await user.click(input);
    await user.keyboard('{Escape}');

    expect(handleChange).toHaveBeenCalledWith('');
    expect(handleEscape).not.toHaveBeenCalled();
  });

  it('blurs and calls onEscape on Escape when input is empty', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const handleEscape = vi.fn();
    render(<SearchInput value="" onChange={handleChange} onEscape={handleEscape} />);

    const input = screen.getByRole('textbox');
    await user.click(input);
    await user.keyboard('{Escape}');

    expect(handleEscape).toHaveBeenCalledTimes(1);
    expect(input).not.toHaveFocus();
  });

  it('applies focused class when focused', async () => {
    const user = userEvent.setup();
    const { container } = render(<SearchInput value="" onChange={() => {}} />);

    const input = screen.getByRole('textbox');
    await user.click(input);

    const wrapper = container.querySelector('.v2-search');
    expect(wrapper).toHaveClass('v2-search--focused');
  });

  it('removes focused class when blurred', async () => {
    const { container } = render(<SearchInput value="" onChange={() => {}} />);

    const input = screen.getByRole('textbox');
    fireEvent.focus(input);
    fireEvent.blur(input);

    const wrapper = container.querySelector('.v2-search');
    expect(wrapper).not.toHaveClass('v2-search--focused');
  });

  it('applies custom className', () => {
    const { container } = render(
      <SearchInput value="" onChange={() => {}} className="custom-search" />
    );
    const wrapper = container.querySelector('.v2-search');
    expect(wrapper).toHaveClass('custom-search');
  });

  it('has accessible label from placeholder', () => {
    render(<SearchInput value="" onChange={() => {}} placeholder="Search objectives" />);
    expect(screen.getByLabelText('Search objectives')).toBeInTheDocument();
  });

  describe('ref methods', () => {
    it('focus method focuses the input', () => {
      const ref = createRef<SearchInputRef>();
      render(<SearchInput ref={ref} value="" onChange={() => {}} />);

      ref.current?.focus();
      expect(screen.getByRole('textbox')).toHaveFocus();
    });

    it('blur method blurs the input', () => {
      const ref = createRef<SearchInputRef>();
      render(<SearchInput ref={ref} value="" onChange={() => {}} autoFocus />);

      expect(screen.getByRole('textbox')).toHaveFocus();
      ref.current?.blur();
      expect(screen.getByRole('textbox')).not.toHaveFocus();
    });
  });
});
