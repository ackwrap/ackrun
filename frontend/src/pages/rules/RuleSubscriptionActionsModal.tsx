import { Trash2 } from 'lucide-react';

import { SyncScheduleControls } from '@/components/ui/SyncScheduleControls';
import { api } from '@/services/api';
import type { RouteRuleSubscription } from '@/services/types';

interface RuleSubscriptionActionsModalProps {
  item: RouteRuleSubscription;
  ruleSetFormats: Array<{ value: string; label: string }>;
  syncModes: Array<{ value: string; label: string }>;
  weekdays: string[];
  onChange: (item: RouteRuleSubscription) => void;
  onClose: () => void;
  onReload: () => Promise<void>;
  onCreateRule: (item: RouteRuleSubscription, outbound: string) => Promise<void>;
  proxyOutbound: string;
  proxyOutboundLabel: string;
  onPreview: (item: RouteRuleSubscription) => void;
  onSync: (item: RouteRuleSubscription) => Promise<void>;
  onAppendTag: (tag: string) => void;
  onToggle: (item: RouteRuleSubscription) => Promise<void>;
  onEdit: (item: RouteRuleSubscription) => void;
  onRemove: (item: RouteRuleSubscription) => Promise<void>;
  formatTime: (value: number) => string;
  syncStatusLabel: (value: string) => string;
  syncStatusClass: (value: string) => string;
}

export function RuleSubscriptionActionsModal({
  item,
  ruleSetFormats,
  syncModes,
  weekdays,
  onChange,
  onClose,
  onReload,
  onCreateRule,
  proxyOutbound,
  proxyOutboundLabel,
  onPreview,
  onSync,
  onAppendTag,
  onToggle,
  onEdit,
  onRemove,
  formatTime,
  syncStatusLabel,
  syncStatusClass,
}: RuleSubscriptionActionsModalProps) {
  const updateItem = async (patch: Partial<RouteRuleSubscription>) => {
    const next = { ...item, ...patch };
    await api.updateRouteRuleSubscription(item.id, next);
    onChange(next);
    await onReload();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
      <div className="max-h-[84vh] w-full max-w-2xl overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(13,24,40,0.98))] shadow-[var(--shadow-card)]">
        <div className="flex items-start justify-between gap-4 border-b border-[var(--border-default)] px-5 py-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-base font-semibold text-white">{item.name}</h3>
              <span className="rounded border border-cyan-400/25 bg-cyan-500/10 px-2 py-0.5 font-mono text-xs text-cyan-100">{item.tag}</span>
              <span className={`rounded px-2 py-0.5 text-xs ${syncStatusClass(item.sync_status)}`}>{syncStatusLabel(item.sync_status)}</span>
            </div>
            <div className="mt-2 truncate font-mono text-xs text-[var(--text-tertiary)]" title={item.url}>{item.url}</div>
          </div>
          <button onClick={onClose} className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-tertiary)] transition-colors hover:border-red-400/30 hover:bg-red-500/10 hover:text-red-300" title="关闭">
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2"><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>

        <div className="space-y-4 overflow-auto p-5">
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.03] px-3 py-2">
              <div className="text-xs text-[var(--text-tertiary)]">格式</div>
              <select value={item.format} onChange={e => updateItem({ format: e.target.value })} className="mt-1 w-full rounded border border-[var(--border-default)] bg-white/[0.06] px-2 py-1 text-sm font-medium text-white focus:border-cyan-400/50 focus:outline-none focus:ring-1 focus:ring-cyan-400/30">
                {ruleSetFormats.map(fmt => <option key={fmt.value} value={fmt.value} className="bg-[#1a2332] text-white">{fmt.label}</option>)}
              </select>
            </div>
            <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.03] px-3 py-2">
              <div className="text-xs text-[var(--text-tertiary)]">下载方式</div>
              <select value={item.use_proxy ? 'proxy' : 'direct'} onChange={e => updateItem({ use_proxy: e.target.value === 'proxy' })} className="mt-1 w-full rounded border border-[var(--border-default)] bg-white/[0.06] px-2 py-1 text-sm font-medium text-white focus:border-cyan-400/50 focus:outline-none focus:ring-1 focus:ring-cyan-400/30">
                <option value="direct" className="bg-[#1a2332] text-white">直连下载</option>
                <option value="proxy" className="bg-[#1a2332] text-white">代理下载</option>
              </select>
            </div>
            <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.03] px-3 py-2 sm:col-span-3">
              <div className="mb-2 text-xs text-[var(--text-tertiary)]">自动更新</div>
              <SyncScheduleControls
                value={{ sync_mode: item.sync_mode, sync_time: item.sync_time, sync_weekday: item.sync_weekday, use_proxy: item.use_proxy }}
                syncModes={syncModes}
                weekdays={weekdays}
                showProxy
                onChange={patch => void updateItem(patch)}
              />
            </div>
          </div>

          <div className="grid gap-2 text-xs text-[var(--text-tertiary)] sm:grid-cols-2">
            <div>最后同步：{formatTime(item.last_sync_at)}</div>
            <div>缓存时间：{formatTime(item.cached_updated_at)}</div>
            <div className="truncate sm:col-span-2" title={item.cached_path}>缓存路径：{item.cached_path || '--'}</div>
          </div>
          {item.sync_error && <div className="rounded border border-red-400/20 bg-red-500/10 px-3 py-2 text-xs text-red-300">{item.sync_error}</div>}

          <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4">
            <div className="mb-3 text-sm font-semibold text-white">生成引用规则</div>
            <div className="grid gap-2 sm:grid-cols-3">
              <button onClick={async () => { await onCreateRule(item, proxyOutbound); onClose(); }} className="rounded-md border border-emerald-400/25 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-100 hover:bg-emerald-500/20">{proxyOutbound === 'direct' ? '直连' : proxyOutboundLabel}</button>
              <button onClick={async () => { await onCreateRule(item, 'direct'); onClose(); }} className="rounded-md border border-emerald-400/25 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-200 hover:bg-emerald-500/20">直连</button>
              <button onClick={async () => { await onCreateRule(item, 'block'); onClose(); }} className="rounded-md border border-red-400/25 bg-red-500/10 px-3 py-2 text-sm text-red-200 hover:bg-red-500/20">阻断</button>
            </div>
            <p className="mt-2 text-xs text-[var(--text-tertiary)]">规则订阅只是 rule_set，需要生成一条 route.rules 引用 tag 才会实际生效。</p>
          </div>

          <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4">
            <div className="mb-3 text-sm font-semibold text-white">内容与更新</div>
            <div className="flex flex-wrap gap-2">
              <button onClick={() => onPreview(item)} className="rounded-md border border-purple-400/25 bg-purple-500/10 px-3 py-2 text-sm text-purple-100 hover:bg-purple-500/20">预览 JSON</button>
              <button onClick={async () => { await onSync(item); onClose(); }} className="rounded-md border border-cyan-400/25 bg-cyan-500/10 px-3 py-2 text-sm text-cyan-100 hover:bg-cyan-500/20">立即同步</button>
              <button onClick={() => { onAppendTag(item.tag); onClose(); }} className="rounded-md border border-cyan-400/25 bg-cyan-500/10 px-3 py-2 text-sm text-cyan-100 hover:bg-cyan-500/20">填入表单</button>
            </div>
          </div>

          <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4">
            <div className="mb-3 text-sm font-semibold text-white">订阅管理</div>
            <div className="flex flex-wrap gap-2">
              <button onClick={async () => { await onToggle(item); onClose(); }} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white hover:bg-white/[0.08]">{item.enabled ? '停用订阅' : '启用订阅'}</button>
              <button onClick={() => onEdit(item)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white hover:bg-white/[0.08]">编辑订阅</button>
              <button onClick={async () => { await onRemove(item); }} className="inline-flex items-center gap-1 rounded-md border border-red-400/30 bg-red-500/10 px-3 py-2 text-sm text-red-200 hover:bg-red-500/20"><Trash2 size={14} />删除订阅</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
