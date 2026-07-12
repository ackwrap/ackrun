import React from 'react';
import { Activity, ArrowDown, ArrowRight, ArrowUp, Gauge, Users } from 'lucide-react';
import { Link } from 'react-router-dom';
import { getClashClient } from '@/services/clash';
import { formatBytes, formatSpeed } from './monitor/monitorUtils';

interface ControlLiveStatusProps {
  isRunning: boolean;
}

interface LiveSnapshot {
  upload: number;
  download: number;
  uploadTotal: number;
  downloadTotal: number;
  connections: number;
  proxyGroups: number;
  memory: number;
  proxyPort: number;
}

const initialSnapshot: LiveSnapshot = {
  upload: 0,
  download: 0,
  uploadTotal: 0,
  downloadTotal: 0,
  connections: 0,
  proxyGroups: 0,
  memory: 0,
  proxyPort: 0,
};

export function ControlLiveStatus({ isRunning }: ControlLiveStatusProps) {
  const [snapshot, setSnapshot] = React.useState<LiveSnapshot>(initialSnapshot);
  const [history, setHistory] = React.useState<Array<{ upload: number; download: number }>>([]);
  const [apiOnline, setAPIOnline] = React.useState(false);
  const [apiError, setAPIError] = React.useState('');

  React.useEffect(() => {
    const client = getClashClient();
    if (!isRunning) {
      client.disconnectTraffic();
      setSnapshot(initialSnapshot);
      setHistory([]);
      setAPIOnline(false);
      setAPIError('核心未运行，启动后显示实时数据');
      return;
    }

    let cancelled = false;
    const loadSnapshot = async (includeProxies = false) => {
      try {
        const [connections, proxies, config] = await Promise.all([
          client.getConnections(),
          includeProxies ? client.getProxies() : Promise.resolve(null),
          includeProxies ? client.getConfig() : Promise.resolve(null),
        ]);
        if (cancelled) return;
        const groupCount = proxies
          ? Object.values(proxies.proxies).filter(proxy => Array.isArray(proxy.all)).length
          : undefined;
        setSnapshot(previous => ({
          ...previous,
          connections: connections.connections.length,
          uploadTotal: connections.uploadTotal,
          downloadTotal: connections.downloadTotal,
          proxyGroups: groupCount ?? previous.proxyGroups,
          memory: connections.memory ?? previous.memory,
          proxyPort: config ? (config['mixed-port'] || config.port || config['socks-port'] || 0) : previous.proxyPort,
        }));
        setAPIOnline(true);
        setAPIError('');
      } catch (error: any) {
        if (cancelled) return;
        setAPIOnline(false);
        setAPIError(error?.message || 'Clash API 暂不可用');
      }
    };

    client.connectTraffic(traffic => {
      if (cancelled) return;
      setSnapshot(previous => ({ ...previous, upload: traffic.up, download: traffic.down }));
      setHistory(previous => [...previous.slice(-23), { upload: traffic.up, download: traffic.down }]);
    });
    loadSnapshot(true);
    const timer = window.setInterval(() => loadSnapshot(false), 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      client.disconnectTraffic();
    };
  }, [isRunning]);

  const maxTraffic = Math.max(1, ...history.flatMap(item => [item.upload, item.download]));

  return (
    <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]">
      <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-semibold text-[var(--text-primary)]">实时运行中心</h2>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">当前吞吐、连接与高频资源维护集中在这里。</p>
        </div>
        <div className={`inline-flex w-fit items-center gap-2 rounded-[var(--radius-full)] px-2.5 py-1 text-xs ${apiOnline ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]' : 'bg-[var(--color-warning-bg)] text-[var(--color-warning)]'}`}>
          <span className={`h-1.5 w-1.5 rounded-full ${apiOnline ? 'bg-[var(--color-success)]' : 'bg-[var(--color-warning)]'}`} />
          {apiOnline ? 'Clash API 已连接' : isRunning ? 'Clash API 检查中' : '核心离线'}
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[1.65fr_1fr]">
        <div className="rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-4">
          <div className="flex items-center justify-between">
            <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]"><Gauge size={15} className="text-[var(--color-primary)]" />网络吞吐</h3>
            <Link to="/" className="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline">查看趋势<ArrowRight size={12} /></Link>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-3">
            <div className="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] px-3 py-3">
              <div className="flex items-center gap-1.5 text-xs text-[var(--text-tertiary)]"><ArrowDown size={13} className="text-[var(--color-primary)]" />实时下载</div>
              <div className="mt-1 text-xl font-semibold text-[var(--text-primary)]">{formatSpeed(snapshot.download)}</div>
            </div>
            <div className="rounded-[var(--radius-md)] bg-[var(--color-success-bg)] px-3 py-3">
              <div className="flex items-center gap-1.5 text-xs text-[var(--text-tertiary)]"><ArrowUp size={13} className="text-[var(--color-success)]" />实时上传</div>
              <div className="mt-1 text-xl font-semibold text-[var(--text-primary)]">{formatSpeed(snapshot.upload)}</div>
            </div>
          </div>
          <div className="mt-4 flex h-12 items-end gap-1" aria-label="最近 24 秒流量脉冲">
            {Array.from({ length: 24 }, (_, index) => history[index] || { upload: 0, download: 0 }).map((item, index) => (
              <div key={index} className="flex h-full min-w-0 flex-1 items-end gap-px">
                <span className="w-1/2 rounded-t-sm bg-[var(--color-primary)] opacity-70 transition-all" style={{ height: `${Math.max(4, item.download / maxTraffic * 100)}%` }} />
                <span className="w-1/2 rounded-t-sm bg-[var(--color-success)] opacity-70 transition-all" style={{ height: `${Math.max(4, item.upload / maxTraffic * 100)}%` }} />
              </div>
            ))}
          </div>
        </div>

        <div className="rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]"><Activity size={15} className="text-[var(--color-primary)]" />会话快照</h3>
          <div className="mt-4 space-y-3">
            <div className="flex items-center justify-between border-b border-[var(--border-light)] pb-3">
              <span className="flex items-center gap-2 text-xs text-[var(--text-secondary)]"><Users size={14} />活动连接</span>
              <span className="text-lg font-semibold text-[var(--text-primary)]">{snapshot.connections}</span>
            </div>
            <div className="flex items-center justify-between border-b border-[var(--border-light)] pb-3">
              <span className="text-xs text-[var(--text-secondary)]">策略组</span>
              <span className="text-sm font-semibold text-[var(--text-primary)]">{snapshot.proxyGroups}</span>
            </div>
            <div className="flex items-center justify-between border-b border-[var(--border-light)] pb-3">
              <span className="text-xs text-[var(--text-secondary)]">会话累计</span>
              <span className="text-sm font-semibold text-[var(--text-primary)]">{formatBytes(snapshot.uploadTotal + snapshot.downloadTotal)}</span>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[11px] text-[var(--text-tertiary)]">核心内存</div>
                <div className="mt-0.5 text-xs font-semibold text-[var(--text-primary)]">{snapshot.memory > 0 ? formatBytes(snapshot.memory) : '--'}</div>
              </div>
              <div>
                <div className="text-[11px] text-[var(--text-tertiary)]">代理端口</div>
                <div className="mt-0.5 text-xs font-semibold text-[var(--text-primary)]">{snapshot.proxyPort || '--'}</div>
              </div>
            </div>
          </div>
          {apiError && <div className="mt-3 rounded-[var(--radius-md)] bg-[var(--color-warning-bg)] px-2.5 py-2 text-[11px] text-[var(--color-warning)]">{apiError}</div>}
        </div>

      </div>
    </section>
  );
}
