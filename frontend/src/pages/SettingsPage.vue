<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "@/services/api";
import Button from "@/components/ui/Button.vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import {
  connectivityTestTargets,
  connectivityTestTargetValues,
  defaultConnectivityTestURL,
} from "@/utils/connectivityTargets";
import {
  Clock3,
  Download,
  FileText,
  FlaskConical,
  Gauge,
  Settings,
} from "lucide-vue-next";
type SettingsTab = "general" | "experimental";
const acceleration = ref(""),
  customMirror = ref(""),
  githubToken = ref(""),
  proxyURL = ref("http://127.0.0.1:2080"),
  message = ref(""),
  messageType = ref<"success" | "error" | "info">("success");
const activeTab = ref<SettingsTab>("general"),
  clashApiPort = ref("9090"),
  clashApiSecret = ref(""),
  clashApiExternalUI = ref(""),
  clashApiExternalUIDownloadURL = ref(""),
  cacheFileEnabled = ref(true),
  cacheFileStoreFakeIP = ref(true),
  cacheFileStoreDNS = ref(true),
  logLevel = ref("info"),
  logTimestamp = ref(true),
  connectivityTargetMode = ref(defaultConnectivityTestURL),
  connectivityCustomURL = ref(""),
  connectivityInterval = ref(300),
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
    .getConnectivitySettings()
    .then((d) => {
      if (connectivityTestTargetValues.has(d.test_url)) {
        connectivityTargetMode.value = d.test_url;
        connectivityCustomURL.value = "";
      } else {
        connectivityTargetMode.value = "custom";
        connectivityCustomURL.value = d.test_url;
      }
      connectivityInterval.value = d.interval_seconds;
    })
    .catch((e: any) => notify(`加载连通性测速设置失败: ${e.message}`, "error"));
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
async function saveConnectivity() {
  const testURL =
    connectivityTargetMode.value === "custom"
      ? connectivityCustomURL.value.trim()
      : connectivityTargetMode.value;
  try {
    const parsed = new URL(testURL);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      throw new Error("unsupported protocol");
    }
  } catch {
    notify("连通性地址必须是完整的 HTTP/HTTPS URL", "error");
    return;
  }
  if (connectivityInterval.value < 60 || connectivityInterval.value > 3600) {
    notify("连通间隔必须在 60 到 3600 秒之间", "error");
    return;
  }
  try {
    await api.setConnectivitySettings({
      test_url: testURL,
      interval_seconds: connectivityInterval.value,
    });
    if (connectivityTargetMode.value === "custom") {
      connectivityCustomURL.value = testURL;
    }
    notify("连通性测速设置已保存，配置将自动生成并应用");
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
    "aw-input w-full outline-none focus:border-[var(--color-primary)]",
  panel =
    "rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]";
</script>
<template>
  <Toast :message="message" :type="messageType" @dismiss="message = ''" />
  <div class="space-y-4">
    <PageHeader title="设置" />
    <div
      class="flex gap-1 overflow-x-auto border-b border-[var(--border-default)]"
      role="tablist"
      aria-label="设置分类"
    >
      <button
        class="relative flex items-center gap-2 px-4 py-2.5 text-sm font-medium outline-none"
        :class="
          activeTab === 'general'
            ? 'text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
        "
        role="tab"
        :aria-selected="activeTab === 'general'"
        @click="activeTab = 'general'"
      >
        <Settings :size="16" />常规功能
      </button>
      <button
        class="relative flex items-center gap-2 px-4 py-2.5 text-sm font-medium outline-none"
        :class="
          activeTab === 'experimental'
            ? 'text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
        "
        role="tab"
        :aria-selected="activeTab === 'experimental'"
        @click="activeTab = 'experimental'"
      >
        <FlaskConical :size="16" />实验性功能
      </button>
    </div>

    <div
      v-if="activeTab === 'general'"
      class="grid grid-cols-1 items-stretch gap-4 lg:grid-cols-2"
      role="tabpanel"
    >
      <section :class="panel" class="flex h-full flex-col">
        <div class="mb-4 flex items-center gap-2">
          <Gauge :size="18" class="text-[var(--color-primary)]" />
          <h2 class="font-semibold">连通性测速</h2>
        </div>
        <div class="flex flex-1 flex-col gap-4">
          <label class="block text-sm">
            连通性地址
            <select v-model="connectivityTargetMode" :class="input">
              <option
                v-for="target in connectivityTestTargets"
                :key="target.value"
                :value="target.value"
              >
                {{ target.label }} · {{ target.value }}
              </option>
              <option value="custom">自定义地址</option>
            </select>
          </label>
          <label
            v-if="connectivityTargetMode === 'custom'"
            class="block text-sm"
          >
            自定义地址
            <input
              v-model.trim="connectivityCustomURL"
              :class="input"
              placeholder="http://example.com/generate_204"
            />
          </label>
          <label class="block text-sm"
            >连通间隔（秒）<input
              v-model.number="connectivityInterval"
              type="number"
              min="60"
              max="3600"
              :class="input"
          /></label>
          <p
            class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-2 text-xs text-[var(--text-secondary)]"
          >
            统一用于节点组、策略组的自动 URLTest 与后台健康检查。HTTP 默认使用
            80 端口，HTTPS 默认使用 443 端口。
          </p>
          <Button class="mt-auto self-start" size="sm" @click="saveConnectivity"
            >保存测速设置</Button
          >
        </div>
      </section>

      <section :class="panel" class="flex h-full flex-col">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <Clock3 :size="18" class="text-[var(--color-primary)]" />
            <h2 class="font-semibold">
              NTP 时间同步
              <span class="text-xs text-[var(--color-primary)]">sing-box</span>
            </h2>
          </div>
          <label class="flex shrink-0 items-center gap-2 text-xs">
            启用
            <input v-model="ntpEnabled" type="checkbox" />
          </label>
        </div>
        <div class="flex flex-1 flex-col gap-4">
          <p class="text-xs text-[var(--text-secondary)]">
            保持 sing-box 内部时间准确，Reality、VLESS-XTLS 和 TLS
            校验场景建议开启。
          </p>
          <div v-if="ntpEnabled" class="grid gap-3 sm:grid-cols-2">
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
          <div
            v-if="ntpEnabled"
            class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-2 text-xs text-[var(--color-primary)]"
          >
            默认每 30 分钟同步一次；支持 30m、1h。
          </div>
          <Button class="mt-auto self-start" size="sm" @click="saveNTP"
            >保存 NTP 设置</Button
          >
        </div>
      </section>

      <section :class="panel" class="flex h-full flex-col">
        <div class="mb-4 flex items-center gap-2">
          <Download :size="18" class="text-[var(--color-primary)]" />
          <h2 class="font-semibold">更新设置</h2>
        </div>
        <div class="flex flex-1 flex-col gap-4">
          <label class="block text-sm"
            >GitHub Token<input
              v-model="githubToken"
              type="password"
              :class="input"
              placeholder="ghp_xxxxxxxxxxxxxxxxxxxx"
            /><small class="text-[var(--text-tertiary)]"
              >用于 GitHub API 调用，避免触发速率限制。</small
            ></label
          ><label class="block text-sm"
            >下载加速<select v-model="acceleration" :class="input">
              <option value="">无加速</option>
              <option value="proxy">本地代理优先（推荐）</option>
              <option value="ghproxy">https://gh-proxy.com/</option>
              <option value="ghproxy_vip">https://ghproxy.vip/</option>
              <option value="jsdelivr_fastly">
                https://fastly.jsdelivr.net/
              </option>
              <option value="jsdelivr_testingcf">
                https://testingcf.jsdelivr.net/
              </option>
              <option value="jsdelivr_cdn">https://cdn.jsdelivr.net/</option>
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
          ><Button class="mt-auto self-start" size="sm" @click="saveUpdate"
            >保存更新设置</Button
          >
        </div>
      </section>

      <section :class="panel" class="flex h-full flex-col">
        <div class="mb-4 flex items-center gap-2">
          <FileText :size="18" class="text-[var(--color-primary)]" />
          <h2 class="font-semibold">
            日志配置
            <span class="text-xs text-[var(--color-primary)]">sing-box</span>
          </h2>
        </div>
        <div class="flex flex-1 flex-col gap-4">
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
            ><small class="text-[var(--text-tertiary)]"
              >控制 sing-box 日志输出详细程度</small
            ></label
          ><label class="flex justify-between text-xs"
            >启用时间戳<input v-model="logTimestamp" type="checkbox"
          /></label>
          <div
            class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-2 text-xs text-[var(--color-primary)]"
          >
            生产环境建议使用 info，调试时可用 debug 或 trace。
          </div>
          <Button class="mt-auto self-start" size="sm" @click="saveLog"
            >保存日志配置</Button
          >
        </div>
      </section>
    </div>

    <section v-else :class="panel" class="flex flex-col" role="tabpanel">
      <div class="mb-4 flex items-center gap-2">
        <h2 class="font-semibold">实验性功能</h2>
        <span
          class="rounded-[var(--radius-sm)] bg-[var(--color-warning-bg)] px-2 py-0.5 text-xs text-[var(--color-warning)]"
          >实验性</span
        >
      </div>
      <div class="grid gap-4 lg:grid-cols-2">
        <div
          class="rounded-[var(--radius-lg)] border border-[var(--border-default)] p-4"
        >
          <div class="mb-3 flex justify-between gap-3">
            <div>
              <h3>Clash API</h3>
              <p class="text-xs text-[var(--text-secondary)]">
                提供 RESTful API 用于实时监控和策略组切换，核心功能（强制开启）
              </p>
            </div>
            <span class="shrink-0 text-xs text-[var(--color-success)]"
              >● 已强制启用</span
            >
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
              class="rounded-[var(--radius-md)] bg-[var(--color-success-bg)] p-2 text-xs text-[var(--color-success)]"
            >
              <b>说明：</b> Clash API 已强制启用。所有请求通过 Ackwrap
              后端代理访问，外部无法直接访问。地址为 127.0.0.1:{{
                clashApiPort || "9090"
              }}。
            </div>
          </div>
        </div>
        <div
          class="rounded-[var(--radius-lg)] border border-[var(--border-default)] p-4"
        >
          <div class="mb-3 flex justify-between gap-3">
            <div>
              <h3>缓存文件</h3>
              <p class="text-xs text-[var(--text-secondary)]">
                缓存 FakeIP、规则集等数据，提高性能
              </p>
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
            <div
              class="rounded-[var(--radius-md)] bg-[var(--color-warning-bg)] p-2 text-xs text-[var(--color-warning)]"
            >
              <b>说明：</b>缓存文件保存到
              <code>cache.db</code>，重启后自动恢复。
            </div>
          </div>
        </div>
      </div>
      <div class="mt-4">
        <Button size="sm" @click="saveExperimental">保存实验性功能设置</Button>
      </div>
    </section>
  </div>
</template>
