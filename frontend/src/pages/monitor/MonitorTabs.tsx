import { Activity, List, Network, Shield } from 'lucide-react';
import type { MonitorTab } from './monitorUtils';

const tabs = [
  { key: 'overview' as MonitorTab, label: '概览', icon: <Activity size={16} /> },
  { key: 'proxies' as MonitorTab, label: '策略组', icon: <Network size={16} /> },
  { key: 'connections' as MonitorTab, label: '连接', icon: <List size={16} /> },
  { key: 'rules' as MonitorTab, label: '规则', icon: <Shield size={16} /> },
];

interface MonitorTabsProps {
  activeTab: MonitorTab;
  onChange: (tab: MonitorTab) => void;
}

export function MonitorTabs({ activeTab, onChange }: MonitorTabsProps) {
  return (
    <div className="flex gap-1 overflow-x-auto border-b border-[var(--border-default)]">
      {tabs.map(tab => (
        <button
          key={tab.key}
          onClick={() => onChange(tab.key)}
          className={`relative flex items-center gap-2 px-4 py-2.5 text-sm font-medium transition-colors ${
            activeTab === tab.key
              ? 'text-[var(--color-primary)] after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full after:bg-[var(--color-primary)]'
              : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--text-primary)]'
          }`}
        >
          {tab.icon}
          {tab.label}
        </button>
      ))}
    </div>
  );
}
