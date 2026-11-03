<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { Activity, Gauge } from "lucide-vue-next";
import {
  getClashClient,
  type ConnectionsSnapshot,
  type ProxyMap,
} from "@/services/clash";
import { formatBytes, formatSpeed } from "@/utils/traffic";
import AnimatedMetric from "@/components/ui/AnimatedMetric.vue";
import MetricSparkline from "@/components/ui/MetricSparkline.vue";
import { vSpotlight } from "@/directives/spotlight";
import CardTexture from "@/components/ui/CardTexture.vue";
import { appendTrafficSample } from "@/utils/metricMotion";

const props = defineProps<{
  isRunning: boolean;
  proxyPort: number;
  snapshot: ConnectionsSnapshot | null;
  proxies: ProxyMap | null;
  error: string;
}>();
const totals = computed(() => props.snapshot);
const proxyGroups = computed(() =>
  props.proxies
    ? Object.values(props.proxies).filter((item) => Array.isArray(item.all))
        .length
    : null,
);
const upload = ref(0);
const download = ref(0);
const trafficReady = ref(false);
const trafficError = ref("");
const uploadTrend = ref<number[]>([]);
const downloadTrend = ref<number[]>([]);
const client = getClashClient();
let generation = 0;
const speed = (value: number) => formatSpeed(Math.round(value));
const items = computed(() => [
  {
    label: "上传",
    value: trafficReady.value ? upload.value : null,
    format: speed,
    animate: true,
    trend: uploadTrend.value,
  },
  {
    label: "下载",
    value: trafficReady.value ? download.value : null,
    format: speed,
    animate: true,
    trend: downloadTrend.value,
  },
  {
    label: "上传总量",
    value: totals.value?.uploadTotal ?? null,
    format: formatBytes,
  },
  {
    label: "下载总量",
    value: totals.value?.downloadTotal ?? null,
    format: formatBytes,
  },
  {
    label: "活动连接",
    value: totals.value?.connections.length ?? null,
    feedback: true,
  },
  {
    label: "内存占用",
    value: totals.value?.memory ?? null,
    format: formatBytes,
  },
  { label: "策略组", value: proxyGroups.value },
  { label: "代理端口", value: props.proxyPort || null },
]);
const status = computed(() =>
  !props.isRunning
    ? "核心未运行"
    : trafficError.value ||
      (props.error
        ? "统计不可用"
        : !trafficReady.value || !totals.value
          ? "正在连接"
          : "实时更新"),
);

function stop() {
  generation++;
  client.disconnectTraffic();
}
function start() {
  stop();
  upload.value = 0;
  download.value = 0;
  trafficReady.value = false;
  trafficError.value = "";
  uploadTrend.value = [];
  downloadTrend.value = [];
  if (!props.isRunning) return;
  const run = generation;
  client.connectTraffic(
    (value) => {
      if (run !== generation) return;
      upload.value = value.up;
      download.value = value.down;
      uploadTrend.value = appendTrafficSample(uploadTrend.value, value.up);
      downloadTrend.value = appendTrafficSample(
        downloadTrend.value,
        value.down,
      );
      trafficReady.value = true;
      trafficError.value = "";
    },
    () => {
      if (run !== generation) return;
      trafficReady.value = false;
      trafficError.value = "实时流量断开";
      uploadTrend.value = [];
      downloadTrend.value = [];
    },
  );
}
watch(() => props.isRunning, start, { immediate: true });
onBeforeUnmount(stop);
</script>

<template>
  <section
    v-spotlight
    class="aw-textured min-w-0 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)]"
  >
    <CardTexture variant="dots" />
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
        v-for="item in items"
        :key="item.label"
        class="aw-stat-tile relative flex min-h-[76px] min-w-0 flex-col items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] p-2 text-center"
      >
        <dt class="text-[11px] text-[var(--text-secondary)]">
          {{ item.label }}
        </dt>
        <dd
          class="aw-stat-value mt-2 max-w-full break-words text-xs font-semibold text-[var(--color-success)]"
        >
          <AnimatedMetric
            :value="item.value"
            :format="item.format"
            :animate="!!item.animate"
            :feedback="!!item.feedback"
          />
        </dd>
        <MetricSparkline v-if="item.trend" :samples="item.trend" />
      </div>
    </dl>
  </section>
</template>
