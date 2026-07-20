<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type { CoreLogEntry, ToolLogEntry, WSEvent } from "@/services/types";

type LogTab = "core" | "tool";
type LogFilter = "all" | "stdout" | "stderr" | "info" | "error";
interface DisplayLogEntry {
  key: string;
  time: number;
  source: string;
  line: string;
  error: boolean;
}
interface ClearContext<T> {
  clearedIdentities: Set<string>;
  events: T[];
}

const retainedLogLimit = 1000;
const activeTab = ref<LogTab>("core");
const coreLogs = ref<CoreLogEntry[]>([]);
const toolLogs = ref<ToolLogEntry[]>([]);
const logFilter = ref<LogFilter>("all");
const autoScroll = ref(true);
const loadingTabs = reactive<Record<LogTab, boolean>>({
  core: true,
  tool: false,
});
const tabErrors = reactive<Record<LogTab, string>>({ core: "", tool: "" });
const requestGenerations: Record<LogTab, number> = { core: 0, tool: 0 };
const clearingTabs = reactive<Record<LogTab, boolean>>({
  core: false,
  tool: false,
});
let coreClearContext: ClearContext<CoreLogEntry> | null = null;
let toolClearContext: ClearContext<ToolLogEntry> | null = null;
const logPanel = ref<HTMLDivElement | null>(null);
const loading = computed(() => loadingTabs[activeTab.value]);
const error = computed(() => tabErrors[activeTab.value]);
const clearing = computed(() => clearingTabs[activeTab.value]);
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
        key: coreLogIdentity(item),
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
      key: toolLogIdentity(item),
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

function coreLogIdentity(item: CoreLogEntry) {
  return JSON.stringify([item.id, item.time, item.source, item.line]);
}

function toolLogIdentity(item: ToolLogEntry) {
  return JSON.stringify([
    item.id,
    item.time,
    item.level,
    item.tag,
    item.message,
  ]);
}

function mergeLogEntries<T extends { time: number }>(
  first: T[],
  second: T[],
  identity: (item: T) => string,
) {
  const seen = new Set<string>();
  return [...first, ...second]
    .filter((item) => {
      const key = identity(item);
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((left, right) => left.time - right.time)
    .slice(-retainedLogLimit);
}

function invalidateLoad(tab: LogTab) {
  requestGenerations[tab]++;
  loadingTabs[tab] = false;
}

async function loadLogs(tab: LogTab = activeTab.value) {
  const generation = ++requestGenerations[tab];
  loadingTabs[tab] = true;
  tabErrors[tab] = "";
  try {
    if (tab === "core") {
      const existingIdentities = new Set(coreLogs.value.map(coreLogIdentity));
      const snapshot = await api.getCoreLogs(retainedLogLimit);
      if (generation !== requestGenerations[tab]) return;
      const liveEntries = coreLogs.value.filter(
        (entry) => !existingIdentities.has(coreLogIdentity(entry)),
      );
      coreLogs.value = mergeLogEntries(
        snapshot,
        liveEntries,
        coreLogIdentity,
      );
    } else {
      const existingIdentities = new Set(toolLogs.value.map(toolLogIdentity));
      const snapshot = await api.getToolLogs(retainedLogLimit);
      if (generation !== requestGenerations[tab]) return;
      const liveEntries = toolLogs.value.filter(
        (entry) => !existingIdentities.has(toolLogIdentity(entry)),
      );
      toolLogs.value = mergeLogEntries(
        snapshot,
        liveEntries,
        toolLogIdentity,
      );
    }
  } catch (e: any) {
    if (generation === requestGenerations[tab])
      tabErrors[tab] = e.message || "日志加载失败";
  } finally {
    if (generation === requestGenerations[tab]) loadingTabs[tab] = false;
  }
  if (
    generation === requestGenerations[tab] &&
    activeTab.value === tab &&
    autoScroll.value
  )
    await scrollToLatest();
}

async function clearLogs() {
  const tab = activeTab.value;
  if (clearingTabs[tab]) return;
  invalidateLoad(tab);
  clearingTabs[tab] = true;
  tabErrors[tab] = "";
  try {
    if (tab === "core") {
      coreClearContext = {
        clearedIdentities: new Set(coreLogs.value.map(coreLogIdentity)),
        events: [],
      };
      await api.clearCoreLogs();
      const context = coreClearContext;
      coreClearContext = null;
      coreLogs.value = mergeLogEntries(
        [],
        context.events.filter(
          (entry) => !context.clearedIdentities.has(coreLogIdentity(entry)),
        ),
        coreLogIdentity,
      );
    } else {
      toolClearContext = {
        clearedIdentities: new Set(toolLogs.value.map(toolLogIdentity)),
        events: [],
      };
      await api.clearToolLogs();
      const context = toolClearContext;
      toolClearContext = null;
      toolLogs.value = mergeLogEntries(
        [],
        context.events.filter(
          (entry) => !context.clearedIdentities.has(toolLogIdentity(entry)),
        ),
        toolLogIdentity,
      );
    }
    await loadLogs(tab);
  } catch (e: any) {
    tabErrors[tab] = e.message || "日志清空失败";
  } finally {
    if (tab === "core") coreClearContext = null;
    else toolClearContext = null;
    clearingTabs[tab] = false;
  }
}

function switchTab(tab: LogTab) {
  if (activeTab.value === tab) return;
  invalidateLoad(activeTab.value);
  activeTab.value = tab;
  logFilter.value = "all";
  void loadLogs(tab);
}

const { connected } = useRealtimeSocket((event: WSEvent) => {
  if (event.type === "core.log") {
    const entry = event.data as CoreLogEntry;
    if (!entry?.line) return;
    coreClearContext?.events.push(entry);
    coreLogs.value = mergeLogEntries(coreLogs.value, [entry], coreLogIdentity);
  } else if (event.type === "tool.log") {
    const entry = event.data as ToolLogEntry;
    if (!entry?.message) return;
    toolClearContext?.events.push(entry);
    toolLogs.value = mergeLogEntries(toolLogs.value, [entry], toolLogIdentity);
  }
});
const hasConnected = ref(connected.value);
const reconnectingAfterDisconnect = ref(false);
const connectionLabel = computed(() => {
  if (connected.value) return "实时连接正常";
  if (hasConnected.value) return "实时连接已断开，正在重连…";
  return "正在建立实时连接…";
});

watch(connected, (isConnected, wasConnected) => {
  if (!isConnected && wasConnected) {
    reconnectingAfterDisconnect.value = true;
    return;
  }
  if (!isConnected) return;
  hasConnected.value = true;
  if (!reconnectingAfterDisconnect.value) return;
  reconnectingAfterDisconnect.value = false;
  void Promise.allSettled([loadLogs("core"), loadLogs("tool")]);
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
onMounted(() => void loadLogs());
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
            :aria-pressed="autoScroll"
            @click="toggleAutoScroll"
          >
            {{ autoScroll ? "跟随中" : "跟随" }}
          </Button>
          <Button size="sm" @click="loadLogs()">刷新</Button>
          <Button
            size="sm"
            variant="danger"
            :loading="clearing"
            @click="clearLogs"
          >
            清空
          </Button>
        </div>
      </div>
      <div
        class="mb-3 flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]"
      >
        <span>已缓存 {{ cachedCount }} 行</span>
        <span>当前显示 {{ displayLogs.length }} 行</span>
        <span
          :class="
            connected
              ? 'text-[var(--color-success)]'
              : hasConnected
                ? 'text-[var(--color-error)]'
                : 'text-[var(--text-tertiary)]'
          "
        >
          {{ connectionLabel }}
        </span>
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
          :key="item.key"
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
