<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Download, RefreshCw } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type {
  InstallerProgressData,
  InstallStateResponse,
} from "@/services/types";

const props = defineProps<{
  currentVersion?: string;
  isRunning: boolean;
  disabled: boolean;
}>();
const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
  busy: [value: boolean];
  installed: [];
}>();
const state = ref<InstallStateResponse | null>(null);
const progress = ref<InstallerProgressData | null>(null);
const loading = ref(false);
const starting = ref(false);
const error = ref("");
const confirmOpen = ref(false);
let timer: number | undefined;
let disposed = false;
let revision = 0;
const installing = computed(() =>
  ["downloading", "extracting"].includes(state.value?.status || ""),
);
const busy = computed(() => starting.value || installing.value);
const percent = computed(() =>
  Math.min(
    100,
    Math.max(0, progress.value?.percent || state.value?.progress || 0),
  ),
);
const current = computed(() => state.value?.version || props.currentVersion);
const statusLabels = {
  idle: "未安装",
  downloading: "下载中",
  extracting: "安装中",
  done: "已安装",
  failed: "安装失败",
};

function compareVersions(candidate: string, current: string) {
  const compareNumeric = (left: string, right: string) => {
    const a = left.replace(/^0+/, "") || "0";
    const b = right.replace(/^0+/, "") || "0";
    return a.length !== b.length
      ? a.length - b.length
      : a === b
        ? 0
        : a < b
          ? -1
          : 1;
  };
  const parse = (value: string) => {
    const normalized = value.replace(/^v/, "").split("+", 1)[0];
    const separator = normalized.indexOf("-");
    return {
      core: (separator < 0 ? normalized : normalized.slice(0, separator)).split(
        ".",
      ),
      prerelease:
        separator < 0 ? [] : normalized.slice(separator + 1).split("."),
    };
  };
  const left = parse(candidate),
    right = parse(current);
  for (let i = 0; i < Math.max(left.core.length, right.core.length); i++) {
    const difference = compareNumeric(
      left.core[i] || "0",
      right.core[i] || "0",
    );
    if (difference) return difference;
  }
  if (!left.prerelease.length || !right.prerelease.length) {
    return left.prerelease.length === right.prerelease.length
      ? 0
      : left.prerelease.length
        ? -1
        : 1;
  }
  for (
    let i = 0;
    i < Math.max(left.prerelease.length, right.prerelease.length);
    i++
  ) {
    const a = left.prerelease[i],
      b = right.prerelease[i];
    if (a === undefined || b === undefined) return a === undefined ? -1 : 1;
    if (a === b) continue;
    const aNumeric = /^\d+$/.test(a),
      bNumeric = /^\d+$/.test(b);
    if (aNumeric && bNumeric) return compareNumeric(a, b);
    if (aNumeric !== bNumeric) return aNumeric ? -1 : 1;
    return a < b ? -1 : 1;
  }
  return 0;
}
const updateAvailable = computed(
  () =>
    !!state.value?.latest_version &&
    !!current.value &&
    compareVersions(state.value.latest_version, current.value) > 0,
);
const installLabel = computed(() =>
  updateAvailable.value
    ? `更新至 ${state.value?.latest_version}`
    : current.value
      ? "重新安装 sing-box"
      : "安装 sing-box",
);

function applyState(value: InstallStateResponse) {
  const previous = state.value;
  state.value = value;
  if (["done", "failed"].includes(value.status)) progress.value = null;
  if (value.error && value.error !== previous?.error)
    emit("notify", `核心安装: ${value.error}`, "error");
  if (
    value.status === "done" &&
    (starting.value ||
      ["downloading", "extracting"].includes(previous?.status || ""))
  ) {
    if (!value.error) emit("notify", "核心安装完成");
    emit("installed");
  }
}
async function load() {
  if (loading.value) return;
  loading.value = true;
  const run = revision;
  try {
    const value = await api.getInstallerStatus();
    if (!disposed && run === revision) {
      applyState(value);
      error.value = value.latest_version
        ? ""
        : "无法获取 sing-box 最新版本，请重新检查";
    }
  } catch (cause: any) {
    if (!disposed) error.value = `安装状态读取失败: ${cause.message}`;
  } finally {
    loading.value = false;
  }
}
async function poll() {
  await load();
  if (!disposed && installing.value) timer = window.setTimeout(poll, 1500);
}
async function install() {
  if (
    busy.value ||
    props.disabled ||
    loading.value ||
    !state.value?.latest_version
  )
    return;
  confirmOpen.value = false;
  starting.value = true;
  const run = revision;
  try {
    await api.install();
    if (disposed) return;
    if (run === revision)
      applyState({
        ...state.value,
        status: "downloading",
        progress: 0,
        error: "",
      });
    emit("notify", "核心安装任务已启动", "info");
  } catch (cause: any) {
    emit("notify", `核心安装启动失败: ${cause.message}`, "error");
  } finally {
    starting.value = false;
  }
}
watch(busy, (value) => emit("busy", value), { immediate: true });
watch(installing, (active) => {
  clearTimeout(timer);
  if (active) timer = window.setTimeout(poll, 1500);
});
useRealtimeSocket((event) => {
  if (event.type === "installer.progress")
    progress.value = event.data as InstallerProgressData;
  else if (event.type === "installer.status") {
    revision++;
    applyState({ ...state.value, ...(event.data as InstallStateResponse) });
  }
});
onMounted(load);
onBeforeUnmount(() => {
  disposed = true;
  clearTimeout(timer);
  emit("busy", false);
});
</script>

<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold">sing-box</h3>
      <Button
        size="sm"
        :loading="loading"
        :disabled="busy || disabled"
        @click="load"
        ><RefreshCw :size="13" />检查更新</Button
      >
    </div>
    <p class="mt-3 text-xs text-[var(--text-secondary)]">
      {{ state ? statusLabels[state.status] : loading ? "读取中" : "状态未知" }}
      · 当前 {{ current || "--" }} · 最新 {{ state?.latest_version || "--" }}
    </p>
    <p
      v-if="state?.latest_version && !installing"
      class="mt-2 text-sm text-[var(--text-secondary)]"
    >
      {{
        updateAvailable
          ? "发现新版本，是否安装更新？"
          : current
            ? "没有新版本，是否重新安装 sing-box？"
            : "尚未安装 sing-box，是否立即安装？"
      }}
    </p>
    <div v-if="installing" class="mt-3">
      <progress
        class="h-2 w-full accent-[var(--color-primary)]"
        :value="percent"
        max="100"
        :aria-label="`安装进度 ${percent.toFixed(1)}%`"
      />
      <span class="text-xs text-[var(--text-secondary)]"
        >{{ percent.toFixed(1) }}%</span
      >
    </div>
    <p
      v-if="error || state?.error"
      role="alert"
      class="mt-3 break-words text-xs text-[var(--color-error)]"
    >
      {{ error || state?.error }}
    </p>
    <Button
      class="mt-4"
      size="sm"
      :loading="busy"
      :disabled="disabled || loading || !state?.latest_version"
      @click="confirmOpen = true"
      ><Download :size="14" />{{ installLabel }}</Button
    >
  </section>
  <ConfirmDialog
    :open="confirmOpen"
    :title="installLabel"
    :message="`将安装 sing-box ${state?.latest_version || ''}${current ? `（当前 ${current}）` : ''}。${isRunning ? '核心会短暂停止，安装完成后自动恢复运行。' : ''}`"
    :confirm-text="current && !updateAvailable ? '确认重新安装' : '确认安装'"
    above-modal
    @confirm="install"
    @cancel="confirmOpen = false"
  />
</template>
