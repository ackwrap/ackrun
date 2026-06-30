import { Plus } from 'lucide-react';

import { defaultEmojis, EmojiPicker } from '@/components/ui/EmojiPicker';
import type { RouteRule, RouteRuleSubscription } from '@/services/types';

interface SelectOption {
  value: string;
  label: string;
}

interface RouteRuleFormModalProps {
  editing: RouteRule | null;
  name: string;
  enabled: boolean;
  ruleType: string;
  valuesText: string;
  outbound: string;
  invert: boolean;
  values: string[];
  subscriptions: RouteRuleSubscription[];
  outboundOptions: SelectOption[];
  onNameChange: (value: string) => void;
  onEnabledChange: (value: boolean) => void;
  onRuleTypeChange: (value: string) => void;
  onValuesTextChange: (value: string) => void;
  onOutboundChange: (value: string) => void;
  onInvertChange: (value: boolean) => void;
  onAppendRuleSetTag: (tag: string) => void;
  onClose: () => void;
  onSave: () => void;
  ruleTypes: SelectOption[];
  ruleTypeLabel: (value: string) => string;
  outboundLabel: (value: string) => string;
  outboundClass: (value: string) => string;
  previewDraft: (ruleType: string, values: string[], outbound: string, invert: boolean) => string;
  ruleValueHelp: (ruleType: string) => string;
  ruleValuePlaceholder: (ruleType: string) => string;
}

const ruleEmojis = defaultEmojis;

function looksLikeEmojiPrefix(value: string) {
  return /^([\p{Extended_Pictographic}\p{Regional_Indicator}\uFE0F\u200D]+)$/u.test(value);
}

function stripRuleEmoji(value: string) {
  const trimmed = value.trimStart();
  for (const emoji of ruleEmojis) {
    if (trimmed === emoji) return '';
    if (trimmed.startsWith(`${emoji} `)) return trimmed.slice(`${emoji} `.length);
    if (trimmed.startsWith(emoji)) return trimmed.slice(emoji.length).trimStart();
  }
  const [first, ...rest] = trimmed.split(/\s+/);
  if (first && looksLikeEmojiPrefix(first)) return rest.join(' ');
  return value;
}

function selectedRuleEmoji(value: string) {
  const trimmed = value.trimStart();
  const knownEmoji = ruleEmojis.find(emoji => trimmed === emoji || trimmed.startsWith(`${emoji} `) || trimmed.startsWith(emoji));
  if (knownEmoji) return knownEmoji;
  const first = trimmed.split(/\s+/)[0] || '';
  return looksLikeEmojiPrefix(first) ? first : '';
}

function parseMixedBlocks(valuesText: string) {
  const blocks = valuesText.split('\n').map(line => {
    const trimmed = line.trim();
    const separator = trimmed.search(/[:=]/);
    if (separator <= 0) return { ruleType: 'domain_suffix', value: trimmed };
    return { ruleType: trimmed.slice(0, separator).trim(), value: trimmed.slice(separator + 1).trim() };
  }).filter(item => item.ruleType || item.value);
  return blocks.length ? blocks : [{ ruleType: 'geosite', value: '' }];
}

function serializeMixedBlocks(blocks: Array<{ ruleType: string; value: string }>) {
  return blocks.map(item => `${item.ruleType}:${item.value}`).join('\n');
}

export function RouteRuleFormModal({
  editing,
  name,
  enabled,
  ruleType,
  valuesText,
  outbound,
  invert,
  values,
  subscriptions,
  outboundOptions,
  onNameChange,
  onEnabledChange,
  onRuleTypeChange,
  onValuesTextChange,
  onOutboundChange,
  onInvertChange,
  onAppendRuleSetTag,
  onClose,
  onSave,
  ruleTypes,
  ruleTypeLabel,
  outboundLabel,
  outboundClass,
  previewDraft,
  ruleValueHelp,
  ruleValuePlaceholder,
}: RouteRuleFormModalProps) {
  const isSystemRule = editing?.is_system ?? false;
  const currentEmoji = selectedRuleEmoji(name);
  const mixedBlocks = parseMixedBlocks(valuesText);
  const mixedRuleTypes = ruleTypes.filter(item => item.value !== 'mixed');
  const updateEmoji = (emoji: string) => {
    const plainName = stripRuleEmoji(name);
    onNameChange(emoji ? `${emoji} ${plainName}`.trim() : plainName);
  };
  const updateMixedBlock = (index: number, patch: Partial<{ ruleType: string; value: string }>) => {
    const next = mixedBlocks.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item);
    onValuesTextChange(serializeMixedBlocks(next));
  };
  const addMixedBlock = () => {
    onValuesTextChange(serializeMixedBlocks([...mixedBlocks, { ruleType: 'domain_suffix', value: '' }]));
  };
  const removeMixedBlock = (index: number) => {
    const next = mixedBlocks.filter((_, itemIndex) => itemIndex !== index);
    onValuesTextChange(serializeMixedBlocks(next.length ? next : [{ ruleType: 'geosite', value: '' }]));
  };

  return (
    <div className="aw-modal-backdrop">
      <div className="aw-modal-panel max-w-5xl">
        <div className="flex items-start justify-between gap-4 border-b border-[var(--border-default)] px-5 py-4">
          <div>
            <h3 className="text-base font-semibold text-white">{isSystemRule ? '查看路由规则' : editing ? '编辑路由规则' : '添加路由规则'}</h3>
            <p className="mt-1 text-xs text-[var(--text-tertiary)]">{ruleValueHelp(ruleType)}</p>
          </div>
          <button onClick={onClose} className="aw-modal-close" title="关闭">×</button>
        </div>

        <div className="grid max-h-[72vh] gap-4 overflow-auto p-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.65fr)]">
          <div className="space-y-4">
            <div className="grid gap-3 lg:grid-cols-2">
              <label className="block lg:col-span-2">
                <span className="text-xs text-[var(--text-tertiary)]">规则名称</span>
                <div className="mt-1 grid gap-2 sm:grid-cols-[48px_minmax(0,1fr)]">
                  <EmojiPicker value={currentEmoji} onChange={updateEmoji} disabled={isSystemRule} />
                  <input value={name} onChange={e => onNameChange(e.target.value)} disabled={isSystemRule} placeholder="例如：🤖 OpenAI 代理" className="w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-emerald-400 disabled:cursor-not-allowed disabled:opacity-70" />
                </div>
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">匹配类型</span>
                <select value={ruleType} onChange={e => onRuleTypeChange(e.target.value)} disabled={isSystemRule} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-emerald-400 disabled:cursor-not-allowed disabled:opacity-70">
                  {ruleTypes.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">命中后走</span>
                <select value={outbound} onChange={e => onOutboundChange(e.target.value)} disabled={isSystemRule} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-emerald-400 disabled:cursor-not-allowed disabled:opacity-70">
                  {outboundOptions.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
                </select>
              </label>
              {ruleType === 'mixed' ? (
                <div className="lg:col-span-2 space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-xs text-[var(--text-tertiary)]">混合匹配条件</span>
                    {!isSystemRule && <button type="button" onClick={addMixedBlock} className="inline-flex h-8 items-center gap-1 rounded-md border border-emerald-400/25 bg-emerald-500/10 px-3 text-xs font-medium text-emerald-100 hover:bg-emerald-500/20"><Plus size={13} />添加下一条规则</button>}
                  </div>
                  {mixedBlocks.map((block, index) => (
                    <div key={index} className="rounded-xl border border-[var(--border-default)] bg-white/[0.025] p-3">
                      <div className="mb-2 flex items-center justify-between gap-3">
                        <span className="text-xs font-medium text-white">条件 #{index + 1}</span>
                        {mixedBlocks.length > 1 && !isSystemRule && <button type="button" onClick={() => removeMixedBlock(index)} className="rounded-md border border-red-400/25 bg-red-500/10 px-2 py-1 text-xs text-red-200 hover:bg-red-500/20">删除</button>}
                      </div>
                      <div className="grid gap-2 sm:grid-cols-[220px_minmax(0,1fr)]">
                        <label className="block">
                          <span className="text-xs text-[var(--text-tertiary)]">匹配类型</span>
                          <select value={block.ruleType} onChange={e => updateMixedBlock(index, { ruleType: e.target.value })} disabled={isSystemRule} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-emerald-400 disabled:cursor-not-allowed disabled:opacity-70">
                            {mixedRuleTypes.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
                          </select>
                        </label>
                        <label className="block">
                          <span className="text-xs text-[var(--text-tertiary)]">匹配值</span>
                          <input value={block.value} onChange={e => updateMixedBlock(index, { value: e.target.value })} disabled={isSystemRule} placeholder={ruleValuePlaceholder(block.ruleType).split('\n')[0]} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-emerald-400 disabled:cursor-not-allowed disabled:opacity-70" />
                        </label>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <label className="block lg:col-span-2">
                  <span className="text-xs text-[var(--text-tertiary)]">匹配值</span>
                  <textarea value={valuesText} onChange={e => onValuesTextChange(e.target.value)} disabled={isSystemRule} rows={7} placeholder={ruleValuePlaceholder(ruleType)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-emerald-400 disabled:cursor-not-allowed disabled:opacity-70" />
                </label>
              )}
              {ruleType === 'rule_set' && subscriptions.length > 0 && (
                <div className="lg:col-span-2 rounded-lg border border-emerald-400/15 bg-emerald-500/[0.06] p-3">
                  <div className="mb-2 text-xs text-emerald-100">可用规则集 tag</div>
                  <div className="flex flex-wrap gap-2">
                    {subscriptions.map(item => <button key={item.id} type="button" onClick={() => onAppendRuleSetTag(item.tag)} disabled={isSystemRule} className="rounded-md border border-emerald-400/25 bg-emerald-500/10 px-2 py-1 font-mono text-xs text-emerald-100 hover:bg-emerald-500/20 disabled:cursor-not-allowed disabled:opacity-70">{item.tag}</button>)}
                  </div>
                </div>
              )}
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex gap-2">
                <label className="inline-flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-[var(--text-secondary)]"><input type="checkbox" checked={enabled} disabled={isSystemRule} onChange={e => onEnabledChange(e.target.checked)} />启用</label>
                <label className="inline-flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-[var(--text-secondary)]"><input type="checkbox" checked={invert} disabled={isSystemRule} onChange={e => onInvertChange(e.target.checked)} />反向匹配</label>
              </div>
              {isSystemRule ? <span className="text-xs text-[var(--text-tertiary)]">系统默认规则仅可在列表中启停。</span> : <button onClick={onSave} className="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-4 text-sm font-medium text-white shadow-sm hover:bg-emerald-500"><Plus size={15} />{editing ? '更新规则' : '添加规则'}</button>}
            </div>
          </div>

          <aside className="space-y-4">
            <div className="rounded-xl border border-[var(--border-default)] bg-black/15 p-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h4 className="text-sm font-semibold text-white">规则草稿</h4>
                  <p className="mt-1 text-xs text-[var(--text-tertiary)]">保存前预览当前表单会生成什么。</p>
                </div>
                <span className={`rounded-full border px-2 py-1 text-xs ${outboundClass(outbound)}`}>{outboundLabel(outbound)}</span>
              </div>
              <div className="mt-4 grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-md border border-[var(--border-default)] bg-white/[0.035] px-3 py-2"><span className="text-[var(--text-tertiary)]">类型</span><div className="mt-1 text-white">{ruleTypeLabel(ruleType)}</div></div>
                <div className="rounded-md border border-[var(--border-default)] bg-white/[0.035] px-3 py-2"><span className="text-[var(--text-tertiary)]">匹配值</span><div className="mt-1 text-white">{values.length}</div></div>
              </div>
              <pre className="mt-4 max-h-[260px] overflow-auto rounded-lg border border-[var(--border-default)] bg-[#07111f] p-3 font-mono text-xs leading-5 text-blue-50">{previewDraft(ruleType, values, outbound, invert)}</pre>
            </div>
            <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4 text-xs leading-6 text-[var(--text-secondary)]">
              <div className="font-semibold text-white">提示</div>
              <p className="mt-2">规则只决定命中后走直连、阻断或某个策略。策略里怎么选节点，请到策略组管理里配置。</p>
            </div>
          </aside>
        </div>
      </div>
    </div>
  );
}
