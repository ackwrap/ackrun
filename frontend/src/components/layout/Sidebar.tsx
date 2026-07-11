import {
  LayoutDashboard,
  Settings,
  Activity,
  RadioTower,
  Network,
  Layers,
  ListChecks,
  ServerCog,
  FileCode,
  ChevronLeft,
  ChevronRight,
  Gauge,
} from 'lucide-react';
import { NavLink, useLocation } from 'react-router-dom';

const navItems = [
  { key: 'monitor', label: '仪表盘', icon: <Gauge size={18} />, path: '/' },
  { key: 'control', label: '控制面板', icon: <LayoutDashboard size={18} />, path: '/control' },
  { key: 'subscriptions', label: '订阅管理', icon: <RadioTower size={18} />, path: '/subscriptions' },
  { key: 'nodes', label: '节点管理', icon: <Network size={18} />, path: '/nodes' },
  { key: 'rules', label: '规则管理', icon: <ListChecks size={18} />, path: '/rules' },
  { key: 'collections', label: '策略组管理', icon: <Layers size={18} />, path: '/collections' },
  { key: 'dns', label: 'DNS 管理', icon: <ServerCog size={18} />, path: '/dns' },
  { key: 'config', label: '配置生成', icon: <FileCode size={18} />, path: '/config' },
  { key: 'logs', label: '日志', icon: <Activity size={18} />, path: '/logs' },
  { key: 'settings', label: '设置', icon: <Settings size={18} />, path: '/settings' },
];

export function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const location = useLocation();

  return (
    <aside className={`hidden lg:flex flex-col h-screen bg-[var(--bg-sidebar)] text-[var(--text-sidebar)] transition-[width] duration-300 shrink-0 ${collapsed ? 'w-16' : 'w-56'}`}>
      <div className="h-[62px] flex items-center justify-between px-5 border-b border-[var(--border-light)]">
        {!collapsed && (
          <div className="flex items-center gap-3">
            <img src="/favicon.png" alt="" className="h-9 w-9 shrink-0 drop-shadow-[0_0_16px_rgba(47,129,247,0.3)]" />
            <span className="text-[var(--text-primary)] font-bold text-lg tracking-wide">AckWrap</span>
          </div>
        )}
        {collapsed && <img src="/favicon.png" alt="Ackwrap" className="mx-auto h-9 w-9 drop-shadow-[0_0_14px_rgba(47,129,247,0.28)]" />}
      </div>
      <nav className="flex-1 px-4 py-5 overflow-y-auto space-y-2">
        {navItems.map(item => {
          const isActive = location.pathname === item.path;
          return (
            <NavLink
              key={item.key}
              to={item.path}
              className={`flex items-center gap-3 px-4 h-11 rounded-[var(--radius-lg)] border transition-colors duration-[var(--duration-fast)] ${collapsed ? 'justify-center px-0' : ''} ${isActive ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)] shadow-sm shadow-blue-500/10' : 'border-transparent text-[var(--text-sidebar)] hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--text-sidebar-active)]'}`}
            >
              <span className="shrink-0">{item.icon}</span>
              {!collapsed && <span className="text-sm truncate">{item.label}</span>}
            </NavLink>
          );
        })}
      </nav>
      <div className="border-t border-[var(--border-light)] p-3">
        <button onClick={onToggle} className="w-full h-8 flex items-center justify-center rounded hover:bg-[var(--bg-sidebar-hover)] text-[var(--text-sidebar)] hover:text-[var(--text-sidebar-active)] transition-colors cursor-pointer">
          {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
        </button>
      </div>
    </aside>
  );
}
