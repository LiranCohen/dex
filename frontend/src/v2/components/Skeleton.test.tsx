import { describe, it, expect } from 'vitest';
import { render, screen } from '../../test/test-utils';
import { Skeleton, SkeletonCard, SkeletonList, SkeletonMessage } from './Skeleton';

describe('Skeleton', () => {
  it('renders with text variant by default', () => {
    const { container } = render(<Skeleton />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveClass('v2-skeleton--text');
  });

  it('renders with title variant', () => {
    const { container } = render(<Skeleton variant="title" />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveClass('v2-skeleton--title');
  });

  it('renders with card variant', () => {
    const { container } = render(<Skeleton variant="card" />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveClass('v2-skeleton--card');
  });

  it('renders with avatar variant', () => {
    const { container } = render(<Skeleton variant="avatar" />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveClass('v2-skeleton--avatar');
  });

  it('renders with button variant', () => {
    const { container } = render(<Skeleton variant="button" />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveClass('v2-skeleton--button');
  });

  it('applies custom width', () => {
    const { container } = render(<Skeleton width="200px" />);
    const skeleton = container.querySelector('.v2-skeleton') as HTMLElement;
    expect(skeleton.style.width).toBe('200px');
  });

  it('applies custom height', () => {
    const { container } = render(<Skeleton height="50px" />);
    const skeleton = container.querySelector('.v2-skeleton') as HTMLElement;
    expect(skeleton.style.height).toBe('50px');
  });

  it('applies custom className', () => {
    const { container } = render(<Skeleton className="custom-skeleton" />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveClass('custom-skeleton');
  });

  it('is aria-hidden', () => {
    const { container } = render(<Skeleton />);
    const skeleton = container.querySelector('.v2-skeleton');
    expect(skeleton).toHaveAttribute('aria-hidden', 'true');
  });
});

describe('SkeletonCard', () => {
  it('renders a skeleton card structure', () => {
    const { container } = render(<SkeletonCard />);
    const card = container.querySelector('.v2-skeleton-card');
    expect(card).toBeInTheDocument();
    expect(card).toHaveClass('v2-card');
  });

  it('contains multiple skeleton lines', () => {
    const { container } = render(<SkeletonCard />);
    const skeletons = container.querySelectorAll('.v2-skeleton');
    expect(skeletons.length).toBe(3);
  });

  it('is aria-hidden', () => {
    const { container } = render(<SkeletonCard />);
    const card = container.querySelector('.v2-skeleton-card');
    expect(card).toHaveAttribute('aria-hidden', 'true');
  });
});

describe('SkeletonList', () => {
  it('renders 3 skeleton cards by default', () => {
    const { container } = render(<SkeletonList />);
    const cards = container.querySelectorAll('.v2-skeleton-card');
    expect(cards.length).toBe(3);
  });

  it('renders specified count of skeleton cards', () => {
    const { container } = render(<SkeletonList count={5} />);
    const cards = container.querySelectorAll('.v2-skeleton-card');
    expect(cards.length).toBe(5);
  });

  it('renders single skeleton card when count is 1', () => {
    const { container } = render(<SkeletonList count={1} />);
    const cards = container.querySelectorAll('.v2-skeleton-card');
    expect(cards.length).toBe(1);
  });

  it('has loading role and aria-label', () => {
    render(<SkeletonList />);
    const list = screen.getByRole('status');
    expect(list).toHaveAttribute('aria-label', 'Loading...');
  });

  it('has skeleton list class', () => {
    const { container } = render(<SkeletonList />);
    const list = container.querySelector('.v2-skeleton-list');
    expect(list).toBeInTheDocument();
  });
});

describe('SkeletonMessage', () => {
  it('renders a skeleton message structure', () => {
    const { container } = render(<SkeletonMessage />);
    const message = container.querySelector('.v2-skeleton-message');
    expect(message).toBeInTheDocument();
    expect(message).toHaveClass('v2-message');
  });

  it('contains header skeletons', () => {
    const { container } = render(<SkeletonMessage />);
    const header = container.querySelector('.v2-message__header');
    expect(header).toBeInTheDocument();
    const headerSkeletons = header?.querySelectorAll('.v2-skeleton');
    expect(headerSkeletons?.length).toBe(2);
  });

  it('contains content skeletons', () => {
    const { container } = render(<SkeletonMessage />);
    const content = container.querySelector('.v2-message__content');
    expect(content).toBeInTheDocument();
    const contentSkeletons = content?.querySelectorAll('.v2-skeleton');
    expect(contentSkeletons?.length).toBe(3);
  });

  it('is aria-hidden', () => {
    const { container } = render(<SkeletonMessage />);
    const message = container.querySelector('.v2-skeleton-message');
    expect(message).toHaveAttribute('aria-hidden', 'true');
  });
});
