import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '../../test/test-utils';
import { Icon } from './Icon';

describe('Icon', () => {
  it('renders an SVG element', () => {
    const { container } = render(<Icon name="check" />);
    const svg = container.querySelector('svg');
    expect(svg).toBeInTheDocument();
  });

  it('renders check icon', () => {
    const { container } = render(<Icon name="check" />);
    const path = container.querySelector('path');
    expect(path).toHaveAttribute('d', 'M5 12l5 5L20 7');
  });

  it('renders x icon', () => {
    const { container } = render(<Icon name="x" />);
    const path = container.querySelector('path');
    expect(path).toHaveAttribute('d', 'M6 18L18 6M6 6l12 12');
  });

  it('applies small size', () => {
    const { container } = render(<Icon name="check" size="sm" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('width', '16');
    expect(svg).toHaveAttribute('height', '16');
    expect(svg).toHaveClass('app-icon--sm');
  });

  it('applies medium size by default', () => {
    const { container } = render(<Icon name="check" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('width', '20');
    expect(svg).toHaveAttribute('height', '20');
    expect(svg).toHaveClass('app-icon--md');
  });

  it('applies large size', () => {
    const { container } = render(<Icon name="check" size="lg" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('width', '24');
    expect(svg).toHaveAttribute('height', '24');
    expect(svg).toHaveClass('app-icon--lg');
  });

  it('sets aria-hidden when no aria-label provided', () => {
    const { container } = render(<Icon name="check" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('aria-hidden', 'true');
    expect(svg).not.toHaveAttribute('role');
  });

  it('sets aria-label and role when aria-label provided', () => {
    const { container } = render(<Icon name="check" aria-label="Checkmark" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('aria-hidden', 'false');
    expect(svg).toHaveAttribute('aria-label', 'Checkmark');
    expect(svg).toHaveAttribute('role', 'img');
  });

  it('can be found by aria-label', () => {
    render(<Icon name="check" aria-label="Success indicator" />);
    expect(screen.getByLabelText('Success indicator')).toBeInTheDocument();
  });

  it('applies spinning class to spinner icon', () => {
    const { container } = render(<Icon name="spinner" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveClass('app-icon--spinning');
  });

  it('does not apply spinning class to non-spinner icons', () => {
    const { container } = render(<Icon name="check" />);
    const svg = container.querySelector('svg');
    expect(svg).not.toHaveClass('app-icon--spinning');
  });

  it('applies custom className', () => {
    const { container } = render(<Icon name="check" className="custom-icon" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveClass('custom-icon');
    expect(svg).toHaveClass('app-icon');
  });

  it('returns null for unknown icon names', () => {
    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { container } = render(<Icon name={'unknown' as 'check'} />);
    expect(container.querySelector('svg')).toBeNull();
    expect(consoleSpy).toHaveBeenCalledWith('Icon "unknown" not found');
    consoleSpy.mockRestore();
  });

  it('renders various icon types', () => {
    const icons = ['arrow-left', 'arrow-right', 'plus', 'minus', 'search', 'home', 'inbox'] as const;

    icons.forEach((iconName) => {
      const { container } = render(<Icon name={iconName} />);
      const svg = container.querySelector('svg');
      expect(svg).toBeInTheDocument();
      const path = container.querySelector('path');
      expect(path).toHaveAttribute('d');
    });
  });

  it('has proper SVG attributes', () => {
    const { container } = render(<Icon name="check" />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('viewBox', '0 0 24 24');
    expect(svg).toHaveAttribute('fill', 'none');
    expect(svg).toHaveAttribute('stroke', 'currentColor');
    expect(svg).toHaveAttribute('stroke-width', '2');
    expect(svg).toHaveAttribute('stroke-linecap', 'round');
    expect(svg).toHaveAttribute('stroke-linejoin', 'round');
  });
});
