import { useState, useEffect } from 'react';
import { Play, Save } from 'lucide-react';
import { PageHeader } from '@/components/layout/PageHeader';
import { JsonPreview } from '@/components/JsonPreview';
import { api } from '@/services/api';
import type { ConfigGenerateRequest } from '@/services/types';

export function ConfigPage() {
  const [config, setConfig] = useState<ConfigGenerateRequest>({
    default_outbound: 'proxy',
    inbound_listen: '127.0.0.1',
    inbound_port: 7890,
    log_level: 'info',
  });

  const [generated, setGenerated] = useState<any>(null);
  const [generating, setGenerating] = useState(false);
  const [applying, setApplying] = useState(false);
  const [message, setMessage] = useState('');
  const [messageType, setMessageType] = useState<'success' | 'error'>('error');

  const showMessage = (msg: string, type: 'success' | 'error' = 'error') => {
    setMessage(msg);
    setMessageType(type);
    if (type === 'success') setTimeout(() => setMessage(''), 3000);
  };

  const handleGenerate = async () => {
    try {
      setGenerating(true);
      const result = await api.generateConfig(config);
      setGenerated(result);

      if (result.valid) {
        showMessage('配置生成成功', 'success');
      } else {
        showMessage(`配置校验失败: ${result.error}`, 'error');
      }
    } catch (e: any) {
      showMessage(`生成失败: ${e.message}`, 'error');
    } finally {
      setGenerating(false);
    }
  };

  const handleApply = async () => {
    if (!generated || !generated.valid) {
      showMessage('请先生成有效配置', 'error');
      return;
    }

    if (!confirm('确定要应用配置并重启核心吗？')) return;

    try {
      setApplying(true);
      await api.applyConfig({ restart_core: true });
      showMessage('配置已应用，核心正在重启', 'success');
    } catch (e: any) {
      showMessage(`应用失败: ${e.message}`, 'error');
    } finally {
      setApplying(false);
    }
  };

  // 自动生成预览
  useEffect(() => {
    handleGenerate();
  }, [config]);

  return (
    <div className="flex h-screen flex-col">
      <div className="shrink-0 border-b border-[var(--border-default)] bg-[var(--bg-primary)] px-6 py-4">
        <div className="flex items-center justify-between">
          <PageHeader
            title="配置生成"
            description="自动生成 sing-box 配置"
          />
          <div className="flex gap-2">
            <button
              onClick={handleGenerate}
              disabled={generating}
              className="inline-flex h-9 items-center gap-2 rounded-md bg-blue-600 px-4 text-sm font-medium text-white hover:bg-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Play size={16} />
              {generating ? '生成中...' : '重新生成'}
            </button>
            {generated && generated.valid && (
              <button
                onClick={handleApply}
                disabled={applying}
                className="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-4 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Save size={16} />
                {applying ? '应用中...' : '应用配置'}
              </button>
            )}
          </div>
        </div>

        {message && (
          <div className={`mt-3 rounded-md border px-3 py-2 text-sm ${
            messageType === 'success'
              ? 'border-emerald-400/20 bg-emerald-500/10 text-emerald-300'
              : 'border-red-400/20 bg-red-500/10 text-red-300'
          }`}>
            {message}
          </div>
        )}
      </div>

      <div className="flex min-h-0 flex-1">
        {/* 左侧表单 */}
        <div className="w-[400px] border-r border-[var(--border-default)] bg-[var(--bg-secondary)] p-6 overflow-y-auto">
          <h3 className="mb-4 text-sm font-semibold text-white">配置参数</h3>
          
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">
                默认出站
              </label>
              <select
                value={config.default_outbound}
                onChange={(e) => setConfig({ ...config, default_outbound: e.target.value })}
                className="w-full rounded border border-[var(--border-default)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-white"
              >
                <option value="proxy">代理</option>
                <option value="direct">直连</option>
                <option value="block">拦截</option>
              </select>
              <p className="mt-1 text-xs text-[var(--text-secondary)] opacity-70">
                未匹配规则的流量走向
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">
                入站监听地址
              </label>
              <input
                type="text"
                value={config.inbound_listen}
                onChange={(e) => setConfig({ ...config, inbound_listen: e.target.value })}
                className="w-full rounded border border-[var(--border-default)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-white"
              />
              <p className="mt-1 text-xs text-[var(--text-secondary)] opacity-70">
                本地代理监听地址，通常为 127.0.0.1
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">
                入站端口
              </label>
              <input
                type="number"
                value={config.inbound_port}
                onChange={(e) => setConfig({ ...config, inbound_port: parseInt(e.target.value) || 7890 })}
                className="w-full rounded border border-[var(--border-default)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-white"
              />
              <p className="mt-1 text-xs text-[var(--text-secondary)] opacity-70">
                本地代理端口，建议 7890
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">
                日志级别
              </label>
              <select
                value={config.log_level}
                onChange={(e) => setConfig({ ...config, log_level: e.target.value })}
                className="w-full rounded border border-[var(--border-default)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-white"
              >
                <option value="trace">Trace</option>
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warn</option>
                <option value="error">Error</option>
              </select>
              <p className="mt-1 text-xs text-[var(--text-secondary)] opacity-70">
                日志详细程度，生产环境建议 info 或 warn
              </p>
            </div>

            {generated && !generated.valid && (
              <div className="rounded-md border border-red-400/20 bg-red-500/10 p-3 text-sm text-red-300">
                <div className="font-medium mb-1">配置校验失败</div>
                <div className="text-xs opacity-80">{generated.error}</div>
              </div>
            )}

            {generated && generated.valid && (
              <div className="rounded-md border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-300">
                <div className="font-medium">✓ 配置有效</div>
                <div className="mt-2 text-xs opacity-80 space-y-1">
                  <div>入站: {generated.config?.inbounds?.length || 0} 个</div>
                  <div>出站: {generated.config?.outbounds?.length || 0} 个</div>
                  <div>路由规则: {generated.config?.route?.rules?.length || 0} 条</div>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* 右侧 JSON 预览 */}
        <div className="flex-1 bg-[var(--bg-primary)] overflow-auto">
          <div className="sticky top-0 border-b border-[var(--border-default)] bg-[var(--bg-secondary)] px-6 py-3 z-10">
            <h3 className="text-sm font-semibold text-white">配置预览</h3>
          </div>
          <div className="p-6">
            {generated && generated.config ? (
              <JsonPreview data={generated.config} maxHeight="none" />
            ) : (
              <div className="flex items-center justify-center h-64 text-[var(--text-secondary)]">
                {generating ? '生成中...' : '暂无配置'}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default ConfigPage;
