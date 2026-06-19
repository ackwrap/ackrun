type CardPadding = 'none' | 'sm' | 'md' | 'lg';

interface CardProps {
  title?: string;
  extra?: React.ReactNode;
  padding?: CardPadding;
  bordered?: boolean;
  hoverable?: boolean;
  loading?: boolean;
  className?: string;
  children: React.ReactNode;
}

const paddingClasses: Record<CardPadding, string> = {
  none: '',
  sm: 'p-3',
  md: 'p-5',
  lg: 'p-6',
};

export function Card({ title, extra, padding = 'md', bordered = false, hoverable = false, loading = false, className = '', children }: CardProps) {
  return (
    <div className={`bg-[var(--bg-surface)] rounded-[var(--radius-lg)] shadow-[var(--shadow-card)] backdrop-blur-xl border border-[var(--border-light)] ${bordered ? 'border-[var(--border-default)]' : ''} ${hoverable ? 'card-interactive cursor-pointer' : ''} ${paddingClasses[padding]} ${className}`}>
      {(title || extra) && (
        <div className="flex items-center justify-between mb-4">
          {title && <h3 className="text-base font-semibold text-[var(--text-primary)]">{title}</h3>}
          {extra && <div>{extra}</div>}
        </div>
      )}
      {loading ? (
        <div className="space-y-3 animate-pulse">
          <div className="h-4 bg-[var(--bg-base)] rounded w-3/4" />
          <div className="h-4 bg-[var(--bg-base)] rounded w-1/2" />
          <div className="h-4 bg-[var(--bg-base)] rounded w-2/3" />
        </div>
      ) : children}
    </div>
  );
}