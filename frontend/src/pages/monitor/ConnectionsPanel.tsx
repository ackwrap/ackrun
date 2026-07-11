import { RefreshCw, X } from 'lucide-react';
import type { Connection } from '@/services/clash';
import { formatBytes, monitorPanelBodyClass, monitorPanelClass } from './monitorUtils';

interface ConnectionsPanelProps {
  connections: Connection[];
  search: string;
  loading: boolean;
  onSearchChange: (value: string) => void;
  onRefresh: () => void;
  onCloseConnection: (id: string) => void;
  onCloseAll: () => void;
}

export function ConnectionsPanel({ connections, search, loading, onSearchChange, onRefresh, onCloseConnection, onCloseAll }: ConnectionsPanelProps) {
  return (
    <div className="space-y-4">
      <div className={`${monitorPanelClass} flex flex-wrap items-center justify-between gap-3`}>
        <div>
          <h3 className="text-sm font-semibold text-[var(--text-primary)]">活动连接</h3>
          <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">共 {connections.length} 个连接</p>
        </div>
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={search}
            onChange={event => onSearchChange(event.target.value)}
            placeholder="搜索域名、IP..."
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
          <button onClick={onCloseAll} className="inline-flex h-8 items-center gap-2 rounded-md border border-red-400/30 bg-red-500/10 px-3 text-xs text-red-200 hover:bg-red-500/20">
            <X size={14} />
            关闭所有
          </button>
        </div>
      </div>

      <div className={`overflow-hidden ${monitorPanelBodyClass}`}>
        {connections.length === 0 ? (
          <div className="p-12 text-center text-sm text-[var(--text-secondary)]">暂无连接</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[800px] border-collapse text-left text-sm">
              <thead className="bg-white/[0.04] text-[var(--text-primary)]">
                <tr>
                  {['目标', '来源', '策略链', '规则', '上传/下载', '操作'].map(col => (
                    <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {connections.map(conn => (
                  <tr key={conn.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 hover:bg-white/[0.02]">
                    <td className="px-4 py-3">
                      <div className="font-medium text-[var(--text-primary)]">{conn.metadata.host || conn.metadata.destinationIP}</div>
                      <div className="text-xs text-[var(--text-tertiary)]">{conn.metadata.destinationPort}</div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="text-xs">{conn.metadata.sourceIP}</div>
                      <div className="text-xs text-[var(--text-tertiary)]">{conn.metadata.sourcePort}</div>
                    </td>
                    <td className="px-4 py-3"><div className="text-xs">{conn.chains?.join(' → ') || '-'}</div></td>
                    <td className="px-4 py-3">
                      <div className="text-xs">{conn.rule || '-'}</div>
                      {conn.rulePayload && <div className="text-xs text-[var(--text-tertiary)]">{conn.rulePayload}</div>}
                    </td>
                    <td className="px-4 py-3">
                      <div className="text-xs text-blue-400">↑ {formatBytes(conn.upload)}</div>
                      <div className="text-xs text-emerald-400">↓ {formatBytes(conn.download)}</div>
                    </td>
                    <td className="px-4 py-3">
                      <button onClick={() => onCloseConnection(conn.id)} className="rounded-md border border-red-400/30 bg-red-500/10 px-2 py-1 text-xs text-red-200 hover:bg-red-500/20">关闭</button>
                    </td>
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
