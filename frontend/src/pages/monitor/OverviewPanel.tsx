import React from 'react';
import { Activity, ArrowDown, ArrowUp, Circle, Radio, WifiOff } from 'lucide-react';
import { TrafficChart, type TrafficChartRef } from '@/components/monitor/TrafficChart';
import { formatBytes, formatSpeed, monitorPanelClass } from './monitorUtils';

interface OverviewPanelProps {
  connected: boolean;
  unavailableReason: string;
  totalUp: number;
  totalDown: number;
  speedUp: number;
  speedDown: number;
  connectionCount: number;
  uploadSpeedHistory: number[];
  downloadSpeedHistory: number[];
  connectionCountHistory: number[];
  chartRef: React.RefObject<TrafficChartRef>;
}

export function OverviewPanel({
  connected,
  unavailableReason,
  totalUp,
  totalDown,
  speedUp,
  speedDown,
  connectionCount,
  uploadSpeedHistory,
  downloadSpeedHistory,
  connectionCountHistory,
  chartRef,
}: OverviewPanelProps) {
  const totalSpeed = speedUp + speedDown;
  const sampleCount = Math.max(uploadSpeedHistory.length, downloadSpeedHistory.length);
  const connectionTrend = connectionCountHistory.length > 1
    ? connectionCount - connectionCountHistory[connectionCountHistory.length - 2]
    : 0;

  return (
    <div className="space-y-4 pb-4">
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

      <section className="overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]">
        <header className="flex flex-col gap-4 border-b border-[var(--border-light)] px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5">
          <div className="flex items-center gap-3">
            <span className={`relative flex h-9 w-9 items-center justify-center rounded-lg ${connected ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]' : 'bg-[var(--color-error-bg)] text-[var(--color-error)]'}`}>
              <Radio size={17} />
              {connected && <span className="absolute right-1 top-1 h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--color-success)]" />}
            </span>
            <div>
              <h2 className="text-sm font-semibold text-[var(--text-primary)]">实时流量</h2>
              <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">{connected ? `已连接 · ${sampleCount} 个采样点` : '等待核心连接'}</p>
            </div>
          </div>
          <div className="flex items-center gap-5 sm:gap-7">
            <HeaderMetric icon={<ArrowDown size={13} />} label="下载" value={formatSpeed(speedDown)} color="text-blue-400" />
            <HeaderMetric icon={<ArrowUp size={13} />} label="上传" value={formatSpeed(speedUp)} color="text-emerald-400" />
          </div>
        </header>

        <div className="grid xl:grid-cols-[minmax(0,1fr)_260px]">
          <div className="min-w-0 p-3 sm:p-5">
            {connected ? (
              <TrafficChart ref={chartRef} className="h-[300px] sm:h-[340px]" />
            ) : (
              <div className="flex h-[300px] items-center justify-center rounded-lg border border-dashed border-[var(--border-default)] text-sm text-[var(--text-tertiary)] sm:h-[340px]">
                暂无实时数据
              </div>
            )}
          </div>

          <aside className="grid grid-cols-2 border-t border-[var(--border-light)] xl:grid-cols-1 xl:border-l xl:border-t-0">
            <PrimaryMetric label="当前吞吐" value={formatSpeed(totalSpeed)} detail="上传与下载合计" />
            <PrimaryMetric label="活动连接" value={String(connectionCount)} detail={connectionTrend === 0 ? '连接数稳定' : `${connectionTrend > 0 ? '+' : ''}${connectionTrend} 较上次`} />
            <PrimaryMetric label="下载流量" value={formatBytes(totalDown)} detail="本次监控会话" />
            <PrimaryMetric label="上传流量" value={formatBytes(totalUp)} detail="本次监控会话" />
          </aside>
        </div>

        <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-light)] px-4 py-3 text-xs text-[var(--text-tertiary)] sm:px-5">
          <div className="flex items-center gap-4">
            <span className="flex items-center gap-1.5"><Circle size={7} className="fill-blue-400 text-blue-400" />下载</span>
            <span className="flex items-center gap-1.5"><Circle size={7} className="fill-emerald-400 text-emerald-400" />上传</span>
          </div>
          <span className="flex items-center gap-1.5"><Activity size={13} />60 秒滚动窗口</span>
        </footer>
      </section>
    </div>
  );
}

function HeaderMetric({ icon, label, value, color }: { icon: React.ReactNode; label: string; value: string; color: string }) {
  return (
    <div>
      <div className={`flex items-center gap-1 text-[11px] ${color}`}>{icon}{label}</div>
      <div className="mt-0.5 whitespace-nowrap text-sm font-semibold tabular-nums text-[var(--text-primary)]">{value}</div>
    </div>
  );
}

function PrimaryMetric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="min-w-0 border-b border-r border-[var(--border-light)] p-4 last:border-b-0 even:border-r-0 xl:border-r-0 xl:p-5">
      <div className="text-[11px] font-medium uppercase tracking-[0.1em] text-[var(--text-tertiary)]">{label}</div>
      <div className="mt-2 truncate text-xl font-semibold tracking-[-0.02em] tabular-nums text-[var(--text-primary)]">{value}</div>
      <div className="mt-1 truncate text-xs text-[var(--text-tertiary)]">{detail}</div>
    </div>
  );
}
