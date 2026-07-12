<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import {
  Activity,
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Gauge,
  Users,
} from "lucide-vue-next";
import { getClashClient } from "@/services/clash";
import { formatBytes, formatSpeed } from "./monitor/monitorUtils";
const props = defineProps<{ isRunning: boolean }>();
interface Snapshot {
  upload: number;
  download: number;
  uploadTotal: number;
  downloadTotal: number;
  connections: number;
  proxyGroups: number;
  memory: number;
  proxyPort: number;
}
const empty = (): Snapshot => ({
  upload: 0,
  download: 0,
  uploadTotal: 0,
  downloadTotal: 0,
  connections: 0,
  proxyGroups: 0,
  memory: 0,
  proxyPort: 0,
});
const snapshot = ref(empty()),
  history = ref<{ upload: number; download: number }[]>([]),
  apiOnline = ref(false),
  apiError = ref("");
let timer: number | undefined,
  cancelled = false;
const maxTraffic = computed(() =>
  Math.max(1, ...history.value.flatMap((i) => [i.upload, i.download])),
);
function stop() {
  cancelled = true;
  if (timer) clearInterval(timer);
  getClashClient().disconnectTraffic();
}
async function start() {
  stop();
  cancelled = false;
  const client = getClashClient();
  if (!props.isRunning) {
    snapshot.value = empty();
    history.value = [];
    apiOnline.value = false;
    apiError.value = "核心未运行，启动后显示实时数据";
    return;
  }
  const load = async (full = false) => {
    try {
      const [connections, proxies, config] = await Promise.all([
        client.getConnections(),
        full ? client.getProxies() : null,
        full ? client.getConfig() : null,
      ]);
      if (cancelled) return;
      snapshot.value = {
        ...snapshot.value,
        connections: connections.connections.length,
        uploadTotal: connections.uploadTotal,
        downloadTotal: connections.downloadTotal,
        proxyGroups: proxies
          ? Object.values(proxies.proxies).filter((p) => Array.isArray(p.all))
              .length
          : snapshot.value.proxyGroups,
        memory: connections.memory ?? snapshot.value.memory,
        proxyPort: config
          ? config["mixed-port"] || config.port || config["socks-port"] || 0
          : snapshot.value.proxyPort,
      };
      apiOnline.value = true;
      apiError.value = "";
    } catch (e: any) {
      if (!cancelled) {
        apiOnline.value = false;
        apiError.value = e?.message || "Clash API 暂不可用";
      }
    }
  };
  client.connectTraffic((t) => {
    if (!cancelled) {
      snapshot.value = { ...snapshot.value, upload: t.up, download: t.down };
      history.value = [
        ...history.value.slice(-23),
        { upload: t.up, download: t.down },
      ];
    }
  });
  void load(true);
  timer = window.setInterval(() => load(), 5000);
}
watch(() => props.isRunning, start, { immediate: true });
onBeforeUnmount(stop);
</script>
<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div
      class="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between"
    >
      <div>
        <h2 class="text-base font-semibold text-[var(--text-primary)]">
          实时运行中心
        </h2>
        <p class="mt-1 text-xs text-[var(--text-tertiary)]">
          当前吞吐、连接与高频资源维护集中在这里。
        </p>
      </div>
      <div
        class="inline-flex w-fit items-center gap-2 rounded-[var(--radius-full)] px-2.5 py-1 text-xs"
        :class="
          apiOnline
            ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]'
            : 'bg-[var(--color-warning-bg)] text-[var(--color-warning)]'
        "
      >
        <span
          class="h-1.5 w-1.5 rounded-full"
          :class="
            apiOnline
              ? 'bg-[var(--color-success)]'
              : 'bg-[var(--color-warning)]'
          "
        />{{
          apiOnline
            ? "Clash API 已连接"
            : isRunning
              ? "Clash API 检查中"
              : "核心离线"
        }}
      </div>
    </div>
    <div class="grid gap-4 xl:grid-cols-[1.65fr_1fr]">
      <div
        class="rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-4"
      >
        <div class="flex items-center justify-between">
          <h3
            class="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]"
          >
            <Gauge :size="15" />网络吞吐
          </h3>
          <RouterLink
            to="/"
            class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)]"
            >查看趋势<ArrowRight :size="12"
          /></RouterLink>
        </div>
        <div class="mt-4 grid grid-cols-2 gap-3">
          <div
            class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] px-3 py-3"
          >
            <div
              class="flex items-center gap-1.5 text-xs text-[var(--text-tertiary)]"
            >
              <ArrowDown :size="13" />实时下载
            </div>
            <div class="mt-1 text-xl font-semibold">
              {{ formatSpeed(snapshot.download) }}
            </div>
          </div>
          <div
            class="rounded-[var(--radius-md)] bg-[var(--color-success-bg)] px-3 py-3"
          >
            <div class="flex items-center gap-1.5 text-xs">
              <ArrowUp :size="13" />实时上传
            </div>
            <div class="mt-1 text-xl font-semibold">
              {{ formatSpeed(snapshot.upload) }}
            </div>
          </div>
        </div>
        <div class="mt-4 flex h-12 items-end gap-1">
          <div
            v-for="(item, i) in Array.from(
              { length: 24 },
              (_, i) => history[i] || { upload: 0, download: 0 },
            )"
            :key="i"
            class="flex h-full min-w-0 flex-1 items-end gap-px"
          >
            <span
              class="w-1/2 rounded-t-sm bg-[var(--color-primary)] opacity-70"
              :style="{
                height: `${Math.max(4, (item.download / maxTraffic) * 100)}%`,
              }"
            /><span
              class="w-1/2 rounded-t-sm bg-[var(--color-success)] opacity-70"
              :style="{
                height: `${Math.max(4, (item.upload / maxTraffic) * 100)}%`,
              }"
            />
          </div>
        </div>
      </div>
      <div
        class="rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-4"
      >
        <h3 class="flex items-center gap-2 text-sm font-semibold">
          <Activity :size="15" />会话快照
        </h3>
        <div class="mt-4 space-y-3">
          <div class="flex justify-between border-b pb-3">
            <span class="flex gap-2 text-xs"><Users :size="14" />活动连接</span
            ><b>{{ snapshot.connections }}</b>
          </div>
          <div class="flex justify-between border-b pb-3">
            <span>策略组</span><b>{{ snapshot.proxyGroups }}</b>
          </div>
          <div class="flex justify-between border-b pb-3">
            <span>会话累计</span
            ><b>{{
              formatBytes(snapshot.uploadTotal + snapshot.downloadTotal)
            }}</b>
          </div>
          <div class="grid grid-cols-2 gap-3 text-xs">
            <div>
              核心内存<br /><b>{{
                snapshot.memory ? formatBytes(snapshot.memory) : "--"
              }}</b>
            </div>
            <div>
              代理端口<br /><b>{{ snapshot.proxyPort || "--" }}</b>
            </div>
          </div>
        </div>
        <div
          v-if="apiError"
          class="mt-3 rounded bg-[var(--color-warning-bg)] p-2 text-xs text-[var(--color-warning)]"
        >
          {{ apiError }}
        </div>
      </div>
    </div>
  </section>
</template>
