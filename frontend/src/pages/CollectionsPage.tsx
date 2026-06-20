import React from 'react';
import { Eye, Plus, Trash2, Edit, Layers, Zap } from 'lucide-react';

import { PageHeader } from '@/components/layout/PageHeader';
import { Pagination } from '@/components/ui/Pagination';
import { Toast } from '@/components/ui/Toast';
import { api } from '@/services/api';
import type { NodeItem } from '@/services/types';
import { defaultFlag, getFlagImageURL } from '@/utils/nodeFlags';
import { NodeGroupDetailModal } from './collections/NodeGroupDetailModal';

interface NodeGroup {
  id: number;
  name: string;
  type: string;
  filter_protocols: string;
  filter_subscriptions: string;
  filter_include: string;
  filter_exclude: string;
  node_uids: string;
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  priority: number;
  matched_node_count: number;
}

interface FacetItem {
  value: string;
  label: string;
  count: number;
}

interface NodeGroupMatchedNode {
  uid: string;
  name: string;
  type: string;
  subscription_id: number;
  subscription_name: string;
  latency_ms: number;
  status: string;
}

interface ProxyCollection {
  id: number;
  name: string;
  type: string;
  source_type: string;
  referenced_group_ids: string;
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  referenced_groups: NodeGroup[];
  route_rule_ids: number[];
  node_uids: string[];
}

interface RouteRule {
  id: number;
  name: string;
  outbound: string;
  enabled: boolean;
}

function parseNodeUIDs(value: string | string[] | undefined) {
  if (Array.isArray(value)) return value;
  if (!value || value === '[]') return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.filter(item => typeof item === 'string') : [];
  } catch {
    return [];
  }
}

export function CollectionsPage() {
  const [activeTab, setActiveTab] = React.useState<'node-groups' | 'collections'>('node-groups');
  
  const [nodeGroups, setNodeGroups] = React.useState<NodeGroup[]>([]);
  const [nodeGroupFlags, setNodeGroupFlags] = React.useState<Record<string, string>>({});
  const [nodeFacets, setNodeFacets] = React.useState<{protocols: FacetItem[], subscriptions: FacetItem[], total: number}>({protocols: [], subscriptions: [], total: 0});
  const [editingNodeGroup, setEditingNodeGroup] = React.useState<NodeGroup | null>(null);
  const [selectedNodeGroupIDs, setSelectedNodeGroupIDs] = React.useState<number[]>([]);
  const [ngFilterProtocolsSelected, setNgFilterProtocolsSelected] = React.useState<string[]>([]);
  const [ngFilterSubscriptionsSelected, setNgFilterSubscriptionsSelected] = React.useState<string[]>([]);
  const [ngName, setNgName] = React.useState('');
  const [ngType, setNgType] = React.useState('selector');
  const [ngFilterProtocols, setNgFilterProtocols] = React.useState<string[]>([]);
  const [ngFilterSubscriptions, setNgFilterSubscriptions] = React.useState<string[]>([]);
  const [ngFilterInclude, setNgFilterInclude] = React.useState('');
  const [ngFilterExclude, setNgFilterExclude] = React.useState('');
  const [ngSelectedNodeUIDs, setNgSelectedNodeUIDs] = React.useState<string[]>([]);
  const [manualNodePickerOpen, setManualNodePickerOpen] = React.useState(false);
  const [manualNodes, setManualNodes] = React.useState<NodeItem[]>([]);
  const [manualNodeKeyword, setManualNodeKeyword] = React.useState('');
  const [ngEnabled, setNgEnabled] = React.useState(true);
  const [ngTestURL, setNgTestURL] = React.useState('https://www.gstatic.com/generate_204');
  const [ngTestInterval, setNgTestInterval] = React.useState(300);
  const [ngTolerance, setNgTolerance] = React.useState(100);
  const [detailNodeGroup, setDetailNodeGroup] = React.useState<NodeGroup | null>(null);
  const [detailNodes, setDetailNodes] = React.useState<NodeGroupMatchedNode[]>([]);
  const [detailLoading, setDetailLoading] = React.useState(false);
  const [nodeGroupPage, setNodeGroupPage] = React.useState(1);
  const [nodeGroupPageSize, setNodeGroupPageSize] = React.useState(25);
  const [quickSetupOpen, setQuickSetupOpen] = React.useState(false);
  const [quickSetupRunning, setQuickSetupRunning] = React.useState(false);
  
  const [collections, setCollections] = React.useState<ProxyCollection[]>([]);
  const [routeRules, setRouteRules] = React.useState<RouteRule[]>([]);
  const [editingCollection, setEditingCollection] = React.useState<ProxyCollection | null>(null);
  const isEditingCollection = Boolean(editingCollection?.id);
  const [colName, setColName] = React.useState('');
  const [colType, setColType] = React.useState('selector');
  const [colSourceType, setColSourceType] = React.useState('node_groups');
  const [colSelectedGroupIDs, setColSelectedGroupIDs] = React.useState<number[]>([]);
  const [colSelectedRuleIDs, setColSelectedRuleIDs] = React.useState<number[]>([]);
  const [colEnabled, setColEnabled] = React.useState(true);
  const [colTestURL, setColTestURL] = React.useState('https://www.gstatic.com/generate_204');
  const [colTestInterval, setColTestInterval] = React.useState(300);
  const [colTolerance, setColTolerance] = React.useState(100);
  const [collectionPage, setCollectionPage] = React.useState(1);
  const [collectionPageSize, setCollectionPageSize] = React.useState(25);
  const [previewCollection, setPreviewCollection] = React.useState<ProxyCollection | null>(null);
  const [previewOutbound, setPreviewOutbound] = React.useState<Record<string, unknown> | null>(null);
  
  const [loading, setLoading] = React.useState(true);
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error' | 'info'>('success');

  const showMessage = (msg: string, type: 'success' | 'error' | 'info' = 'success') => {
    setMessage(msg);
    setMessageType(type);
  };

  React.useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(() => setMessage(''), messageType === 'error' ? 5000 : 3000);
    return () => window.clearTimeout(timer);
  }, [message, messageType]);

  const load = React.useCallback(async () => {
    try {
      const getJSON = async (url: string) => {
        const resp = await fetch(url);
        if (!resp.ok) {
          const err = await resp.json().catch(() => null);
          throw new Error(`${url}: ${err?.error?.message || resp.statusText}`);
        }
        return resp.json();
      };
      const [ngData, colData, facetsData, ruleData] = await Promise.all([
        getJSON('/api/v1/node-groups'),
        getJSON('/api/v1/collections'),
        getJSON('/api/v1/nodes/facets'),
        getJSON('/api/v1/rules'),
      ]);
      const nextNodeGroups = Array.isArray(ngData) ? ngData : [];
      setNodeGroups(nextNodeGroups);
      if (nextNodeGroups.length > 0) {
        const flags = await api.inferNodeFlags(nextNodeGroups.map(group => ({ key: String(group.id), name: group.name, server: '' })));
        setNodeGroupFlags(Object.fromEntries(flags.items.map(item => [item.key, item.flag])));
      } else {
        setNodeGroupFlags({});
      }
      setCollections(Array.isArray(colData) ? colData : []);
      setRouteRules(Array.isArray(ruleData) ? ruleData : []);
      
		const protocols = Array.isArray(facetsData?.types) ? facetsData.types : [];
		const subs = Array.isArray(facetsData?.subscriptions) ? facetsData.subscriptions : [];
      const total = Number(facetsData?.total || protocols.reduce((sum: number, item: FacetItem) => sum + Number(item.count || 0), 0));
      setNodeFacets({protocols, subscriptions: subs, total});
    } catch (e: any) {
      showMessage(`加载失败: ${e.message}`, 'error');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { load(); }, [load]);

  // 节点组操作
  const resetNodeGroupForm = () => {
    setEditingNodeGroup(null);
    setNgName('');
    setNgType('selector');
    setNgFilterProtocols([]);
    setNgFilterSubscriptions([]);
    setNgFilterInclude('');
    setNgFilterExclude('');
    setNgSelectedNodeUIDs([]);
    setNgEnabled(true);
    setNgTestURL('https://www.gstatic.com/generate_204');
    setNgTestInterval(300);
    setNgTolerance(100);
  };

  const editNodeGroup = (ng: NodeGroup) => {
    setEditingNodeGroup(ng);
    setNgName(ng.name);
    setNgType(ng.type);
    setNgFilterProtocols(ng.filter_protocols ? ng.filter_protocols.split(',') : []);
    setNgFilterSubscriptions(ng.filter_subscriptions ? ng.filter_subscriptions.split(',') : []);
    setNgFilterInclude(ng.filter_include);
    setNgFilterExclude(ng.filter_exclude);
    setNgSelectedNodeUIDs(parseNodeUIDs(ng.node_uids));
    setNgEnabled(ng.enabled);
    setNgTestURL(ng.test_url || 'https://www.gstatic.com/generate_204');
    setNgTestInterval(ng.test_interval || 300);
    setNgTolerance(ng.tolerance || 100);
  };

  const createNodeGroup = () => {
    setNgName('');
    setNgType('selector');
    setNgFilterProtocols([]);
    setNgFilterSubscriptions([]);
    setNgFilterInclude('');
    setNgFilterExclude('');
    setNgSelectedNodeUIDs([]);
    setNgEnabled(true);
    setNgTestURL('https://www.gstatic.com/generate_204');
    setNgTestInterval(300);
    setNgTolerance(100);
    setEditingNodeGroup({} as NodeGroup);
  };

  const toggleValue = (values: string[], value: string) => (
    values.includes(value) ? values.filter(item => item !== value) : [...values, value]
  );

  const toggleProtocolFilter = (protocol: string) => {
    setNodeGroupPage(1);
    setNgFilterProtocolsSelected(prev => toggleValue(prev, protocol));
  };

  const toggleSubscriptionFilter = (subscriptionID: string) => {
    setNodeGroupPage(1);
    setNgFilterSubscriptionsSelected(prev => toggleValue(prev, subscriptionID));
  };

  const openManualNodePicker = async () => {
    setManualNodePickerOpen(true);
    try {
      const resp = await api.getNodes({ enabled: true, limit: 1000, keyword: manualNodeKeyword });
      setManualNodes(resp.items || []);
    } catch (e: any) {
      showMessage(`加载节点失败: ${e.message}`, 'error');
    }
  };

  const reloadManualNodes = async () => {
    try {
      const resp = await api.getNodes({ enabled: true, limit: 1000, keyword: manualNodeKeyword });
      setManualNodes(resp.items || []);
    } catch (e: any) {
      showMessage(`加载节点失败: ${e.message}`, 'error');
    }
  };

  const toggleManualNode = (uid: string) => {
    setNgSelectedNodeUIDs(prev => prev.includes(uid) ? prev.filter(item => item !== uid) : [...prev, uid]);
  };

  const saveNodeGroup = async () => {
    try {
      const payload = {
        name: ngName,
        type: ngType,
        filter_protocols: ngFilterProtocols.join(','),
        filter_subscriptions: ngFilterSubscriptions.join(','),
        filter_include: ngFilterInclude,
        filter_exclude: ngFilterExclude,
        node_uids: ngSelectedNodeUIDs,
        enabled: ngEnabled,
        priority: editingNodeGroup?.priority || 0,
        test_url: ngTestURL,
        test_interval: ngTestInterval,
        tolerance: ngTolerance,
      };
      
      const nodeGroupID = editingNodeGroup?.id;
      const isEditing = Boolean(nodeGroupID);
      const resp = await fetch(isEditing ? `/api/v1/node-groups/${nodeGroupID}` : '/api/v1/node-groups', {
        method: isEditing ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        const err = await resp.json().catch(() => null);
        throw new Error(err?.error?.message || '保存节点组失败');
      }

      if (isEditing) {
        showMessage('节点组已更新');
      } else {
        showMessage('节点组已创建');
      }
      resetNodeGroupForm();
      await load();
    } catch (e: any) {
      showMessage(`保存失败: ${e.message}`, 'error');
    }
  };

  const deleteNodeGroup = async (ng: NodeGroup) => {
    if (!confirm(`确定删除节点组 "${ng.name}" 吗？`)) return;
    try {
      await fetch(`/api/v1/node-groups/${ng.id}`, { method: 'DELETE' });
      showMessage('节点组已删除');
      await load();
    } catch (e: any) {
      showMessage(`删除失败: ${e.message}`, 'error');
    }
  };

  const showNodeGroupDetail = async (ng: NodeGroup) => {
    setDetailNodeGroup(ng);
    setDetailNodes([]);
    setDetailLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('filter_protocols', ng.filter_protocols || '');
      params.set('filter_subscriptions', ng.filter_subscriptions || '');
      params.set('filter_include', ng.filter_include || '');
      params.set('filter_exclude', ng.filter_exclude || '');
      const resp = await fetch(`/api/v1/node-groups/preview?${params.toString()}`);
      if (!resp.ok) throw new Error((await resp.json()).error?.message || '加载节点组详情失败');
      const data = await resp.json();
      setDetailNodes(Array.isArray(data) ? data : []);
    } catch (e: any) {
      showMessage(`加载节点组详情失败: ${e.message}`, 'error');
    } finally {
      setDetailLoading(false);
    }
  };

  const batchDeleteNodeGroups = async () => {
    if (selectedNodeGroupIDs.length === 0) {
      showMessage('请先选择要删除的节点组', 'error');
      return;
    }
    if (!confirm(`确定删除选中的 ${selectedNodeGroupIDs.length} 个节点组吗？`)) return;
    try {
      const resp = await fetch('/api/v1/node-groups/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedNodeGroupIDs }),
      });
      if (!resp.ok) throw new Error((await resp.json()).error?.message || '批量删除失败');
      showMessage(`已删除 ${selectedNodeGroupIDs.length} 个节点组`);
      setSelectedNodeGroupIDs([]);
      await load();
    } catch (e: any) {
      showMessage(`批量删除失败: ${e.message}`, 'error');
    }
  };

  const toggleNodeGroupSelection = (id: number) => {
    setSelectedNodeGroupIDs(prev =>
      prev.includes(id) ? prev.filter(gid => gid !== id) : [...prev, id]
    );
  };

  const toggleAllNodeGroups = () => {
    const filteredIDs = pagedNodeGroups.map(ng => ng.id);
    const allFilteredSelected = filteredIDs.length > 0 && filteredIDs.every(id => selectedNodeGroupIDs.includes(id));
    if (allFilteredSelected) {
      setSelectedNodeGroupIDs(prev => prev.filter(id => !filteredIDs.includes(id)));
    } else {
      setSelectedNodeGroupIDs(prev => Array.from(new Set([...prev, ...filteredIDs])));
    }
  };

  const filteredNodeGroups = nodeGroups;

  const nodeGroupTotalPages = Math.max(1, Math.ceil(filteredNodeGroups.length / nodeGroupPageSize));
  const pagedNodeGroups = React.useMemo(() => {
    const start = (nodeGroupPage - 1) * nodeGroupPageSize;
    return filteredNodeGroups.slice(start, start + nodeGroupPageSize);
  }, [filteredNodeGroups, nodeGroupPage, nodeGroupPageSize]);
  const collectionTotalPages = Math.max(1, Math.ceil(collections.length / collectionPageSize));
  const pagedCollections = React.useMemo(() => {
    const start = (collectionPage - 1) * collectionPageSize;
    return collections.slice(start, start + collectionPageSize);
  }, [collections, collectionPage, collectionPageSize]);

  React.useEffect(() => {
    if (nodeGroupPage > nodeGroupTotalPages) setNodeGroupPage(nodeGroupTotalPages);
  }, [nodeGroupPage, nodeGroupTotalPages]);
  React.useEffect(() => {
    if (collectionPage > collectionTotalPages) setCollectionPage(collectionTotalPages);
  }, [collectionPage, collectionTotalPages]);

  const runQuickSetup = async () => {
    if (quickSetupRunning) return;
    try {
      setQuickSetupRunning(true);
      const resp = await fetch('/api/v1/node-groups/quick-setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filter_subscriptions: ngFilterSubscriptionsSelected.join(','),
          filter_protocols: ngFilterProtocolsSelected.join(','),
        }),
      });
      if (!resp.ok) throw new Error((await resp.json()).error?.message || '快速配置失败');
      showMessage('节点组已创建');
      setQuickSetupOpen(false);
      await load();
    } catch (e: any) {
      showMessage(`快速配置失败: ${e.message}`, 'error');
    } finally {
      setQuickSetupRunning(false);
    }
  };

  // 策略组操作
  const resetCollectionForm = () => {
    setEditingCollection(null);
    setColName('');
    setColType('selector');
    setColSourceType('node_groups');
    setColSelectedGroupIDs([]);
    setColSelectedRuleIDs([]);
    setColEnabled(true);
    setColTestURL('https://www.gstatic.com/generate_204');
    setColTestInterval(300);
    setColTolerance(100);
  };

  const createCollection = () => {
    setEditingCollection({} as ProxyCollection);
    setColName('');
    setColType('selector');
    setColSourceType('node_groups');
    setColSelectedGroupIDs([]);
    setColSelectedRuleIDs([]);
    setColEnabled(true);
    setColTestURL('https://www.gstatic.com/generate_204');
    setColTestInterval(300);
    setColTolerance(100);
  };

  const editCollection = (col: ProxyCollection) => {
    setEditingCollection(col);
    setColName(col.name);
    setColType(col.type);
    setColSourceType(col.source_type);
    setColEnabled(col.enabled);
    setColTestURL(col.test_url || 'https://www.gstatic.com/generate_204');
    setColTestInterval(col.test_interval || 300);
    setColTolerance(col.tolerance || 100);
    
    if (col.source_type === 'node_groups' && col.referenced_groups) {
      setColSelectedGroupIDs(col.referenced_groups.map(g => g.id));
    }
    setColSelectedRuleIDs(col.route_rule_ids || []);
  };

  const saveCollection = async () => {
    try {
      const payload = {
        name: colName,
        type: colType,
        source_type: colSourceType,
        referenced_group_ids: colSourceType === 'node_groups' ? colSelectedGroupIDs : [],
        route_rule_ids: colSelectedRuleIDs,
        node_uids: [],
        enabled: colEnabled,
        test_url: colTestURL,
        test_interval: colTestInterval,
        tolerance: colTolerance,
      };
      
      const collectionID = editingCollection?.id;
      const isEditing = Boolean(collectionID);
      const collectionEndpoint = isEditing ? `/api/v1/collections/${collectionID}` : '/api/v1/collections';
      const resp = await fetch(collectionEndpoint, {
        method: isEditing ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        const err = await resp.json().catch(() => null);
        throw new Error(err?.error?.message || '保存策略组失败');
      }

      if (isEditing) {
        showMessage('策略组已更新');
      } else {
        showMessage('策略组已创建');
      }
      resetCollectionForm();
      await load();
    } catch (e: any) {
      showMessage(`保存失败: ${e.message}`, 'error');
    }
  };

  const deleteCollection = async (col: ProxyCollection) => {
    if (!confirm(`确定删除策略组 "${col.name}" 吗？`)) return;
    try {
      await fetch(`/api/v1/collections/${col.id}`, { method: 'DELETE' });
      showMessage('策略组已删除');
      await load();
    } catch (e: any) {
      showMessage(`删除失败: ${e.message}`, 'error');
    }
  };

  const previewCollectionConfig = async (col: ProxyCollection) => {
    setPreviewCollection(col);
    const outbound: Record<string, unknown> = {
      tag: col.name,
      type: col.type,
      outbounds: col.source_type === 'node_groups'
        ? (col.referenced_groups || []).map(group => group.name)
        : (col.node_uids || []),
    };
    if (col.type === 'urltest' || col.type === 'fallback') {
      outbound.url = col.test_url || 'https://www.gstatic.com/generate_204';
      outbound.interval = `${col.test_interval || 300}s`;
      outbound.tolerance = col.tolerance || 100;
    }
    setPreviewOutbound(outbound);
  };

  const toggleGroupSelected = (id: number) => {
    setColSelectedGroupIDs(prev => 
      prev.includes(id) ? prev.filter(gid => gid !== id) : [...prev, id]
    );
  };

  const selectCollectionRule = (id: number) => {
    if (!id) {
      setColSelectedRuleIDs([]);
      setColName('');
      return;
    }
    const rule = routeRules.find(item => item.id === id);
    setColSelectedRuleIDs([id]);
    setColName(rule?.name || '');
  };

  const routeRuleNameByID = new Map(routeRules.map(rule => [rule.id, rule.name]));
  const occupiedRouteRuleIDs = new Set(
    collections
      .filter(collection => collection.id !== editingCollection?.id)
      .flatMap(collection => collection.route_rule_ids || [])
  );
  const selectableRouteRules = routeRules.filter(rule => rule.outbound === 'proxy' && !occupiedRouteRuleIDs.has(rule.id));
  const routeRuleOutboundLabel = (outbound: string) => {
    if (outbound === 'proxy') return '策略';
    if (outbound === 'direct') return '直连';
    if (outbound === 'block') return '阻断';
    return outbound;
  };

  return (
    <div className="space-y-4">
      <PageHeader title="策略组管理" />

      <Toast message={message} type={messageType} />

      {/* Tabs */}
      <div className="flex gap-2 border-b border-[var(--border-default)]">
        <button
          onClick={() => setActiveTab('node-groups')}
          className={`px-4 py-2 text-sm font-medium ${activeTab === 'node-groups' ? 'border-b-2 border-blue-500 text-white' : 'text-[var(--text-secondary)] hover:text-white'}`}
        >
          <Layers size={16} className="mr-2 inline" />
          节点组（地域划分）
        </button>
        <button
          onClick={() => setActiveTab('collections')}
          className={`px-4 py-2 text-sm font-medium ${activeTab === 'collections' ? 'border-b-2 border-blue-500 text-white' : 'text-[var(--text-secondary)] hover:text-white'}`}
        >
          <Zap size={16} className="mr-2 inline" />
          策略组（业务用途）
        </button>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <div className="text-center">
            <div className="mx-auto h-10 w-10 animate-spin rounded-full border-4 border-blue-500/20 border-t-blue-500"></div>
            <div className="mt-4 text-sm text-[var(--text-secondary)]">加载中...</div>
          </div>
        </div>
      ) : (
        <>
          {activeTab === 'node-groups' && (
            <section className="space-y-4">
              <div className="flex items-center justify-between">
                <p className="text-sm text-[var(--text-tertiary)]">使用关键词自动筛选节点，作为策略组的基础单元</p>
                <div className="flex gap-2">
                  {selectedNodeGroupIDs.length > 0 && (
                    <button onClick={batchDeleteNodeGroups} className="inline-flex h-9 items-center gap-2 rounded-md border border-red-400/30 bg-red-500/10 px-3 text-sm font-medium text-red-200 hover:bg-red-500/20">
                      <Trash2 size={14} />批量删除（{selectedNodeGroupIDs.length}）
                    </button>
                  )}
                  <button disabled={quickSetupRunning} onClick={() => setQuickSetupOpen(true)} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-sm font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)] disabled:cursor-not-allowed disabled:opacity-50">
                    <Zap size={14} />智能快速配置
                  </button>
                  <button onClick={createNodeGroup} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-sm font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">
                    <Plus size={14} />新增节点组
                  </button>
                </div>
              </div>

              <div className="overflow-hidden rounded-xl border border-[var(--border-default)]">
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[1120px] border-collapse text-left text-base">
                    <thead className="bg-white/[0.04] text-white">
                      <tr>
                        <th className="w-12 border-b border-[var(--border-default)] px-4 py-3">
                          <input
                            type="checkbox"
                            checked={pagedNodeGroups.length > 0 && pagedNodeGroups.every(ng => selectedNodeGroupIDs.includes(ng.id))}
                            onChange={toggleAllNodeGroups}
                          />
                        </th>
                        {['名称', '类型', '协议限制', '订阅限制', '包含关键词', '排除关键词', '匹配节点', '状态', '操作'].map(col => (
                          <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {filteredNodeGroups.length === 0 ? (
                        <tr><td colSpan={10} className="px-4 py-12 text-center text-[var(--text-tertiary)]">
                          暂无节点组，点击"新增节点组"或"智能快速配置"创建
                        </td></tr>
                      ) : pagedNodeGroups.map(ng => {
                        return (
                          <tr key={ng.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 hover:bg-white/[0.02]">
                            <td className="px-4 py-3">
                              <input
                                type="checkbox"
                                checked={selectedNodeGroupIDs.includes(ng.id)}
                                onChange={() => toggleNodeGroupSelection(ng.id)}
                              />
                            </td>
                            <td className="px-4 py-3">
                              <div className="flex items-center gap-2">
                                <img src={getFlagImageURL(nodeGroupFlags[String(ng.id)] || defaultFlag)} alt="" className="h-4 w-4" />
                                <span className="font-medium text-white">{ng.name}</span>
                              </div>
                            </td>
                            <td className="px-4 py-3">{ng.type === 'urltest' ? '自动' : '手动'}</td>
                            <td className="max-w-[160px] truncate px-4 py-3 text-sm uppercase">{ng.filter_protocols || '全部'}</td>
                            <td className="max-w-[220px] truncate px-4 py-3 text-sm">
                              {ng.filter_subscriptions
                                ? ng.filter_subscriptions.split(',').map(id => nodeFacets.subscriptions.find(item => item.value === id)?.label || id).join('、')
                                : '全部'}
                            </td>
                            <td className="max-w-[250px] truncate px-4 py-3 font-mono text-sm">{ng.filter_include}</td>
                            <td className="max-w-[150px] truncate px-4 py-3 font-mono text-sm">{ng.filter_exclude || '-'}</td>
                            <td className="px-4 py-3">{ng.matched_node_count || 0} 个</td>
                            <td className="px-4 py-3">
                              <span className={`rounded px-2 py-1 text-sm ${ng.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-gray-500/10 text-gray-400'}`}>
                                {ng.enabled ? '启用' : '停用'}
                              </span>
                            </td>
                            <td className="px-4 py-3">
                              <div className="flex gap-2">
                                <button onClick={() => showNodeGroupDetail(ng)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-sm text-white hover:bg-white/[0.08]">
                                  <Eye size={12} className="inline mr-1" />详情
                                </button>
                                <button onClick={() => editNodeGroup(ng)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-sm text-white hover:bg-white/[0.08]">
                                  <Edit size={12} className="inline mr-1" />编辑
                                </button>
                                <button onClick={() => deleteNodeGroup(ng)} className="inline-flex items-center gap-1 rounded-md border border-red-400/30 bg-red-500/10 px-3 py-1 text-sm text-red-200 hover:bg-red-500/20">
                                  <Trash2 size={12} />删除
                                </button>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>

              <Pagination total={filteredNodeGroups.length} page={nodeGroupPage} pageSize={nodeGroupPageSize} totalPages={nodeGroupTotalPages} onPageChange={setNodeGroupPage} onPageSizeChange={setNodeGroupPageSize} />

              {detailNodeGroup && <NodeGroupDetailModal group={detailNodeGroup} nodes={detailNodes} loading={detailLoading} subscriptions={nodeFacets.subscriptions} onClose={() => setDetailNodeGroup(null)} />}

              {quickSetupOpen && (
                <div className="aw-modal-backdrop" onClick={() => setQuickSetupOpen(false)}>
                  <div className="aw-modal-panel w-full max-w-3xl p-6" onClick={e => e.stopPropagation()}>
                    <div className="mb-4 flex items-start justify-between gap-4">
                      <div>
                        <h4 className="text-lg font-semibold text-white">智能快速配置</h4>
                        <p className="mt-1 text-xs text-[var(--text-tertiary)]">选择订阅和协议范围后，自动创建实际有匹配节点的地域节点组。</p>
                      </div>
                      <button onClick={() => setQuickSetupOpen(false)} className="aw-modal-close">✕</button>
                    </div>
                    <div className="space-y-4">
                      <div>
                        <div className="mb-2 text-sm font-medium text-white">按订阅筛选</div>
                        <div className="flex flex-wrap gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.02] p-3">
                          <button
                            onClick={() => setNgFilterSubscriptionsSelected([])}
                            className={`rounded-md px-3 py-1.5 text-sm transition-colors ${ngFilterSubscriptionsSelected.length === 0 ? 'bg-blue-500/20 text-blue-200' : 'bg-white/[0.04] text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white'}`}
                          >
                            全部 ({nodeFacets.total || 0})
                          </button>
                          {nodeFacets.subscriptions.map(sub => (
                            <button
                              key={sub.value}
                              onClick={() => toggleSubscriptionFilter(sub.value)}
                              className={`rounded-md px-3 py-1.5 text-sm transition-colors ${ngFilterSubscriptionsSelected.includes(sub.value) ? 'bg-blue-500/20 text-blue-200' : 'bg-white/[0.04] text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white'}`}
                            >
                              {sub.label} ({sub.count})
                            </button>
                          ))}
                        </div>
                      </div>
                      <div>
                        <div className="mb-2 text-sm font-medium text-white">按协议筛选</div>
                        <div className="flex flex-wrap gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.02] p-3">
                          <button
                            onClick={() => setNgFilterProtocolsSelected([])}
                            className={`rounded-md px-3 py-1.5 text-sm transition-colors ${ngFilterProtocolsSelected.length === 0 ? 'bg-blue-500/20 text-blue-200' : 'bg-white/[0.04] text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white'}`}
                          >
                            全部 ({nodeFacets.total || 0})
                          </button>
                          {nodeFacets.protocols.map(protocol => (
                            <button
                              key={protocol.value}
                              onClick={() => toggleProtocolFilter(protocol.value)}
                              className={`rounded-md px-3 py-1.5 text-sm uppercase transition-colors ${ngFilterProtocolsSelected.includes(protocol.value) ? 'bg-blue-500/20 text-blue-200' : 'bg-white/[0.04] text-[var(--text-secondary)] hover:bg-white/[0.08] hover:text-white'}`}
                            >
                              {protocol.label} ({protocol.count})
                            </button>
                          ))}
                        </div>
                      </div>
                    </div>
                    <div className="mt-6 flex justify-end gap-3">
                      <button onClick={() => setQuickSetupOpen(false)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 py-2 text-sm text-white hover:bg-white/[0.08]">取消</button>
                      <button disabled={quickSetupRunning} onClick={runQuickSetup} className="rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)] disabled:cursor-not-allowed disabled:opacity-50">{quickSetupRunning ? '配置中...' : '开始配置'}</button>
                    </div>
                  </div>
                </div>
              )}

              {editingNodeGroup && (
                <div className="aw-modal-backdrop" onClick={resetNodeGroupForm}>
                  <div className="aw-modal-panel w-full max-w-2xl p-6" onClick={e => e.stopPropagation()}>
                    <div className="mb-4 flex items-center justify-between">
                      <h4 className="text-lg font-semibold text-white">{editingNodeGroup.id ? '编辑节点组' : '新增节点组'}</h4>
                      <button onClick={resetNodeGroupForm} className="aw-modal-close">✕</button>
                    </div>
                    <div className="grid gap-4 md:grid-cols-2">
                      <label className="block md:col-span-2">
                        <span className="text-sm font-medium text-white">名称</span>
                        <input value={ngName} onChange={e => setNgName(e.target.value)} placeholder="🇭🇰 香港节点" className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                      </label>
                      <label className="block">
                        <span className="text-sm font-medium text-white">类型</span>
                        <select value={ngType} onChange={e => setNgType(e.target.value)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-blue-400">
                          <option value="selector">手动切换</option>
                          <option value="urltest">自动选择（测速）</option>
                        </select>
                      </label>
                      <div className="flex items-end">
                        <label className="inline-flex items-center gap-2 text-sm text-slate-300">
                          <input type="checkbox" checked={ngEnabled} onChange={e => setNgEnabled(e.target.checked)} />
                          启用此节点组
                        </label>
                      </div>
                      <div className="md:col-span-2 rounded-lg border border-[var(--border-default)] bg-white/[0.02] p-3">
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div>
                            <div className="text-sm font-medium text-white">手动选择节点</div>
                            <p className="mt-1 text-xs text-[var(--text-tertiary)]">已选择 {ngSelectedNodeUIDs.length} 个节点。选择后配置生成优先使用这些节点。</p>
                          </div>
                          <button type="button" onClick={openManualNodePicker} className="rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 py-2 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">手动选择节点</button>
                        </div>
                        {ngSelectedNodeUIDs.length > 0 && (
                          <button type="button" onClick={() => setNgSelectedNodeUIDs([])} className="mt-2 text-xs text-red-300 hover:text-red-200">清空手动选择，改用筛选条件</button>
                        )}
                      </div>
                      {ngType === 'urltest' && (
                        <div className="grid gap-4 rounded-lg border border-[var(--border-default)] bg-white/[0.02] p-3 md:col-span-2 md:grid-cols-3">
                          <label className="block md:col-span-3">
                            <span className="text-sm font-medium text-white">测速 URL</span>
                            <input value={ngTestURL} onChange={e => setNgTestURL(e.target.value)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                          </label>
                          <label className="block">
                            <span className="text-sm font-medium text-white">测速间隔（秒）</span>
                            <input type="number" min={1} value={ngTestInterval} onChange={e => setNgTestInterval(Number(e.target.value) || 300)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                          </label>
                          <label className="block">
                            <span className="text-sm font-medium text-white">容差（毫秒）</span>
                            <input type="number" min={0} value={ngTolerance} onChange={e => setNgTolerance(Number(e.target.value) || 0)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                          </label>
                          <div className="flex items-end text-xs text-[var(--text-tertiary)]">urltest 会按测速结果自动选择延迟最低的节点。</div>
                        </div>
                      )}
                      <div className="md:col-span-2">
                        <div className="text-sm font-medium text-white">协议范围</div>
                        <p className="mt-1 text-xs text-slate-400">不选择表示允许全部协议；选择后只匹配对应协议节点。</p>
                        <div className="mt-2 flex flex-wrap gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.02] p-3">
                          {nodeFacets.protocols.length === 0 ? (
                            <span className="text-xs text-slate-400">暂无可选协议</span>
                          ) : nodeFacets.protocols.map(item => (
                            <label key={item.value} className="inline-flex cursor-pointer items-center gap-2 rounded-md bg-white/[0.04] px-3 py-1.5 text-xs uppercase text-slate-300 hover:bg-white/[0.08] hover:text-white">
                              <input
                                type="checkbox"
                                checked={ngFilterProtocols.includes(item.value)}
                                onChange={() => setNgFilterProtocols(prev => toggleValue(prev, item.value))}
                              />
                              {item.label} ({item.count})
                            </label>
                          ))}
                        </div>
                      </div>
                      <div className="md:col-span-2">
                        <div className="text-sm font-medium text-white">订阅范围</div>
                        <p className="mt-1 text-xs text-slate-400">不选择表示允许全部订阅；选择后只匹配对应订阅来源。</p>
                        <div className="mt-2 grid max-h-36 gap-2 overflow-y-auto rounded-md border border-[var(--border-default)] bg-white/[0.02] p-3 md:grid-cols-2">
                          {nodeFacets.subscriptions.length === 0 ? (
                            <span className="text-xs text-slate-400">暂无可选订阅</span>
                          ) : nodeFacets.subscriptions.map(item => (
                            <label key={item.value} className="inline-flex cursor-pointer items-center gap-2 rounded-md bg-white/[0.04] px-3 py-1.5 text-xs text-slate-300 hover:bg-white/[0.08] hover:text-white">
                              <input
                                type="checkbox"
                                checked={ngFilterSubscriptions.includes(item.value)}
                                onChange={() => setNgFilterSubscriptions(prev => toggleValue(prev, item.value))}
                              />
                              <span className="min-w-0 flex-1 truncate">{item.label}</span>
                              <span className="text-slate-400">({item.count})</span>
                            </label>
                          ))}
                        </div>
                      </div>
                      <label className="block md:col-span-2">
                        <span className="text-sm font-medium text-white">包含关键词</span>
                        <p className="mt-1 text-xs text-slate-400">用 | 分隔多个普通关键词，命中任意一个就加入</p>
                        <input value={ngFilterInclude} onChange={e => setNgFilterInclude(e.target.value)} placeholder="香港|HK|hk|HongKong|港" className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-blue-400" />
                      </label>
                      <label className="block md:col-span-2">
                        <span className="text-sm font-medium text-white">排除关键词（可选）</span>
                        <p className="mt-1 text-xs text-slate-400">用 | 分隔，匹配这些关键词的节点将被排除</p>
                        <input value={ngFilterExclude} onChange={e => setNgFilterExclude(e.target.value)} placeholder="免费|过期|流量|官网" className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-blue-400" />
                      </label>
                    </div>
                    <div className="mt-6 flex justify-end gap-3">
                      <button onClick={resetNodeGroupForm} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 py-2 text-sm text-white hover:bg-white/[0.08]">
                        取消
                      </button>
                      <button onClick={saveNodeGroup} className="rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)]">
                        {editingNodeGroup.id ? '更新' : '创建'}
                      </button>
                    </div>
                  </div>
                </div>
              )}

              {manualNodePickerOpen && (
                <div className="aw-modal-backdrop" onClick={() => setManualNodePickerOpen(false)}>
                  <div className="aw-modal-panel w-full max-w-5xl p-6" onClick={e => e.stopPropagation()}>
                    <div className="mb-4 flex items-start justify-between gap-4">
                      <div>
                        <h4 className="text-lg font-semibold text-white">手动选择节点</h4>
                        <p className="mt-1 text-xs text-[var(--text-tertiary)]">从节点管理中的启用节点里选择，保存后此节点组优先使用这些节点。</p>
                      </div>
                      <button onClick={() => setManualNodePickerOpen(false)} className="aw-modal-close">✕</button>
                    </div>
                    <div className="mb-3 flex gap-2">
                      <input value={manualNodeKeyword} onChange={e => setManualNodeKeyword(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') void reloadManualNodes(); }} placeholder="搜索节点名称" className="flex-1 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-blue-400" />
                      <button onClick={() => void reloadManualNodes()} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 py-2 text-sm text-white hover:bg-white/[0.08]">搜索</button>
                    </div>
                    <div className="aw-data-table-wrap max-h-[55vh]">
                      <table className="aw-data-table min-w-[820px]">
                        <thead><tr>{['选择', '节点名称', '协议', '订阅来源', '延迟', '状态'].map(col => <th key={col}>{col}</th>)}</tr></thead>
                        <tbody>
                          {manualNodes.length === 0 ? (
                            <tr><td colSpan={6} className="py-10 text-center text-slate-400">没有可选节点</td></tr>
                          ) : manualNodes.map(node => (
                            <tr key={node.uid}>
                              <td><input type="checkbox" checked={ngSelectedNodeUIDs.includes(node.uid)} onChange={() => toggleManualNode(node.uid)} /></td>
                              <td className="max-w-[420px] truncate font-medium text-white" title={node.name}>{node.name || '(未命名节点)'}</td>
                              <td className="uppercase text-blue-200">{node.type}</td>
                              <td>{node.subscription_name || `订阅 ${node.subscription_id}`}</td>
                              <td>{node.latency_ms > 0 ? `${node.latency_ms} ms` : '-'}</td>
                              <td>{node.status || 'unknown'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                    <div className="mt-5 flex justify-end gap-3">
                      <button onClick={() => setManualNodePickerOpen(false)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-4 py-2 text-sm text-white hover:bg-white/[0.08]">完成</button>
                    </div>
                  </div>
                </div>
              )}
            </section>
          )}

          {activeTab === 'collections' && (
            <section className="min-w-0 space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-xs text-[var(--text-tertiary)]">引用节点组，组合成业务用途的策略组（如 YouTube、AI 等）</p>
                <button onClick={createCollection} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">
                  <Plus size={14} />新增策略组
                </button>
              </div>

              {/* 策略组表格 */}
              <div className="overflow-hidden rounded-xl border border-[var(--border-default)]">
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[800px] border-collapse text-left text-sm">
                    <thead className="bg-white/[0.04] text-white">
                      <tr>
                        {['名称', '绑定规则', '类型', '引用节点组', '状态', '操作'].map(col => (
                          <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {collections.length === 0 ? (
                        <tr>
                          <td colSpan={6} className="px-4 py-12 text-center text-[var(--text-tertiary)]">
                            <div>暂无策略组</div>
                            <button onClick={createCollection} className="mt-4 inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-4 text-sm font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">
                              <Plus size={14} />新增策略组
                            </button>
                          </td>
                        </tr>
                      ) : pagedCollections.map(col => (
                        <tr key={col.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 hover:bg-white/[0.02]">
                          <td className="px-4 py-3 font-medium text-white">{col.name}</td>
                          <td className="max-w-[260px] truncate px-4 py-3 text-xs" title={(col.route_rule_ids || []).map(id => routeRuleNameByID.get(id) || `#${id}`).join('\n')}>
                            {(col.route_rule_ids || []).length ? (col.route_rule_ids || []).map(id => routeRuleNameByID.get(id) || `#${id}`).join('、') : '-'}
                          </td>
                          <td className="px-4 py-3">{col.type === 'urltest' ? '自动' : '手动'}</td>
                          <td className="max-w-[400px] truncate px-4 py-3 text-xs">
                            {col.source_type === 'node_groups' && col.referenced_groups 
                              ? col.referenced_groups.map(g => g.name).join('、') 
                              : col.source_type === 'manual' ? `手动选择 (${col.node_uids?.length || 0} 个节点)` : '-'}
                          </td>
                          <td className="px-4 py-3">
                            <span className={`rounded px-2 py-1 text-xs ${col.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-gray-500/10 text-gray-400'}`}>
                              {col.enabled ? '启用' : '停用'}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex gap-2">
                              <button onClick={() => previewCollectionConfig(col)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-xs text-white hover:bg-white/[0.08]">
                                <Eye size={12} className="inline mr-1" />预览
                              </button>
                              <button onClick={() => editCollection(col)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-xs text-white hover:bg-white/[0.08]">
                                <Edit size={12} className="inline mr-1" />编辑
                              </button>
                              <button onClick={() => deleteCollection(col)} className="inline-flex items-center gap-1 rounded-md border border-red-400/30 bg-red-500/10 px-3 py-1 text-xs text-red-200 hover:bg-red-500/20">
                                <Trash2 size={12} />删除
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>

              <Pagination total={collections.length} page={collectionPage} pageSize={collectionPageSize} totalPages={collectionTotalPages} onPageChange={setCollectionPage} onPageSizeChange={setCollectionPageSize} />

              {previewCollection && (
                <div className="aw-modal-backdrop" onClick={() => setPreviewCollection(null)}>
                  <div className="aw-modal-panel w-full max-w-3xl p-6" onClick={e => e.stopPropagation()}>
                    <div className="mb-4 flex items-start justify-between gap-4">
                      <div>
                        <h4 className="text-lg font-semibold text-white">策略组预览</h4>
                        <p className="mt-1 text-xs text-[var(--text-tertiary)]">{previewCollection.name} 将写入 sing-box `outbounds` 的策略组片段</p>
                      </div>
                      <button onClick={() => setPreviewCollection(null)} className="aw-modal-close">✕</button>
                    </div>
                    {previewOutbound ? (
                      <pre className="max-h-[520px] overflow-auto rounded-lg border border-[var(--border-default)] bg-black/30 p-4 text-xs leading-relaxed text-blue-100">{JSON.stringify(previewOutbound, null, 2)}</pre>
                    ) : (
                      <div className="rounded-lg border border-yellow-400/20 bg-yellow-500/10 px-4 py-3 text-sm text-yellow-200">暂无可预览的策略组片段。</div>
                    )}
                  </div>
                </div>
              )}

              {editingCollection && (
                <div className="aw-modal-backdrop" onClick={resetCollectionForm}>
                  <div className="aw-modal-panel w-full max-w-3xl p-6" onClick={e => e.stopPropagation()}>
                    <div className="mb-4 flex items-center justify-between">
                      <h4 className="text-lg font-semibold text-[var(--text-primary)]">{isEditingCollection ? '编辑策略组' : '新增策略组'}</h4>
                      <button onClick={resetCollectionForm} className="aw-modal-close">✕</button>
                    </div>
                    <div className="grid gap-4 md:grid-cols-2">
                      <label className="block">
                        <span className="text-sm font-medium text-[var(--text-primary)]">规则分类</span>
                        <select value={colSelectedRuleIDs[0] || 0} onChange={e => selectCollectionRule(Number(e.target.value))} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                          <option value={0}>{routeRules.length === 0 ? '暂无规则，请先到规则管理创建' : selectableRouteRules.length === 0 ? '没有可用的策略规则' : '请选择规则'}</option>
                          {selectableRouteRules.map(rule => (
                            <option key={rule.id} value={rule.id}>
                              {rule.name} · {routeRuleOutboundLabel(rule.outbound)}{rule.enabled ? '' : '（停用）'}
                            </option>
                          ))}
                        </select>
                        <p className="mt-1 text-xs text-[var(--text-tertiary)]">只显示规则管理中出站为“策略”且未被其他策略组绑定的规则；策略组名称会使用所选规则名称。</p>
                      </label>
                      <label className="block">
                        <span className="text-sm font-medium text-[var(--text-primary)]">类型</span>
                        <select value={colType} onChange={e => setColType(e.target.value)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                          <option value="selector">手动切换</option>
                          <option value="urltest">自动选择（测速）</option>
                        </select>
                      </label>
                      <div className="md:col-span-2">
                        <label className="inline-flex items-center gap-2 text-sm text-[var(--text-secondary)]">
                          <input type="checkbox" checked={colEnabled} onChange={e => setColEnabled(e.target.checked)} />
                          启用此策略组
                        </label>
                      </div>
                      {colType === 'urltest' && (
                        <div className="grid gap-4 rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] p-3 md:col-span-2 md:grid-cols-3">
                          <label className="block md:col-span-3">
                            <span className="text-sm font-medium text-[var(--text-primary)]">测速 URL</span>
                            <input value={colTestURL} onChange={e => setColTestURL(e.target.value)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                          </label>
                          <label className="block">
                            <span className="text-sm font-medium text-[var(--text-primary)]">测速间隔（秒）</span>
                            <input type="number" min={1} value={colTestInterval} onChange={e => setColTestInterval(Number(e.target.value) || 300)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                          </label>
                          <label className="block">
                            <span className="text-sm font-medium text-[var(--text-primary)]">容差（毫秒）</span>
                            <input type="number" min={0} value={colTolerance} onChange={e => setColTolerance(Number(e.target.value) || 0)} className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                          </label>
                          <div className="flex items-end text-xs text-[var(--text-tertiary)]">urltest 会在引用节点组之间自动选择延迟最低的一组。</div>
                        </div>
                      )}
                    </div>

                    <div className="mt-4">
                      <label className="block">
                        <span className="text-sm font-medium text-[var(--text-primary)]">选择引用的节点组</span>
                        <p className="mt-1 text-xs text-[var(--text-tertiary)]">可多选，策略组将包含这些节点组的所有节点</p>
                        <div className="mt-3 grid max-h-[300px] gap-2 overflow-y-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] p-3 md:grid-cols-2">
                          {nodeGroups.filter(ng => ng.enabled).length === 0 ? (
                            <p className="col-span-2 py-8 text-center text-sm text-[var(--text-tertiary)]">暂无可用节点组，请先创建节点组</p>
                          ) : nodeGroups.filter(ng => ng.enabled).map(ng => (
                            <label key={ng.id} className="flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] cursor-pointer">
                              <input
                                type="checkbox"
                                checked={colSelectedGroupIDs.includes(ng.id)}
                                onChange={() => toggleGroupSelected(ng.id)}
                              />
                              <span className="flex-1">{ng.name}</span>
                              <span className="text-xs text-[var(--text-tertiary)]">({ng.matched_node_count || 0})</span>
                            </label>
                          ))}
                        </div>
                      </label>
                    </div>

                    <div className="mt-6 flex justify-end gap-3">
                      <button onClick={resetCollectionForm} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] px-4 py-2 text-sm text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]">
                        取消
                      </button>
                      <button onClick={saveCollection} disabled={colSelectedGroupIDs.length === 0 || colSelectedRuleIDs.length === 0} className="rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)] disabled:opacity-50 disabled:cursor-not-allowed">
                        {isEditingCollection ? '更新' : '创建'}
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </div>
  );
}

export default CollectionsPage;
