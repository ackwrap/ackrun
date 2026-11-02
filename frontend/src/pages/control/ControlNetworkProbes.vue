<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { Globe2, RefreshCw, Wifi } from "lucide-vue-next";
import { writeClipboardText } from "@/utils/clipboard";

const emit = defineEmits<{
  message: [message: string, type: "success" | "error" | "info"];
}>();
const sources = [
  {
    label: "IPv4 · IPIFY",
    url: "https://api.ipify.org?format=json",
    parse: (body: string) => JSON.parse(body).ip || "",
  },
  {
    label: "IPv6 · IPIFY",
    url: "https://api6.ipify.org?format=json",
    parse: (body: string) => JSON.parse(body).ip || "",
  },
  {
    label: "IP.SB",
    url: "https://api.ip.sb/ip",
    parse: (body: string) => body.trim(),
  },
  {
    label: "IDENT.ME",
    url: "https://ident.me",
    parse: (body: string) => body.trim(),
  },
];
const targets = [
  ["百度搜索", "https://www.baidu.com/favicon.ico"],
  ["网易云音乐", "https://s1.music.126.net/style/favicon.ico"],
  ["GitHub", "https://github.com/favicon.ico"],
  ["YouTube", "https://www.youtube.com/favicon.ico"],
] as const;
const ips = ref(
  sources.map((source) => ({ label: source.label, value: "", error: "" })),
);
const access = ref<
  {
    label: string;
    latency: number;
    status: "checking" | "online" | "offline";
  }[]
>(targets.map(([label]) => ({ label, latency: 0, status: "checking" })));
const refreshing = ref(false);
const controllers = new Set<AbortController>();
let timer: number | undefined;
let cancelled = false;

async function timedFetch(url: string, options: RequestInit = {}) {
  const controller = new AbortController();
  controllers.add(controller);
  const timeout = window.setTimeout(() => controller.abort(), 6000);
  try {
    const response = await fetch(url, {
      ...options,
      cache: "no-store",
      signal: controller.signal,
    });
    const body = options.mode === "no-cors" ? "" : await response.text();
    return { ok: response.ok, body };
  } finally {
    clearTimeout(timeout);
    controllers.delete(controller);
  }
}
async function refresh() {
  if (refreshing.value || cancelled) return;
  clearTimeout(timer);
  refreshing.value = true;
  access.value = targets.map(([label]) => ({
    label,
    latency: 0,
    status: "checking",
  }));
  const [nextIPs, nextAccess] = await Promise.all([
    Promise.all(
      sources.map(async (source) => {
        try {
          const response = await timedFetch(source.url);
          if (!response.ok) throw Error("request failed");
          const value = String(source.parse(response.body)).trim();
          if (!value || value.length > 80) throw Error("invalid response");
          return { label: source.label, value, error: "" };
        } catch (error: any) {
          return {
            label: source.label,
            value: "",
            error: error?.name === "AbortError" ? "请求超时" : "获取失败",
          };
        }
      }),
    ),
    Promise.all(
      targets.map(async ([label, url]) => {
        const startedAt = performance.now();
        try {
          await timedFetch(url, { mode: "no-cors" });
          return {
            label,
            latency: Math.max(1, Math.round(performance.now() - startedAt)),
            status: "online" as const,
          };
        } catch {
          return { label, latency: 0, status: "offline" as const };
        }
      }),
    ),
  ]);
  if (cancelled) return;
  ips.value = nextIPs;
  access.value = nextAccess;
  refreshing.value = false;
  timer = window.setTimeout(refresh, 60000);
}
async function copyIP(item: { label: string; value: string }) {
  if (!item.value) return;
  try {
    await writeClipboardText(item.value);
    emit("message", `${item.label} 已复制`, "success");
  } catch (error: any) {
    emit("message", `IP 复制失败: ${error.message}`, "error");
  }
}
onMounted(() => {
  timer = window.setTimeout(refresh, 1200);
});
onBeforeUnmount(() => {
  cancelled = true;
  clearTimeout(timer);
  controllers.forEach((controller) => controller.abort());
});
</script>

<template>
  <section
    class="min-w-0 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="mb-4 flex items-center justify-between gap-3">
      <h2 class="font-semibold">网络探测</h2>
      <button
        class="focus-ring rounded-[var(--radius-md)] p-1 text-[var(--text-secondary)] disabled:opacity-50"
        :disabled="refreshing"
        aria-label="刷新网络探测"
        @click="refresh"
      >
        <RefreshCw :size="15" :class="refreshing && 'animate-spin'" />
      </button>
    </div>
    <h3
      class="mb-2 flex items-center gap-2 text-xs text-[var(--text-secondary)]"
    >
      <Globe2 :size="14" />IP 地址
    </h3>
    <div class="space-y-1.5">
      <button
        v-for="item in ips"
        :key="item.label"
        class="flex min-h-8 w-full min-w-0 items-center justify-between gap-3 rounded-[var(--radius-md)] bg-[var(--bg-base)] px-2.5 py-1.5 text-xs"
        :disabled="!item.value"
        :title="item.value ? `${item.value} · 点击复制` : item.error"
        @click="copyIP(item)"
      >
        <span class="shrink-0 text-[var(--text-secondary)]">{{
          item.label
        }}</span>
        <span
          class="truncate"
          :class="
            item.error
              ? 'text-[var(--color-warning)]'
              : 'text-[var(--text-primary)]'
          "
          >{{ item.value || item.error || "检查中…" }}</span
        >
      </button>
    </div>
    <h3
      class="mb-2 mt-4 flex items-center gap-2 text-xs text-[var(--text-secondary)]"
    >
      <Wifi :size="14" />访问检查
    </h3>
    <div class="space-y-1.5" aria-live="polite">
      <div
        v-for="item in access"
        :key="item.label"
        class="flex min-h-8 items-center justify-between gap-3 rounded-[var(--radius-md)] bg-[var(--bg-base)] px-2.5 py-1.5 text-xs"
      >
        <span class="text-[var(--text-secondary)]">{{ item.label }}</span>
        <span
          :class="
            item.status === 'online'
              ? 'text-[var(--color-success)]'
              : item.status === 'offline'
                ? 'text-[var(--color-warning)]'
                : 'text-[var(--text-tertiary)]'
          "
          >{{
            item.status === "online"
              ? `${item.latency} ms`
              : item.status === "offline"
                ? "连接失败"
                : "检查中…"
          }}</span
        >
      </div>
    </div>
    <p class="mt-3 text-xs leading-5 text-[var(--text-tertiary)]">
      从当前浏览器探测，每分钟刷新。
    </p>
  </section>
</template>
