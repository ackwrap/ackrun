<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type { CoreLogEntry, WSEvent } from "@/services/types";

type SourceFilter = "all" | "stdout" | "stderr";
const logs = ref<CoreLogEntry[]>([]);
const sourceFilter = ref<SourceFilter>("all");
const autoScroll = ref(true);
const loading = ref(true);
const error = ref("");
const logPanel = ref<HTMLDivElement | null>(null);
const visibleLogs = computed(() =>
  sourceFilter.value === "all"
    ? logs.value
    : logs.value.filter((item) => item.source === sourceFilter.value),
);
const formatTime = (value: number) =>
  value ? new Date(value).toLocaleTimeString() : "--:--:--";

async function loadLogs() {
  loading.value = true;
  error.value = "";
  try {
    logs.value = await api.getCoreLogs(500);
  } catch (e: any) {
    error.value = e.message || "日志加载失败";
  } finally {
    loading.value = false;
  }
}
async function clearLogs() {
  try {
    await api.clearCoreLogs();
    logs.value = [];
  } catch (e: any) {
    error.value = e.message || "日志清空失败";
  }
}
useRealtimeSocket((event: WSEvent) => {
  if (event.type !== "core.log") return;
  const entry = event.data as CoreLogEntry;
  if (entry?.line) logs.value = [...logs.value.slice(-500), entry];
});
watch(
  [logs, autoScroll],
  async () => {
    if (!autoScroll.value) return;
    await nextTick();
    if (logPanel.value) logPanel.value.scrollTop = logPanel.value.scrollHeight;
  },
  { deep: true },
);
onMounted(loadLogs);
</script>

<template>
  <div class="space-y-4">
    <PageHeader
      title="日志"
      description="实时查看 sing-box 核心 stdout/stderr 输出。"
    />
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 shadow-[var(--shadow-card)]"
    >
      <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div
          class="flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]"
        >
          <span>已缓存 {{ logs.length }} 行</span
          ><span>当前显示 {{ visibleLogs.length }} 行</span
          ><span v-if="error" class="text-red-300">{{ error }}</span>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-for="item in ['all', 'stdout', 'stderr'] as SourceFilter[]"
            :key="item"
            class="h-8 rounded-md border px-3 text-xs"
            :class="
              sourceFilter === item
                ? 'border-emerald-400/40 bg-emerald-500/15 text-emerald-100'
                : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white'
            "
            @click="sourceFilter = item"
          >
            {{ item === "all" ? "全部" : item }}
          </button>
          <button
            class="h-8 rounded-md border px-3 text-xs"
            :class="
              autoScroll
                ? 'border-blue-400/40 bg-blue-500/15 text-blue-100'
                : 'border-[var(--border-default)] bg-white/[0.04] text-[var(--text-secondary)] hover:text-white'
            "
            @click="autoScroll = !autoScroll"
          >
            {{ autoScroll ? "自动滚动" : "已暂停滚动" }}
          </button>
          <button
            class="h-8 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 text-xs text-[var(--text-secondary)] hover:text-white"
            @click="loadLogs"
          >
            刷新
          </button>
          <button
            class="h-8 rounded-md border border-red-400/30 bg-red-500/10 px-3 text-xs text-red-200 hover:bg-red-500/20"
            @click="clearLogs"
          >
            清空
          </button>
        </div>
      </div>
      <div
        ref="logPanel"
        class="max-h-[calc(100vh-240px)] overflow-y-auto rounded-lg border border-[var(--border-default)] bg-[var(--bg-primary)] p-3 font-mono text-xs text-[var(--text-tertiary)]"
      >
        <div v-if="loading" class="py-8 text-center">加载日志...</div>
        <div v-else-if="!visibleLogs.length" class="py-8 text-center">
          等待日志...
        </div>
        <div
          v-for="(item, index) in visibleLogs"
          v-else
          :key="`${item.id}-${index}`"
          class="grid grid-cols-[82px_58px_minmax(0,1fr)] gap-2 whitespace-pre-wrap break-all py-0.5 hover:bg-white/[0.03]"
        >
          <span class="text-[var(--text-muted)]">{{
            formatTime(item.time)
          }}</span
          ><span
            :class="item.source === 'stderr' ? 'text-red-300' : 'text-blue-300'"
            >{{ item.source }}</span
          ><span class="text-[var(--text-secondary)]">{{ item.line }}</span>
        </div>
      </div>
    </section>
  </div>
</template>
