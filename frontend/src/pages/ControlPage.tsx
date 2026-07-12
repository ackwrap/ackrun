import React from 'react';
import { Play, Square, RotateCcw, Download, AlertTriangle, Power, RefreshCw, FileCheck2, ShieldCheck, DatabaseZap } from 'lucide-react';
import { useRealtimeSocket } from '@/hooks/useRealtimeSocket';
import { api } from '@/services/api';
import type { RuntimeResponse, WSEvent } from '@/services/types';
import { Button } from '@/components/ui/Button';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { PageHeader } from '@/components/layout/PageHeader';
import { Toast } from '@/components/ui/Toast';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { ControlOverview } from './ControlOverview';
import { ControlNetworkOverview } from './ControlNetworkOverview';

function Panel({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <section className={`rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)] ${className}`}>
      {children}
    </section>
  );
}

function PanelTitle({ title, extra }: { title: string; extra?: React.ReactNode }) {
  return (
    <div className="mb-4 flex items-center justify-between">
      <h2 className="text-base font-semibold text-white">{title}</h2>
      {extra}
    </div>
  );
}

const runtimeStatusMap: Record<string, { label: string; badge: string; dot: string }> = {
  running: { label: '运行中', badge: 'bg-emerald-500/15 text-emerald-300 border-emerald-400/20', dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.6)]' },
  starting: { label: '启动中', badge: 'bg-blue-500/15 text-blue-300 border-blue-400/20', dot: 'bg-blue-400 animate-status-pulse' },
  stopping: { label: '停止中', badge: 'bg-yellow-500/15 text-yellow-300 border-yellow-400/20', dot: 'bg-yellow-400 animate-status-pulse' },
  stopped: { label: '已停止', badge: 'bg-slate-500/15 text-slate-300 border-slate-400/20', dot: 'bg-slate-400' },
  error: { label: '异常', badge: 'bg-red-500/15 text-red-300 border-red-400/20', dot: 'bg-red-400' },
  not_installed: { label: '未安装', badge: 'bg-red-500/15 text-red-300 border-red-400/20', dot: 'bg-red-400' },
  no_config: { label: '无配置', badge: 'bg-orange-500/15 text-orange-300 border-orange-400/20', dot: 'bg-orange-400' },
};

const installLabel: Record<string, string> = {
  idle: '未安装', downloading: '下载中', extracting: '解压中', done: '已安装', failed: '失败',
};

function isVersionNewer(candidate?: string, current?: string) {
  if (!candidate || !current) return false;
  const parse = (value: string) => value.replace(/^v/, '').split(/[+-]/, 1)[0].split('.').map(part => Number(part) || 0);
  const left = parse(candidate);
  const right = parse(current);
  for (let index = 0; index < 3; index += 1) {
    if ((left[index] || 0) !== (right[index] || 0)) return (left[index] || 0) > (right[index] || 0);
  }
  return false;
}

function RuntimeAction({ icon, label, tone = 'default', disabled = false, loading = false, title, onClick }: { icon: React.ReactNode; label: string; tone?: 'default' | 'danger'; disabled?: boolean; loading?: boolean; title?: string; onClick?: () => void }) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      title={title}
      className={`inline-flex h-8 items-center justify-center gap-1.5 rounded-[6px] border px-2.5 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-45 ${
        tone === 'danger'
          ? 'border-red-400/25 bg-red-500/10 text-red-200 hover:border-red-300/40 hover:bg-red-500/15'
          : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:border-blue-400/30 hover:bg-blue-500/10 hover:text-white'
      }`}
    >
      {loading ? <RefreshCw size={14} className="animate-spin" /> : icon}{label}
    </button>
  );
}

function ModeOption({ name, value, checked, title, description, disabled = false, onChange }: { name: string; value: string; checked: boolean; title: string; description: string; disabled?: boolean; onChange: (value: string) => void }) {
  return (
    <label
      title={`${title}：${description}`}
      className={`relative flex h-9 min-w-0 items-center justify-start overflow-hidden rounded-md px-3 transition ${disabled ? 'cursor-not-allowed opacity-55' : 'cursor-pointer'} ${
        checked
          ? 'bg-[var(--bg-surface)] text-[var(--color-primary)] shadow-[var(--shadow-card)]'
          : `bg-transparent text-[var(--text-secondary)] ${disabled ? '' : 'hover:bg-[var(--bg-surface)] hover:text-[var(--text-primary)]'}`
      }`}
    >
      <input
        type="radio"
        name={name}
        value={value}
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        className="sr-only"
      />
      <span className={`min-w-0 flex-1 truncate text-left text-xs ${checked ? 'font-semibold' : 'font-medium'}`}>
        {title}
      </span>
      {checked && <span aria-hidden className="absolute inset-y-2 left-0 w-0.5 rounded-full bg-[var(--color-primary)]" />}
    </label>
  );
}

export function ControlPage() {
  const [runtime, setRuntime] = React.useState<RuntimeResponse | null>(null);
  const [installStatus, setInstallStatus] = React.useState<{ status: string; version?: string; latest_version?: string; progress?: number; message?: string; error?: string } | null>(null);
  const [configStatus, setConfigStatus] = React.useState<{ has_config: boolean; valid: boolean; file_name?: string; updated_at?: number; error?: string } | null>(null);
  const [installProgress, setInstallProgress] = React.useState<{ percent: number; downloaded_bytes: number; total_bytes: number } | null>(null);
  const [guideDismissed, setGuideDismissed] = React.useState(false);
  const [inboundMode, setInboundMode] = React.useState<string>('tun_mixed');
  const [proxyMode, setProxyMode] = React.useState<string>('rule');
  const [modeChanging, setModeChanging] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error' | 'info'>('success');
  const [overviewRefreshKey, setOverviewRefreshKey] = React.useState(0);
  const [runtimeAction, setRuntimeAction] = React.useState('');
  const [confirmFirewallReset, setConfirmFirewallReset] = React.useState(false);

  useRealtimeSocket((event: WSEvent) => {
    console.log('[WS]', event.type, event.data);
    switch (event.type) {
      case 'runtime.status': setRuntime(previous => ({ ...previous, ...(event.data as RuntimeResponse) })); break;
      case 'installer.status': {
        const status = event.data as any;
        setInstallStatus(previous => ({ ...previous, ...status }));
        if (status.status === 'done' || status.status === 'failed') {
          setInstallProgress(null);
        }
        if (status.status === 'done') {
          api.getRuntime().then(setRuntime).catch(() => {});
          api.getConfigStatus().then(setConfigStatus).catch(() => {});
        }
        if (status.status === 'failed') {
          showMessage(`安装失败: ${status.error || '请查看安装状态详情'}`, 'error');
        }
        break;
      }
      case 'installer.progress': setInstallProgress(event.data as any); break;
      case 'core.status': {
        const d = event.data as any;
        console.log('[WS] core.status -> mapping to runtime', d.status, d.pid);
        setRuntime(prev => ({ ...prev, status: d.status, pid: d.pid || 0 }));
        if (d.status === 'error' && d.error) showMessage(`核心异常: ${d.error}`, 'error');
        break;
      }
      case 'config.status': setConfigStatus(event.data as any); break;
      case 'subscription.sync': {
        const status = (event.data as any)?.status;
        if (status === 'updated' || status === 'failed') setOverviewRefreshKey(value => value + 1);
        break;
      }
      case 'subscription.sync_all':
      case 'route_rule_subscription.sync_all':
      case 'geo.sync_all': {
        const data = event.data as { failed?: number; total?: number };
        const labels: Record<string, string> = {
          'subscription.sync_all': '节点订阅同步',
          'route_rule_subscription.sync_all': '规则订阅更新',
          'geo.sync_all': 'Geo 资源更新',
        };
        setOverviewRefreshKey(value => value + 1);
        if ((data.failed || 0) > 0) {
          showMessage(`${labels[event.type]}完成，${data.failed}/${data.total || 0} 项失败`, 'error');
        } else {
          showMessage(`${labels[event.type]}完成，共 ${data.total || 0} 项`, 'success');
        }
        break;
      }
    }
  });

  const showMessage = (msg: string, type: 'success' | 'error' | 'info' = 'success') => {
    setMessage(msg);
    setMessageType(type);
  };

  React.useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(() => setMessage(''), messageType === 'error' ? 5000 : 3000);
    return () => window.clearTimeout(timer);
  }, [message, messageType]);

  React.useEffect(() => {
    let cancelled = false;

    async function checkInitialState() {
      try {
        const runtimeStatus = await api.getRuntime();
        if (cancelled) return;
        setRuntime(runtimeStatus);

        const installerStatus = await api.getInstallerStatus();
        if (cancelled) return;
        setInstallStatus(installerStatus);

        if (runtimeStatus.status === 'not_installed') {
          return;
        }

        const config = await api.getConfigStatus();
        if (cancelled) return;
        setConfigStatus(config);

        // 加载运行模式
        const modeResp = await api.getInboundMode();
        if (cancelled) return;
        setInboundMode(modeResp.mode);

        // 加载代理模式
        const proxyModeResp = await api.getProxyMode();
        if (cancelled) return;
        setProxyMode(proxyModeResp.mode);
      } catch (error: any) {
        if (!cancelled) showMessage(`控制面板初始化失败: ${error?.message || '请求失败'}`, 'error');
      }
    }

    checkInitialState();
    return () => { cancelled = true; };
  }, []);

  const rt = runtime?.status || 'not_installed';
  const isRunning = rt === 'running';
  const isRuntimeChecked = runtime !== null;
  const isNotInstalled = isRuntimeChecked && rt === 'not_installed';
  const isNoConfig = rt === 'no_config';
  const isInstalling = installStatus?.status === 'downloading' || installStatus?.status === 'extracting';
  const isWindows = runtime?.platform === 'windows';
  const maintenanceUnsupported = Boolean(runtime?.platform) && !isWindows;
  const currentVersion = runtime?.version || installStatus?.version;
  const latestVersion = installStatus?.latest_version;
  const updateAvailable = isVersionNewer(latestVersion, currentVersion);
  const view = runtimeStatusMap[rt] || runtimeStatusMap.not_installed;
  const showInstallGuide = !guideDismissed && isRuntimeChecked && (isNotInstalled || isInstalling);
  const showConfigGuide = !guideDismissed && !isNotInstalled && !isInstalling && configStatus?.has_config === false;
  const displayedInstallProgress = {
    percent: Math.max(installProgress?.percent ?? 0, installStatus?.progress ?? 0),
    downloaded_bytes: installProgress?.downloaded_bytes ?? 0,
    total_bytes: installProgress?.total_bytes ?? 0,
  };

  const doAction = async (fn: () => Promise<any>, label: string) => {
    if (runtimeAction) return;
    setRuntimeAction(label);
    try {
      const res = await fn();
      showMessage(`${label} 成功${res?.message ? `: ${res.message}` : ''}`);
    } catch (e: any) {
      showMessage(`${label} 失败: ${e.message}`, 'error');
    } finally {
      setRuntimeAction('');
    }
    setTimeout(() => {
      console.log('[REST] re-fetching runtime status after action:', label);
      api.getRuntime().then(r => { console.log('[REST] runtime:', r); setRuntime(r); }).catch(() => {});
      api.getConfigStatus().then(c => setConfigStatus(c)).catch(() => {});
      api.getInstallerStatus().then(i => setInstallStatus(i)).catch(() => {});
    }, 1000);
  };

  const installCore = async (label: string) => {
    try {
      await api.install();
      setInstallStatus(previous => ({ ...previous, status: 'downloading', progress: 0, message: 'preparing download', error: undefined }));
      setInstallProgress(null);
      showMessage(`${label}任务已启动`, 'info');
    } catch (e: any) {
      showMessage(`${label}启动失败: ${e.message}`, 'error');
    }
  };

  React.useEffect(() => {
    if (!isInstalling) return;

    let cancelled = false;
    let refreshing = false;
    const refresh = async () => {
      if (refreshing) return;
      refreshing = true;
      try {
        const status = await api.getInstallerStatus();
        if (cancelled) return;
        setInstallStatus(status);
        if (status.status === 'done' || status.status === 'failed') setInstallProgress(null);
        if (status.status === 'done') {
          api.getRuntime().then(setRuntime).catch(() => {});
          api.getConfigStatus().then(setConfigStatus).catch(() => {});
        }
      } catch (error: any) {
        if (cancelled) return;
        const detail = error?.message || '连接被重置';
        const errorMessage = `安装状态查询失败，后端连接已断开: ${detail}`;
        setInstallStatus(previous => ({ ...previous, status: 'failed', error: errorMessage }));
        setInstallProgress(null);
        showMessage(errorMessage, 'error');
      } finally {
        refreshing = false;
      }
    };

    const timer = window.setInterval(refresh, 1000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [isInstalling]);

  const changeInboundMode = async (mode: string) => {
    if (isRunning || modeChanging) return;
    setModeChanging(true);
    try {
      await api.setInboundMode(mode);
      setInboundMode(mode);
      showMessage(`运行模式已切换为 ${inboundModeLabels[mode]}`);
    } catch (e: any) {
      showMessage(`切换运行模式失败: ${e.message}`, 'error');
    } finally {
      setModeChanging(false);
    }
  };

  const changeProxyMode = async (mode: string) => {
    if (isRunning || modeChanging) return;
    setModeChanging(true);
    try {
      await api.setProxyMode(mode);
      setProxyMode(mode);
      showMessage(`代理模式已切换为 ${proxyModeLabels[mode]}`);
    } catch (e: any) {
      showMessage(`切换代理模式失败: ${e.message}`, 'error');
    } finally {
      setModeChanging(false);
    }
  };

  const inboundModeLabels: Record<string, string> = {
    'tun': 'TUN 模式',
    'mixed': 'Mixed 模式',
    'tun_mixed': 'TUN + Mixed',
  };

  const proxyModeLabels: Record<string, string> = {
    'global': '全局模式',
    'rule': '规则模式',
    'direct': '直连模式',
  };

  const installationPanel = (
    <>
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-[var(--text-primary)]">安装信息</h3>
        <span className="text-[10px] text-[var(--text-tertiary)]">GitHub Release</span>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <div className="rounded-lg bg-white/[0.04] px-2.5 py-2">
          <div className="text-[9px] uppercase tracking-[0.1em] text-[var(--text-tertiary)]">状态</div>
          <div className="mt-1 flex items-center gap-1.5 text-xs font-semibold text-white">
            <StatusBadge status={installStatus?.status === 'done' ? 'online' : installStatus?.status === 'failed' ? 'error' : isInstalling ? 'pending' : 'offline'} />
            <span className={installStatus?.status === 'failed' ? 'text-red-300' : ''}>{installLabel[installStatus?.status || 'idle'] || installStatus?.status || '未安装'}</span>
          </div>
        </div>
        <div className="rounded-lg bg-white/[0.04] px-2.5 py-2">
          <div className="text-[9px] uppercase tracking-[0.1em] text-[var(--text-tertiary)]">当前</div>
          <div className="mt-1 truncate text-xs font-semibold text-white">{currentVersion || '--'}</div>
        </div>
        <div className="rounded-lg bg-white/[0.04] px-2.5 py-2">
          <div className="text-[9px] uppercase tracking-[0.1em] text-[var(--text-tertiary)]">最新</div>
          <div className={`mt-1 truncate text-xs font-semibold ${updateAvailable ? 'text-amber-300' : 'text-white'}`}>{latestVersion || '--'}</div>
        </div>
      </div>
      {installStatus?.error && <div className="mt-2 text-xs text-red-300">{installStatus.error}</div>}
      {isInstalling && (
        <div className="mt-3">
          <div className="h-2 overflow-hidden rounded-full bg-white/[0.08]"><div className="h-full rounded-full bg-blue-400 transition-all" style={{ width: `${Math.min(100, displayedInstallProgress.percent)}%` }} /></div>
          <div className="mt-1 text-[10px] text-[var(--text-tertiary)]">{displayedInstallProgress.percent.toFixed(1)}%{displayedInstallProgress.total_bytes > 0 && ` · ${(displayedInstallProgress.downloaded_bytes / 1024 / 1024).toFixed(1)} MB / ${(displayedInstallProgress.total_bytes / 1024 / 1024).toFixed(1)} MB`}</div>
        </div>
      )}
      <Button className="mt-3" fullWidth variant="primary" size="sm" icon={<Download size={14} />} disabled={isInstalling || isRunning} onClick={() => installCore(updateAvailable ? '更新' : '安装')} loading={isInstalling}>{isInstalling ? (updateAvailable ? '更新中...' : '安装中...') : updateAvailable ? `更新至 ${latestVersion}` : installStatus?.status === 'done' ? '重新安装' : '安装'}</Button>
    </>
  );

  return (
    <div className="flex flex-col h-full">
      <Toast message={message} type={messageType} />
      <ConfirmDialog
        open={confirmFirewallReset}
        title="重置 Windows 防火墙"
        message="此操作会清除并恢复系统防火墙规则，可能影响其他应用的网络访问。确认继续吗？"
        confirmText="确认重置"
        danger
        onCancel={() => setConfirmFirewallReset(false)}
        onConfirm={() => {
          setConfirmFirewallReset(false);
          void doAction(api.resetFirewall, '重置防火墙');
        }}
      />
      <PageHeader title="控制面板" />

      {(showInstallGuide || showConfigGuide) && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(16,27,43,0.96))] p-5 shadow-[var(--shadow-card)]">
            <div className="flex items-start gap-3">
              <div className={`mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${showInstallGuide ? 'bg-red-500/15 text-red-300' : 'bg-orange-500/15 text-orange-300'}`}>
                <AlertTriangle size={20} />
              </div>
              <div className="min-w-0 flex-1">
                <h3 className="text-base font-semibold text-white">{showInstallGuide ? '需要安装 sing-box' : '需要生成默认配置'}</h3>
                <p className="mt-1 text-sm text-[var(--text-secondary)]">
                  {showInstallGuide ? '控制面板检测到 sing-box 未安装，请先安装核心程序。' : '已检测到 sing-box，但当前没有可用配置文件，是否生成默认配置？'}
                </p>
              </div>
            </div>

            {showInstallGuide && isInstalling && (
              <div className="mt-4">
                <div className="h-2 overflow-hidden rounded-full bg-white/[0.08]">
                  <div className="h-full rounded-full bg-blue-400 transition-all" style={{ width: `${Math.min(100, displayedInstallProgress.percent)}%` }} />
                </div>
                <div className="mt-1 text-xs text-[var(--text-tertiary)]">
                  {displayedInstallProgress.percent.toFixed(1)}%
                  {displayedInstallProgress.total_bytes > 0 && ` · ${(displayedInstallProgress.downloaded_bytes / 1024 / 1024).toFixed(1)} MB / ${(displayedInstallProgress.total_bytes / 1024 / 1024).toFixed(1)} MB`}
                </div>
              </div>
            )}

            {showInstallGuide && installStatus?.error && (
              <div className="mt-4 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] px-3 py-2 text-xs text-[var(--color-error)]">
                {installStatus.error}
              </div>
            )}

            <div className="mt-5 flex items-center justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => setGuideDismissed(true)}>稍后处理</Button>
              {showInstallGuide ? (
                <Button size="sm" variant="primary" icon={<Download size={14} />} loading={isInstalling} disabled={isInstalling} onClick={() => installCore('安装')}>{isInstalling ? '安装中...' : '安装 sing-box'}</Button>
              ) : (
                <Button size="sm" variant="primary" disabled={isRunning || Boolean(runtimeAction)} loading={runtimeAction === '生成默认配置'} onClick={() => doAction(api.generateDefaultConfig, '生成默认配置')}>生成默认配置</Button>
              )}
            </div>
          </div>
        </div>
      )}

      {isNotInstalled && (
        <section className="shrink-0 rounded-[var(--radius-xl)] border border-red-400/20 bg-[linear-gradient(135deg,rgba(248,81,73,0.08),rgba(47,129,247,0.04))] p-5">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-red-500/15 text-red-300"><AlertTriangle size={18} /></div>
            <div className="min-w-0 flex-1">
              <h3 className="text-base font-semibold text-white">sing-box 未安装</h3>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">点击下方"安装"按钮下载并安装 sing-box。</p>
            </div>
          </div>
        </section>
      )}

      {isNoConfig && (
        <section className="shrink-0 rounded-[var(--radius-xl)] border border-orange-400/20 bg-[linear-gradient(135deg,rgba(251,146,60,0.08),rgba(47,129,247,0.04))] p-5 mt-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-orange-500/15 text-orange-300"><AlertTriangle size={18} /></div>
            <div className="min-w-0 flex-1">
              <h3 className="text-base font-semibold text-white">配置文件不存在</h3>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">sing-box 已安装但配置文件缺失，请在下方生成默认配置。</p>
            </div>
          </div>
        </section>
      )}

      <div className="mt-4 grid min-h-[720px] flex-1 items-stretch gap-4 lg:grid-cols-2 xl:grid-cols-12">
        <Panel className="flex h-full min-h-0 flex-col p-5 xl:col-span-4">
          <div className="flex items-center gap-3">
            <div className={`relative flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${isRunning ? 'bg-emerald-500/15 text-emerald-300' : 'bg-slate-500/15 text-slate-300'}`}>
              <Power size={19} />
              <span className={`absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full ${view.dot} ${isRunning ? 'animate-status-pulse' : ''}`} />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h2 className="truncate text-base font-semibold text-white">核心控制</h2>
                <span className={`shrink-0 rounded-md border px-2 py-0.5 text-xs font-medium ${view.badge}`}>{view.label}</span>
              </div>
              <div className="mt-1 text-xs text-[var(--text-tertiary)]">sing-box 进程管理</div>
            </div>
          </div>
          <div className="mt-4 grid grid-cols-3 gap-2">
            {[
              ['进程 ID', runtime?.pid || '-'],
              ['核心版本', runtime?.version || installStatus?.version || '--'],
              ['配置状态', configStatus?.has_config && configStatus.valid ? '校验通过' : '需要处理'],
            ].map(([label, value]) => (
              <div key={label} className="min-w-0 rounded-lg bg-[var(--bg-base)] px-2.5 py-2">
                <div className="truncate text-[9px] uppercase tracking-[0.1em] text-[var(--text-tertiary)]">{label}</div>
                <div className="mt-0.5 truncate text-xs font-semibold text-white">{value}</div>
              </div>
            ))}
          </div>
          <div className="my-auto grid grid-cols-2 gap-2">
            <RuntimeAction icon={<Play size={14} />} label="启动核心" loading={runtimeAction === '启动'} disabled={Boolean(runtimeAction) || isRunning || isNotInstalled || isNoConfig} onClick={() => doAction(api.startCore, '启动')} />
            <RuntimeAction icon={<Square size={14} />} label="停止核心" tone="danger" loading={runtimeAction === '停止'} disabled={Boolean(runtimeAction) || !isRunning} onClick={() => doAction(api.stopCore, '停止')} />
            <RuntimeAction icon={<RotateCcw size={14} />} label="重启核心" loading={runtimeAction === '重启'} disabled={Boolean(runtimeAction) || !isRunning} onClick={() => doAction(api.restartCore, '重启')} />
            <RuntimeAction icon={<RefreshCw size={14} />} label="重载配置" loading={runtimeAction === '重载配置'} disabled={Boolean(runtimeAction) || isNotInstalled || isNoConfig} onClick={() => doAction(api.reloadConfig, '重载配置')} />
          </div>
          <div className="flex flex-wrap items-center gap-2 border-t border-[var(--border-light)] pt-3 text-xs text-[var(--text-tertiary)]">
            <span>{isRunning ? '核心正在提供代理服务' : '核心当前未运行'}</span>
          </div>
        </Panel>

        <Panel className="flex h-full min-h-0 flex-col p-5 xl:col-span-4">
          <PanelTitle title="代理模式" extra={<span className="text-[9px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">Route</span>} />
          <p className="text-xs leading-5 text-[var(--text-tertiary)]">决定流量使用规则、代理或直连</p>
          <div className="my-auto grid min-w-0 grid-cols-1 gap-1 rounded-lg bg-[var(--bg-base)] p-1">
            <ModeOption name="proxy-mode" value="rule" checked={proxyMode === 'rule'} title="规则模式" description="根据规则自动分流" disabled={isRunning || modeChanging || Boolean(runtimeAction)} onChange={changeProxyMode} />
            <ModeOption name="proxy-mode" value="global" checked={proxyMode === 'global'} title="全局模式" description="所有流量走代理" disabled={isRunning || modeChanging || Boolean(runtimeAction)} onChange={changeProxyMode} />
            <ModeOption name="proxy-mode" value="direct" checked={proxyMode === 'direct'} title="直连模式" description="所有流量直连" disabled={isRunning || modeChanging || Boolean(runtimeAction)} onChange={changeProxyMode} />
          </div>
          <div className="flex items-center justify-between gap-2 border-t border-[var(--border-light)] pt-3 text-xs text-[var(--text-tertiary)]">
            <span>{isRunning ? '停止核心后可修改' : '当前模式'}</span>
            <span className="rounded-md bg-[var(--bg-base)] px-2 py-1 text-[var(--text-secondary)]">{proxyModeLabels[proxyMode] || '--'}</span>
          </div>
        </Panel>

        <Panel className="flex h-full min-h-0 flex-col p-5 xl:col-span-4">
          <PanelTitle title="运行模式" extra={<span className="text-[9px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">Inbound</span>} />
          <p className="text-xs leading-5 text-[var(--text-tertiary)]">选择流量进入 sing-box 的方式</p>
          <div className="my-auto grid min-w-0 grid-cols-1 gap-1 rounded-lg bg-[var(--bg-base)] p-1">
            <ModeOption name="inbound-mode" value="tun_mixed" checked={inboundMode === 'tun_mixed'} title="TUN + Mixed" description="透明代理 + 端口" disabled={isRunning || modeChanging || Boolean(runtimeAction)} onChange={changeInboundMode} />
            <ModeOption name="inbound-mode" value="tun" checked={inboundMode === 'tun'} title="TUN 模式" description="纯透明代理" disabled={isRunning || modeChanging || Boolean(runtimeAction)} onChange={changeInboundMode} />
            <ModeOption name="inbound-mode" value="mixed" checked={inboundMode === 'mixed'} title="Mixed 模式" description="SOCKS5 + HTTP 混合端口" disabled={isRunning || modeChanging || Boolean(runtimeAction)} onChange={changeInboundMode} />
          </div>
          <div className="flex items-center justify-between gap-2 border-t border-[var(--border-light)] pt-3 text-xs text-[var(--text-tertiary)]">
            <span>{isRunning ? '停止核心后可修改' : '当前模式'}</span>
            <span className="rounded-md bg-[var(--bg-base)] px-2 py-1 text-[var(--text-secondary)]">{inboundModeLabels[inboundMode] || '--'}</span>
          </div>
        </Panel>

        <Panel className="flex h-full min-h-0 flex-col p-5 xl:col-span-4">
          <PanelTitle title="高级维护" extra={<span className="text-xs text-[var(--text-tertiary)]">{isWindows ? 'Windows' : runtime?.platform || '系统'}</span>} />
          <p className="text-xs leading-5 text-[var(--text-tertiary)]">连接与系统网络维护操作</p>
          <div className="my-auto grid grid-cols-2 gap-2">
            <RuntimeAction icon={<ShieldCheck size={14} />} label="关闭连接" loading={runtimeAction === '关闭连接'} disabled={Boolean(runtimeAction) || !isRunning} onClick={() => doAction(api.closeConnections, '关闭连接')} />
            <RuntimeAction icon={<RefreshCw size={14} />} label="重置防火墙" loading={runtimeAction === '重置防火墙'} disabled={Boolean(runtimeAction) || maintenanceUnsupported} title={maintenanceUnsupported ? '仅 Windows 支持此操作' : '恢复 Windows 防火墙默认规则'} onClick={() => setConfirmFirewallReset(true)} />
            <RuntimeAction icon={<DatabaseZap size={14} />} label="清理 DNS 缓存" loading={runtimeAction === '清理 DNS 缓存'} disabled={Boolean(runtimeAction) || maintenanceUnsupported} title={maintenanceUnsupported ? '仅 Windows 支持此操作' : undefined} onClick={() => doAction(api.flushDNS, '清理 DNS 缓存')} />
            <RuntimeAction icon={<FileCheck2 size={14} />} label="检查更新" loading={runtimeAction === '检查更新'} disabled={Boolean(runtimeAction) || isNotInstalled} onClick={() => doAction(api.checkUpdate, '检查更新')} />
          </div>
          <div className="border-t border-[var(--border-light)] pt-3 text-xs text-[var(--text-tertiary)]">
            关闭连接不会重启核心
          </div>
        </Panel>
        <ControlOverview refreshKey={overviewRefreshKey} installationPanel={installationPanel} configStatus={configStatus} proxyMode={proxyMode} onResourcesChanged={() => {
          setOverviewRefreshKey(value => value + 1);
          api.getConfigStatus().then(setConfigStatus).catch(() => {});
        }} onMessage={showMessage} />
        <ControlNetworkOverview isRunning={isRunning} proxyPort={runtime?.proxy_port || 0} onMessage={showMessage} />
      </div>

    </div>
  );
}

export default ControlPage;
