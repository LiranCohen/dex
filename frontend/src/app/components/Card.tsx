import { forwardRef, type HTMLAttributes } from 'react';

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  padding?: 'sm' | 'md' | 'lg';
  interactive?: boolean;
  selected?: boolean;
}

export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ padding = 'md', interactive = false, selected = false, className = '', children, ...props }, ref) => {
    const classes = [
      'app-card',
      `app-card--padding-${padding}`,
      interactive ? 'app-card--interactive' : '',
      selected ? 'app-card--selected' : '',
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
