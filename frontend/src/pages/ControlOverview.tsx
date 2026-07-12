import React from 'react';
import { Activity, ArrowUpRight, CircleAlert, FileCheck2, FileCode, Globe2, Layers, ListChecks, Network, RadioTower, RefreshCw, Rss, ServerCog, Settings, ShieldCheck } from 'lucide-react';
import { Link } from 'react-router-dom';
import { api } from '@/services/api';

interface ControlOverviewProps {
  refreshKey: number;
  installationPanel: React.ReactNode;
  configStatus: { has_config: boolean; valid: boolean } | null;
  proxyMode: string;
  onResourcesChanged: () => void;
  onMessage: (message: string, type: 'success' | 'error' | 'info') => void;
}

interface OverviewSummary {
  totalNodes: number;
  enabledNodes: number;
  availableNodes: number;
  subscriptions: number;
  failedSubscriptions: number;
  syncingSubscriptions: number;
  rules: number;
  enabledRules: number;
  ruleResources: number;
  readyRuleResources: number;
  subscriptionTrafficUsed: number;
  subscriptionTrafficTotal: number;
  expiringSubscriptions: number;
}

const quickLinks = [
  { to: '/subscriptions', label: '订阅管理', detail: '同步与流量信息', icon: RadioTower },
  { to: '/nodes', label: '节点管理', detail: '可用性与启用状态', icon: Network },
  { to: '/rules', label: '规则管理', detail: '分流、规则集与 Geo', icon: ListChecks },
  { to: '/collections', label: '策略组管理', detail: '代理策略与节点选择', icon: Layers },
  { to: '/dns', label: 'DNS 管理', detail: '服务器与解析规则', icon: ServerCog },
  { to: '/config', label: '配置生成', detail: '生成与校验配置', icon: FileCode },
  { to: '/logs', label: '日志', detail: '核心与系统日志', icon: Activity },
  { to: '/settings', label: '设置', detail: '更新与节点过滤', icon: Settings },
];

function resultValue<T>(result: PromiseSettledResult<T>): T | null {
  return result.status === 'fulfilled' ? result.value : null;
}

function resultError(result: PromiseSettledResult<unknown>) {
  if (result.status === 'fulfilled') return '';
  return result.reason instanceof Error ? result.reason.message : '请求失败';
}

export function ControlOverview({ refreshKey, installationPanel, configStatus, proxyMode, onResourcesChanged, onMessage }: ControlOverviewProps) {
  const [summary, setSummary] = React.useState<OverviewSummary | null>(null);
  const [error, setError] = React.useState('');
  const [runningAction, setRunningAction] = React.useState('');
  const mountedRef = React.useRef(true);

  const loadSummary = React.useCallback(async () => {
    const results = await Promise.allSettled([
      api.getNodeFacets(),
      api.getNodes({ enabled: true, limit: 1 }),
      api.getNodes({ status: 'available', limit: 1 }),
      api.getSubscriptions(),
      api.getRouteRules(),
      api.getRouteRuleSubscriptions(),
      api.getGeoAssets(),
    ] as const);
    if (!mountedRef.current) return;

    const facets = resultValue(results[0]);
    const enabledNodes = resultValue(results[1]);
    const availableNodes = resultValue(results[2]);
    const subscriptions = resultValue(results[3])?.filter(item => item.url !== 'manual://local');
    const rules = resultValue(results[4]);
    const ruleSubscriptions = resultValue(results[5]);
    const geoAssets = resultValue(results[6]);
    const enabledRuleSubscriptions = ruleSubscriptions?.filter(item => item.enabled) || [];
    const resources = [...enabledRuleSubscriptions, ...(geoAssets || [])];
    const readyResources = resources.filter(item => item.cached_updated_at > 0 && item.sync_status !== 'failed');
    const expiryThreshold = Date.now() + 7 * 24 * 60 * 60 * 1000;

    const haveAllRuleResources = ruleSubscriptions !== null && geoAssets !== null;
    setSummary(previous => ({
      totalNodes: facets?.total ?? previous?.totalNodes ?? 0,
      enabledNodes: enabledNodes?.total ?? previous?.enabledNodes ?? 0,
      availableNodes: availableNodes?.total ?? previous?.availableNodes ?? 0,
      subscriptions: subscriptions?.length ?? previous?.subscriptions ?? 0,
      failedSubscriptions: subscriptions?.filter(item => item.sync_status === 'failed').length ?? previous?.failedSubscriptions ?? 0,
      syncingSubscriptions: subscriptions?.filter(item => item.sync_status === 'syncing').length ?? previous?.syncingSubscriptions ?? 0,
      rules: rules?.length ?? previous?.rules ?? 0,
      enabledRules: rules?.filter(item => item.enabled).length ?? previous?.enabledRules ?? 0,
      ruleResources: haveAllRuleResources ? resources.length : previous?.ruleResources ?? 0,
      readyRuleResources: haveAllRuleResources ? readyResources.length : previous?.readyRuleResources ?? 0,
      subscriptionTrafficUsed: subscriptions?.reduce((total, item) => total + item.traffic_used_bytes, 0) ?? previous?.subscriptionTrafficUsed ?? 0,
      subscriptionTrafficTotal: subscriptions?.reduce((total, item) => total + item.traffic_total_bytes, 0) ?? previous?.subscriptionTrafficTotal ?? 0,
      expiringSubscriptions: subscriptions?.filter(item => item.expire_at > 0 && item.expire_at <= expiryThreshold).length ?? previous?.expiringSubscriptions ?? 0,
    }));

    const labels = ['节点统计', '启用节点', '可用节点', '订阅状态', '路由规则', '规则订阅', 'Geo 资源'];
    const failures = results
      .map((result, index) => result.status === 'rejected' ? `${labels[index]}: ${resultError(result)}` : '')
      .filter(Boolean);
    setError(failures.length > 0 ? `部分状态加载失败：${failures.join('；')}` : '');
  }, []);

  React.useEffect(() => {
    mountedRef.current = true;
    loadSummary();
    const timer = window.setInterval(loadSummary, 60000);
    return () => {
      mountedRef.current = false;
      window.clearInterval(timer);
    };
  }, [loadSummary]);

  React.useEffect(() => {
    if (refreshKey > 0) loadSummary();
  }, [loadSummary, refreshKey]);

  const runAction = async (key: string, label: string, action: () => Promise<unknown>, background = false) => {
    setRunningAction(key);
    if (background) onMessage(`${label}任务启动中`, 'info');
    try {
      await action();
      if (!background) onMessage(`${label}成功`, 'success');
    } catch (actionError: any) {
      onMessage(`${label}失败：${actionError?.message || '请求失败'}`, 'error');
    } finally {
      onResourcesChanged();
      setRunningAction('');
    }
  };

  const quickActions = [
    { key: 'subscriptions', label: '同步节点订阅', detail: '拉取全部外部订阅', icon: Rss, action: api.syncAllSubscriptions, background: true },
    {
      key: 'validate',
      label: '校验当前配置',
      detail: '执行 sing-box check',
      icon: FileCheck2,
      action: async () => {
        const status = await api.validateConfig();
        if (!status.valid) throw new Error(status.error || '当前配置校验未通过');
        return status;
      },
    },
    { key: 'rules', label: '更新规则订阅', detail: '刷新全部远程规则集', icon: ShieldCheck, action: api.syncAllRouteRuleSubscriptions, background: true },
    { key: 'geo', label: '更新 Geo 资源', detail: '刷新 GeoIP 与 Geosite', icon: Globe2, action: api.syncAllGeoAssets, background: true },
  ];

  const configReady = configStatus === null ? null : configStatus.has_config && configStatus.valid;
  const warnings: Array<{ label: string; detail: string; to: string }> = [];
  if (configReady === false) warnings.push({ label: '配置尚未就绪', detail: configStatus?.has_config ? '当前配置校验未通过' : '需要先生成可用配置', to: '/config' });
  if (summary && summary.enabledNodes === 0) warnings.push({ label: '没有启用节点', detail: `节点池共 ${summary.totalNodes} 个节点`, to: '/nodes' });
  if (summary && summary.failedSubscriptions > 0) warnings.push({ label: `${summary.failedSubscriptions} 个订阅同步失败`, detail: '查看订阅页中的具体失败原因', to: '/subscriptions' });
  if (summary && summary.expiringSubscriptions > 0) warnings.push({ label: `${summary.expiringSubscriptions} 个订阅即将到期`, detail: '到期时间不足 7 天', to: '/subscriptions' });
  if (summary && proxyMode === 'rule' && summary.enabledRules === 0) warnings.push({ label: '规则模式没有启用规则', detail: '流量将只使用默认出站策略', to: '/rules' });
  if (summary && summary.readyRuleResources < summary.ruleResources) warnings.push({ label: '规则资源未全部就绪', detail: `${summary.readyRuleResources}/${summary.ruleResources} 个规则集或 Geo 资源可用`, to: '/rules' });

  return (
    <>
        <div className="order-1 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4">
          <h3 className="mb-2 text-sm font-semibold text-[var(--text-primary)]">快捷入口</h3>
          {error && <div className="mb-1.5 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] px-2.5 py-1.5 text-[11px] text-[var(--color-error)]">{error}</div>}
          <div className="grid gap-1.5 sm:grid-cols-2">
            {quickLinks.map(item => {
              const Icon = item.icon;
              return (
                <Link key={item.to} to={item.to} className="group flex items-center gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-light)] px-3 py-1.5 transition hover:border-[var(--color-primary)] hover:bg-[var(--color-primary-bg)]">
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] text-[var(--color-primary)]"><Icon size={15} /></span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-xs font-medium text-[var(--text-primary)]">{item.label}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-[var(--text-tertiary)]">{item.detail}</span>
                  </span>
                  <ArrowUpRight size={13} className="shrink-0 text-[var(--text-tertiary)] group-hover:text-[var(--color-primary)]" />
                </Link>
              );
            })}
          </div>
        </div>

        <div className="order-2 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4">
          <h3 className="mb-2 text-sm font-semibold text-[var(--text-primary)]">快捷任务</h3>
          <div className="grid flex-1 grid-cols-1 grid-rows-4 gap-1.5">
            {quickActions.map(item => {
              const Icon = item.icon;
              const busy = runningAction === item.key;
              return (
                <button key={item.key} type="button" disabled={Boolean(runningAction)} onClick={() => runAction(item.key, item.label, item.action, 'background' in item && item.background)} className="group flex h-full items-center gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-3 py-1.5 text-left transition hover:border-[var(--color-primary)] hover:bg-[var(--color-primary-bg)] disabled:cursor-not-allowed disabled:opacity-50">
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] text-[var(--color-primary)]">
                    {busy ? <RefreshCw size={15} className="animate-spin" /> : <Icon size={15} />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-xs font-medium text-[var(--text-primary)]">{item.label}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-[var(--text-tertiary)]">{item.detail}</span>
                  </span>
                </button>
              );
            })}
          </div>
        </div>

        <div className="order-6 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)] xl:col-span-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">待处理事项</h3>
            <span className={`rounded-[var(--radius-full)] px-2 py-0.5 text-[11px] ${warnings.length === 0 ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]' : 'bg-[var(--color-warning-bg)] text-[var(--color-warning)]'}`}>{warnings.length === 0 ? '状态良好' : `${warnings.length} 项`}</span>
          </div>
          {warnings.length === 0 ? (
            <div className="flex items-center justify-center gap-2 rounded-[var(--radius-md)] bg-[var(--color-success-bg)] px-3 py-3 text-center text-xs text-[var(--color-success)]">
              无
            </div>
          ) : (
            <div className="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
              {warnings.map(item => (
                <Link key={item.label} to={item.to} className="group flex items-center gap-3 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-3 py-2 transition hover:border-[var(--color-warning)] hover:bg-[var(--color-warning-bg)]">
                  <CircleAlert size={15} className="shrink-0 text-[var(--color-warning)]" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs font-medium text-[var(--text-primary)]">{item.label}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-[var(--text-tertiary)]">{item.detail}</span>
                  </span>
                  <ArrowUpRight size={13} className="shrink-0 text-[var(--text-tertiary)] group-hover:text-[var(--color-warning)]" />
                </Link>
              ))}
            </div>
          )}
        </div>

        <div className="order-7 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)] xl:col-span-4">
          {installationPanel}
        </div>
    </>
  );
}
