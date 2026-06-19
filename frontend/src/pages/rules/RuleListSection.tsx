import { ArrowDown, ArrowUp, Eye, Plus, Route, Trash2 } from 'lucide-react';

import type { RouteRule, RouteRuleSubscription } from '@/services/types';

interface RuleListSectionProps {
  rules: RouteRule[];
  subscriptions: RouteRuleSubscription[];
  onRefresh: () => void;
  onAddGeo: () => void;
  onAdd: () => void;
  onPreview: () => void;
  onMove: (index: number, direction: -1 | 1) => void;
  onToggle: (rule: RouteRule) => void;
  onEdit: (rule: RouteRule) => void;
  onRemove: (rule: RouteRule) => void;
  formatTime: (value: number) => string;
  ruleTypeLabel: (value: string) => string;
  outboundLabel: (value: string) => string;
  outboundClass: (value: string) => string;
}

export function RuleListSection({
  rules,
  subscriptions,
  onRefresh,
  onAddGeo,
  onAdd,
  onPreview,
  onMove,
  onToggle,
  onEdit,
  onRemove,
  formatTime,
  ruleTypeLabel,
  outboundLabel,
  outboundClass,
}: RuleListSectionProps) {
  return (
    <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-white">规则列表</h3>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">上移/下移会立即保存排序。</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button onClick={onPreview} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-sm text-[var(--text-secondary)] hover:border-[var(--border-strong)] hover:bg-white/[0.06] hover:text-[var(--text-primary)]"><Eye size={14} />预览</button>
          <button onClick={onAddGeo} className="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-4 text-sm font-medium text-white shadow-sm hover:bg-emerald-500"><Plus size={15} />添加 GEO 规则</button>
          <button onClick={onAdd} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-default)] bg-transparent px-3 text-sm text-[var(--text-secondary)] hover:border-[var(--border-strong)] hover:bg-white/[0.06] hover:text-[var(--text-primary)]"><Plus size={14} />自定义规则</button>
          <button onClick={onRefresh} className="h-9 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-sm text-[var(--text-secondary)] hover:border-[var(--border-strong)] hover:bg-white/[0.06] hover:text-[var(--text-primary)]">刷新</button>
        </div>
      </div>
      <div className="overflow-hidden rounded-xl border border-[var(--border-default)]">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[980px] border-collapse text-left text-sm">
            <thead className="bg-white/[0.04] text-white">
              <tr>{['排序', '名称', '类型', '匹配值', '出站', '状态', '更新时间', '操作'].map(column => <th key={column} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{column}</th>)}</tr>
            </thead>
            <tbody>
              {rules.length === 0 ? (
                <tr>
                  <td colSpan={8} className="px-4 py-16 text-center">
                    <div className="mx-auto mb-3 flex h-11 w-11 items-center justify-center rounded-xl border border-blue-400/20 bg-blue-500/10 text-blue-200"><Route size={18} /></div>
                    <div className="text-sm font-medium text-white">暂无路由规则</div>
                    <div className="mt-1 text-xs text-[var(--text-tertiary)]">{subscriptions.length > 0 ? '已有规则订阅，但还没有引用规则；点击规则订阅里的代理/直连/阻断生成。' : '添加规则后会显示在这里。'}</div>
                  </td>
                </tr>
              ) : rules.map((rule, index) => (
                <tr key={rule.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <span className="w-7 text-xs text-[var(--text-tertiary)]">#{index + 1}</span>
                      <button onClick={() => onMove(index, -1)} disabled={index === 0} className="rounded border border-[var(--border-default)] bg-white/[0.04] p-1 disabled:opacity-30"><ArrowUp size={13} /></button>
                      <button onClick={() => onMove(index, 1)} disabled={index === rules.length - 1} className="rounded border border-[var(--border-default)] bg-white/[0.04] p-1 disabled:opacity-30"><ArrowDown size={13} /></button>
                    </div>
                  </td>
                  <td className="px-4 py-3 font-medium text-white">{rule.name}</td>
                  <td className="px-4 py-3">{ruleTypeLabel(rule.rule_type)}{rule.invert ? <span className="ml-2 rounded bg-yellow-500/10 px-1.5 py-0.5 text-xs text-yellow-300">反向</span> : null}</td>
                  <td className="max-w-[360px] truncate px-4 py-3 font-mono text-xs" title={rule.values.join('\n')}>{rule.values.join(', ')}</td>
                  <td className="px-4 py-3"><span className={`rounded border px-2 py-1 text-xs ${outboundClass(rule.outbound)}`}>{outboundLabel(rule.outbound)}</span></td>
                  <td className="px-4 py-3"><button onClick={() => onToggle(rule)} className={`rounded px-2 py-1 text-xs ${rule.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{rule.enabled ? '启用' : '停用'}</button></td>
                  <td className="px-4 py-3 text-xs">{formatTime(rule.updated_at)}</td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2">
                      <button onClick={() => onEdit(rule)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-xs text-white hover:bg-white/[0.08]">编辑</button>
                      <button onClick={() => onRemove(rule)} className="inline-flex items-center gap-1 rounded-md border border-red-400/30 bg-red-500/10 px-3 py-1 text-xs text-red-200 hover:bg-red-500/20"><Trash2 size={12} />删除</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}
