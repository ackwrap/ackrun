import React from 'react';
import { useSearchParams } from 'react-router-dom';
import { Edit3, Eye, RefreshCw, Smile, Star, Tags, Trash2, Zap } from 'lucide-react';

import { PageHeader } from '@/components/layout/PageHeader';
import { Toast } from '@/components/ui/Toast';
import { api } from '@/services/api';
import type { NodeFacetItem, NodeItem, Subscription } from '@/services/types';
import { defaultFlag, getFlagImageURL } from '@/utils/nodeFlags';

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '未知', value: 'unknown' },
  { label: '可用', value: 'available' },
  { label: '不可用', value: 'unavailable' },
];

function formatTime(value: number) {
  return value > 0 ? new Date(value).toLocaleString() : '--';
}

function formatLatency(value: number) {
  return value > 0 ? `${value} ms` : '--';
}

function shortUID(uid: string) {
  return uid.length > 12 ? `${uid.slice(0, 12)}...` : uid;
}

function nodeAddress(node: NodeItem) {
  return `${node.server}:${node.server_port}`;
}

function prettyJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value || '{}'), null, 2);
  } catch {
    return value || '{}';
  }
}

export function NodesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialSubscriptionID = searchParams.get('subscription_id') || '';
  const [subscriptions, setSubscriptions] = React.useState<Subscription[]>([]);
  const [nodes, setNodes] = React.useState<NodeItem[]>([]);
  const [nodeFlags, setNodeFlags] = React.useState<Record<string, string>>({});
  const [facetTotal, setFacetTotal] = React.useState(0);
  const [typeFacets, setTypeFacets] = React.useState<NodeFacetItem[]>([]);
  const [subscriptionFacets, setSubscriptionFacets] = React.useState<NodeFacetItem[]>([]);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(50);
  const [loading, setLoading] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const toastType = message.includes('失败') || message.includes('错误') ? 'error' : 'success';
  const [keyword, setKeyword] = React.useState('');
  const [subscriptionID, setSubscriptionID] = React.useState(initialSubscriptionID);
  const [typeFilter, setTypeFilter] = React.useState('');
  const [statusFilter, setStatusFilter] = React.useState('');
  const [enabledFilter, setEnabledFilter] = React.useState('');
  const [preferredFilter, setPreferredFilter] = React.useState('');
  const [detail, setDetail] = React.useState<NodeItem | null>(null);
  const [selectedUIDs, setSelectedUIDs] = React.useState<Set<string>>(new Set());
  const [tcpingLoading, setTcpingLoading] = React.useState(false);
  const [tcpingUIDs, setTcpingUIDs] = React.useState<Set<string>>(new Set());
  const [renameOpen, setRenameOpen] = React.useState(false);
  const [renameMode, setRenameMode] = React.useState<'lines' | 'replace' | 'prefix' | 'suffix'>('prefix');
  const [renameText, setRenameText] = React.useState('');
  const [findText, setFindText] = React.useState('');
  const [replaceText, setReplaceText] = React.useState('');

  const loadSubscriptions = React.useCallback(async () => {
    try {
      setSubscriptions(await api.getSubscriptions());
    } catch (e: any) {
      setMessage(`订阅加载失败: ${e.message}`);
    }
  }, []);

  const loadNodes = React.useCallback(async () => {
    setLoading(true);
    try {
      const resp = await api.getNodes({
        subscription_id: subscriptionID ? Number(subscriptionID) : undefined,
        keyword,
        type: typeFilter,
        status: statusFilter,
        enabled: enabledFilter === '' ? undefined : enabledFilter === 'true',
        preferred: preferredFilter === '' ? undefined : preferredFilter === 'true',
        limit: pageSize,
        offset: (page - 1) * pageSize,
      });
      setNodes(resp.items);
      if (resp.items.length > 0) {
        const flags = await api.inferNodeFlags(resp.items.map(node => ({ key: node.uid, name: node.name, server: node.server })));
        setNodeFlags(Object.fromEntries(flags.items.map(item => [item.key, item.flag])));
      } else {
        setNodeFlags({});
      }
      setTotal(resp.total);
      setSelectedUIDs(new Set());
      const next = new URLSearchParams(searchParams);
      if (subscriptionID) next.set('subscription_id', subscriptionID); else next.delete('subscription_id');
      setSearchParams(next, { replace: true });
    } catch (e: any) {
      setMessage(`节点加载失败: ${e.message}`);
    } finally {
      setLoading(false);
    }
  }, [enabledFilter, keyword, page, pageSize, preferredFilter, searchParams, setSearchParams, statusFilter, subscriptionID, typeFilter]);

  const loadFacetNodes = React.useCallback(async () => {
    try {
      const resp = await api.getNodeFacets();
      setFacetTotal(resp.total);
      setTypeFacets(resp.types);
      setSubscriptionFacets(resp.subscriptions);
    } catch (e: any) {
      setMessage(`筛选统计加载失败: ${e.message}`);
    }
  }, []);

  React.useEffect(() => { loadSubscriptions(); }, [loadSubscriptions]);
  React.useEffect(() => { loadFacetNodes(); }, [loadFacetNodes]);
  React.useEffect(() => { loadNodes(); }, [loadNodes]);
  React.useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(() => setMessage(''), toastType === 'error' ? 5000 : 3000);
    return () => window.clearTimeout(timer);
  }, [message, toastType]);
  React.useEffect(() => {
    const nextTotalPages = Math.max(1, Math.ceil(total / pageSize));
    if (page > nextTotalPages) setPage(nextTotalPages);
  }, [page, pageSize, total]);

  const selectedNodes = React.useMemo(() => nodes.filter(node => selectedUIDs.has(node.uid)), [nodes, selectedUIDs]);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const pageStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const pageEnd = Math.min(total, page * pageSize);
  const chipClass = (active: boolean) => `rounded-md border px-4 py-2 text-sm transition ${active ? 'border-blue-400/50 bg-blue-500/20 text-blue-100 shadow-[0_0_0_1px_rgba(96,165,250,0.16)]' : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:border-blue-400/30 hover:bg-blue-500/10 hover:text-white'}`;
  const updateFilter = (setter: React.Dispatch<React.SetStateAction<string>>, value: string) => {
    setPage(1);
    setter(value);
  };

  const toggleEnabled = async (node: NodeItem) => {
    try {
      await api.setNodeEnabled(node.uid, !node.enabled);
      setNodes(prev => prev.map(item => item.uid === node.uid ? { ...item, enabled: !node.enabled } : item));
    } catch (e: any) {
      setMessage(`更新启用状态失败: ${e.message}`);
    }
  };

  const togglePreferred = async (node: NodeItem) => {
    try {
      await api.setNodePreferred(node.uid, !node.preferred);
      setNodes(prev => prev.map(item => item.uid === node.uid ? { ...item, preferred: !node.preferred } : item));
    } catch (e: any) {
      setMessage(`更新首选状态失败: ${e.message}`);
    }
  };

  const toggleSelected = (uid: string) => {
    setSelectedUIDs(prev => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid); else next.add(uid);
      return next;
    });
  };

  const toggleAllVisible = () => {
    setSelectedUIDs(prev => prev.size === nodes.length ? new Set() : new Set(nodes.map(node => node.uid)));
  };

  const syncSubscriptions = async () => {
    try {
      await api.syncAllSubscriptions();
      setMessage('已触发外部订阅同步，稍后刷新节点列表');
      await loadFacetNodes();
    } catch (e: any) {
      setMessage(`同步外部订阅失败: ${e.message}`);
    }
  };

  const batchSetEnabled = async (value: boolean) => {
    try {
      await Promise.all(selectedNodes.map(node => api.setNodeEnabled(node.uid, value)));
      setNodes(prev => prev.map(node => selectedUIDs.has(node.uid) ? { ...node, enabled: value } : node));
      setMessage(`已${value ? '启用' : '禁用'} ${selectedNodes.length} 个节点`);
    } catch (e: any) {
      setMessage(`批量更新失败: ${e.message}`);
    }
  };

  const batchSetPreferred = async () => {
    try {
      await Promise.all(selectedNodes.map(node => api.setNodePreferred(node.uid, true)));
      setNodes(prev => prev.map(node => selectedUIDs.has(node.uid) ? { ...node, preferred: true } : node));
      setMessage(`已将 ${selectedNodes.length} 个节点标记为首选`);
    } catch (e: any) {
      setMessage(`批量首选失败: ${e.message}`);
    }
  };

  const targetUIDs = () => selectedNodes.length > 0 ? selectedNodes.map(node => node.uid) : nodes.map(node => node.uid);

  const runTCPing = async () => {
    const uids = targetUIDs();
    if (uids.length === 0) return;
    setTcpingLoading(true);
    setTcpingUIDs(new Set(uids));
    try {
      const results = await api.tcpingNodes(uids);
      const resultMap = new Map(results.map(result => [result.uid, result]));
      setNodes(prev => prev.map(node => {
        const result = resultMap.get(node.uid);
        if (!result) return node;
        return { ...node, latency_ms: result.success ? result.latency_ms : 0, status: result.success ? 'available' : 'unavailable' };
      }));
      const success = results.filter(result => result.success).length;
      setMessage(`TCPing 完成：${success}/${results.length} 个节点可连通`);
    } catch (e: any) {
      setMessage(`TCPing 失败: ${e.message}`);
    } finally {
      setTcpingLoading(false);
      setTcpingUIDs(new Set());
    }
  };

  const runSingleTCPing = async (node: NodeItem) => {
    setTcpingUIDs(prev => new Set(prev).add(node.uid));
    try {
      const [result] = await api.tcpingNodes([node.uid]);
      if (result) {
        setNodes(prev => prev.map(item => item.uid === node.uid ? { ...item, latency_ms: result.success ? result.latency_ms : 0, status: result.success ? 'available' : 'unavailable' } : item));
        setMessage(result.success ? `${node.name} TCPing ${result.latency_ms} ms` : `${node.name} TCPing 失败: ${result.error || '不可达'}`);
      }
    } catch (e: any) {
      setMessage(`TCPing 失败: ${e.message}`);
    } finally {
      setTcpingUIDs(prev => {
        const next = new Set(prev);
        next.delete(node.uid);
        return next;
      });
    }
  };

  const addEmoji = async () => {
    if (selectedNodes.length === 0) return;
    try {
      const result = await api.addNodeEmoji(selectedNodes.map(node => node.uid));
      setMessage(`添加 emoji 完成：成功 ${result.success}，失败/跳过 ${result.failed}`);
      await loadNodes();
      await loadFacetNodes();
    } catch (e: any) {
      setMessage(`添加 emoji 失败: ${e.message}`);
    }
  };

  const openRename = () => {
    if (selectedNodes.length === 0) return;
    setRenameText(renameMode === 'lines' ? selectedNodes.map(node => node.name).join('\n') : '');
    setFindText('');
    setReplaceText('');
    setRenameOpen(true);
  };

  const saveRename = async () => {
    try {
      const uids = selectedNodes.map(node => node.uid);
      const payload = renameMode === 'lines'
        ? { uids, mode: renameMode, names: renameText.split('\n') }
        : renameMode === 'replace'
          ? { uids, mode: renameMode, find: findText, replace: replaceText }
          : renameMode === 'prefix'
            ? { uids, mode: renameMode, prefix: renameText }
            : { uids, mode: renameMode, suffix: renameText };
      const result = await api.batchRenameNodes(payload);
      setMessage(`修改名称完成：成功 ${result.success}，失败 ${result.failed}`);
      setRenameOpen(false);
      await loadNodes();
      await loadFacetNodes();
    } catch (e: any) {
      setMessage(`修改名称失败: ${e.message}`);
    }
  };

  const batchDelete = async () => {
    if (selectedNodes.length === 0) return;
    if (!confirm(`确定要删除选中的 ${selectedNodes.length} 个节点吗？此操作不可恢复。`)) return;
    try {
      const uids = selectedNodes.map(node => node.uid);
      const result = await api.batchDeleteNodes(uids);
      setMessage(`删除完成：成功 ${result.success}，失败 ${result.failed}`);
      setSelectedUIDs(new Set());
      await loadNodes();
      await loadFacetNodes();
    } catch (e: any) {
      setMessage(`删除失败: ${e.message}`);
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader title="节点管理" />
      <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]">
        <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div className="text-base font-semibold text-[var(--text-primary)]">节点列表 ({total})</div>
            <div className="mt-1 text-sm text-[var(--text-secondary)]">管理订阅解析后的节点，控制是否参与后续配置生成。</div>
            <div className="mt-1 text-xs text-[var(--text-tertiary)]">共 {total} 个节点{loading ? '，加载中...' : ''}</div>
          </div>
          <div className="flex flex-wrap justify-start gap-2 lg:justify-end">
            <button onClick={runTCPing} disabled={tcpingLoading || nodes.length === 0} className="aw-action-button aw-action-neutral h-9 px-3 text-sm"><Zap size={14} />{tcpingLoading ? '测速中...' : '节点测速'}</button>
            <button onClick={syncSubscriptions} className="aw-action-button aw-action-neutral h-9 px-3 text-sm"><RefreshCw size={14} />同步外部订阅</button>
            <button onClick={addEmoji} disabled={selectedNodes.length === 0} className="aw-action-button aw-action-neutral h-9 px-3 text-sm"><Smile size={14} />添加 emoji ({selectedNodes.length})</button>
            <button onClick={openRename} disabled={selectedNodes.length === 0} className="aw-action-button aw-action-neutral h-9 px-3 text-sm"><Edit3 size={14} />修改名称 ({selectedNodes.length})</button>
            <button onClick={batchSetPreferred} disabled={selectedNodes.length === 0} className="aw-action-button aw-action-neutral h-9 px-3 text-sm"><Tags size={14} />管理首选 ({selectedNodes.length})</button>
            <button onClick={() => batchSetEnabled(false)} disabled={selectedNodes.length === 0} className="aw-action-button aw-action-danger h-9 px-3 text-sm"><Trash2 size={14} />批量禁用 ({selectedNodes.length})</button>
            <button onClick={batchDelete} disabled={selectedNodes.length === 0} className="aw-action-button aw-action-danger h-9 px-3 text-sm"><Trash2 size={14} />批量删除 ({selectedNodes.length})</button>
            <button onClick={() => batchSetEnabled(true)} disabled={selectedNodes.length === 0} className="aw-action-button aw-action-success h-9 px-3 text-sm">批量启用</button>
            <button onClick={() => { loadFacetNodes(); loadNodes(); }} className="aw-action-button aw-action-neutral h-9 px-4 text-sm">刷新</button>
          </div>
        </div>

        <Toast message={message} type={toastType} />

        <div className="mb-4 space-y-3 rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] p-3">
          <div>
            <div className="mb-2 text-xs font-medium text-[var(--text-tertiary)]">按协议筛选</div>
            <div className="flex flex-wrap gap-2">
              <button onClick={() => updateFilter(setTypeFilter, '')} className={chipClass(typeFilter === '')}>全部 ({facetTotal})</button>
              {typeFacets.map(item => <button key={item.value} onClick={() => updateFilter(setTypeFilter, item.value)} className={`${chipClass(typeFilter === item.value)} uppercase`}>{item.label} ({item.count})</button>)}
            </div>
          </div>
          <div>
            <div className="mb-2 text-xs font-medium text-[var(--text-tertiary)]">按订阅筛选</div>
            <div className="flex flex-wrap gap-2">
              <button onClick={() => updateFilter(setSubscriptionID, '')} className={chipClass(subscriptionID === '')}>全部 ({facetTotal})</button>
              {subscriptionFacets.map(item => <button key={item.value} onClick={() => updateFilter(setSubscriptionID, item.value)} className={chipClass(subscriptionID === item.value)}>{item.label} ({item.count})</button>)}
            </div>
          </div>
        </div>

        <div className="mb-4 grid gap-3 md:grid-cols-2 xl:grid-cols-6">
          <input value={keyword} onChange={e => updateFilter(setKeyword, e.target.value)} placeholder="搜索名称 / 地址 / UID" className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400 xl:col-span-2" />
          <select value={subscriptionID} onChange={e => updateFilter(setSubscriptionID, e.target.value)} className="rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
            <option className="bg-[#152235] text-white" value="">全部订阅</option>
            {subscriptions.map(item => <option key={item.id} className="bg-[#152235] text-white" value={item.id}>{item.name}</option>)}
          </select>
          <select value={typeFilter} onChange={e => updateFilter(setTypeFilter, e.target.value)} className="rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
            <option className="bg-[#152235] text-white" value="">全部协议</option>
            {typeFacets.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
          </select>
          <select value={statusFilter} onChange={e => updateFilter(setStatusFilter, e.target.value)} className="rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
            {statusOptions.map(option => <option key={option.value} className="bg-[#152235] text-white" value={option.value}>{option.label}</option>)}
          </select>
          <div className="grid grid-cols-2 gap-3 xl:col-span-1">
            <select value={enabledFilter} onChange={e => updateFilter(setEnabledFilter, e.target.value)} className="rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
              <option className="bg-[#152235] text-white" value="">启用状态</option>
              <option className="bg-[#152235] text-white" value="true">已启用</option>
              <option className="bg-[#152235] text-white" value="false">已禁用</option>
            </select>
            <select value={preferredFilter} onChange={e => updateFilter(setPreferredFilter, e.target.value)} className="rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
              <option className="bg-[#152235] text-white" value="">首选状态</option>
              <option className="bg-[#152235] text-white" value="true">首选</option>
              <option className="bg-[#152235] text-white" value="false">非首选</option>
            </select>
          </div>
        </div>

        <div className="overflow-hidden rounded-none border border-[var(--border-default)]">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1120px] border-collapse text-left text-sm">
              <thead className="bg-white/[0.04] text-sm text-white">
                <tr>
                  <th className="border-b border-[var(--border-default)] px-4 py-3 font-semibold"><input type="checkbox" checked={nodes.length > 0 && selectedUIDs.size === nodes.length} onChange={toggleAllVisible} /></th>
                  {['名称', '协议', '地址', '订阅', 'UID', '延迟', '状态', '启用', '首选', '更新时间', '操作'].map(column => <th key={column} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{column}</th>)}
                </tr>
              </thead>
              <tbody>
                {nodes.length === 0 ? (
                  <tr><td colSpan={12} className="px-4 py-14 text-center text-sm text-[var(--text-secondary)]">{loading ? '加载中...' : '暂无节点，请先同步订阅。'}</td></tr>
                ) : nodes.map(node => (
                  <tr key={node.uid} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0">
                    <td className="px-4 py-3"><input type="checkbox" checked={selectedUIDs.has(node.uid)} onChange={() => toggleSelected(node.uid)} /></td>
                    <td className="max-w-[240px] truncate px-4 py-3 font-medium text-white" title={node.name}>
                      <img src={getFlagImageURL(nodeFlags[node.uid] || defaultFlag)} alt="" className="mr-2 inline-block h-4 w-4 align-[-2px]" />{node.name}
                    </td>
                    <td className="px-4 py-3"><span className="rounded bg-blue-500/10 px-2 py-1 text-xs text-blue-200">{node.type}</span></td>
                    <td className="max-w-[220px] truncate px-4 py-3 font-mono text-xs" title={nodeAddress(node)}>{nodeAddress(node)}</td>
                    <td className="max-w-[160px] truncate px-4 py-3" title={node.subscription_name}>{node.subscription_name || node.subscription_id}</td>
                    <td className="px-4 py-3 font-mono text-xs" title={node.uid}>{shortUID(node.uid)}</td>
                    <td className="px-4 py-3">
                      <button onClick={() => runSingleTCPing(node)} disabled={tcpingUIDs.has(node.uid)} className={`rounded px-2 py-1 text-xs ${tcpingUIDs.has(node.uid) ? 'cursor-wait bg-blue-500/10 text-blue-300' : node.latency_ms > 0 ? 'bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20' : 'bg-white/[0.04] text-[var(--text-secondary)] hover:bg-blue-500/10 hover:text-blue-200'}`}>{tcpingUIDs.has(node.uid) ? '测速中...' : formatLatency(node.latency_ms)}</button>
                    </td>
                    <td className="px-4 py-3"><span className="rounded bg-white/[0.05] px-2 py-1 text-xs">{node.status}</span></td>
                    <td className="px-4 py-3"><button onClick={() => toggleEnabled(node)} className={`rounded px-2 py-1 text-xs ${node.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{node.enabled ? '启用' : '禁用'}</button></td>
                    <td className="px-4 py-3"><button onClick={() => togglePreferred(node)} className={`inline-flex items-center gap-1 rounded px-2 py-1 text-xs ${node.preferred ? 'bg-yellow-500/10 text-yellow-300' : 'bg-white/[0.04] text-[var(--text-tertiary)]'}`}><Star size={12} />{node.preferred ? '首选' : '普通'}</button></td>
                    <td className="px-4 py-3 text-xs">{formatTime(node.updated_at)}</td>
                    <td className="px-4 py-3"><button onClick={() => setDetail(node)} className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-white hover:bg-white/[0.08]"><Eye size={13} />详情</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="mt-4 flex flex-col gap-3 rounded-md border border-[var(--border-default)] bg-white/[0.025] px-3 py-3 text-sm text-[var(--text-secondary)] md:flex-row md:items-center md:justify-between">
          <div>
            显示 {pageStart}-{pageEnd} / 共 {total} 个节点
            {loading ? '，加载中...' : ''}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[var(--text-tertiary)]">每页</span>
            <select
              value={pageSize}
              onChange={e => {
                setPage(1);
                setPageSize(Number(e.target.value));
              }}
              className="h-9 rounded-md border border-[var(--border-default)] bg-[#152235] px-2 text-sm text-white outline-none focus:border-blue-400"
            >
              {[25, 50, 100, 200].map(size => <option key={size} className="bg-[#152235] text-white" value={size}>{size}</option>)}
            </select>
            <button
              onClick={() => setPage(1)}
              disabled={page <= 1}
              className={`h-9 rounded-md border border-[var(--border-default)] px-3 ${page <= 1 ? 'cursor-not-allowed bg-white/[0.02] text-[var(--text-tertiary)]' : 'bg-white/[0.04] text-white hover:bg-white/[0.08]'}`}
            >
              首页
            </button>
            <button
              onClick={() => setPage(prev => Math.max(1, prev - 1))}
              disabled={page <= 1}
              className={`h-9 rounded-md border border-[var(--border-default)] px-3 ${page <= 1 ? 'cursor-not-allowed bg-white/[0.02] text-[var(--text-tertiary)]' : 'bg-white/[0.04] text-white hover:bg-white/[0.08]'}`}
            >
              上一页
            </button>
            <span className="px-2 text-white">第 {page} / {totalPages} 页</span>
            <button
              onClick={() => setPage(prev => Math.min(totalPages, prev + 1))}
              disabled={page >= totalPages}
              className={`h-9 rounded-md border border-[var(--border-default)] px-3 ${page >= totalPages ? 'cursor-not-allowed bg-white/[0.02] text-[var(--text-tertiary)]' : 'bg-white/[0.04] text-white hover:bg-white/[0.08]'}`}
            >
              下一页
            </button>
            <button
              onClick={() => setPage(totalPages)}
              disabled={page >= totalPages}
              className={`h-9 rounded-md border border-[var(--border-default)] px-3 ${page >= totalPages ? 'cursor-not-allowed bg-white/[0.02] text-[var(--text-tertiary)]' : 'bg-white/[0.04] text-white hover:bg-white/[0.08]'}`}
            >
              末页
            </button>
          </div>
        </div>
      </section>

      {detail && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4 backdrop-blur-sm">
          <div className="max-h-[82vh] w-full max-w-3xl overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(16,27,43,0.96))] p-5 shadow-[var(--shadow-card)]">
            <div className="flex items-start justify-between gap-4">
              <div>
                <h3 className="text-base font-semibold text-white">节点详情</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">{detail.name}</p>
              </div>
              <button onClick={() => setDetail(null)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-sm text-[var(--text-secondary)] hover:text-white">关闭</button>
            </div>
            <div className="mt-4 grid gap-3 text-sm text-[var(--text-secondary)] md:grid-cols-2">
              <div>UID：<span className="font-mono text-white">{detail.uid}</span></div>
              <div>地址：<span className="font-mono text-white">{nodeAddress(detail)}</span></div>
              <div>协议：<span className="text-white">{detail.type}</span></div>
              <div>订阅：<span className="text-white">{detail.subscription_name || detail.subscription_id}</span></div>
            </div>
            <pre className="mt-4 max-h-[50vh] overflow-auto rounded-md border border-[var(--border-default)] bg-black/30 p-4 text-xs text-blue-100">{prettyJSON(detail.raw_json)}</pre>
          </div>
        </div>
      )}

      {renameOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4 backdrop-blur-sm">
          <div className="w-full max-w-xl rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(16,27,43,0.96))] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-base font-semibold text-white">批量修改名称 ({selectedNodes.length})</h3>
            <div className="mt-4 space-y-3">
              <select value={renameMode} onChange={e => setRenameMode(e.target.value as any)} className="w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
                <option className="bg-[#152235] text-white" value="prefix">添加前缀</option>
                <option className="bg-[#152235] text-white" value="suffix">添加后缀</option>
                <option className="bg-[#152235] text-white" value="replace">查找替换</option>
                <option className="bg-[#152235] text-white" value="lines">按行改名</option>
              </select>
              {renameMode === 'replace' ? (
                <div className="grid gap-3 md:grid-cols-2">
                  <input value={findText} onChange={e => setFindText(e.target.value)} placeholder="查找文本" className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                  <input value={replaceText} onChange={e => setReplaceText(e.target.value)} placeholder="替换为" className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                </div>
              ) : renameMode === 'lines' ? (
                <textarea value={renameText} onChange={e => setRenameText(e.target.value)} rows={8} className="w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-blue-400" />
              ) : (
                <input value={renameText} onChange={e => setRenameText(e.target.value)} placeholder={renameMode === 'prefix' ? '前缀文本' : '后缀文本'} className="w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
              )}
              <div className="text-xs text-[var(--text-tertiary)]">修改后的名称会被标记为自定义名称，后续同步订阅不会覆盖。</div>
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button onClick={() => setRenameOpen(false)} className="h-9 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 text-sm text-[var(--text-secondary)] hover:text-white">取消</button>
              <button onClick={saveRename} className="h-9 rounded-md bg-[var(--color-primary)] px-4 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)]">保存</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default NodesPage;
