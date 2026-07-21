<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "@/services/api";
import Button from "@/components/ui/Button.vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import { useHashTab } from "@/composables/useHashTab";
import ConnectivityResourcesPanel from "./settings/ConnectivityResourcesPanel.vue";
import GeoIPProvidersPanel from "./settings/GeoIPProvidersPanel.vue";
import TrafficBypassPanel from "./settings/TrafficBypassPanel.vue";
import AppUpdatePanel from "./settings/AppUpdatePanel.vue";
import ExperimentalSettingsPanel from "./settings/ExperimentalSettingsPanel.vue";
import {
  Clock3,
  FlaskConical,
  RefreshCw,
  Settings,
  ShieldOff,
} from "lucide-vue-next";
type SettingsTab = "general" | "bypass" | "experimental" | "update";
const settingsTabs = [
  "general",
  "bypass",
  "experimental",
  "update",
] as const satisfies readonly SettingsTab[];
const message = ref(""),
  messageType = ref<"success" | "error" | "info">("success");
const { activeTab, selectTab } = useHashTab(settingsTabs, "general");
const ntpEnabled = ref(true),
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
        @click="selectTab('general')"
      >
        <Settings :size="16" />常规设置
      </button>
      <button
        class="relative flex items-center gap-2 px-4 py-2.5 text-sm font-medium outline-none"
        :class="
          activeTab === 'bypass'
            ? 'text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
        "
        role="tab"
        :aria-selected="activeTab === 'bypass'"
        @click="selectTab('bypass')"
      >
        <ShieldOff :size="16" />流量排除
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
        @click="selectTab('experimental')"
      >
        <FlaskConical :size="16" />实验性功能
      </button>
      <button
        class="relative flex items-center gap-2 px-4 py-2.5 text-sm font-medium outline-none"
        :class="
          activeTab === 'update'
            ? 'text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
        "
        role="tab"
        :aria-selected="activeTab === 'update'"
        @click="selectTab('update')"
      >
        <RefreshCw :size="16" />检查更新
      </button>
    </div>

    <div
      v-if="activeTab === 'general'"
      class="grid grid-cols-1 items-stretch gap-4 lg:grid-cols-2"
      role="tabpanel"
    >
      <ConnectivityResourcesPanel @notify="notify" />

      <GeoIPProvidersPanel @notify="notify" />

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

    </div>

    <TrafficBypassPanel
      v-else-if="activeTab === 'bypass'"
      @notify="notify"
    />

    <ExperimentalSettingsPanel
      v-else-if="activeTab === 'experimental'"
      @notify="notify"
    />

    <AppUpdatePanel v-else @notify="notify" />
  </div>
</template>
