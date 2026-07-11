import { useEffect, useMemo, useState } from 'react';
import { CheckCircle2, ChevronDown, Play, Save, XCircle } from 'lucide-react';

import { PageHeader } from '@/components/layout/PageHeader';
import { JsonPreview } from '@/components/JsonPreview';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Toast } from '@/components/ui/Toast';
import { api } from '@/services/api';
import type { ConfigGenerateRequest } from '@/services/types';

const defaultRequest: ConfigGenerateRequest = {
  default_outbound: 'proxy',
  inbound_listen: '127.0.0.1',
  inbound_port: 7890,
  log_level: 'info',
};

export function ConfigPage() {
  const [request, setRequest] = useState<ConfigGenerateRequest>(defaultRequest);
  const [generated, setGenerated] = useState<any>(null);
  const [generating, setGenerating] = useState(false);
  const [applying, setApplying] = useState(false);
  const [confirmApply, setConfirmApply] = useState(false);
  const [showFullPreview, setShowFullPreview] = useState(false);
  const [expandedModules, setExpandedModules] = useState<Record<string, boolean>>({});
  const [message, setMessage] = useState('');
  const [messageType, setMessageType] = useState<'success' | 'error'>('error');

  const config = generated?.config;
  const moduleItems = useMemo(() => {
    const route = config?.route || {};
    return [
      { key: 'log', name: '日志', count: config?.log ? 1 : 0, detail: config?.log?.level ? `level=${config.log.level}` : '未生成', data: config?.log, accent: 'border-blue-400/50 bg-blue-500/5', badge: 'bg-blue-500/10 text-blue-300' },
      { key: 'inbounds', name: '入站', count: config?.inbounds?.length || 0, detail: '来自运行模式与本地监听设置', data: config?.inbounds, accent: 'border-cyan-400/50 bg-cyan-500/5', badge: 'bg-cyan-500/10 text-cyan-300' },
      { key: 'outbounds', name: '出站/策略组', count: config?.outbounds?.length || 0, detail: '来自节点、节点组、策略组', data: config?.outbounds, accent: 'border-emerald-400/50 bg-emerald-500/5', badge: 'bg-emerald-500/10 text-emerald-300' },
      { key: 'endpoints', name: '端点', count: config?.endpoints?.length || 0, detail: 'WireGuard 等 endpoint 类型节点', data: config?.endpoints, accent: 'border-teal-400/50 bg-teal-500/5', badge: 'bg-teal-500/10 text-teal-300' },
      { key: 'route.rules', name: '路由规则', count: route?.rules?.length || 0, detail: '来自规则管理和策略组绑定', data: route?.rules, accent: 'border-amber-400/50 bg-amber-500/5', badge: 'bg-amber-500/10 text-amber-300' },
      { key: 'route.rule_set', name: '规则集', count: route?.rule_set?.length || 0, detail: '来自 Geo/规则订阅自动生成', data: route?.rule_set, accent: 'border-orange-400/50 bg-orange-500/5', badge: 'bg-orange-500/10 text-orange-300' },
      { key: 'dns', name: 'DNS', count: config?.dns ? (config.dns.servers?.length || 0) : 0, detail: config?.dns ? `${config.dns.rules?.length || 0} 条 DNS 规则` : '未启用', data: config?.dns, accent: 'border-violet-400/50 bg-violet-500/5', badge: 'bg-violet-500/10 text-violet-300' },
      { key: 'ntp', name: 'NTP', count: config?.ntp ? 1 : 0, detail: config?.ntp ? config.ntp.server : '未启用', data: config?.ntp, accent: 'border-pink-400/50 bg-pink-500/5', badge: 'bg-pink-500/10 text-pink-300' },
      { key: 'experimental', name: '实验功能', count: config?.experimental ? Object.keys(config.experimental).length : 0, detail: 'Clash API / Cache File', data: config?.experimental, accent: 'border-slate-400/50 bg-slate-500/5', badge: 'bg-slate-500/10 text-slate-300' },
    ];
  }, [config]);

  const showMessage = (msg: string, type: 'success' | 'error' = 'error') => {
    setMessage(msg);
    setMessageType(type);
  };

  useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(() => setMessage(''), messageType === 'error' ? 5000 : 3000);
    return () => window.clearTimeout(timer);
  }, [message, messageType]);

  const handleGenerate = async () => {
    try {
      setGenerating(true);
      const result = await api.generateConfig(request);
      setGenerated(result);
      showMessage(result.valid ? '完整配置已生成并校验通过' : `配置校验失败: ${result.error}`, result.valid ? 'success' : 'error');
    } catch (e: any) {
      showMessage(`生成失败: ${e.message}`, 'error');
    } finally {
      setGenerating(false);
    }
  };

  const handleApply = async () => {
    if (!generated?.valid) {
      showMessage('请先生成并校验通过配置', 'error');
      return;
    }
    try {
      setApplying(true);
      await api.applyConfig({ restart_core: true });
      setConfirmApply(false);
      showMessage('配置已应用', 'success');
    } catch (e: any) {
      showMessage(`应用失败: ${e.message}`, 'error');
    } finally {
      setApplying(false);
    }
  };

  const toggleModuleRow = (index: number) => {
    const rowStart = Math.floor(index / 2) * 2;
    const rowItems = moduleItems.slice(rowStart, rowStart + 2);
    const nextExpanded = !rowItems.every(item => expandedModules[item.key]);
    setExpandedModules(prev => {
      const next = { ...prev };
      rowItems.forEach(item => {
        next[item.key] = nextExpanded;
      });
      return next;
    });
  };

  const copyFullConfig = async () => {
    if (!config) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(config, null, 2));
      showMessage('完整配置已复制', 'success');
    } catch (e: any) {
      showMessage(`复制失败: ${e.message || '浏览器不支持剪贴板'}`, 'error');
    }
  };

  const copyModuleConfig = async (key: string, data: any) => {
    try {
      await navigator.clipboard.writeText(JSON.stringify({ [key]: data ?? null }, null, 2));
      showMessage(`${key} 已复制`, 'success');
    } catch (e: any) {
      showMessage(`复制失败: ${e.message || '浏览器不支持剪贴板'}`, 'error');
    }
  };

  useEffect(() => {
    handleGenerate();
  }, []);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeader title="配置生成" description="从当前订阅、节点、策略组、路由规则、DNS、Geo 和运行设置生成完整 sing-box 配置" />
        <div className="flex flex-wrap gap-2">
          <button onClick={handleGenerate} disabled={generating} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-sm font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)] disabled:opacity-50">
            <Play size={15} />{generating ? '生成中...' : '生成完整配置'}
          </button>
          <button onClick={() => setConfirmApply(true)} disabled={!generated?.valid || applying} className="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-3 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50">
            <Save size={15} />{applying ? '应用中...' : '应用当前生成结果'}
          </button>
        </div>
      </div>
      <Toast message={message} type={messageType} />

      <div className="space-y-4">
        <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">生成参数</h3>
            <div className="mt-4 grid gap-3">
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">默认出站</span>
                <select value={request.default_outbound} onChange={e => setRequest(prev => ({ ...prev, default_outbound: e.target.value }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                  <option value="proxy">proxy（策略）</option>
                  <option value="direct">direct（直连）</option>
                </select>
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">Mixed 监听地址</span>
                <input value={request.inbound_listen} onChange={e => setRequest(prev => ({ ...prev, inbound_listen: e.target.value }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">Mixed 监听端口</span>
                <input type="number" value={request.inbound_port} onChange={e => setRequest(prev => ({ ...prev, inbound_port: parseInt(e.target.value) || 7890 }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400" />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">日志级别</span>
                <select value={request.log_level} onChange={e => setRequest(prev => ({ ...prev, log_level: e.target.value }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                  {['trace', 'debug', 'info', 'warn', 'error'].map(level => <option key={level} value={level}>{level}</option>)}
                </select>
              </label>
            </div>
        </section>

        <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border-default)] px-5 py-4">
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">模块化配置预览</h3>
              <p className="mt-1 text-xs text-[var(--text-tertiary)]">每块对应最终 sing-box JSON 的一个顶层字段；生成结果仍会先写入临时文件并执行 sing-box check。</p>
            </div>
            {generated && (
              <div className="flex items-center gap-2">
                <div className={`inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs ${generated.valid ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>
                  {generated.valid ? <CheckCircle2 size={14} /> : <XCircle size={14} />}
                  {generated.valid ? '校验通过' : '校验失败'}
                </div>
                {generated.valid && (
                  <button onClick={() => setShowFullPreview(true)} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">
                    预览最终配置文件
                  </button>
                )}
              </div>
            )}
          </div>
          <div className="p-5">
            {generated?.error && <div className="mb-4 rounded-lg border border-red-400/20 bg-red-500/10 p-3 text-sm text-red-300">{generated.error}</div>}
            {config ? (
              <div className="grid gap-4 xl:grid-cols-2">
                {moduleItems.map((item, index) => (
                  <div key={item.key} className={`overflow-hidden rounded-[var(--radius-xl)] border ${item.accent} p-4`}>
                    <button onClick={() => toggleModuleRow(index)} className="flex w-full items-start justify-between gap-3 text-left">
                      <div>
                        <div className="text-sm font-semibold text-[var(--text-primary)]">{item.name}</div>
                        <div className="mt-1 text-xs text-[var(--text-tertiary)]">{item.detail}</div>
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        {expandedModules[item.key] && (
                          <span onClick={e => { e.stopPropagation(); copyModuleConfig(item.key, item.data); }} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-2.5 py-1 text-xs font-medium text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">
                            复制
                          </span>
                        )}
                        <span className={`rounded-full px-2 py-1 text-xs ${item.badge}`}>{item.count}</span>
                        <ChevronDown size={16} className={`text-[var(--text-tertiary)] transition-transform ${expandedModules[item.key] ? 'rotate-180' : ''}`} />
                      </div>
                    </button>
                    {expandedModules[item.key] && (
                      <div className="mt-3">
                        <JsonPreview data={{ [item.key]: item.data ?? null }} maxHeight="320px" className="bg-[var(--bg-base)]/80" />
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ) : <div className="flex h-64 items-center justify-center text-sm text-[var(--text-secondary)]">{generating ? '生成中...' : '暂无配置预览'}</div>}
          </div>
        </section>
      </div>

      <ConfirmDialog
        open={confirmApply}
        title="应用当前生成结果"
        message="将把已校验通过的临时配置覆盖为正式配置。应用前会备份当前配置。"
        confirmText="应用配置"
        onConfirm={handleApply}
        onCancel={() => setConfirmApply(false)}
      />

      {showFullPreview && config && (
        <div className="aw-modal-backdrop" onClick={() => setShowFullPreview(false)}>
          <div className="aw-modal-panel max-h-[90vh] w-full max-w-6xl overflow-hidden p-5" onClick={e => e.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-[var(--text-primary)]">最终配置文件</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">这是将写入 sing-box 的完整 JSON 配置。</p>
              </div>
              <div className="flex items-center gap-2">
                <button onClick={copyFullConfig} className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">
                  复制
                </button>
                <button onClick={() => setShowFullPreview(false)} className="aw-modal-close">✕</button>
              </div>
            </div>
            <JsonPreview data={config} maxHeight="72vh" />
          </div>
        </div>
      )}
    </div>
  );
}

export default ConfigPage;
