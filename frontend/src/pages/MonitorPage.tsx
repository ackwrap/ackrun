import React from 'react';
import { PageHeader } from '@/components/layout/PageHeader';
import { Activity, Network, List, Shield, RefreshCw, X } from 'lucide-react';
import { getClashClient } from '@/services/clash';
import type { TrafficData, ProxyGroup, ProxyNode, Connection, Rule } from '@/services/clash';
import { TrafficChart, TrafficChartRef } from '@/components/monitor/TrafficChart';
import { MiniSparkline } from '@/components/monitor/MiniSparkline';

type TabType = 'overview' | 'proxies' | 'connections' | 'rules';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

function formatSpeed(bytesPerSecond: number): string {
  return formatBytes(bytesPerSecond) + '/s';
}

function proxyGroupIcon(group: ProxyGroup): string {
  if (group.name === '全球直连') return '🎯';
  if (group.name === '应用净化') return '🍃';
  return group.type === 'Selector' ? '👆' : '🚀';
}

export function MonitorPage() {
  const [activeTab, setActiveTab] = React.useState<TabType>('overview');
  const [totalUp, setTotalUp] = React.useState(0);
  const [totalDown, setTotalDown] = React.useState(0);
  const [speedUp, setSpeedUp] = React.useState(0);
  const [speedDown, setSpeedDown] = React.useState(0);
  const [connected, setConnected] = React.useState(false);
  const [error, setError] = React.useState('');
  
  // 策略组相关状态
  const [proxies, setProxies] = React.useState<Record<string, ProxyGroup | ProxyNode>>({});
  const [loadingProxies, setLoadingProxies] = React.useState(false);
  const [selectedGroup, setSelectedGroup] = React.useState<string | null>(null);
  
  // 连接管理相关状态
  const [connections, setConnections] = React.useState<Connection[]>([]);
  const [loadingConnections, setLoadingConnections] = React.useState(false);
  const [connectionSearch, setConnectionSearch] = React.useState('');
  
  // 规则相关状态
  const [rules, setRules] = React.useState<Rule[]>([]);
  const [loadingRules, setLoadingRules] = React.useState(false);
  const [ruleSearch, setRuleSearch] = React.useState('');
  
  // 迷你图历史数据（保留最近 60 个数据点）
  const [uploadSpeedHistory, setUploadSpeedHistory] = React.useState<number[]>([]);
  const [downloadSpeedHistory, setDownloadSpeedHistory] = React.useState<number[]>([]);
  const [connectionCountHistory, setConnectionCountHistory] = React.useState<number[]>([]);
  
  const lastTrafficRef = React.useRef<TrafficData | null>(null);
  const lastTimeRef = React.useRef<number>(Date.now());
  const chartRef = React.useRef<TrafficChartRef>(null);

  React.useEffect(() => {
    const clashClient = getClashClient();
    
    clashClient.connectTraffic((data: TrafficData) => {
      setConnected(true);
      setError('');
      
      const now = Date.now();
      const timeDiff = (now - lastTimeRef.current) / 1000; // 秒
      
      if (lastTrafficRef.current && timeDiff > 0) {
        const upDiff = data.up - lastTrafficRef.current.up;
        const downDiff = data.down - lastTrafficRef.current.down;
        
        const upSpeed = upDiff / timeDiff;
        const downSpeed = downDiff / timeDiff;
        
        setSpeedUp(upSpeed);
        setSpeedDown(downSpeed);
        
        // 更新迷你图历史数据
        setUploadSpeedHistory(prev => {
          const newData = [...prev, upSpeed];
          return newData.slice(-60); // 保留最近 60 个数据点
        });
        setDownloadSpeedHistory(prev => {
          const newData = [...prev, downSpeed];
          return newData.slice(-60);
        });
        
        // 更新图表数据
        if (chartRef.current) {
          chartRef.current.addData(upSpeed, downSpeed);
        }
      }
      
      setTotalUp(data.up);
      setTotalDown(data.down);
      
      lastTrafficRef.current = data;
      lastTimeRef.current = now;
    });

    // 测试连接
    clashClient.getConfig().catch(() => {
      setError('无法连接到 Clash API，请确保 sing-box 已启动并配置了 Clash API');
      setConnected(false);
    });

    return () => {
      clashClient.disconnectTraffic();
    };
  }, []);

  // 加载策略组
  const loadProxies = React.useCallback(async () => {
    setLoadingProxies(true);
    try {
      const clashClient = getClashClient();
      const result = await clashClient.getProxies();
      setProxies(result.proxies);
      setError('');
    } catch (e: any) {
      setError(`加载策略组失败: ${e.message}`);
    } finally {
      setLoadingProxies(false);
    }
  }, []);

  React.useEffect(() => {
    if (activeTab === 'proxies' && Object.keys(proxies).length === 0) {
      loadProxies();
    }
  }, [activeTab, proxies, loadProxies]);

  // 切换节点
  const selectProxy = async (group: string, proxy: string) => {
    try {
      const clashClient = getClashClient();
      await clashClient.selectProxy(group, proxy);
      await loadProxies();
    } catch (e: any) {
      setError(`切换节点失败: ${e.message}`);
    }
  };

  // 测速
  const testDelay = async (proxyName: string) => {
    try {
      const clashClient = getClashClient();
      await clashClient.delayTest(proxyName);
      await loadProxies();
    } catch (e: any) {
      setError(`测速失败: ${e.message}`);
    }
  };

  // 加载连接
  const loadConnections = React.useCallback(async () => {
    setLoadingConnections(true);
    try {
      const clashClient = getClashClient();
      const result = await clashClient.getConnections();
      setConnections(result.connections);
      
      // 更新连接数历史
      setConnectionCountHistory(prev => {
        const newData = [...prev, result.connections.length];
        return newData.slice(-60);
      });
      
      setError('');
    } catch (e: any) {
      setError(`加载连接失败: ${e.message}`);
    } finally {
      setLoadingConnections(false);
    }
  }, []);

  React.useEffect(() => {
    if (activeTab === 'connections') {
      loadConnections();
      const interval = setInterval(loadConnections, 1000); // 每秒刷新
      return () => clearInterval(interval);
    }
  }, [activeTab, loadConnections]);

  // 关闭连接
  const closeConnection = async (id: string) => {
    try {
      const clashClient = getClashClient();
      await clashClient.closeConnection(id);
      await loadConnections();
    } catch (e: any) {
      setError(`关闭连接失败: ${e.message}`);
    }
  };

  // 关闭所有连接
  const closeAllConnections = async () => {
    if (!confirm('确定关闭所有连接？')) return;
    try {
      const clashClient = getClashClient();
      await clashClient.closeAllConnections();
      await loadConnections();
    } catch (e: any) {
      setError(`关闭所有连接失败: ${e.message}`);
    }
  };

  // 筛选连接
  const filteredConnections = React.useMemo(() => {
    if (!connectionSearch) return connections;
    const search = connectionSearch.toLowerCase();
    return connections.filter(conn => 
      conn.metadata.host?.toLowerCase().includes(search) ||
      conn.metadata.destinationIP?.toLowerCase().includes(search) ||
      conn.chains?.join(',').toLowerCase().includes(search)
    );
  }, [connections, connectionSearch]);

  // 加载规则
  const loadRules = React.useCallback(async () => {
    setLoadingRules(true);
    try {
      const clashClient = getClashClient();
      const result = await clashClient.getRules();
      setRules(result.rules);
      setError('');
    } catch (e: any) {
      setError(`加载规则失败: ${e.message}`);
    } finally {
      setLoadingRules(false);
    }
  }, []);

  React.useEffect(() => {
    if (activeTab === 'rules' && rules.length === 0) {
      loadRules();
    }
  }, [activeTab, rules, loadRules]);

  // 筛选规则
  const filteredRules = React.useMemo(() => {
    if (!ruleSearch) return rules;
    const search = ruleSearch.toLowerCase();
    return rules.filter(rule => 
      rule.type?.toLowerCase().includes(search) ||
      rule.payload?.toLowerCase().includes(search) ||
      rule.proxy?.toLowerCase().includes(search)
    );
  }, [rules, ruleSearch]);

  // 筛选出策略组（Selector 和 URLTest）
  const proxyGroups = React.useMemo(() => {
    return Object.entries(proxies)
      .filter(([_, proxy]) =>
        'type' in proxy && (proxy.type === 'Selector' || proxy.type === 'URLTest')
      )
      .map(([groupName, proxy]) => ({
        ...(proxy as ProxyGroup),
        name: groupName
      } as ProxyGroup));
  }, [proxies]);

  const tabs = [
    { key: 'overview' as TabType, label: '概览', icon: <Activity size={16} /> },
    { key: 'proxies' as TabType, label: '策略组', icon: <Network size={16} /> },
    { key: 'connections' as TabType, label: '连接', icon: <List size={16} /> },
    { key: 'rules' as TabType, label: '规则', icon: <Shield size={16} /> },
  ];

  return (
    <div className="flex h-full flex-col space-y-4">
      <PageHeader title="实时监控" />

      {/* 连接状态提示 */}
      {error && (
        <div className="rounded-md border border-red-400/20 bg-red-500/10 px-3 py-2 text-xs text-red-300">
          {error}
        </div>
      )}

      {/* Tab 导航 */}
      <div className="flex gap-2 overflow-x-auto rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-2 shadow-[var(--shadow-card)]">
        {tabs.map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? 'bg-blue-500/20 text-blue-100 shadow-[0_0_12px_rgba(59,130,246,0.3)]'
                : 'text-[var(--text-secondary)] hover:bg-white/[0.04] hover:text-white'
            }`}
          >
            {tab.icon}
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab 内容 */}
      <div className="flex-1 overflow-auto">
        {activeTab === 'overview' && (
          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-3">
              {/* 上传流量 */}
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-5 shadow-[var(--shadow-card)]">
                <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">上传</div>
                <div className="flex items-baseline gap-1.5">
                  <span className="text-3xl font-extralight tabular-nums text-white">{formatSpeed(speedUp)}</span>
                </div>
                <div className="mt-3 h-14">
                  <MiniSparkline data={uploadSpeedHistory} color="green" />
                </div>
                <div className="mt-2 text-xs text-[var(--text-tertiary)]">总计 {formatBytes(totalUp)}</div>
              </div>

              {/* 下载流量 */}
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-5 shadow-[var(--shadow-card)]">
                <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">下载</div>
                <div className="flex items-baseline gap-1.5">
                  <span className="text-3xl font-extralight tabular-nums text-white">{formatSpeed(speedDown)}</span>
                </div>
                <div className="mt-3 h-14">
                  <MiniSparkline data={downloadSpeedHistory} color="blue" />
                </div>
                <div className="mt-2 text-xs text-[var(--text-tertiary)]">总计 {formatBytes(totalDown)}</div>
              </div>

              {/* 连接数 */}
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-5 shadow-[var(--shadow-card)]">
                <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                  连接数
                  <span className={`inline-block h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-400' : 'bg-red-400'}`} />
                </div>
                <div className="text-3xl font-extralight tabular-nums text-white">
                  {connections.length}
                </div>
                <div className="mt-3 h-14">
                  <MiniSparkline data={connectionCountHistory} color="purple" />
                </div>
                <div className="mt-2 text-xs text-[var(--text-tertiary)]">{connected ? '已连接' : '未连接'}</div>
              </div>
            </div>

            {/* 提示信息 */}
            {!connected && (
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-5 shadow-[var(--shadow-card)]">
                <div className="text-center">
                  <div className="text-lg font-semibold text-white">实时监控功能</div>
                  <div className="mt-2 text-sm text-[var(--text-secondary)]">
                    此功能需要 sing-box 启用 Clash API 并运行中
                  </div>
                  <div className="mt-4 text-xs text-[var(--text-tertiary)]">
                    请在 sing-box 配置中添加：
                  </div>
                  <div className="mt-2 rounded-lg border border-[var(--border-default)] bg-black/20 p-3 text-left">
                    <pre className="text-xs text-blue-300">{`{
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:9090",
      "secret": ""
    }
  }
}`}</pre>
                  </div>
                </div>
              </div>
            )}

            {/* 流量图表 */}
            {connected && (
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-5 shadow-[var(--shadow-card)]">
                <div className="mb-4 text-sm font-medium text-white">实时流量</div>
                <TrafficChart ref={chartRef} />
              </div>
            )}
          </div>
        )}

        {activeTab === 'proxies' && (
          <div className="space-y-4">
            <div className="flex items-center justify-between rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-4 shadow-[var(--shadow-card)]">
              <div>
                <h3 className="text-sm font-semibold text-white">策略组列表</h3>
                <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">共 {proxyGroups.length} 个策略组</p>
              </div>
              <button
                onClick={loadProxies}
                disabled={loadingProxies}
                className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-white hover:bg-white/[0.08] disabled:opacity-50"
              >
                <RefreshCw size={14} className={loadingProxies ? 'animate-spin' : ''} />
                刷新
              </button>
            </div>

            {loadingProxies ? (
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-12 text-center shadow-[var(--shadow-card)]">
                <div className="text-sm text-[var(--text-secondary)]">加载中...</div>
              </div>
            ) : proxyGroups.length === 0 ? (
              <div className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-12 text-center shadow-[var(--shadow-card)]">
                <div className="text-sm text-[var(--text-secondary)]">暂无策略组</div>
              </div>
            ) : (
              <div className="space-y-3">
                {proxyGroups.map((group) => (
                  <div
                    key={group.name}
                    className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] shadow-[var(--shadow-card)]"
                  >
                    <div
                      className="cursor-pointer p-4"
                      onClick={() => setSelectedGroup(selectedGroup === group.name ? null : group.name)}
                    >
                      <div className="flex items-center justify-between">
	                        <div className="flex items-center gap-3">
	                          <div className="text-lg">
	                            {proxyGroupIcon(group)}
	                          </div>
                          <div>
                            <div className="font-semibold text-white">{group.name}</div>
                            <div className="mt-0.5 text-xs text-[var(--text-tertiary)]">
                              {group.type === 'Selector' ? '手动选择' : '自动测速'}
                              {' · '}
                              {group.all?.length || 0} 个节点
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <div className="text-sm text-white">{group.now || '无'}</div>
                          {group.history?.[0]?.delay && (
                            <div className="mt-0.5 text-xs text-emerald-400">{group.history[0].delay}ms</div>
                          )}
                        </div>
                      </div>
                    </div>

                    {selectedGroup === group.name && group.all && (
                      <div className="border-t border-[var(--border-default)] p-4">
                        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                          {group.all.map((proxyName) => {
                            const proxy = proxies[proxyName] as ProxyNode | undefined;
                            const isCurrent = group.now === proxyName;
                            const delay = proxy?.history?.[0]?.delay;

                            return (
                              <button
                                key={proxyName}
                                onClick={() => group.type === 'Selector' && selectProxy(group.name, proxyName)}
                                onContextMenu={(e) => {
                                  e.preventDefault();
                                  testDelay(proxyName);
                                }}
                                disabled={group.type !== 'Selector'}
                                className={`flex items-center justify-between rounded-lg border p-3 text-left text-sm transition-colors ${
                                  isCurrent
                                    ? 'border-blue-400/50 bg-blue-500/20 text-blue-100'
                                    : 'border-[var(--border-default)] bg-white/[0.02] text-[var(--text-secondary)] hover:bg-white/[0.04] hover:text-white'
                                } ${group.type !== 'Selector' ? 'cursor-default' : 'cursor-pointer'}`}
                              >
                                <span className="truncate">{proxyName}</span>
                                {delay !== undefined && (
                                  <span className={`ml-2 text-xs ${delay < 100 ? 'text-emerald-400' : delay < 300 ? 'text-yellow-400' : 'text-red-400'}`}>
                                    {delay}ms
                                  </span>
                                )}
                              </button>
                            );
                          })}
                        </div>
                        <div className="mt-3 text-xs text-[var(--text-tertiary)]">
                          {group.type === 'Selector' ? '点击切换节点' : '自动测速中'}
                          {' · '}
                          右键节点进行测速
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {activeTab === 'connections' && (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-4 shadow-[var(--shadow-card)]">
              <div>
                <h3 className="text-sm font-semibold text-white">活动连接</h3>
                <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">共 {filteredConnections.length} 个连接</p>
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={connectionSearch}
                  onChange={(e) => setConnectionSearch(e.target.value)}
                  placeholder="搜索域名、IP..."
                  className="h-8 w-48 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-white outline-none focus:border-blue-400"
                />
                <button
                  onClick={loadConnections}
                  disabled={loadingConnections}
                  className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-white hover:bg-white/[0.08] disabled:opacity-50"
                >
                  <RefreshCw size={14} className={loadingConnections ? 'animate-spin' : ''} />
                  刷新
                </button>
                <button
                  onClick={closeAllConnections}
                  className="inline-flex h-8 items-center gap-2 rounded-md border border-red-400/30 bg-red-500/10 px-3 text-xs text-red-200 hover:bg-red-500/20"
                >
                  <X size={14} />
                  关闭所有
                </button>
              </div>
            </div>

            <div className="overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] shadow-[var(--shadow-card)]">
              {filteredConnections.length === 0 ? (
                <div className="p-12 text-center text-sm text-[var(--text-secondary)]">暂无连接</div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[800px] border-collapse text-left text-sm">
                    <thead className="bg-white/[0.04] text-white">
                      <tr>
                        {['目标', '来源', '策略链', '规则', '上传/下载', '操作'].map(col => (
                          <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {filteredConnections.map((conn) => (
                        <tr key={conn.id} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 hover:bg-white/[0.02]">
                          <td className="px-4 py-3">
                            <div className="font-medium text-white">{conn.metadata.host || conn.metadata.destinationIP}</div>
                            <div className="text-xs text-[var(--text-tertiary)]">{conn.metadata.destinationPort}</div>
                          </td>
                          <td className="px-4 py-3">
                            <div className="text-xs">{conn.metadata.sourceIP}</div>
                            <div className="text-xs text-[var(--text-tertiary)]">{conn.metadata.sourcePort}</div>
                          </td>
                          <td className="px-4 py-3">
                            <div className="text-xs">{conn.chains?.join(' → ') || '-'}</div>
                          </td>
                          <td className="px-4 py-3">
                            <div className="text-xs">{conn.rule || '-'}</div>
                            {conn.rulePayload && <div className="text-xs text-[var(--text-tertiary)]">{conn.rulePayload}</div>}
                          </td>
                          <td className="px-4 py-3">
                            <div className="text-xs text-blue-400">↑ {formatBytes(conn.upload)}</div>
                            <div className="text-xs text-emerald-400">↓ {formatBytes(conn.download)}</div>
                          </td>
                          <td className="px-4 py-3">
                            <button
                              onClick={() => closeConnection(conn.id)}
                              className="rounded-md border border-red-400/30 bg-red-500/10 px-2 py-1 text-xs text-red-200 hover:bg-red-500/20"
                            >
                              关闭
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
        )}

        {activeTab === 'rules' && (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] p-4 shadow-[var(--shadow-card)]">
              <div>
                <h3 className="text-sm font-semibold text-white">规则列表</h3>
                <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">共 {filteredRules.length} 条规则</p>
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={ruleSearch}
                  onChange={(e) => setRuleSearch(e.target.value)}
                  placeholder="搜索规则..."
                  className="h-8 w-48 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-white outline-none focus:border-blue-400"
                />
                <button
                  onClick={loadRules}
                  disabled={loadingRules}
                  className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-white hover:bg-white/[0.08] disabled:opacity-50"
                >
                  <RefreshCw size={14} className={loadingRules ? 'animate-spin' : ''} />
                  刷新
                </button>
              </div>
            </div>

            <div className="overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(135deg,rgba(20,33,52,0.96),rgba(14,25,41,0.82))] shadow-[var(--shadow-card)]">
              {loadingRules ? (
                <div className="p-12 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
              ) : filteredRules.length === 0 ? (
                <div className="p-12 text-center text-sm text-[var(--text-secondary)]">暂无规则</div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[600px] border-collapse text-left text-sm">
                    <thead className="bg-white/[0.04] text-white">
                      <tr>
                        {['类型', '匹配值', '策略'].map(col => (
                          <th key={col} className="border-b border-[var(--border-default)] px-4 py-3 font-semibold">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {filteredRules.map((rule, index) => (
                        <tr key={index} className="border-b border-[var(--border-default)] text-[var(--text-secondary)] last:border-b-0 hover:bg-white/[0.02]">
                          <td className="px-4 py-3">
                            <span className="rounded bg-blue-500/20 px-2 py-1 text-xs text-blue-200">{rule.type}</span>
                          </td>
                          <td className="px-4 py-3">
                            <span className="break-all text-white">{rule.payload}</span>
                          </td>
                          <td className="px-4 py-3">
                            <span className="text-white">{rule.proxy}</span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default MonitorPage;
