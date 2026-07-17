<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "@/services/api";
import Button from "@/components/ui/Button.vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
const acceleration = ref(""),
  customMirror = ref(""),
  githubToken = ref(""),
  proxyURL = ref("http://127.0.0.1:2080"),
  message = ref(""),
  messageType = ref<"success" | "error" | "info">("success");
const clashApiPort = ref("9090"),
  clashApiSecret = ref(""),
  clashApiExternalUI = ref(""),
  clashApiExternalUIDownloadURL = ref(""),
  cacheFileEnabled = ref(true),
  cacheFileStoreFakeIP = ref(true),
  cacheFileStoreDNS = ref(true),
  logLevel = ref("info"),
  logTimestamp = ref(true),
  ntpEnabled = ref(true),
  ntpServer = ref("time.apple.com"),
  ntpServerPort = ref(123),
  ntpInterval = ref("30m"),
  ntpDetour = ref("direct");
function notify(v: string, t: "success" | "error" | "info" = "success") {
  message.value = v;
  messageType.value = t;
}
onMounted(() => {
  api
    .getUpdateSettings()
    .then((d) => {
      acceleration.value = d.acceleration || "";
      customMirror.value = d.custom_mirror_url || "";
      githubToken.value = d.github_token || "";
      proxyURL.value = d.proxy_url || "http://127.0.0.1:2080";
    })
    .catch(() => {});
  api
    .getExperimentalSettings()
    .then((d) => {
      clashApiPort.value = d.clash_api_port || "9090";
      clashApiSecret.value = d.clash_api_secret || "";
      clashApiExternalUI.value = d.clash_api_external_ui || "";
      clashApiExternalUIDownloadURL.value =
        d.clash_api_external_ui_download_url || "";
      cacheFileEnabled.value = d.cache_file_enabled !== false;
      cacheFileStoreFakeIP.value = d.cache_file_store_fakeip !== false;
      cacheFileStoreDNS.value = d.cache_file_store_dns !== false;
    })
    .catch(() => {});
  api
    .getLogSettings()
    .then((d) => {
      logLevel.value = d.level || "info";
      logTimestamp.value = d.timestamp !== false;
    })
    .catch(() => {});
  api
    .getNTPSettings()
    .then((d) => {
      ntpEnabled.value = d.enabled !== false;
      ntpServer.value = d.server || "time.apple.com";
      ntpServerPort.value = d.server_port || 123;
      ntpInterval.value = d.interval || "30m";
      ntpDetour.value = d.detour || "direct";
    })
    .catch(() => {});
});
async function saveUpdate() {
  try {
    await api.setUpdateSettings({
      acceleration: acceleration.value,
      custom_mirror_url: customMirror.value,
      github_token: githubToken.value,
      proxy_url: proxyURL.value,
    });
    notify("更新设置已保存");
  } catch (e: any) {
    notify(`保存失败: ${e.message}`, "error");
  }
}
async function saveExperimental() {
  try {
    await api.setExperimentalSettings({
      clash_api_enabled: true,
      clash_api_port: clashApiPort.value,
      clash_api_secret: clashApiSecret.value,
      clash_api_external_ui: clashApiExternalUI.value,
      clash_api_external_ui_download_url: clashApiExternalUIDownloadURL.value,
      cache_file_enabled: cacheFileEnabled.value,
      cache_file_store_fakeip: cacheFileStoreFakeIP.value,
      cache_file_store_dns: cacheFileStoreDNS.value,
    });
    notify("实验性功能设置已保存");
  } catch (e: any) {
    notify(`保存失败: ${e.message}`, "error");
  }
}
async function saveLog() {
  try {
    await api.setLogSettings({
      level: logLevel.value,
      timestamp: logTimestamp.value,
    });
    notify("日志配置已保存（下次生成配置时生效）");
  } catch (e: any) {
    notify(`保存失败: ${e.message}`, "error");
  }
}
async function saveNTP() {
  try {
    await api.setNTPSettings({
      enabled: ntpEnabled.value,
      server: ntpServer.value,
      server_port: ntpServerPort.value,
      interval: ntpInterval.value,
      detour: ntpDetour.value,
    });
    notify("NTP 设置已保存（下次生成配置时生效）");
  } catch (e: any) {
    notify(`保存失败: ${e.message}`, "error");
  }
}
const input =
    "w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-[var(--color-primary)]",
  panel =
    "rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]";
</script>
<template>
  <Toast :message="message" :type="messageType" @dismiss="message = ''" />
  <div class="space-y-4">
    <PageHeader title="设置" />
    <div class="grid grid-cols-1 items-stretch gap-4 lg:grid-cols-2">
      <section :class="panel" class="flex h-full flex-col">
        <div class="mb-4 flex gap-2">
          <h2 class="font-semibold text-white">实验性功能</h2>
          <span class="rounded bg-amber-500/20 px-2 text-xs text-amber-300"
            >实验性</span
          >
        </div>
        <div class="flex flex-1 flex-col space-y-4">
          <div class="rounded-lg border border-[var(--border-default)] p-4">
            <div class="mb-3 flex justify-between">
              <div>
                <h3>Clash API</h3>
                <p class="text-xs">
                  提供 RESTful API
                  用于实时监控和策略组切换，核心功能（强制开启）
                </p>
              </div>
              <span class="text-xs text-emerald-400">● 已强制启用</span>
            </div>
            <div class="space-y-3">
              <label class="block text-xs"
                >端口<input
                  v-model="clashApiPort"
                  :class="input"
                  placeholder="9090" /></label
              ><label class="block text-xs"
                >密钥（可选，留空则无密钥）<input
                  v-model="clashApiSecret"
                  type="password"
                  :class="input" /></label
              ><label class="block text-xs"
                >外部 UI 面板路径（可选）<input
                  v-model="clashApiExternalUI"
                  :class="input" /></label
              ><label class="block text-xs"
                >外部 UI 下载 URL（可选）<input
                  v-model="clashApiExternalUIDownloadURL"
                  :class="input"
                  placeholder="https://github.com/MetaCubeX/metacubexd/..."
              /></label>
              <div
                class="rounded bg-emerald-500/10 p-2 text-xs text-emerald-300"
              >
                <b>说明：</b> Clash API 已强制启用。所有请求通过 Ackwrap
                后端代理访问，外部无法直接访问。地址为 127.0.0.1:{{
                  clashApiPort || "9090"
                }}。
              </div>
            </div>
          </div>
          <div class="rounded-lg border p-4">
            <div class="mb-3 flex justify-between">
              <div>
                <h3>缓存文件</h3>
                <p class="text-xs">缓存 FakeIP、规则集等数据，提高性能</p>
              </div>
              <input v-model="cacheFileEnabled" type="checkbox" />
            </div>
            <div v-if="cacheFileEnabled" class="space-y-3">
              <label class="flex justify-between text-xs"
                >缓存 FakeIP<input
                  v-model="cacheFileStoreFakeIP"
                  type="checkbox" /></label
              ><label class="flex justify-between text-xs"
                >持久化完整 DNS 缓存<input
                  v-model="cacheFileStoreDNS"
                  type="checkbox"
              /></label>
              <div class="rounded bg-amber-500/10 p-2 text-xs text-amber-300">
                <b>说明：</b>缓存文件保存到
                <code>cache.db</code>，重启后自动恢复。
              </div>
            </div>
          </div>
          <Button size="sm" @click="saveExperimental"
            >保存实验性功能设置</Button
          >
        </div>
      </section>
      <div class="flex h-full flex-col gap-4">
        <section :class="panel">
          <h2 class="mb-4 font-semibold">更新设置</h2>
          <div class="space-y-4">
            <label class="block text-sm"
              >GitHub Token<input
                v-model="githubToken"
                type="password"
                :class="input"
                placeholder="ghp_xxxxxxxxxxxxxxxxxxxx"
              /><small>用于 GitHub API 调用，避免触发速率限制。</small></label
            ><label class="block text-sm"
              >下载加速<select v-model="acceleration" :class="input">
                <option value="">无加速</option>
                <option value="proxy">本地代理优先（推荐）</option>
                <option value="ghproxy">GHProxy</option>
                <option value="custom">自定义镜像</option>
              </select></label
            ><label v-if="acceleration === 'proxy'" class="block text-sm"
              >本地 HTTP 代理 URL<input
                v-model="proxyURL"
                :class="input" /></label
            ><label v-if="acceleration === 'custom'" class="block text-sm"
              >自定义镜像 URL<input
                v-model="customMirror"
                :class="input" /></label
            ><Button size="sm" @click="saveUpdate">保存</Button>
          </div>
        </section>
        <section :class="panel">
          <h2 class="mb-4 font-semibold">
            日志配置 <span class="text-xs text-blue-300">sing-box</span>
          </h2>
          <div class="space-y-4">
            <label class="block text-xs"
              >日志级别<select v-model="logLevel" :class="input">
                <option
                  v-for="l in [
                    'trace',
                    'debug',
                    'info',
                    'warn',
                    'error',
                    'fatal',
                    'panic',
                  ]"
                  :key="l"
                >
                  {{ l }}
                </option></select
              ><small>控制 sing-box 日志输出详细程度</small></label
            ><label class="flex justify-between text-xs"
              >启用时间戳<input v-model="logTimestamp" type="checkbox"
            /></label>
            <div class="bg-blue-500/10 p-2 text-xs text-blue-300">
              生产环境建议使用 info，调试时可用 debug 或 trace。
            </div>
            <Button size="sm" @click="saveLog">保存日志配置</Button>
          </div>
        </section>
        <section :class="panel" class="flex flex-1 flex-col">
          <h2 class="mb-4 font-semibold">
            NTP 时间同步 <span class="text-xs text-blue-300">sing-box</span>
          </h2>
          <div class="grid flex-1 gap-4 lg:grid-cols-[260px_1fr]">
            <div class="flex flex-col justify-between rounded-lg border p-4">
              <div>
                <label class="flex justify-between"
                  >启用 NTP 同步<input v-model="ntpEnabled" type="checkbox"
                /></label>
                <p class="mt-3 text-xs">
                  用于确保 sing-box 内部时间准确。Reality、VLESS-XTLS、TLS
                  校验等场景建议保持开启。
                </p>
              </div>
              <Button size="sm" @click="saveNTP">保存 NTP 设置</Button>
            </div>
            <div v-if="ntpEnabled" class="rounded-lg border p-4">
              <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <label class="text-xs"
                  >NTP 服务器<input v-model="ntpServer" :class="input" /></label
                ><label class="text-xs"
                  >端口<input
                    v-model.number="ntpServerPort"
                    type="number"
                    :class="input" /></label
                ><label class="text-xs"
                  >同步间隔<input v-model="ntpInterval" :class="input" /></label
                ><label class="text-xs"
                  >出站策略<select v-model="ntpDetour" :class="input">
                    <option value="direct">direct - 直连</option>
                    <option value="proxy">proxy - 代理</option>
                  </select></label
                >
              </div>
              <div class="mt-3 bg-blue-500/10 p-2 text-xs text-blue-300">
                默认每 30 分钟同步一次；支持 30m、1h。
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
