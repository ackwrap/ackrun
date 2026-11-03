<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Clock3, Cpu, Menu, Moon, Sun } from "lucide-vue-next";
import Sidebar from "./Sidebar.vue";
import PageSkeleton from "./PageSkeleton.vue";
import ErrorBoundary from "./ErrorBoundary.vue";
import Toast from "@/components/ui/Toast.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import type { RuntimeResponse, RuntimeStatus } from "@/services/types";

type Theme = "dark" | "light";
const collapsed = ref(false);
const mobileOpen = ref(false);
const theme = ref<Theme>(
  (localStorage.getItem("ackwrap.theme") as Theme) || "dark",
);
const reconcileError = ref("");
const uptimeSeconds = ref<number | null>(null);
const uptimeSyncedAt = ref(0);
const coreUptimeSeconds = ref<number | null>(null);
const coreUptimeSyncedAt = ref(0);
const coreStatus = ref<RuntimeStatus | null>(null);
const now = ref(performance.now());
let clockTimer: ReturnType<typeof setInterval> | undefined;

function formatDuration(baseSeconds: number | null, syncedAt: number) {
  if (baseSeconds === null) return "--:--:--";
  const seconds = Math.max(
    0,
    baseSeconds + Math.floor((now.value - syncedAt) / 1000),
  );
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  const clock = [hours, minutes, remainder]
    .map((value) => String(value).padStart(2, "0"))
    .join(":");
  return days > 0 ? `${days}天 ${clock}` : clock;
}

const runtimeDuration = computed(() =>
  formatDuration(uptimeSeconds.value, uptimeSyncedAt.value),
);
const coreRuntimeDuration = computed(() =>
  coreStatus.value && coreStatus.value !== "running"
    ? "未运行"
    : formatDuration(coreUptimeSeconds.value, coreUptimeSyncedAt.value),
);

watch(
  theme,
  (value) => {
    document.documentElement.dataset.theme = value;
    localStorage.setItem("ackwrap.theme", value);
  },
  { immediate: true },
);
const { connected } = useRealtimeSocket((event) => {
  if (event.type === "runtime.status") {
    const data = event.data as RuntimeResponse;
    coreStatus.value = data.status;
    if (typeof data.uptime_seconds === "number") {
      now.value = performance.now();
      uptimeSeconds.value = data.uptime_seconds;
      uptimeSyncedAt.value = now.value;
    }
    if (typeof data.core_uptime_seconds === "number") {
      now.value = performance.now();
      coreUptimeSeconds.value = data.core_uptime_seconds;
      coreUptimeSyncedAt.value = now.value;
    } else if (data.status !== "running") {
      coreUptimeSeconds.value = null;
    }
    return;
  }
  if (event.type !== "config.reconcile") return;
  const data = event.data as { status?: string; error?: string };
  if (data.status === "failed")
    reconcileError.value = data.error || "配置自动应用失败";
});

onMounted(() => {
  clockTimer = setInterval(() => {
    now.value = performance.now();
  }, 1000);
});

onBeforeUnmount(() => {
  if (clockTimer) clearInterval(clockTimer);
});
</script>
<template>
  <div class="flex h-screen bg-[var(--bg-base)] text-[var(--text-primary)]">
    <Toast
      :message="reconcileError"
      type="error"
      @dismiss="reconcileError = ''"
    />
    <div
      v-if="mobileOpen"
      class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
      @click="mobileOpen = false"
    />
    <Sidebar
      :collapsed="collapsed"
      :mobile-open="mobileOpen"
      @toggle="collapsed = !collapsed"
      @close="mobileOpen = false"
    />
    <main
      id="main-content"
      class="flex min-w-0 flex-1 flex-col overflow-hidden"
    >
      <header
        class="flex h-[62px] shrink-0 items-center justify-between border-b border-[var(--border-light)] bg-[var(--header-bg)] px-5 backdrop-blur-xl"
      >
        <button
          class="flex h-9 w-9 items-center justify-center rounded-full border border-[var(--border-default)] bg-[var(--button-secondary-bg)] lg:hidden"
          title="打开导航"
          @click="mobileOpen = true"
        >
          <Menu :size="18" />
        </button>
        <div class="hidden w-5 lg:block" />
        <div class="ml-auto flex items-center gap-2 text-sm sm:gap-3">
          <div
            class="hidden h-9 items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 sm:inline-flex"
            title="AckWrap 后端运行时间"
          >
            <Clock3 :size="15" class="text-[var(--text-secondary)]" />
            <span class="hidden text-xs text-[var(--text-secondary)] md:inline"
              >运行时间</span
            >
            <span class="font-mono text-xs tabular-nums">{{
              runtimeDuration
            }}</span>
          </div>
          <div
            class="hidden h-9 items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 sm:inline-flex"
            title="sing-box 核心运行时间"
          >
            <Cpu :size="15" class="text-[var(--text-secondary)]" />
            <span class="hidden text-xs text-[var(--text-secondary)] lg:inline"
              >核心运行时间</span
            >
            <span class="font-mono text-xs tabular-nums">{{
              coreRuntimeDuration
            }}</span>
          </div>
          <button
            class="inline-flex h-9 items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs"
            @click="theme = theme === 'dark' ? 'light' : 'dark'"
          >
            <Sun v-if="theme === 'dark'" :size="15" /><Moon
              v-else
              :size="15"
            />{{ theme === "dark" ? "白天" : "夜间" }}
          </button>
          <span class="inline-flex items-center gap-2"
            ><span
              class="h-2 w-2 rounded-full"
              :class="
                connected
                  ? 'bg-emerald-400 animate-status-pulse'
                  : 'bg-gray-500'
              "
            />{{ connected ? "已连接" : "连接中" }}</span
          >
        </div>
      </header>
      <div class="flex-1 overflow-auto">
        <div class="h-full px-4 py-5 sm:px-6 lg:px-7">
          <RouterView v-slot="{ Component }"
            ><ErrorBoundary
              ><Suspense
                ><component :is="Component" /><template #fallback
                  ><PageSkeleton /></template></Suspense></ErrorBoundary
          ></RouterView>
        </div>
      </div>
    </main>
  </div>
</template>
