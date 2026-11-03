<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "@/services/api";
import type { ControlSummary } from "@/services/types";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { formatBytes } from "@/utils/traffic";

const summary = ref<ControlSummary | null>(null);
const error = ref("");
let request: AbortController | undefined;
let timer: ReturnType<typeof setInterval> | undefined;
let disposed = false;

async function refresh() {
  if (request || disposed) return;
  const current = new AbortController();
  request = current;
  try {
    const result = await api.getControlSummary(current.signal);
    if (disposed) return;
    summary.value = result;
    error.value = "";
  } catch (cause: any) {
    if (!disposed && !current.signal.aborted)
      error.value = cause.message || "概览加载失败";
  } finally {
    request = undefined;
  }
}

const usage = computed(() => summary.value?.monthly_traffic);
const totalUsed = computed(() =>
  usage.value
    ? formatBytes(usage.value.upload_bytes + usage.value.download_bytes)
    : "--",
);
const expiry = computed(() => summary.value?.next_expiry);
const expiryDate = computed(() =>
  expiry.value
    ? new Date(expiry.value.expire_at * 1000).toLocaleDateString("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      })
    : "暂无到期信息",
);
const remainingDays = computed(() =>
  expiry.value
    ? Math.max(
        0,
        Math.ceil((expiry.value.expire_at * 1000 - Date.now()) / 86_400_000),
      )
    : 0,
);
const trackingTitle = computed(() => {
  const since = usage.value?.tracking_since;
  return since
    ? `自 ${new Date(since * 1000).toLocaleString("zh-CN")} 开始记录核心上传与下载，每 5 秒累计；未记录的历史流量不补算。跨月首个采样间隔不计入。`
    : "后台按月累计核心上传与下载流量";
});

useRealtimeSocket((event) => {
  if (
    event.type === "subscription.sync" ||
    event.type === "subscription.sync_all"
  )
    void refresh();
});
onMounted(() => {
  void refresh();
  timer = setInterval(refresh, 15_000);
});
onBeforeUnmount(() => {
  disposed = true;
  if (timer) clearInterval(timer);
  request?.abort();
});
defineExpose({ refresh });
</script>

<template>
  <div class="min-w-0 space-y-2">
    <div class="grid grid-cols-3 gap-2">
      <div class="min-w-0" :title="trackingTitle">
        <h2
          class="h-4 whitespace-nowrap text-xs font-normal leading-4 text-[var(--text-secondary)]"
        >
          本月已用流量
        </h2>
        <div
          class="mt-2 flex h-11 items-center rounded-[var(--radius-full)] bg-[var(--bg-inset)] px-3 text-sm font-semibold tabular-nums"
        >
          <span class="truncate">{{ totalUsed }}</span>
        </div>
      </div>
      <div
        class="min-w-0"
        :title="`汇总订阅上报的总额度，不包含本地导入；${summary?.traffic_reported_count || 0} / ${summary?.subscription_count || 0} 个订阅已上报。`"
      >
        <h2
          class="h-4 whitespace-nowrap text-xs font-normal leading-4 text-[var(--text-secondary)]"
        >
          订阅总流量
        </h2>
        <div
          class="mt-2 flex h-11 items-center rounded-[var(--radius-full)] bg-[var(--bg-inset)] px-3 text-sm font-semibold tabular-nums"
        >
          <span class="truncate">{{
            summary
              ? summary.traffic_reported_count || !summary.subscription_count
                ? formatBytes(summary.subscription_total_bytes)
                : "未提供"
              : "--"
          }}</span>
        </div>
      </div>
      <div
        class="min-w-0"
        :title="
          expiry
            ? `${expiry.name} · ${expiryDate} · 剩余 ${remainingDays} 天`
            : expiryDate
        "
      >
        <h2
          class="h-4 whitespace-nowrap text-xs font-normal leading-4 text-[var(--text-secondary)]"
        >
          最近到期订阅
        </h2>
        <div
          class="mt-2 flex h-11 items-center rounded-[var(--radius-full)] bg-[var(--bg-inset)] px-3 text-sm font-semibold"
          :class="
            expiry && remainingDays <= 7 ? 'text-[var(--color-warning)]' : ''
          "
        >
          <span class="truncate">{{
            summary
              ? expiry
                ? `${remainingDays} 天 · ${expiry.name}`
                : summary.expired_subscription_count
                  ? "已到期"
                  : "暂无"
              : "--"
          }}</span>
        </div>
      </div>
    </div>
    <p
      v-if="summary?.traffic_error"
      role="status"
      class="text-xs text-[var(--color-warning)]"
    >
      {{ summary.traffic_error }}，显示已记录用量
    </p>
    <p v-if="error" role="alert" class="px-2 text-xs text-[var(--color-error)]">
      {{ error
      }}<button type="button" class="ml-2 underline" @click="refresh">
        重试
      </button>
    </p>
  </div>
</template>
