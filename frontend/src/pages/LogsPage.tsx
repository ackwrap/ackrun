import React from 'react';
import { useRealtimeSocket } from '@/hooks/useRealtimeSocket';
import type { WSEvent } from '@/services/types';
import { PageHeader } from '@/components/layout/PageHeader';

export function LogsPage() {
  const [logs, setLogs] = React.useState<string[]>([]);
  const ref = React.useRef<HTMLDivElement>(null);

  useRealtimeSocket((event: WSEvent) => {
    if (event.type === 'core.log') {
      const line = (event.data as any).line || '';
      setLogs(prev => [...prev.slice(-500), line]);
    }
  });

  React.useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [logs]);

  return (
    <div className="space-y-4">
      <PageHeader title="日志" />
      <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
        <div ref={ref} className="rounded-lg border border-[var(--border-default)] bg-[#0d0d0d] p-3 max-h-[calc(100vh-200px)] overflow-y-auto font-mono text-xs text-[var(--text-tertiary)]">
          {logs.length === 0 ? <div className="text-center py-8">等待日志...</div> : logs.map((line, i) => <div key={i} className="whitespace-pre-wrap break-all py-0.5 hover:bg-white/[0.03]">{line}</div>)}
        </div>
      </section>
    </div>
  );
}

export default LogsPage;