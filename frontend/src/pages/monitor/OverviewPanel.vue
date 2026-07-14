<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Bolt, Eye, EyeOff, RefreshCw } from "lucide-vue-next";
import type { Connection, ProxyGroup } from "@/services/clash";
import { formatBytes, formatSpeed } from "./monitorUtils";
import MiniSparkline from "@/components/monitor/MiniSparkline.vue";

interface LatencyTarget {
  name: string;
  url: string;
  values: number[];
}

interface IPProbe {
  label: string;
  value: string;
  error: string;
}

const props = defineProps<{
  connected: boolean;
  unavailableReason: string;
  totalUp: number;
  totalDown: number;
  speedUp: number;
  speedDown: number;
  memory: number;
  connectionCount: number;
  connections: Connection[];
  proxyGroups: ProxyGroup[];
  uploadSpeedHistory: number[];
  downloadSpeedHistory: number[];
  connectionCountHistory: number[];
  memoryHistory: number[];
}>();

const latencyTargets = ref<LatencyTarget[]>([
  { name: "Baidu", url: "https://www.baidu.com/favicon.ico", values: [] },
  {
    name: "Cloudflare",
    url: "https://www.cloudflare.com/favicon.ico",
    values: [],
  },
  { name: "GitHub", url: "https://github.com/favicon.ico", values: [] },
  {
    name: "YouTube",
    url: "https://www.youtube.com/favicon.ico",
    values: [],
  },
]);
const ipProbes = ref<IPProbe[]>([]);
const testingLatency = ref(false);
const loadingIPs = ref(false);
const revealIPs = ref(false);
let ipTimer: number | undefined;

const statCards = computed(() => [
  {
    label: "上传",
    value: formatSpeed(props.speedUp),
    total: `总计 ${formatBytes(props.totalUp)}`,
    history: props.uploadSpeedHistory,
    color: "blue" as const,
  },
  {
    label: "下载",
    value: formatSpeed(props.speedDown),
    total: `总计 ${formatBytes(props.totalDown)}`,
    history: props.downloadSpeedHistory,
    color: "purple" as const,
  },
]);

function latencyStats(values: number[]) {
  const successful = values.filter((value) => value > 0);
  if (!successful.length) return null;
  return {
    min: Math.min(...successful),
    avg: Math.round(
      successful.reduce((total, value) => total + value, 0) / successful.length,
    ),
    max: Math.max(...successful),
  };
}

function latencyClass(value: number) {
  if (value < 200) return "text-emerald-500";
  if (value < 800) return "text-amber-500";
  return "text-rose-500";
}

function latencyBarClass(value: number) {
  if (!value) return "bg-rose-400/40";
  if (value < 200) return "bg-emerald-400/70";
  if (value < 800) return "bg-amber-400";
  return "bg-rose-400";
}

function latencyBarHeight(values: number[], value: number) {
  const max = Math.max(1, ...values);
  return `${Math.max(18, Math.round((value / max) * 100))}%`;
}

async function timedFetch(url: string, options: RequestInit = {}) {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 6000);
  try {
    return await fetch(url, {
      ...options,
      cache: "no-store",
      signal: controller.signal,
    });
  } finally {
    window.clearTimeout(timeout);
  }
}

async function testLatency() {
  if (testingLatency.value) return;
  testingLatency.value = true;
  latencyTargets.value.forEach((target) => (target.values = []));
  try {
    await Promise.all(
      latencyTargets.value.map(async (target) => {
        for (let round = 0; round < 8; round += 1) {
          const startedAt = performance.now();
          try {
            await timedFetch(`${target.url}?ackwrap=${Date.now()}-${round}`, {
              mode: "no-cors",
            });
            target.values.push(
              Math.max(1, Math.round(performance.now() - startedAt)),
            );
          } catch {
            target.values.push(0);
          }
        }
      }),
    );
  } finally {
    testingLatency.value = false;
  }
}

async function refreshIPs() {
  if (loadingIPs.value) return;
  loadingIPs.value = true;
  const sources = [
    {
      label: "api.ipify.org",
      url: "https://api.ipify.org?format=json",
      parse: (body: string) => String(JSON.parse(body).ip || ""),
    },
    {
      label: "api.ip.sb",
      url: "https://api.ip.sb/ip",
      parse: (body: string) => body.trim(),
    },
  ];
  ipProbes.value = await Promise.all(
    sources.map(async (source) => {
      try {
        const response = await timedFetch(source.url);
        if (!response.ok) throw new Error("request failed");
        const value = source.parse(await response.text()).slice(0, 80);
        if (!value) throw new Error("empty response");
        return { label: source.label, value, error: "" };
      } catch (error: any) {
        return {
          label: source.label,
          value: "",
          error: error?.name === "AbortError" ? "请求超时" : "获取失败",
        };
      }
    }),
  );
  loadingIPs.value = false;
}

function maskedIP(value: string) {
  if (revealIPs.value || !value) return value;
  if (value.includes(":")) {
    const parts = value.split(":");
    return `${parts.slice(0, 2).join(":")}:****:****`;
  }
  const parts = value.split(".");
  return parts.length === 4 ? `${parts[0]}.${parts[1]}.***.***` : "***";
}

onMounted(() => {
  void testLatency();
  void refreshIPs();
  ipTimer = window.setInterval(refreshIPs, 60000);
});

onBeforeUnmount(() => window.clearInterval(ipTimer));
</script>

<template>
  <div class="grid items-start gap-4 pb-5 xl:grid-cols-2">
    <section
      class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)]"
    >
      <div class="grid gap-3 sm:grid-cols-2">
        <article
          v-for="card in statCards"
          :key="card.label"
          class="flex min-h-44 flex-col rounded-xl bg-[var(--bg-base)] p-4"
        >
          <div
            class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase"
          >
            {{ card.label }}
          </div>
          <div
            class="mt-1 text-3xl font-light tabular-nums text-[var(--text-primary)]"
          >
            {{ card.value }}
          </div>
          <MiniSparkline
            :data="card.history"
            :color="card.color"
            class="mt-auto"
          />
          <div class="text-xs text-[var(--text-tertiary)]">
            {{ card.total }}
          </div>
        </article>

        <article
          class="flex min-h-44 flex-col rounded-xl bg-[var(--bg-base)] p-4 sm:col-span-2"
        >
          <div
            class="flex items-center gap-2 text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase"
          >
            连接
            <span
              class="size-1.5 rounded-full"
              :class="connected ? 'bg-emerald-400' : 'bg-rose-400'"
            />
          </div>
          <div
            class="mt-1 text-3xl font-light tabular-nums text-[var(--text-primary)]"
          >
            {{ connectionCount }}
          </div>
          <MiniSparkline
            :data="connectionCountHistory"
            color="purple"
            class="mt-auto"
          />
          <div
            class="flex justify-between gap-3 text-xs text-[var(--text-tertiary)]"
          >
            <span>内存使用 {{ memory ? formatBytes(memory) : "--" }}</span>
            <span
              v-if="!connected"
              class="truncate text-[var(--color-warning)]"
            >
              {{ unavailableReason || "核心离线" }}
            </span>
          </div>
        </article>
      </div>
    </section>

    <section
      class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)]"
    >
      <article class="rounded-xl bg-[var(--bg-base)] p-4">
        <header class="flex items-center justify-between">
          <h3
            class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase"
          >
            延迟
          </h3>
          <button
            type="button"
            class="flex size-7 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
            :disabled="testingLatency"
            title="重新测试延迟"
            @click="testLatency"
          >
            <Bolt :size="14" :class="testingLatency ? 'animate-pulse' : ''" />
          </button>
        </header>

        <div class="mt-3 grid gap-x-4 gap-y-3 sm:grid-cols-2">
          <div v-for="target in latencyTargets" :key="target.name">
            <div class="flex items-center gap-2">
              <span
                class="w-14 shrink-0 text-xs text-[var(--text-secondary)]"
                >{{ target.name }}</span
              >
              <div class="flex h-8 min-w-0 flex-1 items-end gap-0.5">
                <span
                  v-for="(value, index) in target.values"
                  :key="index"
                  class="min-w-1 flex-1 rounded-[1px]"
                  :class="latencyBarClass(value)"
                  :style="{
                    height: latencyBarHeight(target.values, value),
                  }"
                  :title="value ? `${value} ms` : '测试失败'"
                />
                <span
                  v-for="index in Math.max(0, 8 - target.values.length)"
                  :key="`empty-${index}`"
                  class="h-[18%] min-w-1 flex-1 rounded-[1px] bg-[var(--border-default)]"
                />
              </div>
            </div>
            <div
              v-if="latencyStats(target.values)"
              class="mt-1 flex gap-3 text-[10px] tabular-nums text-[var(--text-tertiary)]"
            >
              <span
                >min
                <b :class="latencyClass(latencyStats(target.values)!.min)"
                  >{{ latencyStats(target.values)!.min }}ms</b
                ></span
              >
              <span
                >avg
                <b :class="latencyClass(latencyStats(target.values)!.avg)"
                  >{{ latencyStats(target.values)!.avg }}ms</b
                ></span
              >
              <span
                >max
                <b :class="latencyClass(latencyStats(target.values)!.max)"
                  >{{ latencyStats(target.values)!.max }}ms</b
                ></span
              >
            </div>
            <div v-else class="mt-1 text-[10px] text-[var(--text-tertiary)]">
              {{ testingLatency ? "测试中..." : "--" }}
            </div>
          </div>
        </div>
      </article>

      <article class="mt-3 rounded-xl bg-[var(--bg-base)] p-4">
        <header class="flex items-center justify-between">
          <h3
            class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase"
          >
            网络信息
          </h3>
          <div class="flex items-center gap-1">
            <button
              type="button"
              class="flex size-7 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
              :title="revealIPs ? '隐藏 IP 地址' : '显示 IP 地址'"
              @click="revealIPs = !revealIPs"
            >
              <Eye v-if="revealIPs" :size="14" />
              <EyeOff v-else :size="14" />
            </button>
            <button
              type="button"
              class="flex size-7 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
              :disabled="loadingIPs"
              title="刷新网络信息"
              @click="refreshIPs"
            >
              <RefreshCw :size="14" :class="loadingIPs ? 'animate-spin' : ''" />
            </button>
          </div>
        </header>

        <div class="mt-3 divide-y divide-[var(--border-light)]">
          <div
            v-for="probe in ipProbes"
            :key="probe.label"
            class="py-3 first:pt-0 last:pb-0"
          >
            <div class="text-xs text-[var(--text-tertiary)]">
              {{ probe.label }}
            </div>
            <div
              class="mt-1 text-sm font-medium"
              :class="
                probe.error
                  ? 'text-[var(--color-warning)]'
                  : 'text-[var(--text-primary)]'
              "
            >
              {{ probe.error || maskedIP(probe.value) }}
            </div>
          </div>
          <div
            v-if="!ipProbes.length"
            class="py-5 text-xs text-[var(--text-tertiary)]"
          >
            正在获取网络信息...
          </div>
        </div>
      </article>
    </section>
  </div>
</template>
