import { useEffect, useMemo, useState } from 'react';
import { CheckCircle2, FileJson, Play, RefreshCw, Save, XCircle } from 'lucide-react';

import { PageHeader } from '@/components/layout/PageHeader';
import { JsonPreview } from '@/components/JsonPreview';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Toast } from '@/components/ui/Toast';
import { api } from '@/services/api';
import type { ConfigFileItem, ConfigGenerateRequest } from '@/services/types';

const defaultRequest: ConfigGenerateRequest = {
  default_outbound: 'proxy',
  inbound_listen: '127.0.0.1',
  inbound_port: 7890,
  log_level: 'info',
};

function formatTime(value?: number) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function ConfigPage() {
  const [request, setRequest] = useState<ConfigGenerateRequest>(defaultRequest);
  const [files, setFiles] = useState<ConfigFileItem[]>([]);
  const [generated, setGenerated] = useState<any>(null);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [generating, setGenerating] = useState(false);
  const [applying, setApplying] = useState(false);
  const [confirmApply, setConfirmApply] = useState(false);
  const [message, setMessage] = useState('');
  const [messageType, setMessageType] = useState<'success' | 'error'>('error');

  const config = generated?.config;
  const moduleItems = useMemo(() => {
    const route = config?.route || {};
    return [
      { name: '日志', count: config?.log ? 1 : 0, detail: config?.log?.level ? `level=${config.log.level}` : '未生成' },
      { name: '入站', count: config?.inbounds?.length || 0, detail: '来自运行模式与本地监听设置' },
      { name: '出站/策略组', count: config?.outbounds?.length || 0, detail: '来自节点、节点组、策略组' },
      { name: '路由规则', count: route?.rules?.length || 0, detail: '来自规则管理和策略组绑定' },
      { name: '规则集', count: route?.rule_set?.length || 0, detail: '来自 Geo/规则订阅自动生成' },
      { name: 'DNS', count: config?.dns ? (config.dns.servers?.length || 0) : 0, detail: config?.dns ? `${config.dns.rules?.length || 0} 条 DNS 规则` : '未启用' },
      { name: 'NTP', count: config?.ntp ? 1 : 0, detail: config?.ntp ? config.ntp.server : '未启用' },
      { name: '实验功能', count: config?.experimental ? Object.keys(config.experimental).length : 0, detail: 'Clash API / Cache File' },
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

  const loadFiles = async () => {
    try {
      setFiles(await api.getConfigFiles());
    } catch (e: any) {
      showMessage(`配置列表加载失败: ${e.message}`, 'error');
    } finally {
      setLoadingFiles(false);
    }
  };

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
      await loadFiles();
    } catch (e: any) {
      showMessage(`应用失败: ${e.message}`, 'error');
    } finally {
      setApplying(false);
    }
  };

  useEffect(() => {
    loadFiles();
    handleGenerate();
  }, []);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeader title="配置生成" description="从当前订阅、节点、策略组、路由规则、DNS、Geo 和运行设置生成完整 sing-box 配置" />
        <div className="flex flex-wrap gap-2">
          <button onClick={loadFiles} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-sm text-[var(--text-primary)] hover:bg-[var(--bg-sidebar-hover)]">
            <RefreshCw size={15} />刷新列表
          </button>
          <button onClick={handleGenerate} disabled={generating} className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-sm font-medium text-[var(--button-primary-text)] hover:bg-[var(--button-primary-hover)] disabled:opacity-50">
            <Play size={15} />{generating ? '生成中...' : '生成完整配置'}
          </button>
          <button onClick={() => setConfirmApply(true)} disabled={!generated?.valid || applying} className="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-3 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50">
            <Save size={15} />{applying ? '应用中...' : '应用当前生成结果'}
          </button>
        </div>
      </div>
      <Toast message={message} type={messageType} />

      <div className="grid gap-4 xl:grid-cols-[420px_1fr]">
        <div className="space-y-4">
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">配置列表</h3>
            <p className="mt-1 text-xs text-[var(--text-tertiary)]">当前配置目录中的 JSON 配置文件。</p>
            <div className="mt-4 space-y-2">
              {loadingFiles ? (
                <div className="rounded-lg border border-[var(--border-default)] p-4 text-sm text-[var(--text-secondary)]">加载中...</div>
              ) : files.length === 0 ? (
                <div className="rounded-lg border border-[var(--border-default)] p-4 text-sm text-[var(--text-secondary)]">暂无配置文件，先生成并应用配置。</div>
              ) : files.map(file => (
                <div key={file.path} className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
                        <FileJson size={15} className="shrink-0" />
                        <span className="truncate">{file.name}</span>
                      </div>
                      <div className="mt-1 text-xs text-[var(--text-tertiary)]">{formatSize(file.size_bytes)} · {formatTime(file.updated_at)}</div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {file.active && <span className="rounded-full bg-blue-500/10 px-2 py-0.5 text-xs text-blue-300">当前</span>}
                      <span className={`rounded-full px-2 py-0.5 text-xs ${file.valid ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{file.valid ? '有效' : '无效'}</span>
                    </div>
                  </div>
                  {file.error && <div className="mt-2 line-clamp-2 text-xs text-red-300">{file.error}</div>}
                </div>
              ))}
            </div>
          </section>

          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">生成参数</h3>
            <div className="mt-4 grid gap-3">
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">默认出站</span>
                <select value={request.default_outbound} onChange={e => setRequest(prev => ({ ...prev, default_outbound: e.target.value }))} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-blue-400">
                  <option value="proxy">proxy（策略）</option>
                  <option value="direct">direct（直连）</option>
                  <option value="block">block（阻断）</option>
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

          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">生成模块清单</h3>
            <div className="mt-4 space-y-2">
              {moduleItems.map(item => (
                <div key={item.name} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2">
                  <div>
                    <div className="text-sm text-[var(--text-primary)]">{item.name}</div>
                    <div className="text-xs text-[var(--text-tertiary)]">{item.detail}</div>
                  </div>
                  <span className="rounded-full bg-white/5 px-2 py-1 text-xs text-[var(--text-secondary)]">{item.count}</span>
                </div>
              ))}
            </div>
          </section>
        </div>

        <section className="min-h-[720px] rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]">
          <div className="flex items-center justify-between border-b border-[var(--border-default)] px-5 py-4">
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">完整配置预览</h3>
              <p className="mt-1 text-xs text-[var(--text-tertiary)]">生成结果会先写入临时文件并执行 sing-box check，应用后才覆盖正式配置。</p>
            </div>
            {generated && (
              <div className={`inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs ${generated.valid ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>
                {generated.valid ? <CheckCircle2 size={14} /> : <XCircle size={14} />}
                {generated.valid ? '校验通过' : '校验失败'}
              </div>
            )}
          </div>
          <div className="p-5">
            {generated?.error && <div className="mb-4 rounded-lg border border-red-400/20 bg-red-500/10 p-3 text-sm text-red-300">{generated.error}</div>}
            {config ? <JsonPreview data={config} maxHeight="none" /> : <div className="flex h-64 items-center justify-center text-sm text-[var(--text-secondary)]">{generating ? '生成中...' : '暂无配置预览'}</div>}
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
    </div>
  );
}

export default ConfigPage;
