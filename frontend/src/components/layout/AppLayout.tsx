import { useEffect, useState } from 'react';
import { Moon, Sun } from 'lucide-react';
import { Sidebar } from './Sidebar';

type ThemeMode = 'dark' | 'light';

export function AppLayout({ children }: { children: React.ReactNode }) {
  const [collapsed, setCollapsed] = useState(false);
  const [theme, setTheme] = useState<ThemeMode>(() => {
    if (typeof window === 'undefined') return 'dark';
    return (localStorage.getItem('ackwrap.theme') as ThemeMode) || 'dark';
  });

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem('ackwrap.theme', theme);
  }, [theme]);

  const toggleTheme = () => setTheme(value => value === 'dark' ? 'light' : 'dark');

  return (
    <div className="flex h-screen bg-[var(--bg-base)] text-[var(--text-primary)]">
      <Sidebar collapsed={collapsed} onToggle={() => setCollapsed(v => !v)} />
      <main id="main-content" className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <header className="flex items-center justify-between px-5 h-[62px] bg-[var(--header-bg)] border-b border-[var(--border-light)] backdrop-blur-xl shrink-0">
          <div className="hidden lg:block w-5" />
          <div className="ml-auto flex items-center gap-3 text-sm">
            <button onClick={toggleTheme} className="inline-flex h-9 items-center gap-2 rounded-full border border-[var(--border-default)] bg-white/[0.08] px-3 text-xs font-medium text-[var(--text-secondary)] transition hover:border-blue-400/30 hover:bg-blue-500/10 hover:text-[var(--text-primary)]" title={theme === 'dark' ? '切换到白天模式' : '切换到夜间模式'}>
              {theme === 'dark' ? <Sun size={15} /> : <Moon size={15} />}
              <span>{theme === 'dark' ? '白天' : '夜间'}</span>
            </button>
            <span id="ws-indicator" className="inline-flex items-center gap-2">
              <span className="h-2 w-2 rounded-full bg-emerald-400 animate-status-pulse" id="ws-dot"></span>
              <span className="text-[var(--text-secondary)]" id="ws-text">已连接</span>
            </span>
          </div>
        </header>
        <div className="flex-1 overflow-auto">
          <div className="h-full px-4 py-5 sm:px-6 lg:px-7">
            {children}
          </div>
        </div>
      </main>
    </div>
  );
}
