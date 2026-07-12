import React from 'react';
import {
  Activity,
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Boxes,
  CircleGauge,
  Cpu,
  GitBranch,
  Radio,
  Route,
  WifiOff,
} from 'lucide-react';
import { MiniSparkline } from '@/components/monitor/MiniSparkline';
import { TrafficChart, type TrafficChartRef } from '@/components/monitor/TrafficChart';
import type { Connection, ProxyGroup } from '@/services/clash';
import { formatBytes, formatSpeed, monitorPanelClass } from './monitorUtils';
import { ProxyGroupIcon } from './ProxyGroupIcon';

interface OverviewPanelProps {
  connected: boolean;
  unavailableReason: string;
  totalUp: number;
  totalDown: number;
  speedUp: number;
  speedDown: number;
  memory: number;
  connectionCount: number;
  connections: Connection[];
  proxyGroups: ProxyGroup[];
  uploadSpeedHistory: number[];
  downloadSpeedHistory: number[];
  connectionCountHistory: number[];
  memoryHistory: number[];
  chartRef: React.RefObject<TrafficChartRef>;
  onOpenConnections: () => void;
  onOpenProxies: () => void;
}

const panelClass = 'overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]';

export function OverviewPanel({
  connected,
  unavailableReason,
  totalUp,
  totalDown,
  speedUp,
  speedDown,
  memory,
  connectionCount,
  connections,
  proxyGroups,
  uploadSpeedHistory,
  downloadSpeedHistory,
  connectionCountHistory,
  memoryHistory,
  chartRef,
  onOpenConnections,
  onOpenProxies,
}: OverviewPanelProps) {
  const sampleCount = Math.max(uploadSpeedHistory.length, downloadSpeedHistory.length);
  const connectionTrend = connectionCountHistory.length > 1
    ? connectionCount - connectionCountHistory[connectionCountHistory.length - 2]
    : 0;
  const recentConnections = [...connections]
    .sort((left, right) => right.upload + right.download - left.upload - left.download)
    .slice(0, 5);
  const outboundUsage = aggregateOutboundUsage(connections).slice(0, 5);
  const maxOutboundCount = Math.max(1, ...outboundUsage.map(item => item.count));

  return (
    <div className="space-y-4 pb-5">
      {!connected && (
        <section className={`${monitorPanelClass} flex flex-col gap-4 p-4 sm:flex-row sm:items-center`}>
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-error-bg)] text-[var(--color-error)]">
            <WifiOff size={18} />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold text-[var(--text-primary)]">无法连接实时监控</h2>
            <p className="mt-1 text-xs text-[var(--text-secondary)]">请确认 sing-box 已启动，且当前配置已启用 Clash API。</p>
            {unavailableReason && <p className="mt-1 break-words text-xs text-[var(--color-error)]">{unavailableReason}</p>}
          </div>
          <a href="/control" className="inline-flex h-9 shrink-0 items-center justify-center rounded-lg border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-4 text-sm font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">
            检查核心状态
          </a>
        </section>
      )}

      <section className={`${panelClass} relative`}>
        <div className="pointer-events-none absolute inset-x-0 top-0 h-24 bg-[linear-gradient(90deg,var(--color-primary-bg),transparent_55%,var(--color-success-bg))] opacity-70" />
        <header className="relative flex flex-col gap-3 border-b border-[var(--border-light)] px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5">
          <div className="flex items-center gap-3">
            <span className={`relative flex h-10 w-10 items-center justify-center rounded-xl ${connected ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]' : 'bg-[var(--color-error-bg)] text-[var(--color-error)]'}`}>
              <Radio size={18} />
              {connected && <span className="absolute right-1 top-1 h-2 w-2 animate-pulse rounded-full bg-[var(--color-success)]" />}
            </span>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-sm font-semibold text-[var(--text-primary)]">网络脉冲</h2>
                <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${connected ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]' : 'bg-[var(--color-error-bg)] text-[var(--color-error)]'}`}>
                  {connected ? 'LIVE' : 'OFFLINE'}
                </span>
              </div>
              <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">最近 60 秒实时状态 · {sampleCount} 个采样点</p>
            </div>
          </div>
          <div className="text-xs text-[var(--text-tertiary)]">累计流量 {formatBytes(totalUp + totalDown)}</div>
        </header>

        <div className="relative grid gap-px bg-[var(--border-light)] sm:grid-cols-2 xl:grid-cols-4">
          <MetricCard icon={<ArrowDown size={15} />} label="实时下载" value={formatSpeed(speedDown)} detail={`累计 ${formatBytes(totalDown)}`} history={downloadSpeedHistory} color="blue" />
          <MetricCard icon={<ArrowUp size={15} />} label="实时上传" value={formatSpeed(speedUp)} detail={`累计 ${formatBytes(totalUp)}`} history={uploadSpeedHistory} color="green" />
          <MetricCard icon={<GitBranch size={15} />} label="活动连接" value={String(connectionCount)} detail={connectionTrend === 0 ? '连接数稳定' : `${connectionTrend > 0 ? '+' : ''}${connectionTrend} 较上次`} history={connectionCountHistory} color="purple" />
          <MetricCard icon={<Cpu size={15} />} label="内存占用" value={memory > 0 ? formatBytes(memory) : '--'} detail="sing-box 运行内存" history={memoryHistory} color="purple" />
        </div>
      </section>

      <div className="grid gap-4 xl:grid-cols-12">
        <section className={`${panelClass} xl:col-span-8`}>
          <PanelHeader icon={<Activity size={15} />} title="实时吞吐" detail="下载与上传速率曲线" />
          <div className="p-3 sm:p-4">
            {connected ? (
              <TrafficChart ref={chartRef} className="h-[260px] sm:h-[300px]" />
            ) : (
              <EmptyPanel className="h-[260px] sm:h-[300px]" text="等待实时流量数据" />
            )}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-light)] px-4 py-3 text-xs text-[var(--text-tertiary)]">
            <div className="flex items-center gap-4">
              <span className="flex items-center gap-1.5"><i className="h-2 w-2 rounded-full bg-blue-400" />下载</span>
              <span className="flex items-center gap-1.5"><i className="h-2 w-2 rounded-full bg-emerald-400" />上传</span>
            </div>
            <span>1 秒采样 · 60 秒窗口</span>
          </div>
        </section>

        <section className={`${panelClass} xl:col-span-4`}>
          <PanelHeader icon={<CircleGauge size={15} />} title="会话健康" detail="当前连接分布" />
          <div className="space-y-5 p-4">
            <HealthGauge value={connectionCount} connected={connected} />
            <div className="grid grid-cols-2 gap-2">
              <HealthStat label="TCP" value={String(connections.filter(item => item.metadata.network?.toLowerCase() === 'tcp').length)} />
              <HealthStat label="UDP" value={String(connections.filter(item => item.metadata.network?.toLowerCase() === 'udp').length)} />
              <HealthStat label="策略链" value={String(new Set(connections.flatMap(item => item.chains || [])).size)} />
              <HealthStat label="内存" value={memory > 0 ? formatBytes(memory) : '--'} />
            </div>
          </div>
        </section>
      </div>

      <div className="grid gap-4 xl:grid-cols-12">
        <section className={`${panelClass} xl:col-span-7`}>
          <PanelHeader icon={<Route size={15} />} title="活动连接流向" detail="按当前流量排序" action="查看全部" onAction={onOpenConnections} />
          {recentConnections.length === 0 ? (
            <EmptyPanel className="h-52" text="暂无活动连接" />
          ) : (
            <div className="divide-y divide-[var(--border-light)]">
              {recentConnections.map(connection => (
                <ConnectionFlowRow key={connection.id} connection={connection} />
              ))}
            </div>
          )}
        </section>

        <section className={`${panelClass} xl:col-span-5`}>
          <PanelHeader icon={<Boxes size={15} />} title="策略组快照" detail={`${proxyGroups.length} 个活动策略组`} action="管理策略" onAction={onOpenProxies} />
          <div className="grid gap-4 p-4 lg:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
            <div className="space-y-2">
              <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--text-tertiary)]">当前选择</div>
              {proxyGroups.length === 0 ? (
                <div className="rounded-lg border border-dashed border-[var(--border-default)] p-4 text-center text-xs text-[var(--text-tertiary)]">暂无策略组</div>
              ) : proxyGroups.slice(0, 4).map(group => (
                <ProxySnapshotRow key={group.name} group={group} />
              ))}
            </div>
            <div className="space-y-2">
              <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--text-tertiary)]">连接出口</div>
              {outboundUsage.length === 0 ? (
                <div className="rounded-lg border border-dashed border-[var(--border-default)] p-4 text-center text-xs text-[var(--text-tertiary)]">暂无出口数据</div>
              ) : outboundUsage.map(item => (
                <div key={item.name} className="rounded-lg bg-[var(--bg-base)] px-3 py-2.5">
                  <div className="flex items-center justify-between gap-3 text-xs">
                    <span className="min-w-0 truncate font-medium text-[var(--text-secondary)]">{item.name}</span>
                    <span className="shrink-0 tabular-nums text-[var(--text-tertiary)]">{item.count}</span>
                  </div>
                  <div className="mt-2 h-1 overflow-hidden rounded-full bg-[var(--border-light)]">
                    <div className="h-full rounded-full bg-[var(--color-primary)]" style={{ width: `${Math.max(8, item.count / maxOutboundCount * 100)}%` }} />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

function MetricCard({ icon, label, value, detail, history, color }: { icon: React.ReactNode; label: string; value: string; detail: string; history: number[]; color: 'green' | 'blue' | 'purple' }) {
  return (
    <div className="relative min-h-36 overflow-hidden bg-[var(--bg-surface)] p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="flex items-center gap-2 text-xs font-medium text-[var(--text-secondary)]">{icon}{label}</span>
        <span className="text-[9px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">Live</span>
      </div>
      <div className="mt-3 text-2xl font-semibold tracking-[-0.03em] tabular-nums text-[var(--text-primary)]">{value}</div>
      <div className="mt-1 text-xs text-[var(--text-tertiary)]">{detail}</div>
      <MiniSparkline data={history} color={color} className="absolute inset-x-0 bottom-0 opacity-70" />
    </div>
  );
}

function PanelHeader({ icon, title, detail, action, onAction }: { icon: React.ReactNode; title: string; detail: string; action?: string; onAction?: () => void }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-[var(--border-light)] px-4 py-3.5">
      <div className="flex min-w-0 items-center gap-2.5">
        <span className="text-[var(--color-primary)]">{icon}</span>
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-[var(--text-primary)]">{title}</h3>
          <p className="truncate text-[11px] text-[var(--text-tertiary)]">{detail}</p>
        </div>
      </div>
      {action && onAction && (
        <button type="button" onClick={onAction} className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:text-[var(--color-primary-hover)]">
          {action}<ArrowRight size={13} />
        </button>
      )}
    </div>
  );
}

function HealthGauge({ value, connected }: { value: number; connected: boolean }) {
  const circumference = 2 * Math.PI * 42;
  const progress = connected ? Math.min(0.92, 0.18 + Math.log10(value + 1) * 0.28) : 0.06;
  return (
    <div className="flex items-center justify-center gap-5 py-2">
      <div className="relative h-28 w-28">
        <svg viewBox="0 0 100 100" className="h-full w-full -rotate-90">
          <circle cx="50" cy="50" r="42" fill="none" stroke="var(--border-light)" strokeWidth="7" />
          <circle cx="50" cy="50" r="42" fill="none" stroke={connected ? 'var(--color-success)' : 'var(--color-error)'} strokeWidth="7" strokeLinecap="round" strokeDasharray={circumference} strokeDashoffset={circumference * (1 - progress)} />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <strong className="text-2xl font-semibold tabular-nums text-[var(--text-primary)]">{value}</strong>
          <span className="text-[10px] text-[var(--text-tertiary)]">连接</span>
        </div>
      </div>
      <div>
        <div className={`text-sm font-semibold ${connected ? 'text-[var(--color-success)]' : 'text-[var(--color-error)]'}`}>{connected ? '运行正常' : '服务离线'}</div>
        <div className="mt-1 max-w-28 text-xs leading-5 text-[var(--text-tertiary)]">{connected ? '实时接口响应正常' : '等待核心恢复连接'}</div>
      </div>
    </div>
  );
}

function HealthStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-[var(--border-light)] bg-[var(--bg-base)] px-3 py-2.5">
      <div className="text-[10px] text-[var(--text-tertiary)]">{label}</div>
      <div className="mt-1 truncate text-sm font-semibold tabular-nums text-[var(--text-primary)]">{value}</div>
    </div>
  );
}

function ConnectionFlowRow({ connection }: { connection: Connection }) {
  const destination = connection.metadata.host || connection.metadata.destinationIP || '未知目标';
  const chain = connection.chains?.length ? connection.chains.join(' → ') : 'DIRECT';
  return (
    <div className="grid gap-2 px-4 py-3 transition hover:bg-[var(--bg-sidebar-hover)] sm:grid-cols-[minmax(0,1fr)_minmax(150px,0.7fr)_auto] sm:items-center">
      <div className="min-w-0">
        <div className="truncate text-xs font-medium text-[var(--text-primary)]" title={destination}>{destination}</div>
        <div className="mt-1 text-[10px] uppercase tracking-[0.08em] text-[var(--text-tertiary)]">{connection.metadata.network || 'TCP'} · {connection.rule || 'MATCH'}</div>
      </div>
      <div className="flex min-w-0 items-center gap-2 text-xs text-[var(--text-secondary)]">
        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-primary)]" />
        <span className="truncate" title={chain}>{chain}</span>
      </div>
      <div className="flex gap-3 text-[10px] tabular-nums sm:justify-end">
        <span className="text-blue-400">↓ {formatBytes(connection.download)}</span>
        <span className="text-emerald-400">↑ {formatBytes(connection.upload)}</span>
      </div>
    </div>
  );
}

function ProxySnapshotRow({ group }: { group: ProxyGroup }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg-base)] px-3 py-2.5">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-primary-bg)]"><ProxyGroupIcon group={group} className="h-4 w-4" /></span>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs font-medium text-[var(--text-primary)]">{group.name}</div>
        <div className="mt-0.5 truncate text-[10px] text-[var(--text-tertiary)]">{group.now || '未选择'}</div>
      </div>
      <span className="shrink-0 text-[10px] tabular-nums text-[var(--text-tertiary)]">{group.all?.length || 0}</span>
    </div>
  );
}

function EmptyPanel({ text, className }: { text: string; className: string }) {
  return <div className={`flex items-center justify-center text-sm text-[var(--text-tertiary)] ${className}`}>{text}</div>;
}

function aggregateOutboundUsage(connections: Connection[]) {
  const counts = new Map<string, number>();
  connections.forEach(connection => {
    const outbound = connection.chains?.[0] || 'DIRECT';
    counts.set(outbound, (counts.get(outbound) || 0) + 1);
  });
  return [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((left, right) => right.count - left.count);
}
