import React from 'react';
import { Activity, Gauge, Globe2, RefreshCw, Wifi } from 'lucide-react';
import { getClashClient } from '@/services/clash';
import { formatBytes, formatSpeed } from './monitor/monitorUtils';

interface ControlNetworkOverviewProps {
  isRunning: boolean;
  proxyPort: number;
  onMessage: (message: string, type: 'success' | 'error' | 'info') => void;
}

interface IPProbe {
  label: string;
  value: string;
  error: string;
}

interface AccessProbe {
  label: string;
  latency: number;
  status: 'checking' | 'online' | 'offline';
}

interface RuntimeStats {
  upload: number;
  download: number;
  uploadTotal: number;
  downloadTotal: number;
  connections: number;
  memory: number;
  proxyGroups: number;
  proxyPort: number;
}

const initialStats: RuntimeStats = {
  upload: 0,
  download: 0,
  uploadTotal: 0,
  downloadTotal: 0,
  connections: 0,
  memory: 0,
  proxyGroups: 0,
  proxyPort: 0,
};

const ipSources = [
  { label: 'IPv4 · IPIFY', url: 'https://api.ipify.org?format=json', parse: (body: string) => JSON.parse(body).ip as string },
  { label: 'IPv6 · IPIFY', url: 'https://api6.ipify.org?format=json', parse: (body: string) => JSON.parse(body).ip as string },
  { label: 'IP.SB', url: 'https://api.ip.sb/ip', parse: (body: string) => body.trim() },
  { label: 'IDENT.ME', url: 'https://ident.me', parse: (body: string) => body.trim() },
];

const accessTargets = [
  { label: '百度搜索', url: 'https://www.baidu.com/favicon.ico' },
  { label: '网易云音乐', url: 'https://s1.music.126.net/style/favicon.ico' },
  { label: 'GitHub', url: 'https://github.com/favicon.ico' },
  { label: 'YouTube', url: 'https://www.youtube.com/favicon.ico' },
];

async function fetchWithTimeout(url: string, options: RequestInit = {}, timeout = 6000) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeout);
  try {
    return await fetch(url, { ...options, cache: 'no-store', signal: controller.signal });
  } finally {
    window.clearTimeout(timer);
  }
}

export function ControlNetworkOverview({ isRunning, proxyPort, onMessage }: ControlNetworkOverviewProps) {
  const [ipProbes, setIPProbes] = React.useState<IPProbe[]>(() => ipSources.map(source => ({ label: source.label, value: '', error: '' })));
  const [accessProbes, setAccessProbes] = React.useState<AccessProbe[]>(() => accessTargets.map(target => ({ label: target.label, latency: 0, status: 'checking' })));
  const [refreshingIPs, setRefreshingIPs] = React.useState(false);
  const [refreshingAccess, setRefreshingAccess] = React.useState(false);
  const [stats, setStats] = React.useState<RuntimeStats>(initialStats);
  const [statsError, setStatsError] = React.useState('');
  const [trafficError, setTrafficError] = React.useState('');
  const onMessageRef = React.useRef(onMessage);
  const lastStatsErrorRef = React.useRef('');
  const lastTrafficErrorRef = React.useRef('');

  React.useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  const refreshIPs = React.useCallback(async () => {
    setRefreshingIPs(true);
    const results = await Promise.all(ipSources.map(async source => {
        try {
          const response = await fetchWithTimeout(source.url);
          if (!response.ok) throw new Error(`HTTP ${response.status}`);
          const value = source.parse(await response.text()).trim();
          if (!value || value.length > 80) throw new Error('响应格式无效');
          return { label: source.label, value, error: '' };
        } catch (error: any) {
          return { label: source.label, value: '', error: error?.name === 'AbortError' ? '请求超时' : '获取失败' };
        }
      }));
    setIPProbes(results);
    setRefreshingIPs(false);
  }, []);

  const refreshAccess = React.useCallback(async () => {
    setRefreshingAccess(true);
    setAccessProbes(accessTargets.map(target => ({ label: target.label, latency: 0, status: 'checking' })));
    const results = await Promise.all(accessTargets.map(async target => {
        const startedAt = performance.now();
        try {
          await fetchWithTimeout(target.url, { mode: 'no-cors' });
          return { label: target.label, latency: Math.max(1, Math.round(performance.now() - startedAt)), status: 'online' as const };
        } catch {
          return { label: target.label, latency: 0, status: 'offline' as const };
        }
      }));
    setAccessProbes(results);
    setRefreshingAccess(false);
  }, []);

  React.useEffect(() => {
    refreshIPs();
    const timer = window.setInterval(refreshIPs, 60000);
    return () => window.clearInterval(timer);
  }, [refreshIPs]);

  React.useEffect(() => {
    refreshAccess();
    const timer = window.setInterval(refreshAccess, 60000);
    return () => window.clearInterval(timer);
  }, [refreshAccess]);

  React.useEffect(() => {
    const client = getClashClient();
    if (!isRunning) {
      client.disconnectTraffic();
      setStats(initialStats);
      setStatsError('核心未运行');
      setTrafficError('');
      lastStatsErrorRef.current = '';
      lastTrafficErrorRef.current = '';
      return;
    }

    let cancelled = false;
    let statsFailures = 0;
    let timer: number | null = null;
    const graceUntil = Date.now() + 5000;
    setStatsError('正在连接');
    setTrafficError('');

    const loadStats = async () => {
      try {
        const [connections, proxies] = await Promise.all([
          client.getConnections(),
          client.getProxies(),
        ]);
        if (cancelled) return;
        const proxyGroups = Object.values(proxies.proxies).filter(proxy => Array.isArray(proxy.all)).length;
        setStats(previous => ({
          ...previous,
          uploadTotal: connections.uploadTotal,
          downloadTotal: connections.downloadTotal,
          connections: connections.connections.length,
          memory: connections.memory || 0,
          proxyGroups,
          proxyPort,
        }));
        statsFailures = 0;
        setStatsError('');
        lastStatsErrorRef.current = '';
      } catch (error: any) {
        if (!cancelled) {
          const detail = error?.message || '运行统计暂不可用';
          statsFailures += 1;
          const stillStarting = Date.now() < graceUntil || statsFailures < 3;
          setStatsError(stillStarting ? '正在连接' : '统计不可用');
          if (!stillStarting && !lastStatsErrorRef.current) {
            onMessageRef.current(`运行统计加载失败: ${detail}`, 'error');
          }
          if (!stillStarting) lastStatsErrorRef.current = detail;
        }
      }
    };

    const startupTimer = window.setTimeout(() => {
      if (cancelled) return;
      client.connectTraffic(
        traffic => {
          if (!cancelled) {
            setTrafficError('');
            lastTrafficErrorRef.current = '';
            setStats(previous => ({ ...previous, upload: traffic.up, download: traffic.down }));
          }
        },
        error => {
          if (!cancelled) {
            const stillStarting = Date.now() < graceUntil;
            setTrafficError(stillStarting ? '正在连接' : '实时流量断开');
            if (!stillStarting && !lastTrafficErrorRef.current) {
              onMessageRef.current(`实时流量连接失败: ${error}`, 'error');
            }
            if (!stillStarting) lastTrafficErrorRef.current = error;
          }
        },
      );
      loadStats();
      timer = window.setInterval(loadStats, 2500);
    }, 800);

    return () => {
      cancelled = true;
      window.clearTimeout(startupTimer);
      if (timer !== null) window.clearInterval(timer);
      client.disconnectTraffic();
    };
  }, [isRunning, proxyPort]);

  const copyIP = async (probe: IPProbe) => {
    if (!probe.value) return;
    try {
      await navigator.clipboard.writeText(probe.value);
      onMessage(`${probe.label} 已复制`, 'success');
    } catch {
      onMessage(`${probe.label} 复制失败`, 'error');
    }
  };

  const statItems = [
    { label: '上传', value: formatSpeed(stats.upload) },
    { label: '下载', value: formatSpeed(stats.download) },
    { label: '上传总量', value: formatBytes(stats.uploadTotal) },
    { label: '下载总量', value: formatBytes(stats.downloadTotal) },
    { label: '活动连接', value: `${stats.connections}` },
    { label: '内存占用', value: stats.memory > 0 ? formatBytes(stats.memory) : '--' },
    { label: '策略组', value: `${stats.proxyGroups}` },
    { label: '代理端口', value: stats.proxyPort > 0 ? `${stats.proxyPort}` : '--' },
  ];

  return (
    <>
      <div className="order-3 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4">
        <div className="mb-2 flex items-center justify-between gap-3">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]"><Globe2 size={15} className="text-[var(--color-primary)]" />IP 地址</h3>
          <button type="button" onClick={refreshIPs} disabled={refreshingIPs} className="flex h-7 w-7 items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-default)] text-[var(--text-secondary)] transition hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] disabled:opacity-50" title="刷新 IP 地址">
            <RefreshCw size={13} className={refreshingIPs ? 'animate-spin' : ''} />
          </button>
        </div>
        <div className="grid flex-1 grid-cols-1 grid-rows-4 gap-1.5">
          {ipProbes.map(probe => (
            <button type="button" key={probe.label} disabled={!probe.value} onClick={() => copyIP(probe)} className="flex min-h-[42px] min-w-0 items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-3.5 py-1.5 text-left transition hover:border-[var(--color-primary)] hover:bg-[var(--color-primary-bg)] disabled:cursor-default disabled:hover:border-[var(--border-light)] disabled:hover:bg-[var(--bg-base)]" title={probe.value ? `点击复制 ${probe.label}` : probe.error || '检查中'}>
              <span className="text-xs font-medium text-[var(--text-secondary)]">{probe.label}</span>
              <span className={`min-w-0 truncate text-right text-xs font-semibold ${probe.error ? 'text-[var(--color-warning)]' : 'text-[var(--color-success)]'}`} title={probe.value || probe.error}>{probe.value || probe.error || '检查中...'}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="order-4 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4">
        <div className="mb-2 flex items-center justify-between gap-3">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]"><Wifi size={15} className="text-[var(--color-primary)]" />访问检查</h3>
          <button type="button" onClick={refreshAccess} disabled={refreshingAccess} className="flex h-7 w-7 items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-default)] text-[var(--text-secondary)] transition hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] disabled:opacity-50" title="刷新访问检查">
            <RefreshCw size={13} className={refreshingAccess ? 'animate-spin' : ''} />
          </button>
        </div>
        <div className="grid flex-1 grid-cols-1 grid-rows-4 gap-1.5">
          {accessProbes.map(probe => (
            <div key={probe.label} className="flex min-h-[42px] items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-3.5 py-1.5">
              <span className="text-xs font-medium text-[var(--text-secondary)]">{probe.label}</span>
              <span className={`min-w-0 truncate text-right text-xs font-medium ${probe.status === 'online' ? 'text-[var(--color-success)]' : probe.status === 'offline' ? 'text-[var(--color-error)]' : 'text-[var(--text-tertiary)]'}`}>{probe.status === 'online' ? `连接正常 · ${probe.latency} ms` : probe.status === 'offline' ? '连接失败' : '检查中...'}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="order-5 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)] xl:col-span-4">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]"><Gauge size={15} className="text-[var(--color-primary)]" />运行统计</h3>
          <span className={`inline-flex items-center gap-1.5 text-[11px] ${statsError || trafficError ? 'text-[var(--color-warning)]' : 'text-[var(--color-success)]'}`}><Activity size={12} />{statsError || trafficError || '实时更新'}</span>
        </div>
        <div className="grid grid-cols-4 gap-2">
          {statItems.map(item => (
            <div key={item.label} className="flex min-h-[76px] min-w-0 flex-col items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-2 py-2 text-center">
              <span className="text-[11px] text-[var(--text-tertiary)]">{item.label}</span>
              <span className="mt-2 max-w-full truncate text-xs font-semibold text-[var(--color-success)]" title={item.value}>{item.value}</span>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
