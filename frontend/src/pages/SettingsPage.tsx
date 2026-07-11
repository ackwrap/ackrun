import React from 'react';
import { api } from '@/services/api';
import { Button } from '@/components/ui/Button';
import { PageHeader } from '@/components/layout/PageHeader';
import { Toast } from '@/components/ui/Toast';

export function SettingsPage() {
  const [acceleration, setAcceleration] = React.useState('');
  const [customMirror, setCustomMirror] = React.useState('');
  const [githubToken, setGithubToken] = React.useState('');
  const [proxyURL, setProxyURL] = React.useState('http://127.0.0.1:2080');
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error' | 'info'>('success');

  // 实验性功能设置
  const [clashApiPort, setClashApiPort] = React.useState('9090');
  const [clashApiSecret, setClashApiSecret] = React.useState('');
  const [clashApiExternalUI, setClashApiExternalUI] = React.useState('');
  const [clashApiExternalUIDownloadURL, setClashApiExternalUIDownloadURL] = React.useState('');
  const [cacheFileEnabled, setCacheFileEnabled] = React.useState(true);
  const [cacheFileStoreFakeIP, setCacheFileStoreFakeIP] = React.useState(true);
  const [cacheFileStoreDNS, setCacheFileStoreDNS] = React.useState(true);

  // 日志配置
  const [logLevel, setLogLevel] = React.useState('info');
  const [logTimestamp, setLogTimestamp] = React.useState(true);

  // NTP 配置
  const [ntpEnabled, setNtpEnabled] = React.useState(true);
  const [ntpServer, setNtpServer] = React.useState('time.apple.com');
  const [ntpServerPort, setNtpServerPort] = React.useState(123);
  const [ntpInterval, setNtpInterval] = React.useState('30m');
  const [ntpDetour, setNtpDetour] = React.useState('direct');

  const showMessage = (msg: string, type: 'success' | 'error' | 'info' = 'success') => {
    setMessage(msg);
    setMessageType(type);
  };

  React.useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(() => setMessage(''), messageType === 'error' ? 5000 : 3000);
    return () => window.clearTimeout(timer);
  }, [message, messageType]);

  React.useEffect(() => {
    api.getUpdateSettings().then(data => {
      setAcceleration(data.acceleration || '');
      setCustomMirror(data.custom_mirror_url || '');
      setGithubToken(data.github_token || '');
      setProxyURL(data.proxy_url || 'http://127.0.0.1:2080');
    }).catch(() => {});

    // 加载实验性功能设置
    api.getExperimentalSettings().then(data => {
      setClashApiPort(data.clash_api_port || '9090');
      setClashApiSecret(data.clash_api_secret || '');
      setClashApiExternalUI(data.clash_api_external_ui || '');
      setClashApiExternalUIDownloadURL(data.clash_api_external_ui_download_url || '');
      setCacheFileEnabled(data.cache_file_enabled !== false);
      setCacheFileStoreFakeIP(data.cache_file_store_fakeip !== false);
      setCacheFileStoreDNS(data.cache_file_store_dns !== false);
    }).catch(() => {});

    api.getLogSettings().then(data => {
      setLogTimestamp(data.timestamp !== false);
    }).catch(() => {});

    // 加载 NTP 设置
    api.getNTPSettings().then(data => {
      setNtpEnabled(data.enabled !== false);
      setNtpServer(data.server || 'time.apple.com');
      setNtpServerPort(data.server_port || 123);
      setNtpInterval(data.interval || '30m');
      setNtpDetour(data.detour || 'direct');
    }).catch(() => {});
  }, []);

  const handleSave = async () => {
    try {
      await api.setUpdateSettings({ acceleration, custom_mirror_url: customMirror, github_token: githubToken, proxy_url: proxyURL });
      showMessage('更新设置已保存');
    } catch (e: any) { showMessage(`保存失败: ${e.message}`, 'error'); }
  };

  const handleSaveExperimental = async () => {
    try {
      await api.setExperimentalSettings({
        clash_api_enabled: true,
        clash_api_port: clashApiPort,
        clash_api_secret: clashApiSecret,
        clash_api_external_ui: clashApiExternalUI,
        clash_api_external_ui_download_url: clashApiExternalUIDownloadURL,
        cache_file_enabled: cacheFileEnabled,
        cache_file_store_fakeip: cacheFileStoreFakeIP,
        cache_file_store_dns: cacheFileStoreDNS,
      });
      showMessage('实验性功能设置已保存');
    } catch (e: any) { showMessage(`保存失败: ${e.message}`, 'error'); }
  };

  const handleSaveNTP = async () => {
    try {
      await api.setNTPSettings({
        enabled: ntpEnabled,
        server: ntpServer,
        server_port: ntpServerPort,
        interval: ntpInterval,
        detour: ntpDetour,
      });
      showMessage('NTP 设置已保存（下次生成配置时生效）');
    } catch (e: any) { showMessage(`保存失败: ${e.message}`, 'error'); }
  };

  const handleSaveLog = async () => {
    try {
      await api.setLogSettings({ timestamp: logTimestamp });
      showMessage('日志配置已保存（下次生成配置时生效）');
    } catch (e: any) { showMessage(`保存失败: ${e.message}`, 'error'); }
  };

  return (
    <>
      <Toast message={message} type={messageType} />
      <div className="space-y-4">
        <PageHeader title="设置" />

      <div className="grid grid-cols-1 items-stretch gap-4 lg:grid-cols-2">
        {/* 实验性功能配置区域 - 左侧占两行高度 */}
        <section className="flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
          <div className="mb-4 flex items-center gap-2">
            <h2 className="font-semibold text-white">实验性功能</h2>
            <span className="rounded bg-amber-500/20 px-2 py-0.5 text-xs text-amber-300">实验性</span>
          </div>
          <div className="flex flex-1 flex-col space-y-4">
            {/* Clash API 配置 - 强制开启 */}
            <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.02] p-4">
              <div className="mb-3 flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-white">Clash API</h3>
                  <p className="mt-1 text-xs text-[var(--text-tertiary)]">提供 RESTful API 用于实时监控和策略组切换，核心功能（强制开启）</p>
                </div>
                <div className="flex items-center gap-1.5 rounded-full bg-emerald-500/15 px-3 py-1 text-xs text-emerald-400">
                  <div className="h-1.5 w-1.5 rounded-full bg-emerald-400"></div>
                  已强制启用
                </div>
              </div>

              <div className="space-y-3">
                <div>
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">端口</label>
                  <input
                    type="text"
                    value={clashApiPort}
                    onChange={e => setClashApiPort(e.target.value)}
                    placeholder="9090"
                    className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">密钥（可选，留空则无密钥）</label>
                  <input
                    type="password"
                    value={clashApiSecret}
                    onChange={e => setClashApiSecret(e.target.value)}
                    placeholder="留空则无密钥"
                    className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">外部 UI 面板路径（可选）</label>
                  <input
                    type="text"
                    value={clashApiExternalUI}
                    onChange={e => setClashApiExternalUI(e.target.value)}
                    placeholder="留空则不启用外部 UI（默认使用 Ackwrap 实时监控）"
                    className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">外部 UI 下载 URL（可选）</label>
                  <input
                    type="text"
                    value={clashApiExternalUIDownloadURL}
                    onChange={e => setClashApiExternalUIDownloadURL(e.target.value)}
                    placeholder="https://github.com/MetaCubeX/metacubexd/..."
                    className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                  />
                </div>
                <div className="rounded bg-emerald-500/10 p-2 text-xs text-emerald-300">
                  <strong>说明：</strong> Clash API 已强制启用。所有请求通过 Ackwrap 后端代理访问，外部无法直接访问。地址为 127.0.0.1:{clashApiPort || '9090'}。
                </div>
              </div>
            </div>

            {/* 缓存文件配置 */}
            <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.02] p-4">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <h3 className="text-sm font-medium text-white">缓存文件</h3>
                  <p className="mt-1 text-xs text-[var(--text-tertiary)]">缓存 FakeIP、规则集等数据，提高性能</p>
                </div>
                <label className="relative inline-flex cursor-pointer items-center">
                  <input
                    type="checkbox"
                    checked={cacheFileEnabled}
                    onChange={e => setCacheFileEnabled(e.target.checked)}
                    className="peer sr-only"
                  />
                  <div className="peer h-6 w-11 rounded-full bg-gray-700 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-blue-500 peer-checked:after:translate-x-full peer-focus:ring-2 peer-focus:ring-blue-500/30"></div>
                </label>
              </div>

              {cacheFileEnabled && (
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <label className="text-xs text-[var(--text-secondary)]">缓存 FakeIP</label>
                    <label className="relative inline-flex cursor-pointer items-center">
                      <input
                        type="checkbox"
                        checked={cacheFileStoreFakeIP}
                        onChange={e => setCacheFileStoreFakeIP(e.target.checked)}
                        className="peer sr-only"
                      />
                      <div className="peer h-5 w-9 rounded-full bg-gray-700 after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-blue-500 peer-checked:after:translate-x-full peer-focus:ring-2 peer-focus:ring-blue-500/30"></div>
                    </label>
                  </div>
                  <div className="flex items-center justify-between">
                    <label className="text-xs text-[var(--text-secondary)]">持久化完整 DNS 缓存</label>
                    <label className="relative inline-flex cursor-pointer items-center">
                      <input
                        type="checkbox"
                        checked={cacheFileStoreDNS}
                        onChange={e => setCacheFileStoreDNS(e.target.checked)}
                        className="peer sr-only"
                      />
                      <div className="peer h-5 w-9 rounded-full bg-gray-700 after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-blue-500 peer-checked:after:translate-x-full peer-focus:ring-2 peer-focus:ring-blue-500/30"></div>
                    </label>
                  </div>
                  <div className="rounded bg-amber-500/10 p-2 text-xs text-amber-300">
                    <strong>说明：</strong> 缓存文件保存到 <code>cache.db</code>，重启后自动恢复。关闭可能导致 DNS 重复查询和规则集重新加载。
                  </div>
                </div>
              )}
            </div>

            <div className="pt-1">
              <Button variant="primary" size="sm" onClick={handleSaveExperimental}>保存实验性功能设置</Button>
            </div>
          </div>
        </section>

        {/* 右侧容器 - 上下堆叠更新设置、日志配置和 NTP */}
        <div className="flex h-full flex-col gap-4">
          {/* 更新设置 - 右上占一行 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <h2 className="mb-4 font-semibold text-white">更新设置</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm text-[var(--text-secondary)] mb-1">GitHub Token</label>
                <input 
                  type="password" 
                  value={githubToken} 
                  onChange={e => setGithubToken(e.target.value)} 
                  placeholder="ghp_xxxxxxxxxxxxxxxxxxxx" 
                  className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]" 
                />
                <span className="mt-1 block text-xs text-[var(--text-tertiary)]">
                  用于 GitHub API 调用（检查更新、下载），避免触发速率限制。留空则使用匿名访问。
                  <a href="https://github.com/settings/tokens" target="_blank" rel="noopener noreferrer" className="ml-1 text-blue-400 hover:underline">获取 Token</a>
                </span>
              </div>
              <div>
                <label className="block text-sm text-[var(--text-secondary)] mb-1">下载加速</label>
                <select value={acceleration} onChange={e => setAcceleration(e.target.value)} className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]">
                  <option value="">无加速</option>
                  <option value="proxy">本地代理优先（推荐）</option>
                  <option value="ghproxy">GHProxy</option>
                  <option value="custom">自定义镜像</option>
                </select>
              </div>
              {acceleration === 'proxy' && (
                <div>
                  <label className="block text-sm text-[var(--text-secondary)] mb-1">本地 HTTP 代理 URL</label>
                  <input value={proxyURL} onChange={e => setProxyURL(e.target.value)} placeholder="http://127.0.0.1:2080" className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]" />
                  <span className="mt-1 block text-xs text-[var(--text-tertiary)]">优先通过本地代理访问 GitHub；代理不可用时自动回退直连。</span>
                </div>
              )}
              {acceleration === 'custom' && (
                <div>
                  <label className="block text-sm text-[var(--text-secondary)] mb-1">自定义镜像 URL</label>
                  <input value={customMirror} onChange={e => setCustomMirror(e.target.value)} placeholder="https://..." className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]" />
                </div>
              )}
              <div className="pt-1">
                <Button variant="primary" size="sm" onClick={handleSave}>保存</Button>
              </div>
            </div>
          </section>

          {/* 日志配置区域 - 右下占一行 */}
          <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4 flex items-center gap-2">
              <h2 className="font-semibold text-white">日志配置</h2>
              <span className="rounded bg-blue-500/20 px-2 py-0.5 text-xs text-blue-300">sing-box</span>
            </div>
            <div className="space-y-4">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">日志级别</label>
                <select
                  value={logLevel}
                  onChange={e => setLogLevel(e.target.value)}
                  className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                >
                  <option value="trace">trace - 最详细（调试用）</option>
                  <option value="debug">debug - 调试信息</option>
                  <option value="info">info - 一般信息（推荐）</option>
                  <option value="warn">warn - 警告信息</option>
                  <option value="error">error - 错误信息</option>
                  <option value="fatal">fatal - 致命错误</option>
                  <option value="panic">panic - 崩溃级别</option>
                </select>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">控制 sing-box 日志输出详细程度</p>
              </div>
              <div className="flex items-center justify-between rounded-md bg-white/[0.03] px-3 py-2">
                <label className="text-xs text-[var(--text-secondary)]">启用时间戳</label>
                <label className="relative inline-flex cursor-pointer items-center">
                  <input
                    type="checkbox"
                    checked={logTimestamp}
                    onChange={e => setLogTimestamp(e.target.checked)}
                    className="peer sr-only"
                  />
                  <div className="peer h-6 w-11 rounded-full bg-gray-700 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-blue-500 peer-checked:after:translate-x-full peer-focus:ring-2 peer-focus:ring-blue-500/30"></div>
                </label>
              </div>
              <div className="rounded bg-blue-500/10 p-2 text-xs text-blue-300">
                <strong>说明：</strong> 生产环境建议使用 "info"，调试时可用 "debug" 或 "trace"。关闭时间戳可略微提高性能。
              </div>

              <div className="pt-1">
                <Button variant="primary" size="sm" onClick={handleSaveLog}>保存日志配置</Button>
              </div>
            </div>
          </section>

          {/* NTP 配置区域 */}
          <section className="flex flex-1 flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]">
            <div className="mb-4 flex items-center gap-2">
            <h2 className="font-semibold text-white">NTP 时间同步</h2>
            <span className="rounded bg-blue-500/20 px-2 py-0.5 text-xs text-blue-300">sing-box</span>
          </div>
            <div className="grid flex-1 gap-4 lg:grid-cols-[260px_1fr]">
            <div className="flex flex-col justify-between rounded-lg border border-[var(--border-default)] bg-white/[0.02] p-4">
              <div>
                <div className="flex items-center justify-between gap-4">
                  <h3 className="text-sm font-medium text-white">启用 NTP 同步</h3>
                  <label className="relative inline-flex cursor-pointer items-center">
                    <input
                      type="checkbox"
                      checked={ntpEnabled}
                      onChange={e => setNtpEnabled(e.target.checked)}
                      className="peer sr-only"
                    />
                    <div className="peer h-6 w-11 rounded-full bg-gray-700 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-blue-500 peer-checked:after:translate-x-full peer-focus:ring-2 peer-focus:ring-blue-500/30"></div>
                  </label>
                </div>
                <p className="mt-3 text-xs leading-5 text-[var(--text-tertiary)]">用于确保 sing-box 内部时间准确。Reality、VLESS-XTLS、TLS 校验等场景建议保持开启。</p>
              </div>
              <Button variant="primary" size="sm" onClick={handleSaveNTP} className="mt-4 w-fit">保存 NTP 设置</Button>
            </div>

            {ntpEnabled && (
              <div className="rounded-lg border border-[var(--border-default)] bg-white/[0.02] p-4">
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                  <div>
                    <label className="mb-1 block text-xs text-[var(--text-secondary)]">NTP 服务器</label>
                    <input
                      type="text"
                      value={ntpServer}
                      onChange={e => setNtpServer(e.target.value)}
                      placeholder="time.apple.com"
                      className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-[var(--text-secondary)]">端口</label>
                    <input
                      type="number"
                      value={ntpServerPort}
                      onChange={e => setNtpServerPort(Number(e.target.value))}
                      placeholder="123"
                      className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-[var(--text-secondary)]">同步间隔</label>
                    <input
                      type="text"
                      value={ntpInterval}
                      onChange={e => setNtpInterval(e.target.value)}
                      placeholder="30m"
                      className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-[var(--text-secondary)]">出站策略</label>
                    <select
                      value={ntpDetour}
                      onChange={e => setNtpDetour(e.target.value)}
                      className="w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]"
                    >
                      <option value="direct">direct - 直连</option>
                      <option value="proxy">proxy - 代理</option>
                    </select>
                  </div>
                </div>
                <div className="mt-3 rounded bg-blue-500/10 p-2 text-xs text-blue-300">
                  <strong>说明：</strong> 默认每 30 分钟同步一次；同步间隔支持 m（分钟）、h（小时），如 30m、1h。
                </div>
              </div>
            )}
            </div>
          </section>
        </div>
      </div>
      </div>
    </>
  );
}

export default SettingsPage;
