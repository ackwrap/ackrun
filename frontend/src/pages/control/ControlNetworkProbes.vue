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
const refreshingIPs = ref(false);
const refreshingAccess = ref(false);
const controllers = new Set<AbortController>();
let ipTimer: number | undefined;
let accessTimer: number | undefined;
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
async function refreshIPs() {
  if (refreshingIPs.value || cancelled) return;
  clearTimeout(ipTimer);
  refreshingIPs.value = true;
  const nextIPs = await Promise.all(
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
  );
  if (cancelled) return;
  ips.value = nextIPs;
  refreshingIPs.value = false;
  ipTimer = window.setTimeout(refreshIPs, 60000);
}
async function refreshAccess() {
  if (refreshingAccess.value || cancelled) return;
  clearTimeout(accessTimer);
  refreshingAccess.value = true;
  access.value = targets.map(([label]) => ({
    label,
    latency: 0,
    status: "checking",
  }));
  const nextAccess = await Promise.all(
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
  );
  if (cancelled) return;
  access.value = nextAccess;
  refreshingAccess.value = false;
  accessTimer = window.setTimeout(refreshAccess, 60000);
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
  ipTimer = window.setTimeout(refreshIPs, 1200);
  accessTimer = window.setTimeout(refreshAccess, 1200);
});
onBeforeUnmount(() => {
  cancelled = true;
  clearTimeout(ipTimer);
  clearTimeout(accessTimer);
  controllers.forEach((controller) => controller.abort());
});
</script>

<template>
  <section
    class="flex min-w-0 flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="mb-4 flex items-center justify-between gap-3">
      <h2 class="flex items-center gap-2 font-semibold">
        <Globe2 :size="16" class="text-[var(--text-secondary)]" />IP 地址
      </h2>
      <button
        class="focus-ring rounded-[var(--radius-md)] p-1 text-[var(--text-secondary)] disabled:opacity-50"
        :disabled="refreshingIPs"
        aria-label="刷新 IP 地址"
        @click="refreshIPs"
      >
        <RefreshCw :size="15" :class="refreshingIPs && 'animate-spin'" />
      </button>
    </div>
    <div class="grid flex-1 grid-rows-4 gap-1.5">
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
    <p class="mt-3 text-xs leading-5 text-[var(--text-tertiary)]">
      从当前浏览器探测，每分钟刷新。
    </p>
  </section>
  <section
    class="flex min-w-0 flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="mb-4 flex items-center justify-between gap-3">
      <h2 class="flex items-center gap-2 font-semibold">
        <Wifi :size="16" class="text-[var(--text-secondary)]" />访问检查
      </h2>
      <button
        class="focus-ring rounded-[var(--radius-md)] p-1 text-[var(--text-secondary)] disabled:opacity-50"
        :disabled="refreshingAccess"
        aria-label="刷新访问检查"
        @click="refreshAccess"
      >
        <RefreshCw :size="15" :class="refreshingAccess && 'animate-spin'" />
      </button>
    </div>
    <div class="grid flex-1 grid-rows-4 gap-1.5" aria-live="polite">
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
