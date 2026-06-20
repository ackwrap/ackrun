import { Database } from 'lucide-react';
import React from 'react';

import { api } from '@/services/api';
import { SyncScheduleControls } from '@/components/ui/SyncScheduleControls';
import { Toast } from '@/components/ui/Toast';
import type { GeoAsset } from '@/services/types';
import type { GeoDomainsResponse } from '@/services/types';
import type { GeoLookupResponse } from '@/services/types';
import type { GeoAssetRequest } from '@/services/types';

function normalizeDisplayIP(value: string) {
  return value.replace(/^::ffff:(\d+\.\d+\.\d+\.\d+)$/i, '$1');
}

function normalizeGeoIPMatch(value: string) {
  return value.replace(/^::ffff:(\d+\.\d+\.\d+\.\d+)(\s*=>\s*)/i, '$1$2');
}

function dnsServerLabel(value: string) {
  switch (value) {
    case 'cloudflare-doh': return 'Cloudflare DoH';
    case 'google-doh': return 'Google DoH';
    case 'aliyun-doh': return '阿里 DoH';
    case 'tencent-doh': return '腾讯 DoH';
    case 'system': return '系统 DNS';
    default: return value || '系统 DNS';
  }
}

function syncScheduleLabel(syncModes: Array<{ value: string; label: string }>, weekdays: string[], mode: string, time: string, weekday: number) {
  const label = syncModes.find(item => item.value === mode)?.label || mode;
  if (mode === 'off') return label;
  if (mode === 'weekly') return `${label} ${weekdays[weekday] || ''} ${time || '--'}`;
  if (mode === 'monthly') return `${label} ${weekday || 1}号 ${time || '--'}`;
  return `${label} ${time || '--'}`;
}

interface GeoDatabaseSectionProps {
  geoAssets: GeoAsset[];
  syncModes: Array<{ value: string; label: string }>;
  weekdays: string[];
  onSyncAll: () => void;
  onSyncOne: (item: GeoAsset) => void;
  onUpdate: (item: GeoAsset, body: GeoAssetRequest) => Promise<void>;
  syncing: boolean;
  message: string;
  messageType: 'success' | 'error';
  formatTime: (value: number) => string;
  syncStatusLabel: (value: string) => string;
  syncStatusClass: (value: string) => string;
}

export function GeoDatabaseSection({
  geoAssets,
  syncModes,
  weekdays,
  onSyncAll,
  onSyncOne,
  onUpdate,
  syncing,
  message,
  messageType,
  formatTime,
  syncStatusLabel,
  syncStatusClass,
}: GeoDatabaseSectionProps) {
  const [lookupTarget, setLookupTarget] = React.useState('');
  const [lookupDNSServer, setLookupDNSServer] = React.useState('cloudflare-doh');
  const [lookupLoading, setLookupLoading] = React.useState(false);
  const [lookupResult, setLookupResult] = React.useState<GeoLookupResponse | null>(null);
  const [lookupError, setLookupError] = React.useState('');
  const [tagLookup, setTagLookup] = React.useState('');
  const [tagLoading, setTagLoading] = React.useState(false);
  const [tagResult, setTagResult] = React.useState<GeoDomainsResponse | null>(null);
  const [tagError, setTagError] = React.useState('');
  const [geoDrafts, setGeoDrafts] = React.useState<Record<number, GeoAssetRequest>>({});
  const [editingScheduleId, setEditingScheduleId] = React.useState<number | null>(null);
  const tagPageSize = 100;
  const tagPage = tagResult ? Math.floor(tagResult.offset / tagResult.limit) + 1 : 1;
  const tagTotalPages = tagResult ? Math.max(1, Math.ceil(tagResult.total / tagResult.limit)) : 1;
  const editingScheduleAsset = editingScheduleId ? geoAssets.find(item => item.id === editingScheduleId) : null;

  React.useEffect(() => {
    setGeoDrafts(current => {
      const next = { ...current };
      for (const item of geoAssets) {
        if (!next[item.id]) {
          next[item.id] = {
            url: item.url,
            use_proxy: item.use_proxy,
            sync_mode: item.sync_mode || 'off',
            sync_time: item.sync_time || '03:30:00',
            sync_weekday: item.sync_weekday || 0,
          };
        }
      }
      return next;
    });
  }, [geoAssets]);

  const updateGeoDraft = (id: number, patch: Partial<GeoAssetRequest>) => {
    setGeoDrafts(current => ({
      ...current,
      [id]: {
        ...(current[id] || { url: '', use_proxy: false, sync_mode: 'off', sync_time: '03:30:00', sync_weekday: 0 }),
        ...patch,
      },
    }));
  };

  const saveGeoDraft = async (item: GeoAsset) => {
    const draft = geoDrafts[item.id];
    if (!draft) return;
    await onUpdate(item, {
      ...draft,
      url: item.url,
      sync_time: draft.sync_mode === 'off' ? '' : draft.sync_time,
      sync_weekday: draft.sync_mode === 'weekly' || draft.sync_mode === 'monthly' ? draft.sync_weekday : 0,
    });
    setEditingScheduleId(null);
  };
  const resolvedIPs = (lookupResult?.resolved_ips ?? []).map(normalizeDisplayIP);
  const geoIPMatches = (lookupResult?.geoip_matches ?? []).map(normalizeGeoIPMatch);
  const geositeMatches = lookupResult?.geosite_matches ?? [];

  const doLookup = async () => {
    const target = lookupTarget.trim();
    if (!target) {
      setLookupError('请输入域名或 IP');
      return;
    }
    setLookupLoading(true);
    setLookupError('');
    try {
      setLookupResult(await api.lookupGeo(target, lookupDNSServer));
    } catch (e: any) {
      setLookupResult(null);
      setLookupError(e.message || 'Geo 查询失败');
    } finally {
      setLookupLoading(false);
    }
  };

  const doTagLookup = async (offset = 0) => {
    const tag = tagLookup.trim();
    if (!tag) {
      setTagError('请输入 geosite tag，例如 google-deepmind');
      return;
    }
    setTagLoading(true);
    setTagError('');
    try {
      setTagResult(await api.lookupGeositeDomains(tag, tagPageSize, offset));
    } catch (e: any) {
      setTagResult(null);
      setTagError(e.message || 'GeoSite tag 查询失败');
    } finally {
      setTagLoading(false);
    }
  };

  const lookupSuggestedTag = async (tag: string) => {
    setTagLookup(tag);
    setTagLoading(true);
    setTagError('');
    try {
      setTagResult(await api.lookupGeositeDomains(tag, tagPageSize, 0));
    } catch (e: any) {
      setTagResult(null);
      setTagError(e.message || 'GeoSite tag 查询失败');
    } finally {
      setTagLoading(false);
    }
  };

  const changeTagPage = async (page: number) => {
    if (!tagResult) return;
    const nextPage = Math.min(Math.max(page, 1), tagTotalPages);
    await doTagLookup((nextPage - 1) * tagResult.limit);
  };

  return (
    <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.9),rgba(16,27,43,0.72))] p-5 shadow-[var(--shadow-card)]">
      <div className="mb-4 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-blue-400/20 bg-blue-500/10 text-blue-200"><Database size={18} /></div>
          <div>
            <h3 className="text-sm font-semibold text-white">Geo 数据库</h3>
            <p className="mt-1 text-xs text-[var(--text-tertiary)]">管理 sing-box 使用的 geoip.db 和 geosite.db，本地缓存后供配置引用。</p>
          </div>
        </div>
        <button disabled={syncing} onClick={onSyncAll} className={`h-8 rounded-md border px-3 text-xs ${syncing ? 'cursor-not-allowed border-blue-400/10 bg-blue-500/5 text-blue-100/45' : 'border-blue-400/25 bg-blue-500/10 text-blue-100 hover:bg-blue-500/20'}`}>更新全部 Geo</button>
      </div>

      <Toast message={message} type={messageType} />

      {geoAssets.length === 0 ? (
        <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] px-4 py-10 text-center text-sm text-[var(--text-secondary)]">暂无 Geo 数据库状态</div>
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {geoAssets.map(item => (
            <div key={item.id} className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-semibold text-white">{item.name}</span>
                  <span className="rounded bg-white/[0.05] px-2 py-0.5 font-mono text-xs text-[var(--text-secondary)]">{item.type}.db</span>
                  <span className={`rounded px-2 py-0.5 text-xs ${syncStatusClass(item.sync_status)}`}>{syncStatusLabel(item.sync_status)}</span>
                  <span className={`rounded px-2 py-0.5 text-xs ${item.use_proxy ? 'bg-blue-500/10 text-blue-200' : 'bg-emerald-500/10 text-emerald-300'}`}>{item.use_proxy ? '代理下载' : '直连下载'}</span>
                  <button
                    type="button"
                    onClick={() => setEditingScheduleId(item.id)}
                    className="rounded border border-blue-400/25 bg-blue-500/10 px-2 py-0.5 text-xs text-blue-100 hover:bg-blue-500/20"
                  >
                    同步周期：{syncScheduleLabel(syncModes, weekdays, item.sync_mode || 'off', item.sync_time, item.sync_weekday)}
                  </button>
                </div>
                <button disabled={syncing} onClick={() => onSyncOne(item)} className={`rounded-md border px-3 py-1 text-xs ${syncing ? 'cursor-not-allowed border-blue-400/10 bg-blue-500/5 text-blue-100/45' : 'border-blue-400/25 bg-blue-500/10 text-blue-100 hover:bg-blue-500/20'}`}>更新</button>
              </div>
              <div className="mt-3 truncate font-mono text-xs text-[var(--text-tertiary)]" title={item.url}>{item.url}</div>
              <div className="mt-3 grid gap-2 text-xs text-[var(--text-tertiary)] sm:grid-cols-2">
                <div>最后同步：{formatTime(item.last_sync_at)}</div>
                <div>本地缓存：{formatTime(item.cached_updated_at)}</div>
                <div className="truncate" title={item.local_path}>路径：{item.local_path || '--'}</div>
                <div>同步周期：{syncScheduleLabel(syncModes, weekdays, item.sync_mode || 'off', item.sync_time, item.sync_weekday)}</div>
              </div>
              {item.sync_error && <div className="mt-3 rounded border border-red-400/20 bg-red-500/10 px-2 py-1 text-xs text-red-300">{item.sync_error}</div>}
            </div>
          ))}
        </div>
      )}

      <div className="mt-5 rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4">
        <div className="mb-3">
          <h4 className="text-sm font-semibold text-white">Geo 数据库深度查询</h4>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">输入域名或 IP，直接读取本地 geoip.db / geosite.db 查询命中结果。</p>
        </div>
        <div className="grid gap-2 lg:grid-cols-[minmax(0,1fr)_auto_auto]">
          <input
            value={lookupTarget}
            onChange={e => setLookupTarget(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') void doLookup(); }}
            placeholder="例如 google.com 或 8.8.8.8"
            className="min-w-0 flex-1 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none placeholder:text-[var(--text-tertiary)] focus:border-blue-400"
          />
          <button disabled={lookupLoading} onClick={doLookup} className={`rounded-md border px-4 py-2 text-sm ${lookupLoading ? 'cursor-not-allowed border-blue-400/10 bg-blue-500/5 text-blue-100/45' : 'border-blue-400/25 bg-blue-500/10 text-blue-100 hover:bg-blue-500/20'}`}>{lookupLoading ? '查询中...' : '查询'}</button>
          <select
            value={lookupDNSServer}
            onChange={e => setLookupDNSServer(e.target.value)}
            className="rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400"
            title="指定 DNS 服务器"
          >
            <option className="bg-[#152235] text-white" value="cloudflare-doh">Cloudflare DoH</option>
            <option className="bg-[#152235] text-white" value="google-doh">Google DoH</option>
            <option className="bg-[#152235] text-white" value="aliyun-doh">阿里 DoH</option>
            <option className="bg-[#152235] text-white" value="tencent-doh">腾讯 DoH</option>
            <option className="bg-[#152235] text-white" value="system">系统 DNS</option>
          </select>
        </div>
        {lookupError && <div className="mt-3 rounded border border-red-400/20 bg-red-500/10 px-3 py-2 text-xs text-red-300">{lookupError}</div>}
        {lookupResult && (
          <div className="mt-4 space-y-3 text-sm text-[var(--text-secondary)]">
            <div className="grid gap-2 md:grid-cols-3">
              <div>目标：<span className="font-mono text-white">{lookupResult.target}</span></div>
              <div>类型：<span className="text-white">{lookupResult.target_type}</span></div>
              <div>DNS：<span className="font-mono text-white">{dnsServerLabel(lookupResult.dns_server)}</span></div>
              <div>状态：<span className="text-white">{lookupResult.message}</span></div>
            </div>
            <div>
              <div className="mb-1 text-xs text-[var(--text-tertiary)]">解析 IP</div>
              <div className="flex flex-wrap gap-2">{resolvedIPs.length ? resolvedIPs.map(ip => <span key={ip} className="rounded bg-white/[0.05] px-2 py-1 font-mono text-xs text-white">{ip}</span>) : <span className="text-xs text-[var(--text-tertiary)]">无</span>}</div>
            </div>
            <div className="grid gap-3 lg:grid-cols-2">
              <div className="rounded-lg border border-[var(--border-default)] bg-black/10 p-3">
                <div className="mb-2 text-xs font-semibold text-white">GeoIP 命中</div>
                {geoIPMatches.length ? geoIPMatches.map(item => <div key={item} className="font-mono text-xs text-[var(--text-secondary)]">{item}</div>) : <div className="text-xs text-[var(--text-tertiary)]">无命中或 GeoIP 数据库未就绪</div>}
              </div>
              <div className="rounded-lg border border-[var(--border-default)] bg-black/10 p-3">
                <div className="mb-2 text-xs font-semibold text-white">GeoSite 命中</div>
                <div className="max-h-48 overflow-auto space-y-1">{geositeMatches.length ? geositeMatches.map(item => <div key={item} className="font-mono text-xs text-[var(--text-secondary)]">{item}</div>) : <div className="text-xs text-[var(--text-tertiary)]">无命中或 GeoSite 数据库未就绪</div>}</div>
              </div>
            </div>
          </div>
        )}

        <div className="mt-5 border-t border-[var(--border-default)] pt-4">
          <div className="mb-3">
            <h4 className="text-sm font-semibold text-white">GeoSite tag 条目反查</h4>
            <p className="mt-1 text-xs text-[var(--text-tertiary)]">输入 geosite tag，例如 google-deepmind，读取该分类下的所有域名/关键词/正则条目。</p>
          </div>
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
            <input
              value={tagLookup}
              onChange={e => setTagLookup(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') void doTagLookup(); }}
              placeholder="例如 google-deepmind 或 openai"
              className="min-w-0 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none placeholder:text-[var(--text-tertiary)] focus:border-blue-400"
            />
            <button disabled={tagLoading} onClick={() => void doTagLookup()} className={`rounded-md border px-4 py-2 text-sm ${tagLoading ? 'cursor-not-allowed border-blue-400/10 bg-blue-500/5 text-blue-100/45' : 'border-blue-400/25 bg-blue-500/10 text-blue-100 hover:bg-blue-500/20'}`}>{tagLoading ? '查询中...' : '反查条目'}</button>
          </div>
          {tagError && <div className="mt-3 rounded border border-red-400/20 bg-red-500/10 px-3 py-2 text-xs text-red-300">{tagError}</div>}
        </div>
      </div>

      {tagResult && (
        <div className="aw-modal-backdrop">
          <div className="aw-modal-panel max-w-3xl">
            <div className="flex items-start justify-between gap-4 border-b border-[var(--border-default)] px-5 py-4">
              <div>
                <h3 className="text-base font-semibold text-white">GeoSite tag 条目</h3>
                <p className="mt-1 font-mono text-xs text-[var(--text-tertiary)]">{tagResult.tag} · 第 {tagPage} / {tagTotalPages} 页 · 共 {tagResult.total} 条</p>
              </div>
              <button onClick={() => setTagResult(null)} className="aw-modal-close" title="关闭">×</button>
            </div>
            <div className="max-h-[70vh] overflow-auto p-5">
              {tagResult.items.length === 0 && tagResult.suggestions?.length ? (
                <div className="space-y-3">
                  <div className="rounded-lg border border-amber-400/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-100">
                    未找到精确 tag：<span className="font-mono">{tagResult.tag}</span>。请选择下面的相似 tag 继续反查。
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {tagResult.suggestions.map(tag => (
                      <button key={tag} type="button" onClick={() => void lookupSuggestedTag(tag)} className="rounded-md border border-emerald-400/25 bg-emerald-500/10 px-3 py-1.5 font-mono text-xs text-emerald-100 hover:bg-emerald-500/20">{tag}</button>
                    ))}
                  </div>
                </div>
              ) : tagResult.items.length ? (
                <div className="space-y-1">
                  {tagResult.items.map((item, index) => (
                    <div key={`${item.type}-${item.value}-${index}`} className="grid gap-2 rounded border border-[var(--border-default)] bg-white/[0.03] px-3 py-2 font-mono text-xs text-[var(--text-secondary)] sm:grid-cols-[150px_minmax(0,1fr)]">
                      <span className="text-blue-200">{item.type}</span>
                      <span className="break-all text-white">{item.value}</span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] px-4 py-10 text-center text-sm text-[var(--text-secondary)]">无条目或 GeoSite 数据库未就绪</div>
              )}
            </div>
            {tagResult.total > tagResult.limit && !tagResult.suggestions?.length && (
              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-default)] px-5 py-4 text-xs text-[var(--text-secondary)]">
                <div>每页 {tagResult.limit} 条，当前显示 {tagResult.offset + 1}-{Math.min(tagResult.offset + tagResult.items.length, tagResult.total)}</div>
                <div className="flex items-center gap-2">
                  <button disabled={tagLoading || tagPage <= 1} onClick={() => void changeTagPage(1)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 hover:bg-white/[0.08]">首页</button>
                  <button disabled={tagLoading || tagPage <= 1} onClick={() => void changeTagPage(tagPage - 1)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 hover:bg-white/[0.08]">上一页</button>
                  <span className="px-2 text-white">{tagPage} / {tagTotalPages}</span>
                  <button disabled={tagLoading || tagPage >= tagTotalPages} onClick={() => void changeTagPage(tagPage + 1)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 hover:bg-white/[0.08]">下一页</button>
                  <button disabled={tagLoading || tagPage >= tagTotalPages} onClick={() => void changeTagPage(tagTotalPages)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 hover:bg-white/[0.08]">末页</button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {editingScheduleAsset && (
        <div className="aw-modal-backdrop">
          <div className="aw-modal-panel max-w-2xl">
            <div className="flex items-start justify-between gap-4 border-b border-[var(--border-default)] px-5 py-4">
              <div>
                <h3 className="text-base font-semibold text-white">自动更新</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">{editingScheduleAsset.name} · {editingScheduleAsset.type}.db</p>
              </div>
              <button onClick={() => setEditingScheduleId(null)} className="aw-modal-close" title="关闭">×</button>
            </div>
            <div className="p-5">
              <SyncScheduleControls
                value={geoDrafts[editingScheduleAsset.id] || { url: editingScheduleAsset.url, use_proxy: editingScheduleAsset.use_proxy, sync_mode: editingScheduleAsset.sync_mode || 'off', sync_time: editingScheduleAsset.sync_time || '03:30:00', sync_weekday: editingScheduleAsset.sync_weekday || 0 }}
                syncModes={syncModes}
                weekdays={weekdays}
                disabled={syncing}
                showProxy
                onChange={patch => updateGeoDraft(editingScheduleAsset.id, patch)}
                onSave={() => void saveGeoDraft(editingScheduleAsset)}
              />
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
