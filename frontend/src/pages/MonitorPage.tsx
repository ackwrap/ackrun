import React from 'react';
import { PageHeader } from '@/components/layout/PageHeader';
import { Toast } from '@/components/ui/Toast';
import { getClashClient } from '@/services/clash';
import type { Connection, ProxyGroup, ProxyNode, Rule, TrafficData } from '@/services/clash';
import type { TrafficChartRef } from '@/components/monitor/TrafficChart';
import { ConnectionsPanel } from './monitor/ConnectionsPanel';
import { MonitorTabs } from './monitor/MonitorTabs';
import { OverviewPanel } from './monitor/OverviewPanel';
import { ProxyGroupsPanel } from './monitor/ProxyGroupsPanel';
import { RulesPanel } from './monitor/RulesPanel';
import type { MonitorTab } from './monitor/monitorUtils';

export function MonitorPage() {
  const [activeTab, setActiveTab] = React.useState<MonitorTab>('overview');
  const [totalUp, setTotalUp] = React.useState(0);
  const [totalDown, setTotalDown] = React.useState(0);
  const [speedUp, setSpeedUp] = React.useState(0);
  const [speedDown, setSpeedDown] = React.useState(0);
  const [connected, setConnected] = React.useState(false);
  const [clashUnavailableReason, setClashUnavailableReason] = React.useState('');
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error' | 'info'>('info');

  const [proxies, setProxies] = React.useState<Record<string, ProxyGroup | ProxyNode>>({});
  const [loadingProxies, setLoadingProxies] = React.useState(false);
  const [selectedGroup, setSelectedGroup] = React.useState<string | null>(null);

  const [connections, setConnections] = React.useState<Connection[]>([]);
  const [loadingConnections, setLoadingConnections] = React.useState(false);
  const [connectionSearch, setConnectionSearch] = React.useState('');

  const [rules, setRules] = React.useState<Rule[]>([]);
  const [loadingRules, setLoadingRules] = React.useState(false);
  const [ruleSearch, setRuleSearch] = React.useState('');

  const [uploadSpeedHistory, setUploadSpeedHistory] = React.useState<number[]>([]);
  const [downloadSpeedHistory, setDownloadSpeedHistory] = React.useState<number[]>([]);
  const [connectionCountHistory, setConnectionCountHistory] = React.useState<number[]>([]);

  const chartRef = React.useRef<TrafficChartRef>(null);

  const showMessage = React.useCallback((text: string, type: 'success' | 'error' | 'info' = 'info') => {
    setMessage(text);
    setMessageType(type);
  }, []);

  const markClashUnavailable = React.useCallback((error?: unknown) => {
    const message = error instanceof Error ? error.message : '';
    setConnected(false);
    setClashUnavailableReason(message || 'Clash API 未连接，请确认核心已启动并应用了启用 Clash API 的配置');
  }, []);

  const markClashAvailable = React.useCallback(() => {
    setConnected(true);
    setClashUnavailableReason('');
  }, []);

  const isClashUnavailableError = React.useCallback((error: unknown) => {
    const message = error instanceof Error ? error.message : String(error || '');
    return message.includes('Failed to connect to Clash API') || message.includes('connectex') || message.includes('connection refused');
  }, []);

  const ensureClashAvailable = React.useCallback(async () => {
    try {
      await getClashClient().getConfig();
      markClashAvailable();
      return true;
    } catch (error) {
      markClashUnavailable(error);
      return false;
    }
  }, [markClashAvailable, markClashUnavailable]);

  React.useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(() => setMessage(''), messageType === 'error' ? 5000 : 3000);
    return () => window.clearTimeout(timer);
  }, [message, messageType]);

  React.useEffect(() => {
    const clashClient = getClashClient();

    clashClient.connectTraffic((data: TrafficData) => {
      markClashAvailable();

      // Clash traffic events already contain the current one-second throughput.
      setSpeedUp(data.up);
      setSpeedDown(data.down);
      setTotalUp(prev => prev + data.up);
      setTotalDown(prev => prev + data.down);
      setUploadSpeedHistory(prev => [...prev, data.up].slice(-60));
      setDownloadSpeedHistory(prev => [...prev, data.down].slice(-60));
      chartRef.current?.addData(data.up, data.down);
    });

    ensureClashAvailable();

    return () => {
      clashClient.disconnectTraffic();
    };
  }, [ensureClashAvailable, markClashAvailable]);

  const loadProxies = React.useCallback(async () => {
    if (!connected && !(await ensureClashAvailable())) return;
    setLoadingProxies(true);
    try {
      const result = await getClashClient().getProxies();
      setProxies(result.proxies);
    } catch (e: any) {
      if (isClashUnavailableError(e)) {
        markClashUnavailable(e);
        return;
      }
      showMessage(`加载策略组失败: ${e.message}`, 'error');
    } finally {
      setLoadingProxies(false);
    }
  }, [connected, ensureClashAvailable, isClashUnavailableError, markClashUnavailable, showMessage]);

  React.useEffect(() => {
    if (activeTab === 'proxies' && Object.keys(proxies).length === 0) {
      loadProxies();
    }
  }, [activeTab, proxies, loadProxies]);

  const selectProxy = async (group: string, proxy: string) => {
    try {
      await getClashClient().selectProxy(group, proxy);
      await loadProxies();
    } catch (e: any) {
      showMessage(`切换节点失败: ${e.message}`, 'error');
    }
  };

  const testDelay = async (proxyName: string) => {
    try {
      await getClashClient().delayTest(proxyName);
      await loadProxies();
    } catch (e: any) {
      showMessage(`测速失败: ${e.message}`, 'error');
    }
  };

  const loadConnections = React.useCallback(async () => {
    if (!connected && !(await ensureClashAvailable())) return;
    setLoadingConnections(true);
    try {
      const result = await getClashClient().getConnections();
      setConnections(result.connections);
      setConnectionCountHistory(prev => [...prev, result.connections.length].slice(-60));
    } catch (e: any) {
      if (isClashUnavailableError(e)) {
        markClashUnavailable(e);
        return;
      }
      showMessage(`加载连接失败: ${e.message}`, 'error');
    } finally {
      setLoadingConnections(false);
    }
  }, [connected, ensureClashAvailable, isClashUnavailableError, markClashUnavailable, showMessage]);

  React.useEffect(() => {
    if (activeTab !== 'overview' && activeTab !== 'connections') return;
    loadConnections();
    if (!connected) return;
    const interval = window.setInterval(loadConnections, activeTab === 'connections' ? 1000 : 3000);
    return () => window.clearInterval(interval);
  }, [activeTab, connected, loadConnections]);

  const closeConnection = async (id: string) => {
    try {
      await getClashClient().closeConnection(id);
      await loadConnections();
    } catch (e: any) {
      showMessage(`关闭连接失败: ${e.message}`, 'error');
    }
  };

  const closeAllConnections = async () => {
    if (!confirm('确定关闭所有连接？')) return;
    try {
      await getClashClient().closeAllConnections();
      await loadConnections();
    } catch (e: any) {
      showMessage(`关闭所有连接失败: ${e.message}`, 'error');
    }
  };

  const loadRules = React.useCallback(async () => {
    if (!connected && !(await ensureClashAvailable())) return;
    setLoadingRules(true);
    try {
      const result = await getClashClient().getRules();
      setRules(result.rules);
    } catch (e: any) {
      if (isClashUnavailableError(e)) {
        markClashUnavailable(e);
        setRules([]);
        return;
      }
      showMessage(`加载规则失败: ${e.message}`, 'error');
    } finally {
      setLoadingRules(false);
    }
  }, [connected, ensureClashAvailable, isClashUnavailableError, markClashUnavailable, showMessage]);

  React.useEffect(() => {
    if (activeTab === 'rules' && rules.length === 0) {
      loadRules();
    }
  }, [activeTab, rules, loadRules]);

  const filteredConnections = React.useMemo(() => {
    if (!connectionSearch) return connections;
    const search = connectionSearch.toLowerCase();
    return connections.filter(conn =>
      conn.metadata.host?.toLowerCase().includes(search) ||
      conn.metadata.destinationIP?.toLowerCase().includes(search) ||
      conn.chains?.join(',').toLowerCase().includes(search)
    );
  }, [connections, connectionSearch]);

  const filteredRules = React.useMemo(() => {
    if (!ruleSearch) return rules;
    const search = ruleSearch.toLowerCase();
    return rules.filter(rule =>
      rule.type?.toLowerCase().includes(search) ||
      rule.payload?.toLowerCase().includes(search) ||
      rule.proxy?.toLowerCase().includes(search)
    );
  }, [rules, ruleSearch]);

  const proxyGroups = React.useMemo(() => {
    return Object.entries(proxies)
      .filter(([, proxy]) => 'type' in proxy && (proxy.type === 'Selector' || proxy.type === 'URLTest'))
      .map(([groupName, proxy]) => ({ ...(proxy as ProxyGroup), name: groupName } as ProxyGroup));
  }, [proxies]);

  return (
    <div className="flex h-full flex-col space-y-2">
      <Toast message={message} type={messageType} />
      <PageHeader title="仪表盘" className="!mb-0" />

      <MonitorTabs activeTab={activeTab} onChange={setActiveTab} />

      <div className="flex-1 overflow-auto">
        {activeTab === 'overview' && (
          <OverviewPanel
            connected={connected}
            unavailableReason={clashUnavailableReason}
            totalUp={totalUp}
            totalDown={totalDown}
            speedUp={speedUp}
            speedDown={speedDown}
            connectionCount={connections.length}
            uploadSpeedHistory={uploadSpeedHistory}
            downloadSpeedHistory={downloadSpeedHistory}
            connectionCountHistory={connectionCountHistory}
            chartRef={chartRef}
          />
        )}

        {activeTab === 'proxies' && (
          <ProxyGroupsPanel
            proxies={proxies}
            proxyGroups={proxyGroups}
            selectedGroup={selectedGroup}
            loading={loadingProxies}
            onRefresh={loadProxies}
            onSelectGroup={setSelectedGroup}
            onSelectProxy={selectProxy}
            onTestDelay={testDelay}
          />
        )}

        {activeTab === 'connections' && (
          <ConnectionsPanel
            connections={filteredConnections}
            search={connectionSearch}
            loading={loadingConnections}
            onSearchChange={setConnectionSearch}
            onRefresh={loadConnections}
            onCloseConnection={closeConnection}
            onCloseAll={closeAllConnections}
          />
        )}

        {activeTab === 'rules' && (
          <RulesPanel rules={filteredRules} search={ruleSearch} loading={loadingRules} unavailableReason={connected ? '' : clashUnavailableReason} onSearchChange={setRuleSearch} onRefresh={loadRules} />
        )}
      </div>
    </div>
  );
}

export default MonitorPage;
