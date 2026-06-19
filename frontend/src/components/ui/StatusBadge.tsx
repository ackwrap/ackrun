type StatusType = 'running' | 'stopped' | 'error' | 'pending' | 'online' | 'offline';

interface StatusBadgeProps {
  status: StatusType;
  label?: string;
  pulse?: boolean;
  size?: 'sm' | 'md';
  className?: string;
}

const statusStyles: Record<StatusType, { dot: string; glow?: string }> = {
  running: { dot: 'bg-[var(--color-success)]', glow: '0 0 8px rgba(82,196,26,0.6)' },
  stopped: { dot: 'bg-[var(--text-disabled)]' },
  error: { dot: 'bg-[var(--color-error)]', glow: '0 0 8px rgba(255,77,79,0.5)' },
  pending: { dot: 'bg-[var(--color-warning)]', glow: '0 0 8px rgba(250,173,20,0.5)' },
  online: { dot: 'bg-[var(--color-success)]', glow: '0 0 8px rgba(82,196,26,0.6)' },
  offline: { dot: 'bg-[var(--text-disabled)]' },
};

const dotSizes = { sm: 'w-1.5 h-1.5', md: 'w-2 h-2' };

export function StatusBadge({ status, label, pulse, size = 'md', className = '' }: StatusBadgeProps) {
  const style = statusStyles[status];
  const shouldPulse = pulse ?? (status === 'running' || status === 'online');
  return (
    <span className={`inline-flex items-center gap-1.5 ${className}`}>
      <span className={`inline-block rounded-full ${style.dot} ${dotSizes[size]} ${shouldPulse ? 'animate-status-pulse' : ''}`} style={style.glow ? { boxShadow: style.glow } : undefined} />
      {label && <span className="text-sm text-[var(--text-secondary)]">{label}</span>}
    </span>
  );
}