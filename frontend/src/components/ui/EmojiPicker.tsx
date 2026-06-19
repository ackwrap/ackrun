import React from 'react';

const emojiGroups = [
  { key: 'common', label: '常用', emojis: ['🌐', '🤖', '🎬', '📺', '🎮', '💬', '🛡️', '🚫', '🇨🇳', '✈️', '⚡', '🚀', '✨', '🔎', '🧠', '☁️'] },
  { key: 'network', label: '网络', emojis: ['🌐', '🛜', '📡', '🛰️', '☁️', '🔗', '🧭', '🧬', '🧠', '🤖', '💬', '📨', '📧', '🔎', '💻', '📱', '🖥️', '⌨️', '🖱️', '🧰', '🧪', '📦', '📁', '🗂️'] },
  { key: 'media', label: '娱乐', emojis: ['🎬', '📺', '🎵', '🎧', '🎤', '🎮', '🕹️', '📹', '📷', '🎞️', '🎥', '🍿', '🎭', '🎨', '📚', '📰', '🏀', '⚽', '🏈', '🎾', '🎲', '🧩'] },
  { key: 'security', label: '安全', emojis: ['🛡️', '🚫', '🔒', '🔓', '🔐', '🔑', '🧱', '⚠️', '✅', '❌', '⛔', '🧯', '🚨', '👁️', '🕵️', '🧹', '🗑️', '📛', '🔞', '☢️', '☣️'] },
  { key: 'region', label: '地区', emojis: ['🇨🇳', '🇭🇰', '🇲🇴', '🇹🇼', '🇯🇵', '🇰🇷', '🇸🇬', '🇺🇸', '🇬🇧', '🇩🇪', '🇫🇷', '🇳🇱', '🇨🇦', '🇦🇺', '🇮🇳', '🇷🇺', '🇧🇷', '🇪🇺', '🇹🇭', '🇻🇳', '🇲🇾', '🇵🇭', '🇮🇩', '🇹🇷'] },
  { key: 'symbol', label: '符号', emojis: ['⭐', '🌟', '✨', '🔥', '⚡', '💎', '🎯', '📌', '📍', '🔴', '🟠', '🟡', '🟢', '🔵', '🟣', '⚫', '⚪', '🟤', '🔺', '🔻', '🔸', '🔹', '🔶', '🔷'] },
  { key: 'more', label: '更多', emojis: ['😀', '😄', '😁', '😎', '🤔', '😺', '🐶', '🐱', '🦊', '🐼', '🐳', '🦄', '🌈', '☀️', '🌙', '⭐', '🌍', '🏠', '🏢', '🚗', '🚄', '✈️', '🚢', '⏱️', '📅', '💰', '💡', '🔧', '🧲', '🪄'] },
];

export const defaultEmojis = emojiGroups.flatMap(group => group.emojis).filter((emoji, index, items) => items.indexOf(emoji) === index);

interface EmojiPickerProps {
  value: string;
  onChange: (value: string) => void;
  emojis?: string[];
}

export function EmojiPicker({ value, onChange, emojis = defaultEmojis }: EmojiPickerProps) {
  const [open, setOpen] = React.useState(false);
  const [customEmoji, setCustomEmoji] = React.useState('');
  const [activeGroup, setActiveGroup] = React.useState(emojiGroups[0].key);
  const [query, setQuery] = React.useState('');

  const visibleEmojis = React.useMemo(() => {
    if (query.trim()) {
      return emojis.filter(emoji => emoji.includes(query.trim()));
    }
    return emojiGroups.find(group => group.key === activeGroup)?.emojis ?? emojis;
  }, [activeGroup, emojis, query]);

  const selectEmoji = (emoji: string) => {
    onChange(emoji);
    setOpen(false);
  };

  const applyCustomEmoji = () => {
    const emoji = customEmoji.trim();
    if (!emoji) return;
    selectEmoji(emoji);
    setCustomEmoji('');
  };

  return (
    <div className="relative">
      <button type="button" onClick={() => setOpen(current => !current)} className="flex h-10 w-12 items-center justify-center rounded-md border border-[var(--border-default)] bg-[#152235] text-base text-white outline-none hover:border-emerald-400/60" title="选择 emoji">
        {value || '无'}
      </button>
      {open && (
        <div className="absolute left-0 top-11 z-20 w-[380px] rounded-xl border border-[var(--border-default)] bg-[#101b2b] p-3 shadow-[var(--shadow-card)]">
          <div className="mb-2 flex items-center justify-between gap-2">
            <span className="text-xs font-medium text-white">选择 emoji</span>
            <button type="button" onClick={() => selectEmoji('')} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-2 py-1 text-xs text-[var(--text-secondary)] hover:text-white">清除</button>
          </div>
          <input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索或粘贴 emoji" className="mb-2 h-8 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-2 text-sm text-white outline-none placeholder:text-[var(--text-tertiary)] focus:border-emerald-400" />
          <div className="mb-2 flex gap-1 overflow-x-auto pb-1">
            {emojiGroups.map(group => (
              <button key={group.key} type="button" onClick={() => { setActiveGroup(group.key); setQuery(''); }} className={`shrink-0 rounded-md border px-2 py-1 text-xs ${activeGroup === group.key && !query ? 'border-emerald-400/40 bg-emerald-500/15 text-emerald-100' : 'border-[var(--border-default)] bg-white/[0.03] text-[var(--text-secondary)] hover:text-white'}`}>{group.label}</button>
            ))}
          </div>
          <div className="grid max-h-56 grid-cols-10 gap-1 overflow-auto pr-1">
            {visibleEmojis.map(emoji => (
              <button key={emoji} type="button" onClick={() => selectEmoji(emoji)} className={`flex h-8 items-center justify-center rounded-md text-lg hover:bg-white/[0.08] ${value === emoji ? 'bg-emerald-500/20 ring-1 ring-emerald-400/40' : 'bg-white/[0.03]'}`}>{emoji}</button>
            ))}
            {visibleEmojis.length === 0 && <div className="col-span-10 py-5 text-center text-xs text-[var(--text-tertiary)]">没有匹配的 emoji，可在下方自定义输入</div>}
          </div>
          <div className="mt-3 grid grid-cols-[minmax(0,1fr)_auto] gap-2">
            <input value={customEmoji} onChange={event => setCustomEmoji(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') applyCustomEmoji(); }} placeholder="自定义 emoji" className="h-8 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-2 text-sm text-white outline-none focus:border-emerald-400" />
            <button type="button" onClick={applyCustomEmoji} className="h-8 rounded-md bg-emerald-600 px-3 text-xs font-medium text-white hover:bg-emerald-500">使用</button>
          </div>
        </div>
      )}
    </div>
  );
}
