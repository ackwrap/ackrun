import { RefreshCw } from 'lucide-react';
import type { ProxyGroup, ProxyNode } from '@/services/clash';
import { monitorPanelBodyClass, monitorPanelClass, proxyGroupIcon } from './monitorUtils';

interface ProxyGroupsPanelProps {
  proxies: Record<string, ProxyGroup | ProxyNode>;
  proxyGroups: ProxyGroup[];
  selectedGroup: string | null;
  loading: boolean;
  onRefresh: () => void;
  onSelectGroup: (group: string | null) => void;
  onSelectProxy: (group: string, proxy: string) => void;
  onTestDelay: (proxyName: string) => void;
}

export function ProxyGroupsPanel({
  proxies,
  proxyGroups,
  selectedGroup,
  loading,
  onRefresh,
  onSelectGroup,
  onSelectProxy,
  onTestDelay,
}: ProxyGroupsPanelProps) {
  return (
    <div className="space-y-4">
      <div className={`${monitorPanelClass} flex items-center justify-between`}>
        <div>
          <h3 className="text-sm font-semibold text-[var(--text-primary)]">策略组列表</h3>
          <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">共 {proxyGroups.length} 个策略组</p>
        </div>
        <button
          onClick={onRefresh}
          disabled={loading}
          className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-[var(--text-primary)] hover:bg-white/[0.08] disabled:opacity-50"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          刷新
        </button>
      </div>

      {loading ? (
        <EmptyState text="加载中..." />
      ) : proxyGroups.length === 0 ? (
        <EmptyState text="暂无策略组" />
      ) : (
        <div className="space-y-3">
          {proxyGroups.map(group => (
            <ProxyGroupCard
              key={group.name}
              group={group}
              proxies={proxies}
              expanded={selectedGroup === group.name}
              onToggle={() => onSelectGroup(selectedGroup === group.name ? null : group.name)}
              onSelectProxy={onSelectProxy}
              onTestDelay={onTestDelay}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className={`${monitorPanelBodyClass} p-12 text-center`}>
      <div className="text-sm text-[var(--text-secondary)]">{text}</div>
    </div>
  );
}

interface ProxyGroupCardProps {
  group: ProxyGroup;
  proxies: Record<string, ProxyGroup | ProxyNode>;
  expanded: boolean;
  onToggle: () => void;
  onSelectProxy: (group: string, proxy: string) => void;
  onTestDelay: (proxyName: string) => void;
}

function ProxyGroupCard({ group, proxies, expanded, onToggle, onSelectProxy, onTestDelay }: ProxyGroupCardProps) {
  return (
    <div className={monitorPanelBodyClass}>
      <div className="cursor-pointer p-4" onClick={onToggle}>
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <div className="text-lg">{proxyGroupIcon(group)}</div>
            <div className="min-w-0">
              <div className="truncate font-semibold text-[var(--text-primary)]">{group.name}</div>
              <div className="mt-0.5 text-xs text-[var(--text-tertiary)]">
                {group.type === 'Selector' ? '手动选择' : '自动测速'} · {group.all?.length || 0} 个节点
              </div>
            </div>
          </div>
          <div className="shrink-0 text-right">
            <div className="max-w-[160px] truncate text-sm text-[var(--text-primary)]">{group.now || '无'}</div>
            {group.history?.[0]?.delay && <div className="mt-0.5 text-xs text-emerald-400">{group.history[0].delay}ms</div>}
          </div>
        </div>
      </div>

      {expanded && group.all && (
        <div className="border-t border-[var(--border-default)] p-4">
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {group.all.map(proxyName => {
              const proxy = proxies[proxyName] as ProxyNode | undefined;
              const isCurrent = group.now === proxyName;
              const delay = proxy?.history?.[0]?.delay;

              return (
                <button
                  key={proxyName}
                  onClick={() => group.type === 'Selector' && onSelectProxy(group.name, proxyName)}
                  onContextMenu={event => {
                    event.preventDefault();
                    onTestDelay(proxyName);
                  }}
                  disabled={group.type !== 'Selector'}
                  className={`flex items-center justify-between rounded-lg border p-3 text-left text-sm transition-colors ${
                    isCurrent
                      ? 'border-blue-400/50 bg-blue-500/20 text-blue-100'
                      : 'border-[var(--border-default)] bg-white/[0.02] text-[var(--text-secondary)] hover:bg-white/[0.04] hover:text-white'
                  } ${group.type !== 'Selector' ? 'cursor-default' : 'cursor-pointer'}`}
                >
                  <span className="truncate">{proxyName}</span>
                  {delay !== undefined && (
                    <span className={`ml-2 text-xs ${delay < 100 ? 'text-emerald-400' : delay < 300 ? 'text-yellow-400' : 'text-red-400'}`}>{delay}ms</span>
                  )}
                </button>
              );
            })}
          </div>
          <div className="mt-3 text-xs text-[var(--text-tertiary)]">
            {group.type === 'Selector' ? '点击切换节点' : '自动测速中'} · 右键节点进行测速
          </div>
        </div>
      )}
    </div>
  );
}
