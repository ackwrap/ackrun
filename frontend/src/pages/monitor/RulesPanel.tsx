import { RefreshCw } from 'lucide-react';
import type { Rule } from '@/services/clash';
import { monitorPanelBodyClass, monitorPanelClass } from './monitorUtils';

interface RulesPanelProps {
  rules: Rule[];
  search: string;
  loading: boolean;
  unavailableReason: string;
  onSearchChange: (value: string) => void;
  onRefresh: () => void;
}

export function RulesPanel({ rules, search, loading, unavailableReason, onSearchChange, onRefresh }: RulesPanelProps) {
  return (
    <div className="space-y-4">
      <div className={`${monitorPanelClass} flex flex-wrap items-center justify-between gap-3`}>
        <div>
          <h3 className="text-sm font-semibold text-[var(--text-primary)]">规则列表</h3>
          <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">共 {rules.length} 条规则</p>
        </div>
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={search}
            onChange={event => onSearchChange(event.target.value)}
            placeholder="搜索规则..."
            className="h-8 w-48 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-[var(--text-primary)] outline-none focus:border-blue-400"
          />
          <button
            onClick={onRefresh}
            disabled={loading}
            className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-[var(--text-primary)] hover:bg-white/[0.08] disabled:opacity-50"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            刷新
          </button>
        </div>
      </div>

      <div className={`overflow-hidden ${monitorPanelBodyClass}`}>
        {loading ? (
          <div className="p-12 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
        ) : unavailableReason ? (
          <div className="p-12 text-center">
            <div className="text-sm text-[var(--text-secondary)]">Clash API 未连接，无法读取运行中规则</div>
            <div className="mt-2 text-xs text-red-300">{unavailableReason}</div>
          </div>
        ) : rules.length === 0 ? (
          <div className="p-12 text-center text-sm text-[var(--text-secondary)]">暂无规则</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[600px] border-collapse text-left text-sm">
              <thead className="bg-white/[0.04] text-[var(--text-primary)]">
                <tr>
                  {['类型', '匹配值', '策略'].map(col => (
                    <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rules.map((rule, index) => (
                  <tr key={index} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 hover:bg-white/[0.02]">
                    <td className="px-4 py-3"><span className="rounded bg-blue-500/20 px-2 py-1 text-xs text-blue-200">{rule.type}</span></td>
                    <td className="px-4 py-3"><span className="break-all text-[var(--text-primary)]">{rule.payload}</span></td>
                    <td className="px-4 py-3"><span className="text-[var(--text-primary)]">{rule.proxy}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
