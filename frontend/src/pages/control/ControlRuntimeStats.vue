<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { Gauge } from "lucide-vue-next";
import { getClashClient } from "@/services/clash";
import { formatBytes } from "../monitor/monitorUtils";

const props = defineProps<{ isRunning: boolean; proxyPort: number }>();
const totals = ref<{
  uploadTotal: number;
  downloadTotal: number;
  connections: number;
  memory?: number;
} | null>(null);
const proxyGroups = ref<number | null>(null);
const error = ref("");
const client = getClashClient();
let generation = 0;
let timer: number | undefined;
const items = computed(() => [
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
    : error.value
      ? "部分统计不可用"
      : totals.value
        ? "每 5 秒刷新"
        : "正在读取…",
);

function stop() {
  generation++;
  clearTimeout(timer);
}
function start() {
  stop();
  totals.value = null;
  proxyGroups.value = null;
  error.value = "";
  if (!props.isRunning) return;
  const run = generation;
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
    timer = window.setTimeout(load, 5000);
  };
  void load();
}
watch(() => props.isRunning, start, { immediate: true });
onBeforeUnmount(stop);
</script>

<template>
  <section
    class="flex min-w-0 flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <h2 class="mb-4 flex items-center gap-2 font-semibold">
      <Gauge :size="16" class="text-[var(--text-secondary)]" />运行统计
    </h2>
    <dl class="grid flex-1 grid-cols-2 gap-2">
      <div
        v-for="[label, value] in items"
        :key="label"
        class="min-w-0 rounded-[var(--radius-md)] bg-[var(--bg-base)] px-3 py-2"
      >
        <dt class="text-xs text-[var(--text-secondary)]">{{ label }}</dt>
        <dd class="mt-1 truncate text-sm font-semibold" :title="String(value)">
          {{ value }}
        </dd>
      </div>
    </dl>
    <p
      role="status"
      class="mt-3 break-words text-xs leading-5"
      :class="
        error ? 'text-[var(--color-warning)]' : 'text-[var(--text-tertiary)]'
      "
    >
      {{ error || status }}
    </p>
  </section>
</template>
