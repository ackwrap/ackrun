<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { ArrowDown, ArrowUp, Users } from "lucide-vue-next";
import { getClashClient } from "@/services/clash";
import MiniSparkline from "@/components/monitor/MiniSparkline.vue";
import { formatSpeed } from "./monitor/monitorUtils";

const props = defineProps<{ isRunning: boolean }>();
const uploadHistory = ref<number[]>([]);
const downloadHistory = ref<number[]>([]);
const upload = ref(0);
const download = ref(0);
const connections = ref<number | null>(null);
const connectionsError = ref("");
const trafficError = ref("");
const trafficReady = ref(false);
const client = getClashClient();
let generation = 0;
let timer: number | undefined;
const status = computed(() =>
  !props.isRunning
    ? "核心未运行，启动后显示实时数据"
    : trafficError.value ||
      connectionsError.value ||
      (!trafficReady.value ? "正在连接实时数据…" : "实时更新"),
);

function stop() {
  generation++;
  clearTimeout(timer);
  client.disconnectTraffic();
}
function start() {
  stop();
  const run = generation;
  upload.value = 0;
  download.value = 0;
  connections.value = null;
  connectionsError.value = "";
  trafficError.value = "";
  trafficReady.value = false;
  uploadHistory.value = [];
  downloadHistory.value = [];
  if (!props.isRunning) return;
  client.connectTraffic(
    (data) => {
      if (run !== generation) return;
      upload.value = data.up;
      download.value = data.down;
      trafficError.value = "";
      trafficReady.value = true;
      uploadHistory.value = [...uploadHistory.value.slice(-59), data.up];
      downloadHistory.value = [...downloadHistory.value.slice(-59), data.down];
    },
    (error) => {
      if (run !== generation) return;
      trafficError.value = error;
      trafficReady.value = false;
      upload.value = 0;
      download.value = 0;
    },
  );
  const load = async () => {
    try {
      const data = await client.getConnections();
      if (run !== generation) return;
      connections.value = data.connections.length;
      connectionsError.value = "";
    } catch (error: any) {
      if (run !== generation) return;
      connections.value = null;
      connectionsError.value = `活动连接读取失败: ${error.message}`;
    } finally {
      if (run === generation) timer = window.setTimeout(load, 5000);
    }
  };
  void load();
}
watch(() => props.isRunning, start, { immediate: true });
onBeforeUnmount(stop);
</script>

<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h2 class="font-semibold">实时流量</h2>
      <RouterLink to="/dashboard" class="text-xs text-[var(--color-primary)]"
        >打开仪表盘 →</RouterLink
      >
    </div>
    <div class="my-5 grid grid-cols-3 gap-3">
      <div>
        <span
          class="flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"
          ><ArrowUp :size="14" />上传</span
        ><b class="mt-1 block text-base sm:text-xl">{{
          isRunning && trafficReady ? formatSpeed(upload) : "--"
        }}</b>
      </div>
      <div>
        <span
          class="flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"
          ><ArrowDown :size="14" />下载</span
        ><b class="mt-1 block text-base sm:text-xl">{{
          isRunning && trafficReady ? formatSpeed(download) : "--"
        }}</b>
      </div>
      <div>
        <span
          class="flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"
          ><Users :size="14" />活动连接</span
        ><b class="mt-1 block text-base sm:text-xl">{{
          connections ?? "--"
        }}</b>
      </div>
    </div>
    <div
      v-if="isRunning"
      class="grid gap-4 sm:grid-cols-2"
      role="img"
      aria-label="最近 60 次采样的上传和下载流量趋势"
    >
      <div class="min-w-0 rounded-[var(--radius-lg)] bg-[var(--bg-base)] p-3">
        <span class="text-xs text-[var(--text-tertiary)]">上传趋势</span>
        <MiniSparkline
          :data="uploadHistory"
          color="green"
          class="!h-32 sm:!h-40"
        />
      </div>
      <div class="min-w-0 rounded-[var(--radius-lg)] bg-[var(--bg-base)] p-3">
        <span class="text-xs text-[var(--text-tertiary)]">下载趋势</span>
        <MiniSparkline
          :data="downloadHistory"
          color="blue"
          class="!h-32 sm:!h-40"
        />
      </div>
    </div>
    <p
      class="mt-2 break-words text-xs"
      :class="
        trafficError || connectionsError
          ? 'text-[var(--color-warning)]'
          : 'text-[var(--text-tertiary)]'
      "
      role="status"
    >
      {{ status }}
    </p>
  </section>
</template>
