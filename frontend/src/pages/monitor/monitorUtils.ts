import type { ProxyGroup } from '@/services/clash';

export type MonitorTab = 'overview' | 'proxies' | 'connections' | 'rules';

export const monitorPanelClass = 'rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)]';
export const monitorPanelBodyClass = 'rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]';

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
}

export function formatSpeed(bytesPerSecond: number): string {
  return formatBytes(bytesPerSecond) + '/s';
}

export function proxyGroupIcon(group: ProxyGroup): string {
  if (group.name === '全球直连') return '🎯';
  if (group.name === '应用净化') return '🍃';
  return group.type === 'Selector' ? '👆' : '🚀';
}
