import { forwardRef, type HTMLAttributes } from 'react';

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  padding?: 'sm' | 'md' | 'lg';
  interactive?: boolean;
  selected?: boolean;
}

export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ padding = 'md', interactive = false, selected = false, className = '', children, ...props }, ref) => {
    const classes = [
      'v2-card',
      `v2-card--padding-${padding}`,
      interactive ? 'v2-card--interactive' : '',
      selected ? 'v2-card--selected' : '',
      className,
    ].filter(Boolean).join(' ');

    return (
      <div
        ref={ref}
        className={classes}
        {...props}
      >
        {children}
      </div>
    );
  }
);

Card.displayName = 'Card';
