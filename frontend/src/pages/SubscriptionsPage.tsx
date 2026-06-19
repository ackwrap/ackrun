import React from 'react';
import { useNavigate } from 'react-router-dom';
import { PageHeader } from '@/components/layout/PageHeader';
import { Button } from '@/components/ui/Button';
import { SyncScheduleControls } from '@/components/ui/SyncScheduleControls';
import { api } from '@/services/api';
import { useRealtimeSocket } from '@/hooks/useRealtimeSocket';
import type { NodeImportPreviewItem, Subscription, UserAgentOption, WSEvent, NodeFilter } from '@/services/types';
import { Edit3, Eye, FileJson, Link2, Plus, RefreshCw, Trash2, Upload, X } from 'lucide-react';
import type { ReactNode } from 'react';

const columns = ['名称', '订阅链接', '节点数', '流量使用', '到期时间', '最后同步', '同步周期', '状态', '操作'];
const customUserAgentValue = '__custom__';
const manualSubscriptionURL = 'manual://local';
const syncModeOptions = [
  { value: 'off', label: '关闭自动同步' },
  { value: 'daily', label: '每天' },
  { value: 'weekly', label: '每周' },
  { value: 'monthly', label: '每月' },
];

const filterTargets = [
  { value: 'all', label: '全部字段' },
  { value: 'name', label: '节点名称' },
  { value: 'type', label: '协议类型' },
  { value: 'server', label: '服务器地址' },
  { value: 'raw', label: '原始内容' },
  { value: 'raw_json', label: '解析 JSON' },
];

const localSubscriptionPlaceholder: Subscription = {
  id: 0,
  name: '本地订阅',
  url: manualSubscriptionURL,
  user_agent: 'manual',
  sync_interval_minutes: 0,
  sync_mode: 'off',
  sync_time: '',
  sync_weekday: 0,
  sync_status: 'updated',
  sync_progress: 100,
  sync_timeout_seconds: 60,
  node_count: 0,
  traffic_used_bytes: 0,
  traffic_total_bytes: 0,
  expire_at: 0,
  last_sync_at: 0,
  created_at: 0,
  updated_at: 0,
};

function IconButton({ title, children, onClick }: { title: string; children: ReactNode; onClick?: () => void }) {
  const disabled = !onClick;
  return (
    <button disabled={disabled} title={title} onClick={onClick} className={`inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] ${disabled ? 'cursor-not-allowed opacity-40' : 'hover:border-blue-400/30 hover:bg-blue-500/10 hover:text-white'}`}>
      {children}
    </button>
  );
}

function formatTime(value: number) {
  return value > 0 ? new Date(value).toLocaleString() : '--';
}

function formatTraffic(item: Subscription) {
  if (item.traffic_total_bytes <= 0) return { text: '--', percent: 0 };
  const percent = Math.min(100, item.traffic_used_bytes / item.traffic_total_bytes * 100);
  const used = (item.traffic_used_bytes / 1024 / 1024 / 1024).toFixed(1);
  const total = (item.traffic_total_bytes / 1024 / 1024 / 1024).toFixed(1);
  return { text: `${used} GB / ${total} GB`, percent };
}

const weekdayLabels: Record<number, string> = {
  1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日',
};
const subscriptionWeekdays = Object.entries(weekdayLabels).map(([value, label]) => ({ value: Number(value), label }));

function formatSyncSchedule(item: Subscription) {
  if (item.sync_mode === 'daily') return `每天 ${item.sync_time || '--'}`;
  if (item.sync_mode === 'weekly') return `每周${weekdayLabels[item.sync_weekday] || ''} ${item.sync_time || '--'}`;
  if (item.sync_mode === 'monthly') return `每月${item.sync_weekday || 1}号 ${item.sync_time || '--'}`;
  return '关闭';
}

function formatSyncStatus(item: Subscription) {
  if (item.sync_status === 'syncing') return `同步中 ${Math.round(item.sync_progress || 0)}%`;
  if (item.sync_status === 'failed') return '失败';
  return '已更新';
}

function prettyJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value || '{}'), null, 2);
  } catch {
    return value || '{}';
  }
}

function toYAML(value: unknown, indent = 0): string {
  const pad = ' '.repeat(indent);
  if (Array.isArray(value)) {
    return value.map(item => {
      if (item && typeof item === 'object') return `${pad}-\n${toYAML(item, indent + 2)}`;
      return `${pad}- ${String(item)}`;
    }).join('\n');
  }
  if (value && typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>).map(([key, val]) => {
      if (val && typeof val === 'object') return `${pad}${key}:\n${toYAML(val, indent + 2)}`;
      if (typeof val === 'string') return `${pad}${key}: ${val}`;
      return `${pad}${key}: ${String(val)}`;
    }).join('\n');
  }
  return `${pad}${String(value)}`;
}

function previewYAML(value: string) {
  try {
    return toYAML(JSON.parse(value || '{}'));
  } catch {
    return value || '{}';
  }
}

export function SubscriptionsPage() {
  const navigate = useNavigate();
  const [subscriptions, setSubscriptions] = React.useState<Subscription[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [editing, setEditing] = React.useState<Subscription | null>(null);
  const [formOpen, setFormOpen] = React.useState(false);
  const [name, setName] = React.useState('');
  const [url, setURL] = React.useState('');
  const [userAgent, setUserAgent] = React.useState('clash-meta/2.4.0');
  const [userAgentPreset, setUserAgentPreset] = React.useState('clash-meta/2.4.0');
  const [userAgentOptions, setUserAgentOptions] = React.useState<UserAgentOption[]>([]);
  const [syncMode, setSyncMode] = React.useState<'off' | 'daily' | 'weekly' | 'monthly'>('off');
  const [syncTime, setSyncTime] = React.useState('00:00:00');
  const [syncWeekday, setSyncWeekday] = React.useState('1');
  const [syncTimeout, setSyncTimeout] = React.useState('60');
  const [deleteTarget, setDeleteTarget] = React.useState<Subscription | null>(null);
  const [syncErrors, setSyncErrors] = React.useState<Record<number, string>>({});
  const [importContent, setImportContent] = React.useState('');
  const [previewItems, setPreviewItems] = React.useState<NodeImportPreviewItem[]>([]);
  const [previewLoading, setPreviewLoading] = React.useState(false);
  const [previewDetail, setPreviewDetail] = React.useState<NodeImportPreviewItem | null>(null);
  const [previewDetailFormat, setPreviewDetailFormat] = React.useState<'json' | 'yaml'>('json');
  
  // 节点过滤规则状态
  const [filters, setFilters] = React.useState<NodeFilter[]>([]);
  const [editingFilter, setEditingFilter] = React.useState<NodeFilter | null>(null);
  const [filterName, setFilterName] = React.useState('');
  const [filterTarget, setFilterTarget] = React.useState('name');
  const [filterPattern, setFilterPattern] = React.useState('');
  const [filterEnabled, setFilterEnabled] = React.useState(true);

  useRealtimeSocket((event: WSEvent) => {
    if (event.type !== 'subscription.sync') return;
    const data = event.data as Partial<Subscription> & { id?: number; status?: string; progress?: number; error?: string; warning?: string };
    if (!data.id) return;
    setSyncErrors(prev => {
      if (data.error) return { ...prev, [data.id!]: data.error };
      if (data.warning) return { ...prev, [data.id!]: data.warning };
      if (data.status === 'syncing' || data.status === 'updated') {
        const next = { ...prev };
        delete next[data.id!];
        return next;
      }
      return prev;
    });
    setSubscriptions(prev => prev.map(item => item.id === data.id ? {
      ...item,
      sync_status: data.status || item.sync_status,
      sync_progress: data.progress ?? item.sync_progress,
      node_count: data.node_count ?? item.node_count,
      traffic_used_bytes: data.traffic_used_bytes ?? item.traffic_used_bytes,
      traffic_total_bytes: data.traffic_total_bytes ?? item.traffic_total_bytes,
      expire_at: data.expire_at ?? item.expire_at,
      last_sync_at: data.last_sync_at ?? (data.status === 'updated' ? Date.now() : item.last_sync_at),
    } : item));
  });

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      setSubscriptions(await api.getSubscriptions());
    } catch (e: any) {
      setMessage(`加载失败: ${e.message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadFilters = React.useCallback(async () => {
    try {
      setFilters(await api.getNodeFilters());
    } catch (e: any) {
      setMessage(`过滤规则加载失败: ${e.message}`);
    }
  }, []);

  React.useEffect(() => { 
    load();
    loadFilters();
  }, [load, loadFilters]);

  React.useEffect(() => {
    api.getSubscriptionUserAgents()
      .then(options => setUserAgentOptions(options))
      .catch(() => setUserAgentOptions([]));
  }, []);

  const setUserAgentFromValue = (value: string) => {
    const isPreset = value === 'clash-meta/2.4.0' || userAgentOptions.some(option => option.value === value);
    setUserAgentPreset(isPreset ? value : customUserAgentValue);
    setUserAgent(value || 'clash-meta/2.4.0');
  };

  const selectUserAgentPreset = (value: string) => {
    setUserAgentPreset(value);
    if (value !== customUserAgentValue) {
      setUserAgent(value);
    }
  };

  const openCreate = () => {
    setEditing(null);
    setName('');
    setURL('');
    setUserAgentFromValue('clash-meta/2.4.0');
    setSyncMode('off');
    setSyncTime('00:00:00');
    setSyncWeekday('1');
    setSyncTimeout('60');
    setFormOpen(true);
  };

  const openEdit = (item: Subscription) => {
    setEditing(item);
    setName(item.name);
    setURL(item.url);
    setUserAgentFromValue(item.user_agent || 'clash-meta/2.4.0');
    setSyncMode(item.sync_mode === 'daily' || item.sync_mode === 'weekly' || item.sync_mode === 'monthly' ? item.sync_mode : 'off');
    setSyncTime(item.sync_time || '00:00:00');
    setSyncWeekday(String(item.sync_weekday || 1));
    setSyncTimeout(String(item.sync_timeout_seconds || 60));
    setFormOpen(true);
  };

  const save = async () => {
    try {
      const payload = {
        name,
        url,
        user_agent: userAgent,
        sync_mode: syncMode,
        sync_time: syncMode === 'off' ? '' : syncTime,
        sync_weekday: syncMode === 'weekly' || syncMode === 'monthly' ? Number(syncWeekday) : 0,
        sync_timeout_seconds: Number(syncTimeout) || 60,
      } as const;
      if (editing) {
        await api.updateSubscription(editing.id, payload);
        setMessage('订阅已更新');
      } else {
        await api.createSubscription(payload);
        setMessage('订阅已添加，正在同步节点');
      }
      setFormOpen(false);
      await load();
    } catch (e: any) {
      setMessage(`保存失败: ${e.message}`);
    }
  };

  const syncOne = async (item: Subscription) => {
    try {
      setSubscriptions(prev => prev.map(row => row.id === item.id ? { ...row, sync_status: 'syncing', sync_progress: 0 } : row));
      setSyncErrors(prev => {
        const next = { ...prev };
        delete next[item.id];
        return next;
      });
      await api.syncSubscription(item.id);
    } catch (e: any) {
      setMessage(`同步失败: ${e.message}`);
      setSyncErrors(prev => ({ ...prev, [item.id]: e.message }));
      setSubscriptions(prev => prev.map(row => row.id === item.id ? { ...row, sync_status: 'failed', sync_progress: 0 } : row));
    }
  };

  const syncAll = async () => {
    try {
      setSubscriptions(prev => prev.map(row => ({ ...row, sync_status: 'syncing', sync_progress: 0 })));
      setSyncErrors({});
      await api.syncAllSubscriptions();
    } catch (e: any) {
      setMessage(`同步失败: ${e.message}`);
      await load();
    }
  };

  const remoteSubscriptions = React.useMemo(() => subscriptions.filter(item => item.url !== manualSubscriptionURL), [subscriptions]);
  const manualSubscription = React.useMemo(() => subscriptions.find(item => item.url === manualSubscriptionURL), [subscriptions]);
  const displayedSubscriptions = React.useMemo(() => [manualSubscription || localSubscriptionPlaceholder, ...remoteSubscriptions], [manualSubscription, remoteSubscriptions]);
  const anySyncing = remoteSubscriptions.some(item => item.sync_status === 'syncing');

  const importNodes = async () => {
    try {
      const result = await api.importNodes({ content: importContent });
      setMessage(`手动导入完成：导入 ${result.imported} 个节点`);
      setImportContent('');
      setPreviewItems([]);
      await load();
    } catch (e: any) {
      setMessage(`手动导入失败: ${e.message}`);
    }
  };

  const previewImportNodes = async () => {
    setPreviewLoading(true);
    try {
      const result = await api.previewImportNodes({ content: importContent });
      setPreviewItems(result.items);
      setMessage(`预览完成：识别到 ${result.count} 个节点`);
    } catch (e: any) {
      setPreviewItems([]);
      setMessage(`节点预览失败: ${e.message}`);
    } finally {
      setPreviewLoading(false);
    }
  };

  const importLineCount = React.useMemo(() => importContent.split('\n').filter(line => line.trim()).length, [importContent]);

  const clearImportContent = () => {
    setImportContent('');
    setPreviewItems([]);
    setPreviewDetail(null);
  };

  const remove = async () => {
    if (!deleteTarget) return;
    try {
      await api.deleteSubscription(deleteTarget.id);
      setMessage('订阅已删除');
      setDeleteTarget(null);
      await load();
    } catch (e: any) {
      setMessage(`删除失败: ${e.message}`);
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader title="订阅管理" />
      <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
        <div className="mb-4 flex items-start justify-between gap-3">
          <p className="text-sm text-[var(--text-secondary)]">管理从节点管理导入的外部订阅源，用于从第三方订阅同步节点</p>
          <div className="flex items-center gap-2">
            <button onClick={openCreate} className="inline-flex h-9 w-11 items-center justify-center rounded-none border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white">
              <Plus size={17} />
            </button>
            <button disabled={anySyncing || remoteSubscriptions.length === 0} onClick={syncAll} className={`inline-flex h-9 items-center justify-center gap-2 rounded-none border border-[var(--border-default)] bg-white/[0.04] px-4 text-sm font-medium text-white ${anySyncing || remoteSubscriptions.length === 0 ? 'cursor-not-allowed opacity-40' : 'hover:bg-white/[0.08]'}`}>
              <RefreshCw size={15} />同步所有订阅
            </button>
          </div>
        </div>

        {message && <div className="mb-3 rounded-md bg-white/[0.04] px-3 py-2 text-xs text-[var(--text-secondary)]">{message}</div>}

        <div className="overflow-hidden rounded-none border border-[var(--border-default)]">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-white/[0.04] text-sm text-white">
              <tr>{columns.map((column) => <th key={column} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{column}</th>)}</tr>
            </thead>
            <tbody>
              {displayedSubscriptions.length === 0 ? (
                <tr>
                  <td colSpan={columns.length} className="px-4 py-14 text-center">
                    <div className="text-sm font-medium text-white">{loading ? '加载中...' : '暂无订阅'}</div>
                    <div className="mt-1 text-xs text-[var(--text-tertiary)]">点击“+”添加订阅后，这里会显示名称、链接、节点数、流量和同步状态。</div>
                  </td>
                </tr>
              ) : displayedSubscriptions.map((subscription) => {
                const traffic = formatTraffic(subscription);
                const isSyncing = subscription.sync_status === 'syncing';
                const isManual = subscription.url === manualSubscriptionURL;
                return (
                  <tr key={subscription.id} className={`border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 ${isManual ? 'bg-blue-500/[0.04]' : ''}`}>
                    <td className="px-4 py-3 font-medium text-white">{isManual ? '本地订阅' : subscription.name}</td>
                    <td className="max-w-[260px] truncate px-4 py-3 font-mono text-xs" title={subscription.url}>{isManual ? '本地节点源' : subscription.url}</td>
                    <td className="px-4 py-3">
                      <button disabled={subscription.node_count <= 0 || subscription.id <= 0} onClick={() => navigate(`/nodes?subscription_id=${subscription.id}`)} className={`rounded px-2 py-1 ${subscription.node_count > 0 && subscription.id > 0 ? 'bg-blue-500/10 text-blue-200 hover:bg-blue-500/20' : 'cursor-not-allowed bg-blue-500/10 text-blue-200/60'}`}>{subscription.node_count}</button>
                    </td>
                    <td className="px-4 py-3">
                      {isManual ? <span className="text-xs text-[var(--text-tertiary)]">--</span> : <><div className="h-2 w-20 overflow-hidden rounded-full bg-white/[0.08]"><div className="h-full bg-blue-400" style={{ width: `${traffic.percent}%` }} /></div><div className="mt-1 text-xs">{traffic.text}</div></>}
                    </td>
                    <td className="px-4 py-3 font-medium text-white">{isManual ? '--' : formatTime(subscription.expire_at)}</td>
                    <td className="px-4 py-3">{isManual ? '--' : formatTime(subscription.last_sync_at)}</td>
                    <td className="px-4 py-3">{isManual ? '本地' : formatSyncSchedule(subscription)}</td>
                    <td className="px-4 py-3">
                      <span title={syncErrors[subscription.id]} className={`rounded px-2 py-1 text-xs ${isManual ? 'bg-blue-500/10 text-blue-300' : isSyncing ? 'bg-blue-500/10 text-blue-300' : subscription.sync_status === 'failed' ? 'bg-red-500/10 text-red-300' : 'bg-emerald-500/10 text-emerald-300'}`}>{isManual ? '本地' : formatSyncStatus(subscription)}</span>
                      {syncErrors[subscription.id] && (
                        <div className={`mt-1 max-w-[180px] truncate text-xs ${syncErrors[subscription.id].includes('已忽略') || syncErrors[subscription.id].includes('ignored') ? 'text-orange-300' : 'text-red-300'}`} title={syncErrors[subscription.id]}>
                          {syncErrors[subscription.id]}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <IconButton title={isManual ? '手动导入源不可编辑' : '编辑'} onClick={isManual || isSyncing ? undefined : () => openEdit(subscription)}><Edit3 size={14} /></IconButton>
                        <IconButton title={isManual ? '手动导入源不可同步' : '同步'} onClick={isManual || isSyncing ? undefined : () => syncOne(subscription)}><RefreshCw size={14} /></IconButton>
                        <IconButton title={isManual ? '手动导入源不可删除' : '删除'} onClick={isManual || isSyncing ? undefined : () => setDeleteTarget(subscription)}><Trash2 size={14} /></IconButton>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      <section className="overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] shadow-[var(--shadow-card)]">
        <div className="border-b border-[var(--border-default)] bg-white/[0.025] px-5 py-3">
          <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <div className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-blue-400/25 bg-blue-500/10 text-blue-200">
                <Upload size={18} />
              </div>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <h2 className="text-sm font-semibold text-white">本地订阅导入</h2>
                  <p className="text-xs leading-5 text-[var(--text-secondary)]">粘贴 URI / Clash YAML / sing-box JSON 导入到置顶“本地订阅”，不会被远程订阅清空，UID 已存在则追加/更新。</p>
                </div>
              </div>
            </div>
            <div className="flex shrink-0 flex-wrap gap-2 xl:justify-end">
              <span className="inline-flex items-center gap-1 rounded-full border border-blue-400/20 bg-blue-500/10 px-3 py-1 text-xs text-blue-100"><Link2 size={12} />URI List</span>
              <span className="inline-flex items-center gap-1 rounded-full border border-cyan-400/20 bg-cyan-500/10 px-3 py-1 text-xs text-cyan-100"><FileJson size={12} />Clash YAML</span>
              <span className="inline-flex items-center gap-1 rounded-full border border-violet-400/20 bg-violet-500/10 px-3 py-1 text-xs text-violet-100"><FileJson size={12} />sing-box JSON</span>
            </div>
          </div>
        </div>

        <div className="p-5">
            <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <div className="text-sm font-medium text-white">粘贴导入内容</div>
                <div className="mt-1 text-xs text-[var(--text-tertiary)]">已输入 {importLineCount} 行，支持自动识别格式</div>
              </div>
              <div className="flex gap-2">
                {importContent && <button onClick={clearImportContent} className="h-9 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-sm text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white">清空</button>}
                <button disabled={!importContent.trim() || previewLoading} onClick={previewImportNodes} className={`inline-flex h-9 items-center gap-2 rounded-md border px-4 text-sm font-medium ${importContent.trim() && !previewLoading ? 'border-cyan-400/30 bg-cyan-500/15 text-cyan-100 hover:bg-cyan-500/25' : 'cursor-not-allowed border-[var(--border-default)] bg-white/[0.03] text-[var(--text-tertiary)]'}`}>
                  <FileJson size={15} />{previewLoading ? '预览中...' : '节点预览'}
                </button>
                <button disabled={!importContent.trim()} onClick={importNodes} className={`inline-flex h-9 items-center gap-2 rounded-md border px-4 text-sm font-medium ${importContent.trim() ? 'border-blue-400/30 bg-blue-500/20 text-blue-100 hover:bg-blue-500/30' : 'cursor-not-allowed border-[var(--border-default)] bg-white/[0.03] text-[var(--text-tertiary)]'}`}>
                  <Upload size={15} />导入节点
                </button>
              </div>
            </div>
            <div className="grid gap-4 xl:grid-cols-[minmax(0,3fr)_minmax(320px,2fr)]">
              <div className="relative">
                <textarea value={importContent} onChange={e => { setImportContent(e.target.value); setPreviewItems([]); }} rows={11} placeholder={'vless://...\nvmess://...\nss://...\n\n# 或直接粘贴 Clash YAML / sing-box JSON'} className="h-full min-h-[300px] w-full resize-y rounded-xl border border-blue-400/25 bg-[#0d1a2b]/80 px-4 py-4 font-mono text-sm leading-6 text-blue-50 outline-none ring-1 ring-transparent transition placeholder:text-[var(--text-tertiary)] focus:border-blue-300/70 focus:ring-blue-400/20" />
                {!importContent && <div className="pointer-events-none absolute bottom-4 right-4 rounded-md border border-[var(--border-default)] bg-black/20 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-[var(--text-tertiary)]">auto parse</div>}
              </div>

              <aside className="min-h-[300px] rounded-xl border border-[var(--border-default)] bg-black/15 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium text-white">节点预览</div>
                    <div className="mt-1 text-xs text-[var(--text-tertiary)]">{previewItems.length > 0 ? `已识别 ${previewItems.length} 个节点` : '预览后显示解析结果'}</div>
                  </div>
                  <span className="rounded-full border border-blue-400/20 bg-blue-500/10 px-2 py-1 text-xs text-blue-100">preview</span>
                </div>
                <div className="mt-4 max-h-[238px] space-y-2 overflow-auto pr-1">
                  {previewItems.length === 0 ? (
                    <div className="flex h-[210px] items-center justify-center rounded-lg border border-dashed border-[var(--border-default)] text-center text-xs leading-5 text-[var(--text-tertiary)]">点击“节点预览”后，节点名称、协议和地址会显示在这里。</div>
                  ) : previewItems.slice(0, 50).map(item => (
                    <div key={item.uid} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--border-default)] bg-white/[0.035] px-3 py-2">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-white" title={item.name}>{item.name}</div>
                        <div className="mt-1 flex items-center gap-2 text-xs text-[var(--text-tertiary)]">
                          <span className="rounded bg-blue-500/10 px-1.5 py-0.5 text-blue-200">{item.type}</span>
                          <span className="truncate font-mono" title={`${item.server}:${item.server_port}`}>{item.server}:{item.server_port}</span>
                        </div>
                      </div>
                      <button onClick={() => { setPreviewDetail(item); setPreviewDetailFormat('json'); }} className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-2 text-[11px] text-[var(--text-secondary)] hover:border-blue-400/30 hover:bg-blue-500/10 hover:text-white"><Eye size={11} />详情</button>
                    </div>
                  ))}
                  {previewItems.length > 50 && <div className="text-center text-xs text-[var(--text-tertiary)]">仅显示前 50 个节点</div>}
                </div>
              </aside>
            </div>
        </div>
      </section>

      {formOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4 backdrop-blur-sm">
          <div className="w-full max-w-lg rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(16,27,43,0.96))] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-base font-semibold text-white">{editing ? '编辑订阅' : '添加订阅'}</h3>
            <div className="mt-4 space-y-3">
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">名称</span>
                <input value={name} onChange={e => setName(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">订阅链接</span>
                <input value={url} onChange={e => setURL(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" placeholder="https://..." />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">User-Agent</span>
                <select value={userAgentPreset} onChange={e => selectUserAgentPreset(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
                  {userAgentOptions.length === 0 && <option className="bg-[#152235] text-white" value="clash-meta/2.4.0">Clash Meta - clash-meta/2.4.0</option>}
                  {userAgentOptions.map(option => <option key={option.value} className="bg-[#152235] text-white" value={option.value}>{option.label} - {option.value}</option>)}
                  <option className="bg-[#152235] text-white" value={customUserAgentValue}>自定义</option>
                </select>
                {userAgentPreset !== customUserAgentValue && userAgentOptions.find(option => option.value === userAgentPreset)?.description && <span className="mt-1 block text-xs text-[var(--text-tertiary)]">{userAgentOptions.find(option => option.value === userAgentPreset)?.description}</span>}
                {userAgentPreset === customUserAgentValue && <input value={userAgent} onChange={e => setUserAgent(e.target.value)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" placeholder="自定义 User-Agent" />}
                <span className="mt-1 block text-xs text-[var(--text-tertiary)]">部分订阅会根据 UA 返回 Clash / v2ray 格式或流量信息。</span>
              </label>
              <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.03] p-3">
                <div className="mb-2 text-xs font-semibold text-white">同步周期</div>
                <SyncScheduleControls
                  value={{ sync_mode: syncMode, sync_time: syncTime, sync_weekday: Number(syncWeekday) }}
                  syncModes={syncModeOptions}
                  weekdays={[]}
                  weekdayOptions={subscriptionWeekdays}
                  onChange={patch => {
                    if (patch.sync_mode !== undefined) setSyncMode(patch.sync_mode as 'off' | 'daily' | 'weekly' | 'monthly');
                    if (patch.sync_time !== undefined) setSyncTime(patch.sync_time);
                    if (patch.sync_weekday !== undefined) setSyncWeekday(String(patch.sync_weekday));
                  }}
                />
              </div>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">订阅超时（秒）</span>
                <input type="number" min={5} max={300} value={syncTimeout} onChange={e => setSyncTimeout(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                <span className="mt-1 block text-xs text-[var(--text-tertiary)]">用于远端订阅下载，建议 60 秒，范围 5-300 秒。</span>
              </label>
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button onClick={() => setFormOpen(false)} className="h-9 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 text-sm text-[var(--text-secondary)] hover:text-white">取消</button>
              <button onClick={save} className="h-9 rounded-md bg-[var(--color-primary)] px-4 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)]">保存</button>
            </div>
          </div>
        </div>
      )}

      {previewDetail && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
          <div className="max-h-[86vh] w-full max-w-4xl overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(13,24,40,0.98))] shadow-[var(--shadow-card)]">
            <div className="flex items-start justify-between gap-4 border-b border-[var(--border-default)] px-5 py-4">
              <div className="min-w-0">
                <h3 className="text-base font-semibold text-white">节点配置详情</h3>
                <p className="mt-1 truncate text-xs text-[var(--text-tertiary)]">{previewDetail.name}</p>
              </div>
              <button onClick={() => setPreviewDetail(null)} className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white"><X size={16} /></button>
            </div>
            <div className="px-5 py-4">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
                <div className="flex gap-2">
                  <button onClick={() => setPreviewDetailFormat('json')} className={`rounded-md border px-3 py-1.5 text-xs font-medium ${previewDetailFormat === 'json' ? 'border-blue-400/50 bg-blue-500/20 text-blue-100' : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white'}`}>JSON</button>
                  <button onClick={() => setPreviewDetailFormat('yaml')} className={`rounded-md border px-3 py-1.5 text-xs font-medium ${previewDetailFormat === 'yaml' ? 'border-blue-400/50 bg-blue-500/20 text-blue-100' : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white'}`}>YAML</button>
                </div>
                <div className="text-xs text-[var(--text-tertiary)]">{previewDetail.type} · {previewDetail.server}:{previewDetail.server_port}</div>
              </div>
              <pre className="max-h-[58vh] overflow-auto rounded-xl border border-[var(--border-default)] bg-[#07111f] p-4 font-mono text-xs leading-6 text-blue-50">{previewDetailFormat === 'json' ? prettyJSON(previewDetail.raw_json) : previewYAML(previewDetail.raw_json)}</pre>
              <div className="mt-4 flex justify-end">
                <button onClick={() => setPreviewDetail(null)} className="h-9 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 text-sm text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white">关闭</button>
              </div>
            </div>
          </div>
        </div>
      )}

      {deleteTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(16,27,43,0.96))] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-base font-semibold text-white">删除订阅</h3>
            <p className="mt-2 text-sm text-[var(--text-secondary)]">确定删除订阅「{deleteTarget.name}」？这会删除该订阅下的节点记录，但不会影响已生成的配置文件。</p>
            <div className="mt-5 flex justify-end gap-2">
              <button onClick={() => setDeleteTarget(null)} className="h-9 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 text-sm text-[var(--text-secondary)] hover:text-white">取消</button>
              <button onClick={remove} className="h-9 rounded-md bg-red-500 px-4 text-sm font-medium text-white hover:bg-red-600">删除</button>
            </div>
          </div>
        </div>
      )}

      {/* 节点过滤规则 */}
      <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
        <div className="mb-4">
          <h2 className="font-semibold text-white">节点过滤规则</h2>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">用于订阅同步入库前过滤节点。规则支持 Go 正则表达式，匹配后该节点不会写入节点库。</p>
        </div>

        <div className="mb-4 grid gap-3 lg:grid-cols-[1fr_160px_1.5fr_110px]">
          <input value={filterName} onChange={e => setFilterName(e.target.value)} placeholder="规则名称，如：过滤倍率节点" className="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]" />
          <select value={filterTarget} onChange={e => setFilterTarget(e.target.value)} className="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]">
            {filterTargets.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
          </select>
          <input value={filterPattern} onChange={e => setFilterPattern(e.target.value)} placeholder="正则，如：(?i)官网|过期|剩余|倍率" className="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-[var(--color-primary)]" />
          <label className="inline-flex items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-[var(--text-secondary)]">
            <input type="checkbox" checked={filterEnabled} onChange={e => setFilterEnabled(e.target.checked)} />启用
          </label>
        </div>
        <div className="mb-4 flex gap-2">
          <Button variant="primary" size="sm" onClick={async () => {
            try {
              const payload = { name: filterName, target: filterTarget, pattern: filterPattern, enabled: filterEnabled };
              if (editingFilter) {
                await api.updateNodeFilter(editingFilter.id, payload);
                setMessage('过滤规则已更新');
              } else {
                await api.createNodeFilter(payload);
                setMessage('过滤规则已添加');
              }
              setEditingFilter(null);
              setFilterName('');
              setFilterTarget('name');
              setFilterPattern('');
              setFilterEnabled(true);
              await loadFilters();
            } catch (e: any) {
              setMessage(`过滤规则保存失败: ${e.message}`);
            }
          }}>{editingFilter ? '更新规则' : '添加规则'}</Button>
          {editingFilter && <Button variant="secondary" size="sm" onClick={() => {
            setEditingFilter(null);
            setFilterName('');
            setFilterTarget('name');
            setFilterPattern('');
            setFilterEnabled(true);
          }}>取消编辑</Button>}
        </div>

        <div className="overflow-hidden rounded-none border border-[var(--border-default)]">
          <table className="w-full min-w-[680px] border-collapse text-left text-sm">
            <thead className="bg-white/[0.04] text-white">
              <tr>
                {['名称', '目标字段', '正则', '状态', '操作'].map(col => <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>)}
              </tr>
            </thead>
            <tbody>
              {filters.length === 0 ? (
                <tr><td colSpan={5} className="px-4 py-10 text-center text-sm text-[var(--text-secondary)]">暂无过滤规则</td></tr>
              ) : filters.map(item => (
                <tr key={item.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0">
                  <td className="px-4 py-3 font-medium text-white">{item.name}</td>
                  <td className="px-4 py-3">{filterTargets.find(target => target.value === item.target)?.label || item.target}</td>
                  <td className="max-w-[420px] truncate px-4 py-3 font-mono text-xs" title={item.pattern}>{item.pattern}</td>
                  <td className="px-4 py-3"><button onClick={async () => {
                    try {
                      await api.updateNodeFilter(item.id, { name: item.name, target: item.target, pattern: item.pattern, enabled: !item.enabled });
                      await loadFilters();
                    } catch (e: any) {
                      setMessage(`过滤规则状态更新失败: ${e.message}`);
                    }
                  }} className={`rounded px-2 py-1 text-xs ${item.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{item.enabled ? '启用' : '停用'}</button></td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2">
                      <button onClick={() => {
                        setEditingFilter(item);
                        setFilterName(item.name);
                        setFilterTarget(item.target || 'name');
                        setFilterPattern(item.pattern);
                        setFilterEnabled(item.enabled);
                      }} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-xs text-white hover:bg-white/[0.08]">编辑</button>
                      <button onClick={async () => {
                        try {
                          await api.deleteNodeFilter(item.id);
                          setMessage('过滤规则已删除');
                          if (editingFilter?.id === item.id) {
                            setEditingFilter(null);
                            setFilterName('');
                            setFilterTarget('name');
                            setFilterPattern('');
                            setFilterEnabled(true);
                          }
                          await loadFilters();
                        } catch (e: any) {
                          setMessage(`过滤规则删除失败: ${e.message}`);
                        }
                      }} className="rounded-md border border-red-400/30 bg-red-500/10 px-3 py-1 text-xs text-red-200 hover:bg-red-500/20">删除</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

export default SubscriptionsPage;
