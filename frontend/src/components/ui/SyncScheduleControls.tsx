interface SyncModeOption {
  value: string;
  label: string;
}

interface WeekdayOption {
  value: number;
  label: string;
}

export interface SyncScheduleValue {
  sync_mode: string;
  sync_time: string;
  sync_weekday: number;
  use_proxy?: boolean;
}

interface SyncScheduleControlsProps {
  value: SyncScheduleValue;
  syncModes: SyncModeOption[];
  weekdays: string[];
  weekdayOptions?: WeekdayOption[];
  disabled?: boolean;
  showProxy?: boolean;
  saveText?: string;
  onChange: (patch: Partial<SyncScheduleValue>) => void;
  onSave?: () => void;
}

export function SyncScheduleControls({
  value,
  syncModes,
  weekdays,
  weekdayOptions,
  disabled = false,
  showProxy = false,
  saveText = '保存',
  onChange,
  onSave,
}: SyncScheduleControlsProps) {
  const monthlyItems = Array.from({ length: 31 }, (_, index) => ({ value: index + 1, label: `${index + 1} 号` }));
  const weeklyItems = weekdayOptions || weekdays.map((label, value) => ({ value, label }));
  const dayItems = value.sync_mode === 'monthly' ? monthlyItems : weeklyItems;
  const dayLabel = value.sync_mode === 'monthly' ? '每月' : '每周';

  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <label className="block">
        <span className="text-xs text-[var(--text-tertiary)]">周期</span>
        <select value={value.sync_mode} disabled={disabled} onChange={event => onChange({ sync_mode: event.target.value })} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400 disabled:opacity-50">
          {syncModes.map(mode => <option key={mode.value} className="bg-[#152235] text-white" value={mode.value}>{mode.label}</option>)}
        </select>
      </label>
      <label className="block">
        <span className="text-xs text-[var(--text-tertiary)]">时间</span>
        <input type="time" step="1" disabled={disabled || value.sync_mode === 'off'} value={value.sync_time || '03:30:00'} onChange={event => onChange({ sync_time: event.target.value })} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-blue-400 disabled:opacity-50" />
      </label>
      <label className="block">
        <span className="text-xs text-[var(--text-tertiary)]">{dayLabel}</span>
        <select disabled={disabled || (value.sync_mode !== 'weekly' && value.sync_mode !== 'monthly')} value={value.sync_weekday} onChange={event => onChange({ sync_weekday: Number(event.target.value) })} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400 disabled:opacity-50">
          {dayItems.map(weekday => <option key={weekday.value} className="bg-[#152235] text-white" value={weekday.value}>{weekday.label}</option>)}
        </select>
      </label>
      {(showProxy || onSave) && <div className="flex gap-2 sm:col-span-3">
        {showProxy && (
          <label className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-sm text-[var(--text-secondary)]">
            <input type="checkbox" disabled={disabled} checked={!!value.use_proxy} onChange={event => onChange({ use_proxy: event.target.checked })} />代理
          </label>
        )}
        {onSave && <button disabled={disabled} onClick={onSave} className="h-9 rounded-md border border-blue-400/25 bg-blue-500/10 px-4 text-sm text-blue-100 hover:bg-blue-500/20 disabled:cursor-not-allowed disabled:opacity-50">{saveText}</button>}
      </div>}
    </div>
  );
}
