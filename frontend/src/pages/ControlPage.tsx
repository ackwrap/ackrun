import React from 'react';
import { Play, Square, RotateCcw, Download, AlertTriangle, Power, RefreshCw, FileCheck2, ShieldCheck, DatabaseZap } from 'lucide-react';
import { useRealtimeSocket } from '@/hooks/useRealtimeSocket';
import { api } from '@/services/api';
import type { WSEvent } from '@/services/types';
import { Button } from '@/components/ui/Button';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { PageHeader } from '@/components/layout/PageHeader';
import { Toast } from '@/components/ui/Toast';

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

function RuntimeAction({ icon, label, tone = 'default', disabled = false, onClick }: { icon: React.ReactNode; label: string; tone?: 'default' | 'danger'; disabled?: boolean; onClick?: () => void }) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={`inline-flex h-8 items-center justify-center gap-1.5 rounded-[6px] border px-2.5 text-xs font-medium transition ${
        tone === 'danger'
          ? 'border-red-400/25 bg-red-500/10 text-red-200 hover:border-red-300/40 hover:bg-red-500/15'
          : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:border-blue-400/30 hover:bg-blue-500/10 hover:text-white'
      }`}
    >
      {icon}{label}
    </button>
  );
}

function ModeOption({ name, value, checked, title, description, recommended = false, onChange }: { name: string; value: string; checked: boolean; title: string; description: string; recommended?: boolean; onChange: (value: string) => void }) {
  return (
    <label
      className={`flex cursor-pointer items-center justify-between gap-3 rounded-lg border px-3 py-2.5 transition ${
        checked
          ? 'border-blue-400/45 bg-blue-500/10 text-[var(--text-primary)]'
          : 'border-transparent bg-[var(--bg-base)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--bg-sidebar-hover)]'
      }`}
    >
      <input
        type="radio"
        name={name}
        value={value}
        checked={checked}
        onChange={(event) => onChange(event.target.value)}
        className="sr-only"
      />
      <span className="min-w-0">
        <span className="flex items-center gap-1.5">
          <span className="text-sm font-semibold">{title}</span>
          {recommended && <span className="rounded bg-blue-500/15 px-1.5 py-0.5 text-[10px] text-blue-200">推荐</span>}
        </span>
        <span className="mt-0.5 block text-xs text-[var(--text-tertiary)]">{description}</span>
      </span>
      <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${checked ? 'bg-blue-300 shadow-[0_0_8px_rgba(96,165,250,0.7)]' : 'bg-[var(--border-default)]'}`} />
    </label>
  );
}

export function ControlPage() {
  const [runtime, setRuntime] = React.useState<{ status: string; pid?: number; version?: string } | null>(null);
  const [installStatus, setInstallStatus] = React.useState<{ status: string; version?: string; progress?: number; message?: string; error?: string } | null>(null);
  const [configStatus, setConfigStatus] = React.useState<{ has_config: boolean; valid: boolean; file_name?: string; updated_at?: number; error?: string } | null>(null);
  const [installProgress, setInstallProgress] = React.useState<{ percent: number; downloaded_bytes: number; total_bytes: number } | null>(null);
  const [guideDismissed, setGuideDismissed] = React.useState(false);
  const [inboundMode, setInboundMode] = React.useState<string>('tun_mixed');
  const [proxyMode, setProxyMode] = React.useState<string>('rule');
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error' | 'info'>('success');

  useRealtimeSocket((event: WSEvent) => {
    console.log('[WS]', event.type, event.data);
    switch (event.type) {
      case 'runtime.status': setRuntime(event.data as any); break;
      case 'installer.status': setInstallStatus(event.data as any); break;
      case 'installer.progress': setInstallProgress(event.data as any); break;
      case 'core.status': {
        const d = event.data as any;
        console.log('[WS] core.status -> mapping to runtime', d.status, d.pid);
        setRuntime(prev => ({ ...prev, status: d.status, pid: d.pid || 0 }));
        break;
      }
      case 'config.status': setConfigStatus(event.data as any); break;
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
      } catch {
        // API errors are surfaced by action feedback and websocket updates.
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
  const view = runtimeStatusMap[rt] || runtimeStatusMap.not_installed;
  const configUpdatedAt = configStatus?.updated_at ? new Date(configStatus.updated_at).toLocaleString() : '--';
  const configName = configStatus?.file_name || (configStatus?.has_config ? 'config.json' : '--');
  const showInstallGuide = !guideDismissed && isRuntimeChecked && (isNotInstalled || isInstalling);
  const showConfigGuide = !guideDismissed && !isNotInstalled && !isInstalling && configStatus?.has_config === false;

  const doAction = async (fn: () => Promise<any>, label: string) => {
    try {
      const res = await fn();
      showMessage(`${label} 成功${res?.message ? `: ${res.message}` : ''}`);
    } catch (e: any) {
      showMessage(`${label} 失败: ${e.message}`, 'error');
    }
    setTimeout(() => {
      console.log('[REST] re-fetching runtime status after action:', label);
      api.getRuntime().then(r => { console.log('[REST] runtime:', r); setRuntime(r); }).catch(() => {});
      api.getConfigStatus().then(c => setConfigStatus(c)).catch(() => {});
      api.getInstallerStatus().then(i => setInstallStatus(i)).catch(() => {});
    }, 1000);
  };

  const changeInboundMode = async (mode: string) => {
    try {
      await api.setInboundMode(mode);
      setInboundMode(mode);
      showMessage(`运行模式已切换为 ${inboundModeLabels[mode]}`);
    } catch (e: any) {
      showMessage(`切换运行模式失败: ${e.message}`, 'error');
    }
  };

  const changeProxyMode = async (mode: string) => {
    try {
      await api.setProxyMode(mode);
      setProxyMode(mode);
      showMessage(`代理模式已切换为 ${proxyModeLabels[mode]}`);
    } catch (e: any) {
      showMessage(`切换代理模式失败: ${e.message}`, 'error');
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

  return (
    <div className="flex flex-col h-full">
      <Toast message={message} type={messageType} />
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

            {showInstallGuide && installProgress && (
              <div className="mt-4">
                <div className="h-2 overflow-hidden rounded-full bg-white/[0.08]">
                  <div className="h-full rounded-full bg-blue-400 transition-all" style={{ width: `${Math.min(100, installProgress.percent)}%` }} />
                </div>
                <div className="mt-1 text-xs text-[var(--text-tertiary)]">
                  {installProgress.percent.toFixed(1)}%
                  {installProgress.total_bytes > 0 && ` · ${(installProgress.downloaded_bytes / 1024 / 1024).toFixed(1)} MB / ${(installProgress.total_bytes / 1024 / 1024).toFixed(1)} MB`}
                </div>
              </div>
            )}

            <div className="mt-5 flex items-center justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => setGuideDismissed(true)}>稍后处理</Button>
              {showInstallGuide ? (
                <Button size="sm" variant="primary" icon={<Download size={14} />} loading={isInstalling} disabled={isInstalling} onClick={() => doAction(api.install, '安装')}>{isInstalling ? '安装中...' : '安装 sing-box'}</Button>
              ) : (
                <Button size="sm" variant="primary" disabled={isRunning} onClick={() => doAction(api.generateDefaultConfig, '生成默认配置')}>生成默认配置</Button>
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

      <div className="shrink-0 grid grid-cols-1 gap-4 mt-4 lg:grid-cols-3">
        {/* Status Panel */}
        <Panel className="h-full">
          <div className="mb-2 text-sm font-semibold text-white">服务状态</div>
          <div className="flex flex-1 flex-col justify-between gap-2">
            <div className="flex items-center gap-3">
              <div className={`relative flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${isRunning ? 'bg-emerald-500/15 text-emerald-300' : 'bg-slate-500/15 text-slate-300'}`}>
                <Power size={18} />
                <span className={`absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full ${view.dot} ${isRunning ? 'animate-status-pulse' : ''}`} />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-base font-semibold text-white">sing-box</span>
                  <span className={`rounded-md border px-2 py-0.5 text-xs font-medium ${view.badge}`}>{view.label}</span>
                </div>
              </div>
            </div>
            <div className="grid grid-cols-3 gap-2">
              <div className="rounded-lg bg-white/[0.04] px-3 py-1">
                <div className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">进程ID</div>
                <div className="mt-0.5 text-sm font-semibold text-white">{runtime?.pid || '-'}</div>
              </div>
              <div className="rounded-lg bg-white/[0.04] px-3 py-1">
                <div className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">状态</div>
                <div className="mt-0.5 text-sm font-semibold text-white">{view.label}</div>
              </div>
              <div className="rounded-lg bg-white/[0.04] px-3 py-1">
                <div className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">版本</div>
                <div className="mt-0.5 text-sm font-semibold text-white">{runtime?.version || installStatus?.version || '--'}</div>
              </div>
            </div>
          </div>
        </Panel>

        {/* Actions Panel */}
        <Panel className="h-full">
          <div className="mb-2">
            <div className="text-sm font-semibold text-white">服务操作</div>
            <div className="mt-0.5 text-xs text-[var(--text-tertiary)]">控制 sing-box 核心运行状态</div>
          </div>
          <div className="grid grid-cols-2 gap-x-2 gap-y-3">
            <RuntimeAction icon={<Play size={14} />} label="启动核心" disabled={isRunning || isNotInstalled || isNoConfig} onClick={() => doAction(api.startCore, '启动')} />
            <RuntimeAction icon={<Square size={14} />} label="停止核心" tone="danger" disabled={!isRunning} onClick={() => doAction(api.stopCore, '停止')} />
            <RuntimeAction icon={<RotateCcw size={14} />} label="重启核心" disabled={!isRunning} onClick={() => doAction(api.restartCore, '重启')} />
            <RuntimeAction icon={<RefreshCw size={14} />} label="重载配置" disabled={isNotInstalled || isNoConfig} onClick={() => doAction(api.reloadConfig, '重载配置')} />
          </div>
        </Panel>

        {/* 高级维护 */}
        <Panel className="h-full">
          <div className="mb-2">
            <div className="text-sm font-semibold text-white">高级维护</div>
            <div className="mt-0.5 text-xs text-[var(--text-tertiary)]">低频维护动作，按需使用</div>
          </div>
          <div className="grid grid-cols-2 gap-x-2 gap-y-3">
            <RuntimeAction icon={<ShieldCheck size={14} />} label="关闭连接" disabled={!isRunning} onClick={() => doAction(api.closeConnections, '关闭连接')} />
            <RuntimeAction icon={<RefreshCw size={14} />} label="重置防火墙" onClick={() => doAction(api.resetFirewall, '重置防火墙')} />
            <RuntimeAction icon={<DatabaseZap size={14} />} label="清理DNS缓存" onClick={() => doAction(api.flushDNS, '清理 DNS 缓存')} />
            <RuntimeAction icon={<FileCheck2 size={14} />} label="检查更新" onClick={() => doAction(api.checkUpdate, '检查更新')} />
          </div>
        </Panel>
      </div>

      {/* 模式配置 */}
      <div className="shrink-0 grid grid-cols-1 gap-4 mt-4 md:grid-cols-2">
        <Panel className="h-full">
          <PanelTitle title="运行模式" extra={<span className="text-xs text-[var(--text-tertiary)]">Inbound</span>} />
          <div className="space-y-1.5">
            <ModeOption
              name="inbound-mode"
              value="tun_mixed"
              checked={inboundMode === 'tun_mixed'}
              title="TUN + Mixed"
              description="透明代理 + 端口"
              recommended
              onChange={changeInboundMode}
            />
            <ModeOption
              name="inbound-mode"
              value="tun"
              checked={inboundMode === 'tun'}
              title="TUN 模式"
              description="纯透明代理"
              onChange={changeInboundMode}
            />
            <ModeOption
              name="inbound-mode"
              value="mixed"
              checked={inboundMode === 'mixed'}
              title="Mixed 模式"
              description="SOCKS5 + HTTP 混合端口"
              onChange={changeInboundMode}
            />
          </div>
        </Panel>

        <Panel className="h-full">
          <PanelTitle title="代理模式" extra={<span className="text-xs text-[var(--text-tertiary)]">Route</span>} />
          <div className="space-y-1.5">
            <ModeOption
              name="proxy-mode"
              value="rule"
              checked={proxyMode === 'rule'}
              title="规则模式"
              description="根据规则自动分流"
              recommended
              onChange={changeProxyMode}
            />
            <ModeOption
              name="proxy-mode"
              value="global"
              checked={proxyMode === 'global'}
              title="全局模式"
              description="所有流量走代理"
              onChange={changeProxyMode}
            />
            <ModeOption
              name="proxy-mode"
              value="direct"
              checked={proxyMode === 'direct'}
              title="直连模式"
              description="所有流量直连"
              onChange={changeProxyMode}
            />
          </div>
        </Panel>
      </div>

      {/* 配置文件 + 安装维护 */}
      <div className="shrink-0 grid grid-cols-1 gap-4 mt-4 lg:grid-cols-2">
        {/* 配置文件 */}
        <Panel>
          <div className="mb-3 text-sm font-semibold text-white">配置文件</div>
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <span className={`h-2 w-2 shrink-0 rounded-full ${configStatus?.valid ? 'bg-blue-400' : configStatus?.has_config ? 'bg-red-400' : 'bg-orange-400'}`} />
              <div className="truncate text-lg font-semibold text-white">{configName}</div>
            </div>
            <div className="shrink-0 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-2 py-1 text-xs text-[var(--text-tertiary)]">
              更新时间: {configUpdatedAt}
            </div>
          </div>

          <div className="mt-3 flex items-center gap-2">
            <div className="flex h-9 min-w-0 flex-1 items-center rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-sm text-[var(--text-secondary)]">
              <span className="truncate">{configName}</span>
            </div>
            <button className="flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white" title="刷新状态" onClick={() => api.getConfigStatus().then(setConfigStatus).catch(() => {})}><RefreshCw size={14} /></button>
            <button className="flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white" title="重新生成" disabled={isRunning} onClick={() => doAction(api.generateDefaultConfig, '生成默认配置')}><RotateCcw size={14} /></button>
          </div>
        </Panel>

        {/* 安装维护 */}
        <Panel>
          <PanelTitle title="安装维护" extra={<span className="text-xs text-[var(--text-tertiary)]">GitHub Release</span>} />
          <div className="grid grid-cols-3 gap-2">
            <div className="rounded-lg bg-white/[0.04] px-3 py-2">
              <div className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">安装状态</div>
              <div className="mt-1 flex items-center gap-2 text-sm font-semibold text-white">
                <StatusBadge status={installStatus?.status === 'done' ? 'online' : installStatus?.status === 'failed' ? 'error' : isInstalling ? 'pending' : 'offline'} />
                <span className={installStatus?.status === 'failed' ? 'text-red-300' : ''}>{installLabel[installStatus?.status || 'idle'] || installStatus?.status || '未安装'}</span>
              </div>
            </div>
            <div className="rounded-lg bg-white/[0.04] px-3 py-2">
              <div className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">当前版本</div>
              <div className="mt-1 text-sm font-semibold text-white">{runtime?.version || installStatus?.version || '--'}</div>
            </div>
            <div className="rounded-lg bg-white/[0.04] px-3 py-2">
              <div className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-tertiary)]">最新版本</div>
              <div className="mt-1 text-sm font-semibold text-white">--</div>
            </div>
          </div>

          {installStatus?.error && <div className="mt-2 text-xs text-red-300">{installStatus.error}</div>}
          {installProgress && (
            <div className="mt-3">
              <div className="h-2 overflow-hidden rounded-full bg-white/[0.08]">
                <div className="h-full rounded-full bg-blue-400 transition-all" style={{ width: `${Math.min(100, installProgress.percent)}%` }} />
              </div>
              <div className="mt-1 text-xs text-[var(--text-tertiary)]">
                {installProgress.percent.toFixed(1)}%
                {installProgress.total_bytes > 0 && ` · ${(installProgress.downloaded_bytes / 1024 / 1024).toFixed(1)} MB / ${(installProgress.total_bytes / 1024 / 1024).toFixed(1)} MB`}
              </div>
            </div>
          )}

          <div className="mt-3 flex justify-end">
            <Button variant="primary" size="sm" icon={<Download size={14} />} disabled={isInstalling || isRunning} onClick={() => doAction(api.install, '安装')} loading={isInstalling}>
              {isInstalling ? '安装中...' : installStatus?.status === 'done' ? '重新安装' : '安装'}
            </Button>
          </div>
        </Panel>
      </div>

    </div>
  );
}

export default ControlPage;
