<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  Play,
  Square,
  RotateCcw,
  Download,
  AlertTriangle,
  Power,
  RefreshCw,
  FileCheck2,
  ShieldCheck,
  DatabaseZap,
  Activity,
  AlertCircle,
  CheckCircle2,
  Eraser,
  FileDown,
  ListX,
  XCircle,
} from "lucide-vue-next";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type {
  ConfigFileItem,
  MaintenanceCheckResponse,
  RuntimeResponse,
  WSEvent,
} from "@/services/types";
import Button from "@/components/ui/Button.vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import ControlOverview from "./ControlOverview.vue";
import ControlNetworkOverview from "./ControlNetworkOverview.vue";
import ControlRestartSchedule from "./ControlRestartSchedule.vue";
const runtime = ref<RuntimeResponse | null>(null),
  installStatus = ref<any>(null),
  configStatus = ref<any>(null),
  configFiles = ref<ConfigFileItem[]>([]),
  selectedConfig = ref(""),
  configChanging = ref(false),
  installProgress = ref<any>(null),
  guideDismissed = ref(false),
  inboundMode = ref("tun_mixed"),
  proxyMode = ref("rule"),
  modeChanging = ref(false),
  message = ref(""),
  messageType = ref<"success" | "error" | "info">("success"),
  refreshKey = ref(0),
  runtimeAction = ref(""),
  confirmFirewallReset = ref(false),
  confirmLogClear = ref(false),
  maintenanceReport = ref<MaintenanceCheckResponse | null>(null);
const labels: any = {
    running: "运行中",
    starting: "启动中",
    stopping: "停止中",
    stopped: "已停止",
    error: "异常",
    not_installed: "未安装",
    no_config: "无配置",
  },
  inboundLabels: any = {
    tun: "TUN 模式",
    mixed: "Mixed 模式",
    tun_mixed: "TUN + Mixed",
  },
  proxyLabels: any = {
    global: "全局模式",
    rule: "规则模式",
    direct: "直连模式",
  };
const rt = computed(() => runtime.value?.status || "not_installed"),
  isRunning = computed(() => rt.value === "running"),
  notInstalled = computed(
    () => runtime.value !== null && rt.value === "not_installed",
  ),
  noConfig = computed(() => rt.value === "no_config"),
  installing = computed(() =>
    ["downloading", "extracting"].includes(installStatus.value?.status),
  ),
  isWindows = computed(() => runtime.value?.platform === "windows"),
  systemDNSUnsupported = computed(
    () =>
      !!runtime.value?.platform &&
      !["windows", "linux"].includes(runtime.value.platform),
  ),
  unsupported = computed(() => !!runtime.value?.platform && !isWindows.value),
  currentVersion = computed(
    () => runtime.value?.version || installStatus.value?.version,
  ),
  latestVersion = computed(() => installStatus.value?.latest_version);
function newer(a?: string, b?: string) {
  if (!a || !b) return false;
  const p = (v: string) =>
    v.replace(/^v/, "").split(/[+-]/, 1)[0].split(".").map(Number);
  for (let i = 0; i < 3; i++)
    if ((p(a)[i] || 0) !== (p(b)[i] || 0))
      return (p(a)[i] || 0) > (p(b)[i] || 0);
  return false;
}
const updateAvailable = computed(() =>
    newer(latestVersion.value, currentVersion.value),
  ),
  showInstallGuide = computed(
    () =>
      !guideDismissed.value &&
      runtime.value !== null &&
      (notInstalled.value || installing.value),
  ),
  showConfigGuide = computed(
    () =>
      !guideDismissed.value &&
      !notInstalled.value &&
      !installing.value &&
      configStatus.value?.has_config === false,
  ),
  progress = computed(() => ({
    percent: Math.max(
      installProgress.value?.percent || 0,
      installStatus.value?.progress || 0,
    ),
    downloaded_bytes: installProgress.value?.downloaded_bytes || 0,
    total_bytes: installProgress.value?.total_bytes || 0,
  }));
let installTimer: number,
  initialRun = 0,
  cancelled = false;
function notify(v: string, t: "success" | "error" | "info" = "success") {
  message.value = v;
  messageType.value = t;
}
async function initial() {
  const run = ++initialRun;
  const initialResults = await Promise.allSettled([
    api.getRuntime(),
    api.getInstallerStatus(),
  ]);
  if (cancelled || run !== initialRun) return;
  if (initialResults[0].status === "fulfilled")
    runtime.value = initialResults[0].value;
  if (initialResults[1].status === "fulfilled")
    installStatus.value = initialResults[1].value;
  const initialLabels = ["运行状态", "安装状态"];
  const initialFailures = initialResults
    .map((result, index) =>
      result.status === "rejected"
        ? `${initialLabels[index]}: ${result.reason?.message || "请求失败"}`
        : "",
    )
    .filter(Boolean);
  if (initialFailures.length)
    notify(`控制面板部分状态加载失败: ${initialFailures.join("；")}`, "error");
  if (!runtime.value || runtime.value.status === "not_installed") return;

  const localResults = await Promise.allSettled([
    api.getConfigStatus().then((value) => {
      if (!cancelled && run === initialRun) configStatus.value = value;
    }),
    api.getConfigFiles().then((value) => {
      if (cancelled || run !== initialRun) return;
      configFiles.value = value;
      selectedConfig.value = value.find((item) => item.active)?.name || "";
    }),
    api.getInboundMode().then((value) => {
      if (!cancelled && run === initialRun) inboundMode.value = value.mode;
    }),
    api.getProxyMode().then((value) => {
      if (!cancelled && run === initialRun) proxyMode.value = value.mode;
    }),
  ]);
  if (cancelled || run !== initialRun) return;
  const labels = ["配置状态", "配置文件", "运行模式", "代理模式"];
  const failures = localResults
    .map((result, index) =>
      result.status === "rejected"
        ? `${labels[index]}: ${result.reason?.message || "请求失败"}`
        : "",
    )
    .filter(Boolean);
  if (failures.length)
    notify(`控制面板部分数据加载失败: ${failures.join("；")}`, "error");
}
async function changeActiveConfig() {
  const previous = configFiles.value.find((item) => item.active)?.name || "";
  const next = selectedConfig.value;
  if (!next || next === previous || configChanging.value) return;
  const wasRunning = isRunning.value;
  configChanging.value = true;
  try {
    configStatus.value = await api.setActiveConfig(next);
    configFiles.value = configFiles.value.map((item) => ({
      ...item,
      active: item.name === next,
    }));
    api
      .getRuntime()
      .then((value) => (runtime.value = value))
      .catch(() => {});
    notify(
      wasRunning
        ? `当前配置已切换为 ${next}，点击“重载配置”后生效`
        : `当前配置已切换为 ${next}`,
      wasRunning ? "info" : "success",
    );
  } catch (e: any) {
    selectedConfig.value = previous;
    notify(`切换配置失败: ${e.message}`, "error");
  } finally {
    configChanging.value = false;
  }
}
async function action(fn: () => Promise<any>, label: string) {
  if (runtimeAction.value) return;
  runtimeAction.value = label;
  try {
    const r = await fn();
    notify(`${label} 成功${r?.message ? `: ${r.message}` : ""}`);
  } catch (e: any) {
    notify(`${label} 失败: ${e.message}`, "error");
  } finally {
    runtimeAction.value = "";
  }
  setTimeout(() => {
    api
      .getRuntime()
      .then((r) => (runtime.value = r))
      .catch(() => {});
    api
      .getConfigStatus()
      .then((r) => (configStatus.value = r))
      .catch(() => {});
    api
      .getInstallerStatus()
      .then((r) => (installStatus.value = r))
      .catch(() => {});
  }, 1000);
}

async function runNetworkCheck() {
  if (runtimeAction.value) return;
  runtimeAction.value = "网络自检";
  try {
    maintenanceReport.value = await api.networkCheck();
    notify(
      maintenanceReport.value.success
        ? "网络自检通过"
        : "网络自检发现需要处理的项目",
      maintenanceReport.value.success ? "success" : "info",
    );
  } catch (e: any) {
    notify(`网络自检失败: ${e.message}`, "error");
  } finally {
    runtimeAction.value = "";
  }
}

async function exportDiagnostics() {
  if (runtimeAction.value) return;
  runtimeAction.value = "导出诊断";
  try {
    const report = await api.getDiagnostics();
    const blob = new Blob([JSON.stringify(report, null, 2)], {
      type: "application/json;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `ackwrap-diagnostics-${new Date(report.generated_at)
      .toISOString()
      .replace(/[:.]/g, "-")}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
    notify("脱敏诊断报告已导出");
  } catch (e: any) {
    notify(`导出诊断失败: ${e.message}`, "error");
  } finally {
    runtimeAction.value = "";
  }
}
async function install(label: string) {
  try {
    await api.install();
    installStatus.value = {
      ...installStatus.value,
      status: "downloading",
      progress: 0,
      message: "preparing download",
    };
    installProgress.value = null;
    notify(`${label}任务已启动`, "info");
  } catch (e: any) {
    notify(`${label}启动失败: ${e.message}`, "error");
  }
}
async function changeMode(kind: "inbound" | "proxy", mode: string) {
  if (isRunning.value || modeChanging.value) return;
  modeChanging.value = true;
  try {
    if (kind === "inbound") {
      await api.setInboundMode(mode);
      inboundMode.value = mode;
    } else {
      await api.setProxyMode(mode);
      proxyMode.value = mode;
    }
    notify(
      `${kind === "inbound" ? "运行" : "代理"}模式已切换为 ${(kind === "inbound" ? inboundLabels : proxyLabels)[mode]}，配置已自动生成并应用`,
    );
  } catch (e: any) {
    notify(`切换模式失败: ${e.message}`, "error");
  } finally {
    modeChanging.value = false;
  }
}
useRealtimeSocket((e: WSEvent) => {
  const d: any = e.data;
  if (e.type === "runtime.status")
    runtime.value = { ...runtime.value, ...d } as RuntimeResponse;
  else if (e.type === "installer.progress") installProgress.value = d;
  else if (e.type === "installer.status") {
    installStatus.value = { ...installStatus.value, ...d };
    if (["done", "failed"].includes(d.status)) installProgress.value = null;
    if (d.status === "failed")
      notify(`安装失败: ${d.error || "请查看安装状态详情"}`, "error");
    if (d.status === "done") void initial();
  } else if (e.type === "core.status") {
    runtime.value = {
      ...runtime.value,
      status: d.status,
      pid: d.pid || 0,
    } as RuntimeResponse;
    if (d.status === "error" && d.error)
      notify(`核心异常: ${d.error}`, "error");
  } else if (e.type === "config.status") {
    configStatus.value = d;
    if (d.file_name) selectedConfig.value = d.file_name;
  } else if (e.type === "core.restart_schedule") {
    if (d.status === "succeeded") notify("核心定时重启完成");
    else if (d.status === "failed")
      notify(`核心定时重启失败: ${d.error || "未知错误"}`, "error");
  } else if (
    e.type === "subscription.sync" &&
    ["updated", "failed"].includes(d?.status)
  )
    refreshKey.value++;
  else if (
    [
      "subscription.sync_all",
      "route_rule_subscription.sync_all",
      "geo.sync_all",
    ].includes(e.type)
  ) {
    refreshKey.value++;
    notify(
      `${e.type}完成${d.failed ? `，${d.failed}/${d.total || 0} 项失败` : `，共 ${d.total || 0} 项`}`,
      d.failed ? "error" : "success",
    );
  }
});
watch(installing, (v) => {
  clearInterval(installTimer);
  if (v)
    installTimer = window.setInterval(async () => {
      try {
        const s = await api.getInstallerStatus();
        installStatus.value = s;
        if (["done", "failed"].includes(s.status)) installProgress.value = null;
        if (s.status === "done") void initial();
      } catch (e: any) {
        const m = `安装状态查询失败，后端连接已断开: ${e?.message || "连接被重置"}`;
        installStatus.value = {
          ...installStatus.value,
          status: "failed",
          error: m,
        };
        notify(m, "error");
      }
    }, 1000);
});
const modes = (kind: "proxy" | "inbound") =>
  kind === "proxy"
    ? [
        ["rule", "规则模式"],
        ["global", "全局模式"],
        ["direct", "直连模式"],
      ]
    : [
        ["tun_mixed", "TUN + Mixed"],
        ["tun", "TUN 模式"],
        ["mixed", "Mixed 模式"],
      ];
onMounted(initial);
onBeforeUnmount(() => {
  cancelled = true;
  clearInterval(installTimer);
});
</script>
<template>
  <div class="flex h-full flex-col">
    <Toast
      :message="message"
      :type="messageType"
      @dismiss="message = ''"
    /><ConfirmDialog
      :open="confirmFirewallReset"
      title="重置 Windows 防火墙"
      message="此操作会清除并恢复系统防火墙规则，可能影响其他应用的网络访问。确认继续吗？"
      confirm-text="确认重置"
      danger
      @cancel="confirmFirewallReset = false"
      @confirm="
        confirmFirewallReset = false;
        action(api.resetFirewall, '重置防火墙');
      "
    /><ConfirmDialog
      :open="confirmLogClear"
      title="清理核心日志"
      message="此操作会清空当前内存中的核心日志，且无法恢复。确认继续吗？"
      confirm-text="确认清理"
      danger
      @cancel="confirmLogClear = false"
      @confirm="
        confirmLogClear = false;
        action(api.clearCoreLogs, '清理核心日志');
      "
    /><Modal
      :open="!!maintenanceReport"
      title="网络自检结果"
      size="md"
      @close="maintenanceReport = null"
    >
      <div class="space-y-2">
        <div
          v-for="check in maintenanceReport?.checks || []"
          :key="check.key"
          class="flex items-start gap-3 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
        >
          <CheckCircle2
            v-if="check.status === 'pass'"
            :size="17"
            class="mt-0.5 shrink-0 text-[var(--color-success)]"
          />
          <AlertCircle
            v-else-if="check.status === 'warn'"
            :size="17"
            class="mt-0.5 shrink-0 text-[var(--color-warning)]"
          />
          <XCircle
            v-else
            :size="17"
            class="mt-0.5 shrink-0 text-[var(--color-error)]"
          />
          <div>
            <div class="font-medium text-[var(--text-primary)]">
              {{ check.label }}
            </div>
            <p class="mt-0.5 text-xs text-[var(--text-secondary)]">
              {{ check.message }}
            </p>
          </div>
        </div>
      </div> </Modal
    ><PageHeader title="控制面板" />
    <div
      v-if="showInstallGuide || showConfigGuide"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4"
    >
      <div
        class="w-full max-w-md rounded-[var(--radius-xl)] border bg-[var(--bg-surface)] p-5"
      >
        <div class="flex gap-3">
          <AlertTriangle :size="20" />
          <div>
            <h3 class="font-semibold">
              {{ showInstallGuide ? "需要安装 sing-box" : "需要生成默认配置" }}
            </h3>
            <p class="text-sm">
              {{
                showInstallGuide
                  ? "控制面板检测到 sing-box 未安装，请先安装核心程序。"
                  : "已检测到 sing-box，但当前没有可用配置文件，是否生成默认配置？"
              }}
            </p>
          </div>
        </div>
        <div v-if="showInstallGuide && installing" class="mt-4">
          <div class="h-2 bg-white/10">
            <div
              class="h-full bg-blue-400"
              :style="{ width: `${Math.min(100, progress.percent)}%` }"
            />
          </div>
          <small>{{ progress.percent.toFixed(1) }}%</small>
        </div>
        <div v-if="installStatus?.error" class="mt-3 text-red-300">
          {{ installStatus.error }}
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <Button variant="ghost" size="sm" @click="guideDismissed = true"
            >稍后处理</Button
          ><Button
            v-if="showInstallGuide"
            size="sm"
            :loading="installing"
            :disabled="installing"
            @click="install('安装')"
            >安装 sing-box</Button
          ><Button
            v-else
            size="sm"
            :disabled="isRunning || !!runtimeAction"
            @click="action(api.generateDefaultConfig, '生成默认配置')"
            >生成默认配置</Button
          >
        </div>
      </div>
    </div>
    <section
      v-if="notInstalled || noConfig"
      class="mt-4 rounded-[var(--radius-xl)] border border-orange-400/20 p-5"
    >
      <h3 class="font-semibold">
        {{ notInstalled ? "sing-box 未安装" : "配置文件不存在" }}
      </h3>
      <p class="text-sm">
        {{
          notInstalled
            ? "点击下方安装按钮下载并安装 sing-box。"
            : "sing-box 已安装但配置文件缺失，请生成默认配置。"
        }}
      </p>
    </section>
    <div
      class="mt-4 grid min-h-[720px] flex-1 items-stretch gap-4 lg:grid-cols-2 xl:grid-cols-12"
    >
      <section
        class="flex h-full flex-col rounded-[var(--radius-xl)] border bg-[var(--bg-surface)] p-5 xl:col-span-4"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="flex min-w-0 gap-3">
            <Power :size="19" class="shrink-0" />
            <div class="min-w-0">
              <h2 class="font-semibold">
                核心控制 <span class="text-xs">{{ labels[rt] }}</span>
              </h2>
              <small>sing-box 进程管理</small>
            </div>
          </div>
          <div
            class="flex min-w-0 max-w-56 flex-1 items-end gap-2 sm:flex-none"
          >
            <ControlRestartSchedule @notify="notify" />
            <label class="min-w-0 flex-1">
              <span class="sr-only">当前配置文件</span>
              <select
                v-model="selectedConfig"
                class="aw-input w-full min-w-32"
                title="选择当前配置文件"
                :disabled="
                  configChanging ||
                  !!runtimeAction ||
                  notInstalled ||
                  !configFiles.length
                "
                @change="changeActiveConfig"
              >
                <option value="" disabled>选择配置文件</option>
                <option
                  v-for="item in configFiles"
                  :key="item.path"
                  :value="item.name"
                  :disabled="!item.valid"
                >
                  {{ item.name }}{{ item.valid ? "" : "（校验失败）" }}
                </option>
              </select>
            </label>
          </div>
        </div>
        <div class="mt-3 grid grid-cols-3 gap-2">
          <div
            v-for="x in [
              ['进程 ID', runtime?.pid || '-'],
              ['核心版本', currentVersion || '--'],
              [
                '配置状态',
                configStatus?.has_config && configStatus.valid
                  ? '校验通过'
                  : '需要处理',
              ],
            ]"
            :key="x[0]"
            class="bg-[var(--bg-base)] px-2 py-1.5"
          >
            <small>{{ x[0] }}</small
            ><b class="block truncate">{{ x[1] }}</b>
          </div>
        </div>
        <div class="mt-3 grid grid-cols-2 gap-2 px-1 sm:grid-cols-4">
          <button
            class="aw-control-action"
            :disabled="!!runtimeAction || isRunning || notInstalled || noConfig"
            @click="action(api.startCore, '启动')"
          >
            <Play :size="13" />启动核心</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || !isRunning"
            @click="action(api.stopCore, '停止')"
          >
            <Square :size="13" />停止核心</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || !isRunning"
            @click="action(api.restartCore, '重启')"
          >
            <RotateCcw :size="13" />重启核心</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || notInstalled || noConfig"
            @click="action(api.reloadConfig, '重载配置')"
          >
            <RefreshCw :size="13" />重载配置
          </button>
        </div>
      </section>
      <section
        v-for="kind in ['proxy', 'inbound'] as const"
        :key="kind"
        class="flex h-full flex-col rounded-[var(--radius-xl)] border bg-[var(--bg-surface)] p-5 xl:col-span-4"
      >
        <h2>{{ kind === "proxy" ? "代理模式" : "运行模式" }}</h2>
        <p class="text-xs">
          {{
            kind === "proxy"
              ? "决定流量使用规则、代理或直连"
              : "TUN 模式在 Linux/OpenWrt 自动接管 LAN 流量；Mixed 模式需要客户端显式设置代理"
          }}
        </p>
        <div
          class="my-auto grid grid-cols-3 gap-1 rounded-[var(--radius-lg)] bg-[var(--bg-base)] p-1"
        >
          <label
            v-for="m in modes(kind)"
            :key="m[0]"
            class="flex h-8 min-w-0 items-center justify-center truncate rounded-[var(--radius-md)] px-2 text-center text-xs transition-colors"
            :class="[
              (kind === 'proxy' ? proxyMode : inboundMode) === m[0]
                ? 'bg-[var(--button-primary-bg)] text-[var(--button-primary-text)] shadow-sm'
                : 'text-[var(--text-secondary)] hover:bg-[var(--button-secondary-hover)]',
              isRunning || modeChanging || !!runtimeAction
                ? 'cursor-not-allowed opacity-50'
                : 'cursor-pointer',
            ]"
            ><input
              type="radio"
              class="sr-only"
              :checked="(kind === 'proxy' ? proxyMode : inboundMode) === m[0]"
              :disabled="isRunning || modeChanging || !!runtimeAction"
              @change="changeMode(kind, m[0])"
            />{{ m[1] }}</label
          >
        </div>
        <small>{{
          isRunning
            ? "停止核心后可修改"
            : `当前模式：${kind === "proxy" ? proxyLabels[proxyMode] : inboundLabels[inboundMode]}`
        }}</small>
      </section>
      <section
        class="flex h-full flex-col rounded-[var(--radius-xl)] border bg-[var(--bg-surface)] p-5 xl:col-span-4"
      >
        <h2>高级维护</h2>
        <p class="text-xs">连接与系统网络维护操作</p>
        <div class="my-auto grid grid-cols-2 gap-2 px-1 sm:grid-cols-3">
          <button
            class="aw-control-action"
            :disabled="!!runtimeAction || !isRunning"
            @click="action(api.closeConnections, '关闭连接')"
          >
            <ShieldCheck :size="13" />关闭连接</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || !isRunning"
            @click="action(api.flushCoreDNS, '清理核心 DNS')"
          >
            <DatabaseZap :size="13" />清理核心 DNS</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || !isRunning"
            @click="action(api.flushFakeIP, '清理 FakeIP')"
          >
            <Eraser :size="13" />清理 FakeIP</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction"
            @click="runNetworkCheck"
          >
            <Activity :size="13" />网络自检</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || systemDNSUnsupported"
            @click="action(api.flushDNS, '清理系统 DNS')"
          >
            <RefreshCw :size="13" />清理系统 DNS</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction"
            @click="confirmLogClear = true"
          >
            <ListX :size="13" />清理核心日志</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction"
            @click="exportDiagnostics"
          >
            <FileDown :size="13" />导出诊断</button
          ><button
            class="aw-control-action"
            :disabled="!!runtimeAction || notInstalled"
            @click="action(api.checkUpdate, '检查更新')"
          >
            <FileCheck2 :size="13" />检查更新</button
          ><button
            class="aw-control-action aw-action-danger"
            :disabled="!!runtimeAction || unsupported"
            @click="confirmFirewallReset = true"
          >
            <AlertTriangle :size="13" />重置防火墙
          </button>
        </div>
      </section>
      <ControlOverview
        :refresh-key="refreshKey"
        :config-status="configStatus"
        :proxy-mode="proxyMode"
        @message="notify"
        @resources-changed="
          refreshKey++;
          api.getConfigStatus().then((r) => (configStatus = r));
        "
        ><template #installation
          ><h3>安装信息</h3>
          <div class="grid grid-cols-3 gap-2">
            <div>
              状态<br /><b>{{ installStatus?.status || "未安装" }}</b>
            </div>
            <div>
              当前<br /><b>{{ currentVersion || "--" }}</b>
            </div>
            <div>
              最新<br /><b>{{ latestVersion || "--" }}</b>
            </div>
          </div>
          <div v-if="installing" class="mt-3">
            {{ progress.percent.toFixed(1) }}%
          </div>
          <Button
            class="mt-3"
            full-width
            size="sm"
            :disabled="installing || isRunning"
            :loading="installing"
            @click="install(updateAvailable ? '更新' : '安装')"
            ><Download :size="14" />{{
              updateAvailable
                ? `更新至 ${latestVersion}`
                : installStatus?.status === "done"
                  ? "重新安装"
                  : "安装"
            }}</Button
          ></template
        ></ControlOverview
      ><ControlNetworkOverview
        :is-running="isRunning"
        :proxy-port="runtime?.proxy_port || 0"
        @message="notify"
      />
    </div>
  </div>
</template>
