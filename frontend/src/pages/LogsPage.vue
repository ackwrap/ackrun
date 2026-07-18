<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type { CoreLogEntry, ToolLogEntry, WSEvent } from "@/services/types";

type LogTab = "core" | "tool";
type LogFilter = "all" | "stdout" | "stderr" | "info" | "error";
interface DisplayLogEntry {
  id: number;
  time: number;
  source: string;
  line: string;
  error: boolean;
}

const retainedLogLimit = 1000;
const activeTab = ref<LogTab>("core");
const coreLogs = ref<CoreLogEntry[]>([]);
const toolLogs = ref<ToolLogEntry[]>([]);
const logFilter = ref<LogFilter>("all");
const autoScroll = ref(false);
const loading = ref(true);
const error = ref("");
const logPanel = ref<HTMLDivElement | null>(null);
const filters = computed<LogFilter[]>(() =>
  activeTab.value === "core"
    ? ["all", "stdout", "stderr"]
    : ["all", "info", "error"],
);
const displayLogs = computed<DisplayLogEntry[]>(() => {
  if (activeTab.value === "core") {
    return coreLogs.value
      .filter(
        (item) => logFilter.value === "all" || item.source === logFilter.value,
      )
      .map((item) => ({
        id: item.id,
        time: item.time,
        source: item.source,
        line: item.line,
        error: item.source === "stderr",
      }));
  }
  return toolLogs.value
    .filter(
      (item) => logFilter.value === "all" || item.level === logFilter.value,
    )
    .map((item) => ({
      id: item.id,
      time: item.time,
      source: item.level,
      line: `[${item.tag}] ${item.message}`,
      error: item.level === "error",
    }));
});
const cachedCount = computed(() =>
  activeTab.value === "core" ? coreLogs.value.length : toolLogs.value.length,
);
const formatTime = (value: number) =>
  value ? new Date(value).toLocaleTimeString() : "--:--:--";

async function loadLogs() {
  loading.value = true;
  error.value = "";
  try {
    if (activeTab.value === "core") coreLogs.value = await api.getCoreLogs(500);
    else toolLogs.value = await api.getToolLogs(500);
  } catch (e: any) {
    error.value = e.message || "日志加载失败";
  } finally {
    loading.value = false;
  }
}

async function clearLogs() {
  error.value = "";
  try {
    if (activeTab.value === "core") {
      await api.clearCoreLogs();
      coreLogs.value = [];
    } else {
      await api.clearToolLogs();
      toolLogs.value = [];
    }
  } catch (e: any) {
    error.value = e.message || "日志清空失败";
  }
}

function switchTab(tab: LogTab) {
  if (activeTab.value === tab) return;
  activeTab.value = tab;
  logFilter.value = "all";
  autoScroll.value = false;
  void loadLogs();
}

useRealtimeSocket((event: WSEvent) => {
  if (event.type !== "core.log") return;
  const entry = event.data as CoreLogEntry;
  if (!entry?.line) return;
  const nextLogs = [...coreLogs.value, entry];
  coreLogs.value =
    autoScroll.value && nextLogs.length > retainedLogLimit
      ? nextLogs.slice(-retainedLogLimit)
      : nextLogs;
});

async function scrollToLatest() {
  await nextTick();
  if (logPanel.value) logPanel.value.scrollTop = logPanel.value.scrollHeight;
}

async function toggleAutoScroll() {
  if (autoScroll.value) {
    autoScroll.value = false;
    return;
  }
  if (activeTab.value === "core" && coreLogs.value.length > retainedLogLimit)
    coreLogs.value = coreLogs.value.slice(-retainedLogLimit);
  if (activeTab.value === "tool" && toolLogs.value.length > retainedLogLimit)
    toolLogs.value = toolLogs.value.slice(-retainedLogLimit);
  autoScroll.value = true;
  await scrollToLatest();
}

function pauseAutoScroll() {
  autoScroll.value = false;
}

function handleLogScroll() {
  const panel = logPanel.value;
  if (
    autoScroll.value &&
    panel &&
    panel.scrollHeight - panel.scrollTop - panel.clientHeight > 24
  )
    autoScroll.value = false;
}

watch(displayLogs, () => {
  if (autoScroll.value) void scrollToLatest();
});
onMounted(loadLogs);
</script>

<template>
  <div class="space-y-4">
    <PageHeader
      title="日志"
      description="查看 sing-box 核心输出与 Ackwrap 工具运行日志。"
    />
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
    >
      <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div
          class="flex items-center gap-2"
          role="tablist"
          aria-label="日志类型"
        >
          <button
            class="aw-filter-chip"
            :class="{ active: activeTab === 'core' }"
            role="tab"
            :aria-selected="activeTab === 'core'"
            @click="switchTab('core')"
          >
            核心日志
          </button>
          <button
            class="aw-filter-chip"
            :class="{ active: activeTab === 'tool' }"
            role="tab"
            :aria-selected="activeTab === 'tool'"
            @click="switchTab('tool')"
          >
            工具日志
          </button>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-for="item in filters"
            :key="item"
            class="aw-filter-chip"
            :class="{ active: logFilter === item }"
            @click="logFilter = item"
          >
            {{ item === "all" ? "全部" : item }}
          </button>
          <Button
            size="sm"
            :variant="autoScroll ? 'primary' : 'secondary'"
            @click="toggleAutoScroll"
          >
            {{ autoScroll ? "跟随最新" : "定屏" }}
          </Button>
          <Button size="sm" @click="loadLogs">刷新</Button>
          <Button size="sm" variant="danger" @click="clearLogs">清空</Button>
        </div>
      </div>
      <div
        class="mb-3 flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]"
      >
        <span>已缓存 {{ cachedCount }} 行</span>
        <span>当前显示 {{ displayLogs.length }} 行</span>
        <span v-if="error" class="text-[var(--color-error)]">{{ error }}</span>
      </div>
      <div
        ref="logPanel"
        class="max-h-[calc(100vh-260px)] overflow-y-auto rounded-lg border border-[var(--border-default)] bg-[var(--bg-primary)] p-3 font-mono text-xs text-[var(--text-tertiary)]"
        role="tabpanel"
        @scroll.passive="handleLogScroll"
        @selectstart="pauseAutoScroll"
      >
        <div v-if="loading" class="py-8 text-center">加载日志...</div>
        <div v-else-if="!displayLogs.length" class="py-8 text-center">
          等待日志...
        </div>
        <div
          v-for="item in displayLogs"
          v-else
          :key="item.id"
          class="grid grid-cols-[82px_58px_minmax(0,1fr)] gap-2 whitespace-pre-wrap break-all py-0.5 hover:bg-[var(--table-row-hover)]"
        >
          <span class="text-[var(--text-muted)]">{{
            formatTime(item.time)
          }}</span>
          <span
            :class="
              item.error
                ? 'text-[var(--color-error)]'
                : 'text-[var(--color-primary)]'
            "
          >
            {{ item.source }}
          </span>
          <span class="text-[var(--text-secondary)]">{{ item.line }}</span>
        </div>
      </div>
    </section>
  </div>
</template>
