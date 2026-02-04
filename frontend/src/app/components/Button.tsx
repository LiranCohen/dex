import { forwardRef, type ButtonHTMLAttributes } from 'react';

type ButtonVariant = 'primary' | 'secondary' | 'ghost';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  loading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'primary', loading = false, className = '', disabled, children, ...props }, ref) => {
    const classes = [
      'app-btn',
      `app-btn--${variant}`,
      loading ? 'app-btn--loading' : '',
      className,
    ].filter(Boolean).join(' ');

    return (
      <button
        ref={ref}
        disabled={disabled || loading}
        className={classes}
        aria-busy={loading}
        {...props}
      >
        {loading ? (
          <span className="app-btn__spinner" aria-hidden="true" />
        ) : null}
        <span className={loading ? 'app-sr-only' : ''}>{children}</span>
      </button>
    );
  }
);

Button.displayName = 'Button';
