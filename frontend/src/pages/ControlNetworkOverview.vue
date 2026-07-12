<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Activity, Gauge, Globe2, RefreshCw, Wifi } from "lucide-vue-next";
import { getClashClient } from "@/services/clash";
import { formatBytes, formatSpeed } from "./monitor/monitorUtils";
const props = defineProps<{ isRunning: boolean; proxyPort: number }>(),
  emit = defineEmits<{ message: [string, "success" | "error" | "info"] }>();
interface IPProbe {
  label: string;
  value: string;
  error: string;
}
interface AccessProbe {
  label: string;
  latency: number;
  status: "checking" | "online" | "offline";
}
const ipSources = [
  {
    label: "IPv4 · IPIFY",
    url: "https://api.ipify.org?format=json",
    parse: (b: string) => JSON.parse(b).ip,
  },
  {
    label: "IPv6 · IPIFY",
    url: "https://api6.ipify.org?format=json",
    parse: (b: string) => JSON.parse(b).ip,
  },
  {
    label: "IP.SB",
    url: "https://api.ip.sb/ip",
    parse: (b: string) => b.trim(),
  },
  {
    label: "IDENT.ME",
    url: "https://ident.me",
    parse: (b: string) => b.trim(),
  },
];
const targets = [
  ["百度搜索", "https://www.baidu.com/favicon.ico"],
  ["网易云音乐", "https://s1.music.126.net/style/favicon.ico"],
  ["GitHub", "https://github.com/favicon.ico"],
  ["YouTube", "https://www.youtube.com/favicon.ico"],
];
const ips = ref<IPProbe[]>([]),
  access = ref<AccessProbe[]>([]),
  refreshingIPs = ref(false),
  refreshingAccess = ref(false),
  stats = ref({
    upload: 0,
    download: 0,
    uploadTotal: 0,
    downloadTotal: 0,
    connections: 0,
    memory: 0,
    proxyGroups: 0,
    proxyPort: 0,
  }),
  statsError = ref(""),
  trafficError = ref("");
let ipTimer: number,
  accessTimer: number,
  statsTimer: number | undefined,
  startupTimer: number | undefined,
  cancelled = false,
  lastStats = "",
  lastTraffic = "";
async function timed(url: string, options: RequestInit = {}) {
  const c = new AbortController(),
    t = setTimeout(() => c.abort(), 6000);
  try {
    return await fetch(url, {
      ...options,
      cache: "no-store",
      signal: c.signal,
    });
  } finally {
    clearTimeout(t);
  }
}
async function refreshIPs() {
  refreshingIPs.value = true;
  ips.value = await Promise.all(
    ipSources.map(async (s) => {
      try {
        const r = await timed(s.url);
        if (!r.ok) throw Error();
        const value = String(s.parse(await r.text())).trim();
        if (!value || value.length > 80) throw Error();
        return { label: s.label, value, error: "" };
      } catch (e: any) {
        return {
          label: s.label,
          value: "",
          error: e?.name === "AbortError" ? "请求超时" : "获取失败",
        };
      }
    }),
  );
  refreshingIPs.value = false;
}
async function refreshAccess() {
  refreshingAccess.value = true;
  access.value = targets.map((t) => ({
    label: t[0],
    latency: 0,
    status: "checking",
  }));
  access.value = await Promise.all(
    targets.map(async (t) => {
      const start = performance.now();
      try {
        await timed(t[1], { mode: "no-cors" });
        return {
          label: t[0],
          latency: Math.max(1, Math.round(performance.now() - start)),
          status: "online" as const,
        };
      } catch {
        return { label: t[0], latency: 0, status: "offline" as const };
      }
    }),
  );
  refreshingAccess.value = false;
}
function stopStats() {
  cancelled = true;
  clearTimeout(startupTimer);
  clearInterval(statsTimer);
  getClashClient().disconnectTraffic();
}
function startStats() {
  stopStats();
  cancelled = false;
  lastStats = "";
  lastTraffic = "";
  if (!props.isRunning) {
    stats.value = {
      upload: 0,
      download: 0,
      uploadTotal: 0,
      downloadTotal: 0,
      connections: 0,
      memory: 0,
      proxyGroups: 0,
      proxyPort: 0,
    };
    statsError.value = "核心未运行";
    trafficError.value = "";
    return;
  }
  const client = getClashClient(),
    grace = Date.now() + 5000;
  let failures = 0;
  statsError.value = "正在连接";
  trafficError.value = "";
  const load = async () => {
    try {
      const [c, p] = await Promise.all([
        client.getConnections(),
        client.getProxies(),
      ]);
      if (cancelled) return;
      stats.value = {
        ...stats.value,
        uploadTotal: c.uploadTotal,
        downloadTotal: c.downloadTotal,
        connections: c.connections.length,
        memory: c.memory || 0,
        proxyGroups: Object.values(p.proxies).filter((x) =>
          Array.isArray(x.all),
        ).length,
        proxyPort: props.proxyPort,
      };
      failures = 0;
      statsError.value = "";
      lastStats = "";
    } catch (e: any) {
      if (cancelled) return;
      failures++;
      const starting = Date.now() < grace || failures < 3;
      statsError.value = starting ? "正在连接" : "统计不可用";
      if (!starting && !lastStats)
        emit(
          "message",
          `运行统计加载失败: ${e?.message || "运行统计暂不可用"}`,
          "error",
        );
      if (!starting) lastStats = e?.message || "error";
    }
  };
  startupTimer = window.setTimeout(() => {
    client.connectTraffic(
      (t) => {
        if (!cancelled) {
          trafficError.value = "";
          lastTraffic = "";
          stats.value = { ...stats.value, upload: t.up, download: t.down };
        }
      },
      (e) => {
        if (cancelled) return;
        const starting = Date.now() < grace;
        trafficError.value = starting ? "正在连接" : "实时流量断开";
        if (!starting && !lastTraffic)
          emit("message", `实时流量连接失败: ${e}`, "error");
        if (!starting) lastTraffic = e;
      },
    );
    void load();
    statsTimer = window.setInterval(load, 2500);
  }, 800);
}
const items = computed(() => [
  ["上传", formatSpeed(stats.value.upload)],
  ["下载", formatSpeed(stats.value.download)],
  ["上传总量", formatBytes(stats.value.uploadTotal)],
  ["下载总量", formatBytes(stats.value.downloadTotal)],
  ["活动连接", `${stats.value.connections}`],
  ["内存占用", stats.value.memory ? formatBytes(stats.value.memory) : "--"],
  ["策略组", `${stats.value.proxyGroups}`],
  ["代理端口", stats.value.proxyPort ? `${stats.value.proxyPort}` : "--"],
]);
async function copy(p: IPProbe) {
  if (!p.value) return;
  try {
    await navigator.clipboard.writeText(p.value);
    emit("message", `${p.label} 已复制`, "success");
  } catch {
    emit("message", `${p.label} 复制失败`, "error");
  }
}
watch(() => [props.isRunning, props.proxyPort], startStats);
onMounted(() => {
  void refreshIPs();
  void refreshAccess();
  ipTimer = window.setInterval(refreshIPs, 60000);
  accessTimer = window.setInterval(refreshAccess, 60000);
  startStats();
});
onBeforeUnmount(() => {
  clearInterval(ipTimer);
  clearInterval(accessTimer);
  stopStats();
});
</script>
<template>
  <div
    class="order-3 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <div class="mb-2 flex justify-between">
      <h3 class="flex gap-2 text-sm font-semibold">
        <Globe2 :size="15" />IP 地址
      </h3>
      <button :disabled="refreshingIPs" @click="refreshIPs">
        <RefreshCw :size="13" :class="{ 'animate-spin': refreshingIPs }" />
      </button>
    </div>
    <div class="grid flex-1 grid-rows-4 gap-1.5">
      <button
        v-for="p in ips"
        :key="p.label"
        class="flex min-h-[42px] justify-between rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-3.5 py-1.5"
        :disabled="!p.value"
        @click="copy(p)"
      >
        <span>{{ p.label }}</span
        ><span
          :class="
            p.error
              ? 'text-[var(--color-warning)]'
              : 'text-[var(--color-success)]'
          "
          >{{ p.value || p.error || "检查中..." }}</span
        >
      </button>
    </div>
  </div>
  <div
    class="order-4 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <div class="mb-2 flex justify-between">
      <h3 class="flex gap-2 text-sm font-semibold">
        <Wifi :size="15" />访问检查
      </h3>
      <button @click="refreshAccess">
        <RefreshCw :size="13" :class="{ 'animate-spin': refreshingAccess }" />
      </button>
    </div>
    <div class="grid flex-1 grid-rows-4 gap-1.5">
      <div
        v-for="p in access"
        :key="p.label"
        class="flex min-h-[42px] justify-between rounded border bg-[var(--bg-base)] px-3.5 py-1.5"
      >
        <span>{{ p.label }}</span
        ><span
          :class="
            p.status === 'online'
              ? 'text-[var(--color-success)]'
              : p.status === 'offline'
                ? 'text-[var(--color-error)]'
                : ''
          "
          >{{
            p.status === "online"
              ? `连接正常 · ${p.latency} ms`
              : p.status === "offline"
                ? "连接失败"
                : "检查中..."
          }}</span
        >
      </div>
    </div>
  </div>
  <div
    class="order-5 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <div class="mb-3 flex justify-between">
      <h3 class="flex gap-2 text-sm font-semibold">
        <Gauge :size="15" />运行统计
      </h3>
      <span
        class="flex gap-1 text-xs"
        :class="
          statsError || trafficError
            ? 'text-[var(--color-warning)]'
            : 'text-[var(--color-success)]'
        "
        ><Activity :size="12" />{{
          statsError || trafficError || "实时更新"
        }}</span
      >
    </div>
    <div class="grid grid-cols-4 gap-2">
      <div
        v-for="item in items"
        :key="item[0]"
        class="flex min-h-[76px] flex-col items-center justify-center rounded border bg-[var(--bg-base)] p-2 text-center"
      >
        <span class="text-[11px]">{{ item[0] }}</span
        ><b class="mt-2 truncate text-xs text-[var(--color-success)]">{{
          item[1]
        }}</b>
      </div>
    </div>
  </div>
</template>
