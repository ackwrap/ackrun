<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import {
  ChevronDown,
  Play,
  Power,
  RefreshCw,
  RotateCcw,
  Square,
} from "lucide-vue-next";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type {
  ConfigFileItem,
  ConfigStatus,
  RuntimeResponse,
  WSEvent,
} from "@/services/types";
import Button from "@/components/ui/Button.vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import ControlLiveStatus from "./ControlLiveStatus.vue";
import ControlUpdateDialog from "./control/ControlUpdateDialog.vue";
import ControlMaintenance from "./control/ControlMaintenance.vue";
import ControlNetworkProbes from "./control/ControlNetworkProbes.vue";
import ControlRestartSchedule from "./ControlRestartSchedule.vue";
import ControlMixedProxyCard from "./ControlMixedProxyCard.vue";

const runtime = ref<RuntimeResponse | null>(null);
const configStatus = ref<ConfigStatus | null>(null);
const configFiles = ref<ConfigFileItem[]>([]);
const selectedConfig = ref("");
const inboundMode = ref("");
const proxyMode = ref("");
const loading = ref(false);
const loadError = ref("");
const runtimeAction = ref("");
const modeChanging = ref(false);
const configChanging = ref(false);
const installing = ref(false);
const advancedOpen = ref(false);
const advancedLoaded = ref(false);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
let cancelled = false;

const labels: Record<string, string> = {
  running: "运行中",
  starting: "启动中",
  stopping: "停止中",
  stopped: "已停止",
  error: "异常",
  not_installed: "未安装",
  no_config: "无配置",
};
const isRunning = computed(() => runtime.value?.status === "running");
const notInstalled = computed(() => runtime.value?.status === "not_installed");
const noConfig = computed(
  () =>
    runtime.value?.status === "no_config" ||
    configStatus.value?.has_config === false,
);
const busy = computed(
  () =>
    loading.value ||
    !!runtimeAction.value ||
    modeChanging.value ||
    configChanging.value ||
    installing.value ||
    ["starting", "stopping"].includes(runtime.value?.status || ""),
);
const configReady = computed(
  () =>
    !!runtime.value &&
    !notInstalled.value &&
    configStatus.value?.has_config === true,
);
const modeGroups = [
  {
    key: "proxy",
    label: "代理模式",
    options: [
      ["rule", "规则"],
      ["global", "全局"],
      ["direct", "直连"],
    ],
  },
  {
    key: "inbound",
    label: "运行模式",
    options: [
      ["tun_mixed", "TUN + Mixed"],
      ["tun", "TUN"],
      ["mixed", "Mixed"],
    ],
  },
] as const;

function notify(value: string, type: "success" | "error" | "info" = "success") {
  message.value = value;
  messageType.value = type;
}
async function refresh() {
  if (loading.value) return;
  loading.value = true;
  loadError.value = "";
  try {
    const status = await api.getRuntime();
    if (cancelled) return;
    runtime.value = status;
    if (status.status === "not_installed") return;
    const results = await Promise.allSettled([
      api.getConfigStatus(false).then((value) => {
        if (!cancelled) configStatus.value = value;
      }),
      api.getConfigFiles(false).then((value) => {
        if (cancelled) return;
        configFiles.value = value;
        selectedConfig.value = value.find((item) => item.active)?.name || "";
      }),
      api.getInboundMode().then((value) => {
        if (!cancelled) inboundMode.value = value.mode;
      }),
      api.getProxyMode().then((value) => {
        if (!cancelled) proxyMode.value = value.mode;
      }),
    ]);
    if (cancelled) return;
    const names = ["配置状态", "配置文件", "运行模式", "代理模式"];
    loadError.value = results
      .map((result, index) =>
        result.status === "rejected"
          ? `${names[index]}: ${result.reason?.message || "请求失败"}`
          : "",
      )
      .filter(Boolean)
      .join("；");
  } catch (error: any) {
    if (!cancelled) loadError.value = `运行状态加载失败: ${error.message}`;
  } finally {
    loading.value = false;
  }
}
async function action(fn: () => Promise<unknown>, label: string) {
  if (busy.value) return;
  runtimeAction.value = label;
  try {
    await fn();
    notify(`${label}成功`);
  } catch (error: any) {
    notify(`${label}失败: ${error.message}`, "error");
  } finally {
    await refresh();
    runtimeAction.value = "";
  }
}
async function changeActiveConfig() {
  const previous = configFiles.value.find((item) => item.active)?.name || "";
  const next = selectedConfig.value;
  if (!next || next === previous || busy.value) return;
  configChanging.value = true;
  try {
    configStatus.value = await api.setActiveConfig(next);
    configFiles.value = configFiles.value.map((item) => ({
      ...item,
      active: item.name === next,
    }));
    notify(
      isRunning.value ? `已切换为 ${next}，重载配置后生效` : `已切换为 ${next}`,
      isRunning.value ? "info" : "success",
    );
    await refresh();
  } catch (error: any) {
    selectedConfig.value = previous;
    notify(`切换配置失败: ${error.message}`, "error");
  } finally {
    configChanging.value = false;
  }
}
async function changeMode(kind: "inbound" | "proxy", mode: string) {
  if (isRunning.value || busy.value || !configReady.value) return;
  modeChanging.value = true;
  try {
    if (kind === "inbound") {
      await api.setInboundMode(mode);
      inboundMode.value = mode;
    } else {
      await api.setProxyMode(mode);
      proxyMode.value = mode;
    }
    notify("模式已切换，配置已自动生成并应用");
  } catch (error: any) {
    notify(`切换模式失败: ${error.message}`, "error");
  } finally {
    modeChanging.value = false;
  }
}
function toggleAdvanced() {
  advancedOpen.value = !advancedOpen.value;
  if (advancedOpen.value) advancedLoaded.value = true;
}
useRealtimeSocket((event: WSEvent) => {
  const data: any = event.data;
  if (event.type === "runtime.status" || event.type === "core.status") {
    runtime.value = { ...runtime.value, ...data } as RuntimeResponse;
    if (data.status === "error" && data.error)
      notify(`核心异常: ${data.error}`, "error");
  } else if (event.type === "config.status") {
    configStatus.value = data;
    if (data.file_name) {
      selectedConfig.value = data.file_name;
      configFiles.value = configFiles.value.map((item) => ({
        ...item,
        active: item.name === data.file_name,
      }));
    }
  } else if (event.type === "core.restart_schedule") {
    if (data.status === "succeeded") notify("核心定时重启完成");
    else if (data.status === "failed")
      notify(`核心定时重启失败: ${data.error || "未知错误"}`, "error");
  }
});
onMounted(refresh);
onBeforeUnmount(() => {
  cancelled = true;
});
</script>

<template>
  <div class="space-y-4">
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <PageHeader title="控制面板">
      <template #actions
        ><Button :disabled="busy" :loading="loading" @click="refresh"
          ><RefreshCw :size="14" />刷新</Button
        ></template
      >
    </PageHeader>
    <div
      v-if="loadError"
      role="alert"
      class="rounded-[var(--radius-lg)] bg-[var(--color-error-bg)] p-3 text-sm text-[var(--color-error)]"
    >
      {{ loadError }}
    </div>

    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
    >
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div class="flex flex-wrap items-center gap-3">
          <Power :size="20" class="text-[var(--color-primary)]" />
          <h2 class="font-semibold">sing-box</h2>
          <span
            role="status"
            class="text-sm"
            :class="
              isRunning
                ? 'text-[var(--color-success)]'
                : 'text-[var(--text-secondary)]'
            "
            >{{
              runtime
                ? labels[runtime.status] || runtime.status
                : loading
                  ? "读取中"
                  : "状态未知"
            }}</span
          >
          <span
            v-if="runtime?.version"
            class="text-xs text-[var(--text-tertiary)]"
            >{{ runtime.version }}</span
          >
          <ControlRestartSchedule @notify="notify" />
        </div>
        <div class="flex flex-wrap gap-2">
          <Button
            v-if="!isRunning"
            variant="primary"
            :disabled="busy || !configReady"
            :loading="runtimeAction === '启动核心'"
            @click="action(api.startCore, '启动核心')"
            ><Play :size="14" />启动核心</Button
          >
          <Button
            v-else
            :disabled="busy"
            :loading="runtimeAction === '停止核心'"
            @click="action(api.stopCore, '停止核心')"
            ><Square :size="14" />停止核心</Button
          >
          <Button
            :disabled="busy || !isRunning"
            :loading="runtimeAction === '重启核心'"
            @click="action(api.restartCore, '重启核心')"
            ><RotateCcw :size="14" />重启</Button
          >
          <Button
            :disabled="busy || !configReady"
            :loading="runtimeAction === '重载配置'"
            @click="action(api.reloadConfig, '重载配置')"
            ><RefreshCw :size="14" />重载配置</Button
          >
          <ControlUpdateDialog
            :current-version="runtime?.version"
            :is-running="isRunning"
            :disabled="busy && !installing"
            @notify="notify"
            @busy="installing = $event"
            @installed="refresh"
          />
        </div>
      </div>
      <div
        v-if="notInstalled || noConfig"
        class="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-lg)] bg-[var(--color-warning-bg)] p-3 text-sm text-[var(--color-warning)]"
      >
        <span>{{
          notInstalled
            ? "sing-box 未安装，请点击检查更新安装核心。"
            : "当前没有配置文件，请生成配置后启动。"
        }}</span>
        <Button
          v-if="!notInstalled"
          :disabled="busy"
          @click="action(api.generateDefaultConfig, '生成默认配置')"
          >生成默认配置</Button
        >
      </div>
      <div
        class="mt-5 grid gap-4 border-t border-[var(--border-light)] pt-4 lg:grid-cols-3"
      >
        <label class="min-w-0 text-xs text-[var(--text-secondary)]"
          >当前配置文件
          <select
            v-model="selectedConfig"
            class="aw-input mt-2 w-full"
            :disabled="busy || notInstalled || !configFiles.length"
            @change="changeActiveConfig"
          >
            <option value="" disabled>选择配置文件</option>
            <option
              v-for="item in configFiles"
              :key="item.path"
              :value="item.name"
              :disabled="item.validated && !item.valid"
            >
              {{ item.name
              }}{{ item.validated && !item.valid ? "（校验失败）" : "" }}
            </option>
          </select>
        </label>
        <fieldset
          v-for="group in modeGroups"
          :key="group.key"
          :disabled="
            busy ||
            isRunning ||
            !configReady ||
            !(group.key === 'proxy' ? proxyMode : inboundMode)
          "
        >
          <legend class="text-xs text-[var(--text-secondary)]">
            {{ group.label }}
          </legend>
          <div
            class="mt-2 grid grid-cols-3 gap-1 rounded-[var(--radius-lg)] bg-[var(--bg-base)] p-1"
          >
            <label
              v-for="[value, label] in group.options"
              :key="value"
              class="relative flex h-8 items-center justify-center rounded-[var(--radius-md)] text-xs has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50 has-[:focus-visible]:outline has-[:focus-visible]:outline-[var(--color-primary)]"
              :class="
                (group.key === 'proxy' ? proxyMode : inboundMode) === value
                  ? 'bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                  : 'text-[var(--text-secondary)]'
              "
            >
              <input
                type="radio"
                :name="group.key"
                class="sr-only"
                :checked="
                  (group.key === 'proxy' ? proxyMode : inboundMode) === value
                "
                @change="changeMode(group.key, value)"
              />{{ label }}
            </label>
          </div>
        </fieldset>
      </div>
      <p class="mt-3 text-xs text-[var(--text-tertiary)]">
        {{
          isRunning
            ? "停止核心后可修改模式。"
            : "TUN 接管系统流量；Mixed 供客户端设置 HTTP / SOCKS 代理。"
        }}
      </p>
    </section>

    <div
      class="grid items-stretch gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(280px,1fr)]"
    >
      <ControlLiveStatus :is-running="isRunning" />
      <ControlNetworkProbes @message="notify" />
    </div>

    <div class="border-t border-[var(--border-light)] pt-2">
      <button
        class="flex w-full items-center justify-between gap-3 py-3 text-left text-sm text-[var(--text-secondary)]"
        :aria-expanded="advancedOpen"
        aria-controls="control-advanced"
        @click="toggleAdvanced"
      >
        <span
          >高级选项<span
            v-if="installing"
            class="ml-2 text-xs text-[var(--color-primary)]"
            >更新安装中</span
          ><span class="mt-1 block text-xs text-[var(--text-tertiary)]"
            >网络维护、代理认证</span
          ></span
        >
        <ChevronDown
          :size="16"
          class="shrink-0 transition-transform"
          :class="advancedOpen && 'rotate-180'"
        />
      </button>
      <div
        id="control-advanced"
        v-show="advancedOpen"
        class="grid items-start gap-4 lg:grid-cols-2"
      >
        <ControlMaintenance
          v-if="advancedLoaded"
          :is-running="isRunning"
          :platform="runtime?.platform"
          :disabled="busy"
          @notify="notify"
          @busy="runtimeAction = $event"
        />
        <ControlMixedProxyCard
          v-if="advancedLoaded"
          :proxy-port="runtime?.proxy_port || 0"
          :inbound-mode="inboundMode"
          @message="notify"
        />
      </div>
    </div>
  </div>
</template>
