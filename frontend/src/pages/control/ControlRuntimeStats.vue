<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { Activity, Gauge } from "lucide-vue-next";
import { getClashClient } from "@/services/clash";
import { formatBytes, formatSpeed } from "@/utils/traffic";

const props = defineProps<{ isRunning: boolean; proxyPort: number }>();
const totals = ref<{
  uploadTotal: number;
  downloadTotal: number;
  connections: number;
  memory?: number;
} | null>(null);
const proxyGroups = ref<number | null>(null);
const error = ref("");
const upload = ref(0);
const download = ref(0);
const trafficReady = ref(false);
const trafficError = ref("");
const client = getClashClient();
let generation = 0;
let timer: number | undefined;
const items = computed(() => [
  ["上传", trafficReady.value ? formatSpeed(upload.value) : "--"],
  ["下载", trafficReady.value ? formatSpeed(download.value) : "--"],
  ["上传总量", totals.value ? formatBytes(totals.value.uploadTotal) : "--"],
  ["下载总量", totals.value ? formatBytes(totals.value.downloadTotal) : "--"],
  ["活动连接", totals.value?.connections ?? "--"],
  [
    "内存占用",
    totals.value?.memory == null ? "--" : formatBytes(totals.value.memory),
  ],
  ["策略组", proxyGroups.value ?? "--"],
  ["代理端口", props.proxyPort || "--"],
]);
const status = computed(() =>
  !props.isRunning
    ? "核心未运行"
    : trafficError.value ||
      (error.value
        ? "统计不可用"
        : !trafficReady.value || !totals.value
          ? "正在连接"
          : "实时更新"),
);

function stop() {
  generation++;
  clearTimeout(timer);
  client.disconnectTraffic();
}
function start() {
  stop();
  totals.value = null;
  proxyGroups.value = null;
  error.value = "";
  upload.value = 0;
  download.value = 0;
  trafficReady.value = false;
  trafficError.value = "";
  if (!props.isRunning) return;
  const run = generation;
  client.connectTraffic(
    (value) => {
      if (run !== generation) return;
      upload.value = value.up;
      download.value = value.down;
      trafficReady.value = true;
      trafficError.value = "";
    },
    () => {
      if (run !== generation) return;
      trafficReady.value = false;
      trafficError.value = "实时流量断开";
    },
  );
  const load = async () => {
    const [connections, proxies] = await Promise.allSettled([
      client.getConnections(),
      client.getProxies(),
    ]);
    if (run !== generation) return;
    const errors: string[] = [];
    if (connections.status === "fulfilled") {
      const value = connections.value;
      totals.value = {
        uploadTotal: value.uploadTotal,
        downloadTotal: value.downloadTotal,
        connections: value.connections.length,
        memory: value.memory,
      };
    } else {
      totals.value = null;
      errors.push(`连接统计: ${connections.reason?.message || "请求失败"}`);
    }
    if (proxies.status === "fulfilled") {
      proxyGroups.value = Object.values(proxies.value.proxies).filter((proxy) =>
        Array.isArray(proxy.all),
      ).length;
    } else {
      proxyGroups.value = null;
      errors.push(`策略组: ${proxies.reason?.message || "请求失败"}`);
    }
    error.value = errors.join("；");
    timer = window.setTimeout(load, 2500);
  };
  void load();
}
watch(() => props.isRunning, start, { immediate: true });
onBeforeUnmount(stop);
</script>

<template>
  <section
    class="min-w-0 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)]"
  >
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 class="flex items-center gap-2 text-sm font-semibold">
        <Gauge :size="15" />运行统计
      </h2>
      <span
        role="status"
        class="flex items-center gap-1 text-xs"
        :title="error || trafficError"
        :class="
          error || trafficError || !isRunning
            ? 'text-[var(--color-warning)]'
            : 'text-[var(--color-success)]'
        "
      >
        <Activity :size="12" />{{ status }}
      </span>
    </div>
    <dl class="grid grid-cols-4 gap-2">
      <div
        v-for="[label, value] in items"
        :key="label"
        class="flex min-h-[76px] min-w-0 flex-col items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] p-2 text-center"
      >
        <dt class="text-[11px] text-[var(--text-secondary)]">{{ label }}</dt>
        <dd
          class="mt-2 max-w-full break-words text-xs font-semibold text-[var(--color-success)]"
          :title="String(value)"
        >
          {{ value }}
        </dd>
      </div>
    </dl>
  </section>
</template>
