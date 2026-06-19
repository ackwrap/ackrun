import React from 'react';

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'link';
type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: React.ReactNode;
  loading?: boolean;
  fullWidth?: boolean;
}

const variantClasses: Record<ButtonVariant, string> = {
  primary: 'border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)] shadow-sm shadow-blue-500/10 hover:bg-[var(--button-primary-hover)] active:bg-[var(--button-primary-active)]',
  secondary: 'bg-white/[0.04] text-[var(--text-primary)] border border-[var(--border-default)] hover:bg-white/[0.08]',
  ghost: 'bg-transparent text-[var(--text-secondary)] hover:bg-white/[0.06] hover:text-[var(--text-primary)]',
  danger: 'bg-[var(--color-error)] text-white hover:opacity-90',
  link: 'bg-transparent text-[var(--color-primary)] hover:underline p-0 h-auto',
};

const sizeClasses: Record<ButtonSize, string> = {
  sm: 'h-[30px] px-3.5 text-xs rounded-[var(--radius-md)]',
  md: 'h-[36px] px-4 text-sm rounded-[var(--radius-lg)]',
  lg: 'h-[44px] px-6 text-base rounded-[var(--radius-lg)]',
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'secondary', size = 'md', icon, loading, fullWidth, disabled, className = '', children, ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={`inline-flex items-center justify-center gap-1.5 font-medium transition-colors duration-[var(--duration-fast)] ease-[var(--easing-default)] btn-press focus-ring disabled:opacity-50 disabled:cursor-not-allowed ${variantClasses[variant]} ${sizeClasses[size]} ${fullWidth ? 'w-full' : ''} ${className}`}
        disabled={disabled || loading}
        {...props}
      >
        {loading && <span className="inline-block w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />}
        {!loading && icon && <span className="inline-flex shrink-0">{icon}</span>}
        {children}
      </button>
    );
  },
);

Button.displayName = 'Button';
