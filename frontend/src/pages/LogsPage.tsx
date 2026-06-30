import React from 'react';
import { useRealtimeSocket } from '@/hooks/useRealtimeSocket';
import { api } from '@/services/api';
import type { CoreLogEntry, WSEvent } from '@/services/types';
import { PageHeader } from '@/components/layout/PageHeader';

type SourceFilter = 'all' | 'stdout' | 'stderr';

function formatTime(value: number) {
  if (!value) return '--:--:--';
  return new Date(value).toLocaleTimeString();
}

export function LogsPage() {
  const [logs, setLogs] = React.useState<CoreLogEntry[]>([]);
  const [sourceFilter, setSourceFilter] = React.useState<SourceFilter>('all');
  const [autoScroll, setAutoScroll] = React.useState(true);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState('');
  const ref = React.useRef<HTMLDivElement>(null);

  const loadLogs = React.useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setLogs(await api.getCoreLogs(500));
    } catch (e: any) {
      setError(e.message || '日志加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void loadLogs();
  }, [loadLogs]);

  useRealtimeSocket((event: WSEvent) => {
    if (event.type === 'core.log') {
      const entry = event.data as CoreLogEntry;
      if (!entry?.line) return;
      setLogs(prev => [...prev.slice(-500), entry]);
    }
  });

  React.useEffect(() => {
    if (autoScroll && ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [autoScroll, logs]);

  const visibleLogs = React.useMemo(() => {
    if (sourceFilter === 'all') return logs;
    return logs.filter(item => item.source === sourceFilter);
  }, [logs, sourceFilter]);

  const clearLogs = async () => {
    try {
      await api.clearCoreLogs();
      setLogs([]);
    } catch (e: any) {
      setError(e.message || '日志清空失败');
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader title="日志" description="实时查看 sing-box 核心 stdout/stderr 输出。" />
      <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]">
            <span>已缓存 {logs.length} 行</span>
            <span>当前显示 {visibleLogs.length} 行</span>
            {error ? <span className="text-red-300">{error}</span> : null}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {(['all', 'stdout', 'stderr'] as SourceFilter[]).map(item => (
              <button key={item} onClick={() => setSourceFilter(item)} className={`h-8 rounded-md border px-3 text-xs ${sourceFilter === item ? 'border-emerald-400/40 bg-emerald-500/15 text-emerald-100' : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white'}`}>{item === 'all' ? '全部' : item}</button>
            ))}
            <button onClick={() => setAutoScroll(value => !value)} className={`h-8 rounded-md border px-3 text-xs ${autoScroll ? 'border-blue-400/40 bg-blue-500/15 text-blue-100' : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white'}`}>{autoScroll ? '自动滚动' : '已暂停滚动'}</button>
            <button onClick={loadLogs} className="h-8 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-[var(--text-secondary)] hover:text-white">刷新</button>
            <button onClick={clearLogs} className="h-8 rounded-md border border-red-400/30 bg-red-500/10 px-3 text-xs text-red-200 hover:bg-red-500/20">清空</button>
          </div>
        </div>
        <div ref={ref} className="max-h-[calc(100vh-240px)] overflow-y-auto rounded-lg border border-[var(--border-default)] bg-[var(--bg-primary)] p-3 font-mono text-xs text-[var(--text-tertiary)]">
          {loading ? <div className="py-8 text-center">加载日志...</div> : visibleLogs.length === 0 ? <div className="py-8 text-center">等待日志...</div> : visibleLogs.map((item, index) => (
            <div key={`${item.id}-${index}`} className="grid grid-cols-[82px_58px_minmax(0,1fr)] gap-2 whitespace-pre-wrap break-all py-0.5 hover:bg-white/[0.03]">
              <span className="text-[var(--text-muted)]">{formatTime(item.time)}</span>
              <span className={item.source === 'stderr' ? 'text-red-300' : 'text-blue-300'}>{item.source}</span>
              <span className="text-[var(--text-secondary)]">{item.line}</span>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

export default LogsPage;
