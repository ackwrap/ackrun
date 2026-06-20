import React from 'react';
import { ServerCog, Plus, Trash2 } from 'lucide-react';

import { PageHeader } from '@/components/layout/PageHeader';
import { Toast } from '@/components/ui/Toast';

// 临时类型定义，后续会补充到 services/types.ts
interface DNSServer {
  id: number;
  tag: string;
  enabled: boolean;
  server_type: string;
  address: string;
  address_resolver: string;
  address_strategy: string;
  strategy: string;
  detour: string;
  client_subnet: string;
  options_json: string;
  created_at: number;
  updated_at: number;
}

interface DNSRule {
  id: number;
  enabled: boolean;
  priority: number;
  rule_type: string;
  conditions_json: string;
  server: string;
  disable_cache: boolean;
  rewrite_ttl: number;
  client_subnet: string;
  created_at: number;
  updated_at: number;
}

interface DNSGlobalSettings {
  enabled: boolean;
  final: string;
  strategy: string;
  disable_cache: boolean;
  disable_expire: boolean;
  independent_cache: boolean;
  reverse_mapping: boolean;
  cache_capacity: number;
  client_subnet: string;
  fakeip_enabled: boolean;
  fakeip_inet4_range: string;
  fakeip_inet6_range: string;
}

interface ProxyCollectionOption {
  id: number;
  name: string;
  enabled: boolean;
}

const serverTypes = [
  { value: 'udp', label: 'UDP' },
  { value: 'tcp', label: 'TCP' },
  { value: 'https', label: 'HTTPS (DoH)' },
  { value: 'tls', label: 'TLS (DoT)' },
  { value: 'quic', label: 'QUIC (DoQ)' },
  { value: 'h3', label: 'HTTP/3 (DoH3)' },
  { value: 'local', label: 'Local' },
  { value: 'hosts', label: 'Hosts' },
  { value: 'dhcp', label: 'DHCP' },
  { value: 'fakeip', label: 'FakeIP（高级）' },
  { value: 'rcode', label: 'RCode' },
];

const dnsRuleConditionTypes = [
  { value: 'domain', label: '完整域名', hint: 'domain' },
  { value: 'domain_suffix', label: '域名后缀', hint: 'domain_suffix' },
  { value: 'domain_keyword', label: '域名关键词', hint: 'domain_keyword' },
  { value: 'domain_regex', label: '域名正则', hint: 'domain_regex' },
  { value: 'geosite', label: 'GeoSite', hint: 'geosite' },
  { value: 'outbound', label: 'Outbound', hint: 'outbound' },
  { value: 'query_type', label: '查询类型', hint: 'query_type' },
  { value: 'network', label: '网络类型', hint: 'network' },
  { value: 'protocol', label: '协议', hint: 'protocol' },
  { value: 'clash_mode', label: 'Clash Mode', hint: 'clash_mode' },
  { value: 'rule_set', label: '规则集', hint: 'rule_set' },
];

const strategyOptions = [
  { value: 'prefer_ipv4', label: '优先 IPv4' },
  { value: 'prefer_ipv6', label: '优先 IPv6' },
  { value: 'ipv4_only', label: '仅 IPv4' },
  { value: 'ipv6_only', label: '仅 IPv6' },
];

const builtinDNSServers = [
  { key: 'alidns-doh', label: '阿里 DoH', tag: 'dns_ali', server_type: 'https', address: 'https://dns.alidns.com/dns-query', detour: 'direct' },
  { key: 'alidns-udp', label: '阿里 DNS UDP', tag: 'dns_ali_udp', server_type: 'udp', address: '223.5.5.5', detour: 'direct' },
  { key: 'alidns-udp-backup', label: '阿里 DNS UDP 备用', tag: 'dns_ali_udp_2', server_type: 'udp', address: '223.6.6.6', detour: 'direct' },
  { key: 'dnspod-doh', label: '腾讯 DNSPod DoH', tag: 'dns_tencent', server_type: 'https', address: 'https://doh.pub/dns-query', detour: 'direct' },
  { key: 'dnspod-udp', label: '腾讯 DNSPod UDP', tag: 'dns_tencent_udp', server_type: 'udp', address: '119.29.29.29', detour: 'direct' },
  { key: 'dnspod-udp-backup', label: '腾讯 DNSPod UDP 备用', tag: 'dns_tencent_udp_2', server_type: 'udp', address: '119.28.28.28', detour: 'direct' },
  { key: 'cloudflare-doh', label: 'Cloudflare DoH', tag: 'dns_cloudflare', server_type: 'https', address: 'https://cloudflare-dns.com/dns-query', detour: 'proxy' },
  { key: 'google-doh', label: 'Google DoH', tag: 'dns_google', server_type: 'https', address: 'https://dns.google/dns-query', detour: 'proxy' },
  { key: 'quad9-doh', label: 'Quad9 DoH', tag: 'dns_quad9', server_type: 'https', address: 'https://dns.quad9.net/dns-query', detour: 'proxy' },
  { key: '114-udp', label: '114 DNS UDP', tag: 'dns_114', server_type: 'udp', address: '114.114.114.114', detour: 'direct' },
  { key: 'baidu-udp', label: '百度 DNS UDP', tag: 'dns_baidu', server_type: 'udp', address: '180.76.76.76', detour: 'direct' },
  { key: 'mobile-udp', label: '移动 DNS UDP', tag: 'dns_mobile', server_type: 'udp', address: '211.136.192.6', detour: 'direct' },
  { key: 'unicom-udp', label: '联通 DNS UDP', tag: 'dns_unicom', server_type: 'udp', address: '123.125.81.6', detour: 'direct' },
  { key: 'telecom-udp', label: '电信 DNS UDP', tag: 'dns_telecom', server_type: 'udp', address: '202.96.128.86', detour: 'direct' },
];

export function DNSPage() {
  const [servers, setServers] = React.useState<DNSServer[]>([]);
  const [rules, setRules] = React.useState<DNSRule[]>([]);
  const [collections, setCollections] = React.useState<ProxyCollectionOption[]>([]);
  const [globalSettings, setGlobalSettings] = React.useState<DNSGlobalSettings>({
    enabled: true,
    final: 'dns_proxy',
    strategy: 'prefer_ipv4',
    disable_cache: false,
    disable_expire: false,
    independent_cache: false,
    reverse_mapping: false,
    cache_capacity: 4096,
    client_subnet: '',
    fakeip_enabled: false,
    fakeip_inet4_range: '198.19.0.0/16',
    fakeip_inet6_range: 'fdfe:dcba:9876::/48',
  });
  const [loading, setLoading] = React.useState(true);
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error'>('success');
  
  // DNS Server 表单状态
  const [editingServer, setEditingServer] = React.useState<DNSServer | null>(null);
  const [serverTag, setServerTag] = React.useState('');
  const [serverEnabled, setServerEnabled] = React.useState(true);
  const [serverType, setServerType] = React.useState('https');
  const [serverAddress, setServerAddress] = React.useState('');
  const [serverAddressResolver, setServerAddressResolver] = React.useState('');
  const [serverAddressStrategy, setServerAddressStrategy] = React.useState('');
  const [serverStrategy, setServerStrategy] = React.useState('');
  const [serverDetour, setServerDetour] = React.useState('');
  const [serverDetourCustom, setServerDetourCustom] = React.useState(false);
  const [serverClientSubnet, setServerClientSubnet] = React.useState('');
  const [selectedBuiltinServer, setSelectedBuiltinServer] = React.useState(builtinDNSServers[0].key);

  // DNS Rule 表单状态
  const [editingRule, setEditingRule] = React.useState<DNSRule | null>(null);
  const [ruleEnabled, setRuleEnabled] = React.useState(true);
  const [ruleServer, setRuleServer] = React.useState('');
  const [ruleDisableCache, setRuleDisableCache] = React.useState(false);
  const [ruleRewriteTTL, setRuleRewriteTTL] = React.useState(0);
  const [ruleClientSubnet, setRuleClientSubnet] = React.useState('');
  // 条件字段
  const [ruleConditionType, setRuleConditionType] = React.useState('domain_suffix');
  const [ruleConditionValues, setRuleConditionValues] = React.useState('');
  const [previewJSON, setPreviewJSON] = React.useState('');

  const detourOptions = React.useMemo(() => {
    const options = [
      { value: '', label: '留空（默认出站）' },
      { value: 'direct', label: 'direct（直连）' },
      { value: 'proxy', label: 'proxy（默认策略）' },
      { value: 'block', label: 'block（阻断）' },
    ];
    for (const collection of collections) {
      if (!collection.enabled) continue;
      if (options.some(item => item.value === collection.name)) continue;
      options.push({ value: collection.name, label: `${collection.name}（策略组）` });
    }
    return options;
  }, [collections]);

  const dnsBindingTargets = React.useMemo(() => {
    return detourOptions.filter(item => item.value === 'direct' || item.value === 'proxy' || collections.some(collection => collection.enabled && collection.name === item.value));
  }, [collections, detourOptions]);

  const [dnsBindings, setDNSBindings] = React.useState<Record<string, string>>({});

  const getOutboundCondition = (rule: DNSRule) => {
    try {
      const conditions = JSON.parse(rule.conditions_json || '{}');
      const outbound = conditions.outbound;
      if (Array.isArray(outbound)) return outbound.map(String);
      if (typeof outbound === 'string' && outbound) return [outbound];
    } catch {}
    return [];
  };

  const findOutboundBindingRule = (outbound: string) => {
    return rules.find(rule => getOutboundCondition(rule).includes(outbound));
  };

  const showMessage = (msg: string, type: 'success' | 'error' = 'success') => {
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
      const [serversData, rulesData, globalData, collectionsData] = await Promise.all([
        fetch('/api/v1/dns/servers').then(r => r.json()),
        fetch('/api/v1/dns/rules').then(r => r.json()),
        fetch('/api/v1/dns/global').then(r => r.json()),
        fetch('/api/v1/collections').then(r => r.json()).catch(() => []),
      ]);
      setServers(Array.isArray(serversData) ? serversData : []);
      setRules(Array.isArray(rulesData) ? rulesData : []);
      setCollections(Array.isArray(collectionsData) ? collectionsData : []);
      if (globalData) {
        setGlobalSettings(prev => ({ ...prev, ...globalData }));
      }
    } catch (e: any) {
      showMessage(`加载失败: ${e.message}`, 'error');
      setServers([]);
      setRules([]);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { load(); }, [load]);

  const saveGlobalSettings = async () => {
    try {
      await fetch('/api/v1/dns/global', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(globalSettings),
      });
      showMessage('全局设置已保存');
      await load();
    } catch (e: any) {
      showMessage(`保存失败: ${e.message}`, 'error');
    }
  };

  // DNS Server CRUD
  const resetServerForm = () => {
    setEditingServer(null);
    setServerTag('');
    setServerEnabled(true);
    setServerType('https');
    setServerAddress('');
    setServerAddressResolver('');
    setServerAddressStrategy('');
    setServerStrategy('');
    setServerDetour('');
    setServerDetourCustom(false);
    setServerClientSubnet('');
  };

  const editServer = (server: DNSServer) => {
    setEditingServer(server);
    setServerTag(server.tag);
    setServerEnabled(server.enabled);
    setServerType(server.server_type);
    setServerAddress(server.address);
    setServerAddressResolver(server.address_resolver);
    setServerAddressStrategy(server.address_strategy);
    setServerStrategy(server.strategy);
    setServerDetour(server.detour);
    setServerDetourCustom(Boolean(server.detour) && !detourOptions.some(item => item.value === server.detour));
    setServerClientSubnet(server.client_subnet);
  };

  const saveServer = async () => {
    try {
      const payload = {
        tag: serverTag,
        enabled: serverEnabled,
        server_type: serverType,
        address: serverAddress,
        address_resolver: serverAddressResolver,
        address_strategy: serverAddressStrategy,
        strategy: serverStrategy,
        detour: serverDetour,
        client_subnet: serverClientSubnet,
        options: {},
      };
      if (editingServer) {
        await fetch(`/api/v1/dns/servers/${editingServer.id}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        showMessage('DNS 服务器已更新');
      } else {
        await fetch('/api/v1/dns/servers', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        showMessage('DNS 服务器已添加');
      }
      resetServerForm();
      await load();
    } catch (e: any) {
      showMessage(`保存失败: ${e.message}`, 'error');
    }
  };

  const addBuiltinServer = async () => {
    const preset = builtinDNSServers.find(item => item.key === selectedBuiltinServer);
    if (!preset) return;
    try {
      const resp = await fetch('/api/v1/dns/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tag: preset.tag,
          enabled: true,
          server_type: preset.server_type,
          address: preset.address,
          address_resolver: '',
          address_strategy: '',
          strategy: '',
          detour: preset.detour,
          client_subnet: '',
          options: {},
        }),
      });
      if (!resp.ok) {
        const err = await resp.json().catch(() => null);
        throw new Error(err?.error?.message || `${preset.tag} 添加失败`);
      }
      showMessage(`${preset.label} 已加入`);
      await load();
    } catch (e: any) {
      showMessage(`加入内置 Server 失败: ${e.message}`, 'error');
    }
  };

  const deleteServer = async (server: DNSServer) => {
    if (!confirm(`确定删除 DNS 服务器 "${server.tag}" 吗？`)) return;
    try {
      await fetch(`/api/v1/dns/servers/${server.id}`, { method: 'DELETE' });
      showMessage('DNS 服务器已删除');
      await load();
    } catch (e: any) {
      showMessage(`删除失败: ${e.message}`, 'error');
    }
  };

  const toggleServer = async (server: DNSServer) => {
    try {
      await fetch(`/api/v1/dns/servers/${server.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...server, enabled: !server.enabled, options: {} }),
      });
      await load();
    } catch (e: any) {
      showMessage(`状态更新失败: ${e.message}`, 'error');
    }
  };

  const saveDNSBinding = async (outbound: string) => {
    const server = dnsBindings[outbound] ?? findOutboundBindingRule(outbound)?.server ?? '';
    const existingRule = findOutboundBindingRule(outbound);
    try {
      if (!server) {
        if (existingRule) {
          await fetch(`/api/v1/dns/rules/${existingRule.id}`, { method: 'DELETE' });
          setRules(prev => prev.filter(rule => rule.id !== existingRule.id));
          setDNSBindings(prev => ({ ...prev, [outbound]: '' }));
          showMessage(`${outbound} 的 DNS 出口绑定已删除`);
        }
        return;
      }

      const payload = {
        enabled: true,
        priority: existingRule?.priority || 0,
        rule_type: 'default',
        conditions: { outbound: [outbound] },
        server,
        disable_cache: existingRule?.disable_cache || false,
        rewrite_ttl: existingRule?.rewrite_ttl || 0,
        client_subnet: existingRule?.client_subnet || '',
      };
      const url = existingRule ? `/api/v1/dns/rules/${existingRule.id}` : '/api/v1/dns/rules';
      const method = existingRule ? 'PUT' : 'POST';
      const resp = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        const err = await resp.json().catch(() => null);
        throw new Error(err?.error?.message || '保存绑定失败');
      }
      const savedRule = await resp.json().catch(() => null);
      if (savedRule?.id) {
        setRules(prev => existingRule ? prev.map(rule => rule.id === existingRule.id ? savedRule : rule) : [...prev, savedRule]);
      }
      setDNSBindings(prev => ({ ...prev, [outbound]: server }));
      showMessage(`${outbound} 的 DNS 出口绑定已保存`);
    } catch (e: any) {
      showMessage(`保存 DNS 出口绑定失败: ${e.message}`, 'error');
    }
  };

  const previewDNSBinding = (outbound: string) => {
    const server = dnsBindings[outbound] ?? findOutboundBindingRule(outbound)?.server ?? '';
    if (!server) {
      setPreviewJSON(JSON.stringify({ outbound: [outbound], server: '(未绑定)' }, null, 2));
      return;
    }
    setPreviewJSON(JSON.stringify({ outbound: [outbound], server }, null, 2));
  };

  // DNS Rule CRUD
  const resetRuleForm = () => {
    setEditingRule(null);
    setRuleEnabled(true);
    setRuleServer('');
    setRuleDisableCache(false);
    setRuleRewriteTTL(0);
    setRuleClientSubnet('');
    setRuleConditionType('domain_suffix');
    setRuleConditionValues('');
  };

  const editRule = (rule: DNSRule) => {
    setEditingRule(rule);
    setRuleEnabled(rule.enabled);
    setRuleServer(rule.server);
    setRuleDisableCache(rule.disable_cache);
    setRuleRewriteTTL(rule.rewrite_ttl);
    setRuleClientSubnet(rule.client_subnet);
    
    // 解析 conditions_json 并填充到表单
    try {
      const conditions = JSON.parse(rule.conditions_json);
      const firstKey = Object.keys(conditions)[0];
      if (firstKey) {
        setRuleConditionType(firstKey);
        const value = conditions[firstKey];
        if (Array.isArray(value)) {
          setRuleConditionValues(value.join('\n'));
        } else {
          setRuleConditionValues(String(value));
        }
      }
    } catch {
      setRuleConditionType('domain_suffix');
      setRuleConditionValues('');
    }
  };

  const saveRule = async () => {
    try {
      // 从表单构建 conditions
      const values = ruleConditionValues.split('\n').map(v => v.trim()).filter(Boolean);
      const conditions: any = {};
      
      // clash_mode 是字符串，其他是数组
      if (ruleConditionType === 'clash_mode') {
        conditions[ruleConditionType] = values[0] || 'rule';
      } else {
        conditions[ruleConditionType] = values;
      }
      
      const payload = {
        enabled: ruleEnabled,
        priority: editingRule?.priority || 0,
        rule_type: 'default',
        conditions,
        server: ruleServer,
        disable_cache: ruleDisableCache,
        rewrite_ttl: ruleRewriteTTL,
        client_subnet: ruleClientSubnet,
      };
      if (editingRule?.id) {
        await fetch(`/api/v1/dns/rules/${editingRule.id}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        showMessage('DNS 规则已更新');
      } else {
        await fetch('/api/v1/dns/rules', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        showMessage('DNS 规则已添加');
      }
      resetRuleForm();
      await load();
    } catch (e: any) {
      showMessage(`保存失败: ${e.message}`, 'error');
    }
  };

  const deleteRule = async (rule: DNSRule) => {
    if (!confirm('确定删除此 DNS 规则吗？')) return;
    try {
      await fetch(`/api/v1/dns/rules/${rule.id}`, { method: 'DELETE' });
      showMessage('DNS 规则已删除');
      await load();
    } catch (e: any) {
      showMessage(`删除失败: ${e.message}`, 'error');
    }
  };

  const toggleRule = async (rule: DNSRule) => {
    try {
      let conditions = {};
      try { conditions = JSON.parse(rule.conditions_json); } catch {}
      await fetch(`/api/v1/dns/rules/${rule.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...rule, enabled: !rule.enabled, conditions }),
      });
      await load();
    } catch (e: any) {
      showMessage(`状态更新失败: ${e.message}`, 'error');
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader title="DNS 管理" />
      <Toast message={message} type={messageType} />

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <div className="text-center">
            <div className="mx-auto h-10 w-10 animate-spin rounded-full border-4 border-blue-500/20 border-t-blue-500"></div>
            <div className="mt-4 text-sm text-[var(--text-secondary)]">加载中...</div>
          </div>
        </div>
      ) : (
        <>
          {/* 全局设置 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-blue-400/20 bg-blue-500/10 text-blue-200"><ServerCog size={18} /></div>
                <div>
                  <h3 className="text-sm font-semibold text-white">全局设置</h3>
                  <p className="mt-1 text-xs text-[var(--text-tertiary)]">DNS 模块总开关和默认行为</p>
                </div>
              </div>
              <label className="inline-flex items-center gap-2 text-xs text-[var(--text-secondary)]">
                <input type="checkbox" checked={globalSettings.enabled} onChange={e => setGlobalSettings(prev => ({ ...prev, enabled: e.target.checked }))} />
                启用 DNS 管理
              </label>
            </div>
            <div className="grid gap-3 md:grid-cols-5">
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">默认 DNS Server Tag</span>
                <select disabled={!globalSettings.enabled} value={globalSettings.final} onChange={e => setGlobalSettings(prev => ({ ...prev, final: e.target.value }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400 disabled:opacity-50">
                  <option value="">请选择</option>
                  {servers.filter(server => server.enabled).map(server => <option key={server.id} value={server.tag}>{server.tag}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">IP 返回策略</span>
                <select disabled={!globalSettings.enabled} value={globalSettings.strategy} onChange={e => setGlobalSettings(prev => ({ ...prev, strategy: e.target.value }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400 disabled:opacity-50">
                  {strategyOptions.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">缓存容量</span>
                <input type="number" min={0} disabled={!globalSettings.enabled} value={globalSettings.cache_capacity} onChange={e => setGlobalSettings(prev => ({ ...prev, cache_capacity: parseInt(e.target.value) || 0 }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400 disabled:opacity-50" />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">Client Subnet (可选)</span>
                <input disabled={!globalSettings.enabled} value={globalSettings.client_subnet} onChange={e => setGlobalSettings(prev => ({ ...prev, client_subnet: e.target.value }))} placeholder="1.2.3.0/24" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400 disabled:opacity-50" />
              </label>
              <div className="flex items-end">
                <button onClick={saveGlobalSettings} className="h-9 w-full rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">保存全局设置</button>
              </div>
            </div>
            <div className="mt-3 flex flex-wrap gap-3 text-xs">
              <label className="inline-flex items-center gap-2 text-[var(--text-secondary)]">
                <input type="checkbox" disabled={!globalSettings.enabled} checked={globalSettings.disable_cache} onChange={e => setGlobalSettings(prev => ({ ...prev, disable_cache: e.target.checked }))} />
                禁用缓存
              </label>
              <label className="inline-flex items-center gap-2 text-[var(--text-secondary)]">
                <input type="checkbox" disabled={!globalSettings.enabled} checked={globalSettings.disable_expire} onChange={e => setGlobalSettings(prev => ({ ...prev, disable_expire: e.target.checked }))} />
                禁用过期缓存
              </label>
              <label className="inline-flex items-center gap-2 text-[var(--text-secondary)]">
                <input type="checkbox" disabled={!globalSettings.enabled} checked={globalSettings.independent_cache} onChange={e => setGlobalSettings(prev => ({ ...prev, independent_cache: e.target.checked }))} />
                独立缓存
              </label>
              <label className="inline-flex items-center gap-2 text-[var(--text-secondary)]">
                <input type="checkbox" disabled={!globalSettings.enabled} checked={globalSettings.reverse_mapping} onChange={e => setGlobalSettings(prev => ({ ...prev, reverse_mapping: e.target.checked }))} />
                反向映射
              </label>
            </div>
          </section>

          {/* FakeIP 设置 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-white">FakeIP</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">启用后自动生成 fakeip DNS server，并优先添加 A/AAAA 查询规则。</p>
              </div>
              <label className="inline-flex items-center gap-2 text-xs text-[var(--text-secondary)]">
                <input type="checkbox" disabled={!globalSettings.enabled} checked={globalSettings.fakeip_enabled} onChange={e => setGlobalSettings(prev => ({ ...prev, fakeip_enabled: e.target.checked }))} />
                启用 FakeIP
              </label>
            </div>
            <div className="grid gap-3 md:grid-cols-[1fr_1fr_auto]">
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">IPv4 地址池</span>
                <input disabled={!globalSettings.enabled || !globalSettings.fakeip_enabled} value={globalSettings.fakeip_inet4_range} onChange={e => setGlobalSettings(prev => ({ ...prev, fakeip_inet4_range: e.target.value }))} placeholder="198.19.0.0/16" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400 disabled:opacity-50" />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">IPv6 地址池</span>
                <input disabled={!globalSettings.enabled || !globalSettings.fakeip_enabled} value={globalSettings.fakeip_inet6_range} onChange={e => setGlobalSettings(prev => ({ ...prev, fakeip_inet6_range: e.target.value }))} placeholder="fdfe:dcba:9876::/48" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400 disabled:opacity-50" />
              </label>
              <div className="flex items-end">
                <button onClick={saveGlobalSettings} className="h-9 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-4 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">保存 FakeIP</button>
              </div>
            </div>
          </section>

          {/* DNS 服务器管理 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-white">DNS 服务器</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">管理 DNS server，每个 server 有唯一 tag。</p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <select value={selectedBuiltinServer} onChange={e => setSelectedBuiltinServer(e.target.value)} className="h-9 rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-xs text-[var(--text-primary)] outline-none focus:border-blue-400">
                  {builtinDNSServers.map(item => <option key={item.key} value={item.key}>{item.label}</option>)}
                </select>
                <button onClick={addBuiltinServer} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-xs font-medium text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">加入内置 Server</button>
                <button onClick={() => setEditingServer({} as DNSServer)} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]"><Plus size={14} />新增 Server</button>
              </div>
            </div>

            {/* 服务器表格 */}
            <div className="overflow-hidden rounded-xl border border-[var(--border-default)] mb-4">
              <div className="overflow-x-auto">
                <table className="w-full min-w-[900px] border-collapse text-left text-sm">
                  <thead className="bg-white/[0.04] text-white">
                    <tr>
                      {['Tag', '类型', '地址', 'Detour', '状态', '操作'].map(col => <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {servers.length === 0 ? (
                      <tr><td colSpan={6} className="px-4 py-12 text-center text-[var(--text-tertiary)]">暂无 DNS 服务器</td></tr>
                    ) : servers.map(server => (
                      <tr key={server.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0">
                        <td className="px-4 py-3 font-mono text-sm text-white">{server.tag}</td>
                        <td className="px-4 py-3">{server.server_type}</td>
                        <td className="max-w-[300px] truncate px-4 py-3 font-mono text-xs">{server.address || '-'}</td>
                        <td className="px-4 py-3">{server.detour || '-'}</td>
                        <td className="px-4 py-3">
                          <button onClick={() => toggleServer(server)} className={`rounded px-2 py-1 text-xs ${server.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{server.enabled ? '启用' : '停用'}</button>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex gap-2">
                            <button onClick={() => editServer(server)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-xs text-white hover:bg-white/[0.08]">编辑</button>
                            <button onClick={() => deleteServer(server)} className="inline-flex items-center gap-1 rounded-md border border-red-400/30 bg-red-500/10 px-3 py-1 text-xs text-red-200 hover:bg-red-500/20"><Trash2 size={12} />删除</button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            {/* 服务器表单 */}
            {editingServer !== null && (
              <div className="aw-modal-backdrop" onClick={resetServerForm}>
                <div className="aw-modal-panel max-h-[90vh] w-full max-w-4xl overflow-y-auto p-5" onClick={e => e.stopPropagation()}>
                <div className="mb-4 flex items-center justify-between">
                  <h4 className="text-sm font-semibold text-[var(--text-primary)]">{editingServer.id ? '编辑 DNS 服务器' : '新增 DNS 服务器'}</h4>
                  <button onClick={resetServerForm} className="aw-modal-close">✕</button>
                </div>
                <div className="grid gap-3 md:grid-cols-3">
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Tag（唯一标识）</span>
                    <input value={serverTag} onChange={e => setServerTag(e.target.value)} placeholder="dns_proxy" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">类型</span>
                    <select value={serverType} onChange={e => setServerType(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                      {serverTypes.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">地址</span>
                    <input value={serverAddress} onChange={e => setServerAddress(e.target.value)} placeholder={serverType === 'rcode' ? 'success / name_error / refused' : serverType === 'local' ? 'local' : serverType === 'dhcp' ? 'auto 或网卡名' : 'https://1.1.1.1/dns-query'} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Address Resolver</span>
                    <input value={serverAddressResolver} onChange={e => setServerAddressResolver(e.target.value)} placeholder="dns_resolver" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Address Strategy</span>
                    <select value={serverAddressStrategy} onChange={e => setServerAddressStrategy(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                      <option value="">（留空）</option>
                      {strategyOptions.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Strategy</span>
                    <select value={serverStrategy} onChange={e => setServerStrategy(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                      <option value="">（留空）</option>
                      {strategyOptions.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Detour</span>
                    <select
                      value={serverDetourCustom ? '__custom__' : serverDetour}
                      onChange={e => {
                        if (e.target.value === '__custom__') {
                          setServerDetourCustom(true);
                          return;
                        }
                        setServerDetourCustom(false);
                        setServerDetour(e.target.value);
                      }}
                      className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400"
                    >
                      {detourOptions.map(item => <option key={item.value || 'empty'} value={item.value}>{item.label}</option>)}
                      <option value="__custom__">自定义...</option>
                    </select>
                    {serverDetourCustom && (
                      <input value={serverDetour} onChange={e => setServerDetour(e.target.value)} placeholder="自定义 outbound tag" className="mt-2 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                    )}
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Client Subnet</span>
                    <input value={serverClientSubnet} onChange={e => setServerClientSubnet(e.target.value)} placeholder="1.2.3.0/24" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                  </label>
                  <div className="flex items-end">
                    <label className="inline-flex items-center gap-2 text-xs text-[var(--text-secondary)]">
                      <input type="checkbox" checked={serverEnabled} onChange={e => setServerEnabled(e.target.checked)} />
                      启用
                    </label>
                  </div>
                </div>
                <div className="mt-4 flex justify-end gap-2">
                  <button onClick={resetServerForm} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-4 py-2 text-sm text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">取消</button>
                  <button onClick={saveServer} className="inline-flex h-9 items-center gap-2 rounded-md bg-[var(--color-primary)] px-4 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)]">{editingServer.id ? '更新' : '添加'}</button>
                </div>
                </div>
              </div>
            )}
          </section>

          {/* DNS 出口绑定 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-white">DNS 出口绑定</h3>
              <p className="mt-1 text-xs text-[var(--text-tertiary)]">让不同策略组使用对应地区出口查询 DNS，避免 CDN 返回和实际代理地区不一致。</p>
            </div>
            <div className="overflow-hidden rounded-xl border border-[var(--border-default)]">
              <div className="overflow-x-auto">
                <table className="w-full min-w-[720px] border-collapse text-left text-sm">
                  <thead className="bg-white/[0.04] text-white">
                    <tr>
                      {['出站/策略组', 'DNS Server', '当前规则', '操作'].map(col => <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {dnsBindingTargets.length === 0 ? (
                      <tr><td colSpan={4} className="px-4 py-10 text-center text-[var(--text-tertiary)]">暂无可绑定的策略组</td></tr>
                    ) : dnsBindingTargets.map(target => {
                      const existingRule = findOutboundBindingRule(target.value);
                      const selectedServer = dnsBindings[target.value] ?? existingRule?.server ?? '';
                      return (
                        <tr key={target.value} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0">
                          <td className="px-4 py-3 font-mono text-sm text-white">{target.label}</td>
                          <td className="px-4 py-3">
                            <select value={selectedServer} onChange={e => setDNSBindings(prev => ({ ...prev, [target.value]: e.target.value }))} className="w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                              <option value="">不绑定</option>
                              {servers.filter(server => server.enabled && server.server_type !== 'fakeip').map(server => <option key={server.id} value={server.tag}>{server.tag}{server.detour ? ` · detour=${server.detour}` : ''}</option>)}
                            </select>
                          </td>
                          <td className="px-4 py-3 text-xs text-[var(--text-tertiary)]">{existingRule ? `dns.rules #${existingRule.id}` : '未生成'}</td>
                          <td className="px-4 py-3">
                            <div className="flex gap-2">
                              <button onClick={() => previewDNSBinding(target.value)} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">预览</button>
                              <button onClick={() => saveDNSBinding(target.value)} className="rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 py-1.5 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]">保存绑定</button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
            <p className="mt-3 text-xs text-[var(--text-tertiary)]">建议为香港、美国、日本等单地区策略组分别创建真实 DNS Server，并让 DNS Server 的 Detour 指向同一个策略组。FakeIP 只用于返回给客户端，不参与这里的真实 CDN 解析出口绑定。</p>
          </section>

          {/* DNS 规则管理 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <h3 className="text-sm font-semibold text-white">DNS 规则</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">管理 DNS 路由规则，决定查询使用哪个 DNS server。</p>
              </div>
              <button onClick={() => setEditingRule({} as DNSRule)} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-xs font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)]"><Plus size={14} />新增规则</button>
            </div>

            {/* 规则表格 */}
            <div className="overflow-hidden rounded-xl border border-[var(--border-default)] mb-4">
              <div className="overflow-x-auto">
                <table className="w-full min-w-[900px] border-collapse text-left text-sm">
                  <thead className="bg-white/[0.04] text-white">
                    <tr>
                      {['匹配条件', 'DNS Server', '禁用缓存', '状态', '操作'].map(col => <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {rules.length === 0 ? (
                      <tr><td colSpan={5} className="px-4 py-12 text-center text-[var(--text-tertiary)]">暂无 DNS 规则</td></tr>
                    ) : rules.map(rule => {
                      let conditionsPreview = '';
                      try {
                        const cond = JSON.parse(rule.conditions_json);
                        conditionsPreview = Object.keys(cond).map(k => `${k}: ${JSON.stringify(cond[k])}`).join(', ');
                      } catch {}
                      return (
                        <tr key={rule.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0">
                          <td className="max-w-[400px] truncate px-4 py-3 font-mono text-xs">{conditionsPreview || '(空)'}</td>
                          <td className="px-4 py-3 font-mono text-sm text-white">{rule.server}</td>
                          <td className="px-4 py-3">{rule.disable_cache ? '是' : '否'}</td>
                          <td className="px-4 py-3">
                            <button onClick={() => toggleRule(rule)} className={`rounded px-2 py-1 text-xs ${rule.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{rule.enabled ? '启用' : '停用'}</button>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex gap-2">
                              <button onClick={() => editRule(rule)} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1 text-xs text-white hover:bg-white/[0.08]">编辑</button>
                              <button onClick={() => deleteRule(rule)} className="inline-flex items-center gap-1 rounded-md border border-red-400/30 bg-red-500/10 px-3 py-1 text-xs text-red-200 hover:bg-red-500/20"><Trash2 size={12} />删除</button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>

            {/* 规则表单 */}
            {editingRule !== null && (
              <div className="aw-modal-backdrop" onClick={resetRuleForm}>
                <div className="aw-modal-panel max-h-[90vh] w-full max-w-4xl overflow-y-auto p-5" onClick={e => e.stopPropagation()}>
                <div className="mb-4 flex items-center justify-between">
                  <h4 className="text-sm font-semibold text-[var(--text-primary)]">{editingRule.id ? '编辑 DNS 规则' : '新增 DNS 规则'}</h4>
                  <button onClick={resetRuleForm} className="aw-modal-close">✕</button>
                </div>
                <div className="grid gap-3 md:grid-cols-3">
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">匹配条件类型</span>
                    <select value={ruleConditionType} onChange={e => setRuleConditionType(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                      {dnsRuleConditionTypes.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
                    </select>
                  </label>
                  <label className="block md:col-span-2">
                    <span className="text-xs text-[var(--text-tertiary)]">匹配值（每行一个{ruleConditionType === 'clash_mode' ? '，仅第一行生效' : ''}）</span>
                    <textarea value={ruleConditionValues} onChange={e => setRuleConditionValues(e.target.value)} rows={3} placeholder={ruleConditionType === 'clash_mode' ? 'rule 或 direct 或 global' : ruleConditionType === 'query_type' ? 'A\nAAAA\nHTTPS' : ruleConditionType === 'outbound' ? 'direct\nproxy' : ruleConditionType === 'domain_suffix' ? 'example.com\ngoogle.com' : ruleConditionType === 'geosite' ? 'cn\ngeolocation-!cn' : '每行一个值'} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 font-mono text-xs text-[var(--text-primary)] outline-none focus:border-blue-400"></textarea>
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">DNS Server Tag</span>
                    <select value={ruleServer} onChange={e => setRuleServer(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                      <option value="">请选择 DNS Server</option>
                      {servers.filter(server => server.enabled).map(server => <option key={server.id} value={server.tag}>{server.tag}</option>)}
                      {globalSettings.fakeip_enabled && <option value="fakeip">fakeip</option>}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Rewrite TTL（秒，0 = 不重写）</span>
                    <input type="number" value={ruleRewriteTTL} onChange={e => setRuleRewriteTTL(parseInt(e.target.value) || 0)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                  </label>
                  <label className="block">
                    <span className="text-xs text-[var(--text-tertiary)]">Client Subnet（可选）</span>
                    <input value={ruleClientSubnet} onChange={e => setRuleClientSubnet(e.target.value)} placeholder="1.2.3.0/24" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
                  </label>
                </div>
                <div className="mt-3 flex flex-wrap gap-3 text-xs">
                  <label className="inline-flex items-center gap-2 text-[var(--text-secondary)]">
                    <input type="checkbox" checked={ruleDisableCache} onChange={e => setRuleDisableCache(e.target.checked)} />
                    禁用 DNS 缓存
                  </label>
                  <label className="inline-flex items-center gap-2 text-[var(--text-secondary)]">
                    <input type="checkbox" checked={ruleEnabled} onChange={e => setRuleEnabled(e.target.checked)} />
                    启用规则
                  </label>
                </div>
                <div className="mt-4 flex justify-end gap-2">
                  <button onClick={resetRuleForm} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-4 py-2 text-sm text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">取消</button>
                  <button onClick={saveRule} className="inline-flex h-9 items-center gap-2 rounded-md bg-[var(--color-primary)] px-4 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)]">{editingRule.id ? '更新' : '添加'}</button>
                </div>
                </div>
              </div>
            )}
          </section>

          {previewJSON && (
            <div className="aw-modal-backdrop" onClick={() => setPreviewJSON('')}>
              <div className="aw-modal-panel w-full max-w-xl p-5" onClick={e => e.stopPropagation()}>
                <div className="mb-4 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-[var(--text-primary)]">DNS 绑定预览</h3>
                  <button onClick={() => setPreviewJSON('')} className="aw-modal-close">✕</button>
                </div>
                <pre className="max-h-[420px] overflow-auto rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 text-xs text-[var(--text-primary)]">
                  {previewJSON}
                </pre>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

export default DNSPage;
