import React from 'react';
import { ChevronDown, Gauge, RefreshCw, Zap } from 'lucide-react';
import type { ProxyGroup, ProxyNode } from '@/services/clash';
import { monitorPanelBodyClass } from './monitorUtils';
import { ProxyGroupIcon } from './ProxyGroupIcon';

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
  const [filter, setFilter] = React.useState<'all' | 'selector' | 'automatic'>('all');
  const selectorCount = proxyGroups.filter(group => group.type === 'Selector').length;
  const automaticCount = proxyGroups.length - selectorCount;
  const filteredGroups = proxyGroups.filter(group => (
    filter === 'all' || (filter === 'selector' ? group.type === 'Selector' : group.type !== 'Selector')
  ));
  const desktopColumns = [
    filteredGroups.filter((_, index) => index % 2 === 0),
    filteredGroups.filter((_, index) => index % 2 === 1),
  ];
  const changeFilter = (nextFilter: 'all' | 'selector' | 'automatic') => {
    setFilter(nextFilter);
    onSelectGroup(null);
  };

  return (
    <div className="space-y-3 pb-4">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-2 shadow-[var(--shadow-card)]">
        <div className="flex flex-wrap items-center gap-1">
          <SummaryChip label="全部" count={proxyGroups.length} active={filter === 'all'} onClick={() => changeFilter('all')} />
          <SummaryChip label="手动策略" count={selectorCount} active={filter === 'selector'} onClick={() => changeFilter('selector')} />
          <SummaryChip label="自动选择" count={automaticCount} active={filter === 'automatic'} onClick={() => changeFilter('automatic')} />
        </div>
        <button
          type="button"
          onClick={onRefresh}
          disabled={loading}
          className="inline-flex h-8 items-center gap-2 rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] px-3 text-xs font-medium text-[var(--text-secondary)] transition hover:border-[var(--color-primary)] hover:text-[var(--text-primary)] disabled:opacity-50"
        >
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          刷新
        </button>
      </div>

      {loading ? (
        <EmptyState text="加载中..." />
      ) : proxyGroups.length === 0 ? (
        <EmptyState text="暂无策略组" />
      ) : filteredGroups.length === 0 ? (
        <EmptyState text="当前分类暂无策略组" />
      ) : (
        <>
          <div className="space-y-3 lg:hidden">
            {filteredGroups.map(group => (
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
          <div className="hidden items-start gap-3 lg:grid lg:grid-cols-2">
            {desktopColumns.map((groups, columnIndex) => (
              <div key={columnIndex} className="space-y-3">
                {groups.map(group => (
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
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function SummaryChip({ label, count, active = false, onClick }: { label: string; count: number; active?: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className={`inline-flex h-8 items-center gap-2 rounded-lg px-3 text-xs font-medium transition ${active ? 'bg-[var(--color-primary-bg)] text-[var(--color-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--text-primary)]'}`}>
      {label}<b className="font-semibold tabular-nums opacity-70">{count}</b>
    </button>
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
  const members = group.all || [];
  const delays = members.map(name => latestDelay(proxies[name]));
  const knownCount = delays.filter(delay => delay > 0).length;
  const currentDelay = latestDelay(proxies[group.now]);
  const distribution = delayDistribution(delays);

  return (
    <section className={`overflow-hidden rounded-[var(--radius-xl)] border bg-[var(--bg-surface)] shadow-[var(--shadow-card)] transition ${expanded ? 'border-[var(--color-primary)]' : 'border-[var(--border-default)] hover:border-[var(--border-strong)]'}`}>
      <button type="button" onClick={onToggle} className="block w-full p-4 text-left">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-center gap-2.5">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-base)]"><ProxyGroupIcon group={group} className="h-5 w-5" /></span>
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
                <h3 className="truncate text-sm font-semibold text-[var(--text-primary)]">{group.name}</h3>
                <span className="text-[9px] font-medium uppercase tracking-[0.12em] text-[var(--text-tertiary)]">{group.type}</span>
                <span className="text-[10px] tabular-nums text-[var(--text-tertiary)]">{knownCount}/{members.length}</span>
              </div>
              <div className="mt-2 flex min-w-0 items-center gap-2 text-xs">
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-primary)]" />
                <span className="truncate font-medium text-[var(--text-secondary)]">{group.now || '未选择节点'}</span>
              </div>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {currentDelay > 0 && <DelayBadge delay={currentDelay} />}
            <ChevronDown size={15} className={`text-[var(--text-tertiary)] transition-transform ${expanded ? 'rotate-180' : ''}`} />
          </div>
        </div>

        <LatencyBar distribution={distribution} total={members.length} />
      </button>

      {expanded && (
        <div className="border-t border-[var(--border-light)] bg-[var(--bg-base)]/45 p-3 sm:p-4">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-[var(--text-tertiary)]">
            <span>{group.type === 'Selector' ? '点击节点切换策略' : '自动测速策略，仅展示状态'}</span>
            <span>测速结果 {knownCount}/{members.length}</span>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 2xl:grid-cols-3">
            {members.map(proxyName => {
              const isCurrent = group.now === proxyName;
              const delay = latestDelay(proxies[proxyName]);
              return (
                <div key={proxyName} className={`flex min-w-0 items-center rounded-lg border transition ${isCurrent ? 'border-[var(--color-primary)] bg-[var(--color-primary-bg)]' : 'border-[var(--border-light)] bg-[var(--bg-surface)] hover:border-[var(--border-strong)]'}`}>
                  <button
                    type="button"
                    onClick={() => group.type === 'Selector' && onSelectProxy(group.name, proxyName)}
                    disabled={group.type !== 'Selector'}
                    className={`min-w-0 flex-1 px-3 py-2.5 text-left ${group.type === 'Selector' ? 'cursor-pointer' : 'cursor-default'}`}
                  >
                    <span className={`block truncate text-xs font-medium ${isCurrent ? 'text-[var(--color-primary)]' : 'text-[var(--text-primary)]'}`}>{proxyName}</span>
                    <span className="mt-1 block text-[10px] uppercase tracking-[0.08em] text-[var(--text-tertiary)]">{proxyType(proxies[proxyName])}</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => onTestDelay(proxyName)}
                    title="测试延迟"
                    className="mr-2 flex h-7 shrink-0 items-center gap-1 rounded-md bg-[var(--bg-base)] px-2 text-[10px] tabular-nums text-[var(--text-tertiary)] hover:text-[var(--color-primary)]"
                  >
                    {delay > 0 ? <span className={delayColor(delay)}>{delay}</span> : <Zap size={11} />}
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </section>
  );
}

function LatencyBar({ distribution, total }: { distribution: DelayDistribution; total: number }) {
  if (total === 0) return <div className="mt-4 h-1.5 rounded-full bg-[var(--border-light)]" />;
  return (
    <div className="mt-4 flex h-1.5 overflow-hidden rounded-full bg-[var(--border-light)]" aria-label={`延迟分布：快速 ${distribution.fast}，正常 ${distribution.medium}，较慢 ${distribution.slow}，未知 ${distribution.unknown}`}>
      {distribution.fast > 0 && <span className="bg-emerald-500" style={{ flex: distribution.fast }} />}
      {distribution.medium > 0 && <span className="bg-amber-400" style={{ flex: distribution.medium }} />}
      {distribution.slow > 0 && <span className="bg-rose-400" style={{ flex: distribution.slow }} />}
      {distribution.unknown > 0 && <span className="bg-[var(--text-tertiary)] opacity-55" style={{ flex: distribution.unknown }} />}
    </div>
  );
}

function DelayBadge({ delay }: { delay: number }) {
  return (
    <span className={`inline-flex h-7 items-center gap-1 rounded-full bg-[var(--bg-base)] px-2.5 text-[11px] font-semibold tabular-nums ${delayColor(delay)}`}>
      <Gauge size={11} />{delay}
    </span>
  );
}

interface DelayDistribution {
  fast: number;
  medium: number;
  slow: number;
  unknown: number;
}

function delayDistribution(delays: number[]): DelayDistribution {
  return delays.reduce<DelayDistribution>((result, delay) => {
    if (delay <= 0) result.unknown += 1;
    else if (delay < 200) result.fast += 1;
    else if (delay < 800) result.medium += 1;
    else result.slow += 1;
    return result;
  }, { fast: 0, medium: 0, slow: 0, unknown: 0 });
}

function latestDelay(proxy: ProxyGroup | ProxyNode | undefined) {
  const history = proxy?.history;
  return Number(history?.[history.length - 1]?.delay || 0);
}

function proxyType(proxy: ProxyGroup | ProxyNode | undefined) {
  return proxy?.type || 'proxy';
}

function delayColor(delay: number) {
  if (delay < 200) return 'text-emerald-500';
  if (delay < 800) return 'text-amber-500';
  return 'text-rose-500';
}
