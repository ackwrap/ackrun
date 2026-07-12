<script setup lang="ts">
import { computed } from "vue";
import { ArrowDown, ArrowUp, Cpu, GitBranch, WifiOff } from "lucide-vue-next";
import type { Connection, ProxyGroup } from "@/services/clash";
import { formatBytes, formatSpeed } from "./monitorUtils";
import MiniSparkline from "@/components/monitor/MiniSparkline.vue";
import TrafficChart from "@/components/monitor/TrafficChart.vue";
import ProxyGroupIcon from "./ProxyGroupIcon.vue";
const p = defineProps<{
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
defineEmits<{ openConnections: []; openProxies: [] }>();
defineExpose({});
const recent = computed(() =>
  [...p.connections]
    .sort((a, b) => b.upload + b.download - a.upload - a.download)
    .slice(0, 5),
);
</script>
<template>
  <div class="space-y-4 pb-5">
    <section v-if="!connected" class="rounded-xl border p-4">
      <WifiOff /><b>无法连接实时监控</b>
      <p>{{ unavailableReason }}</p>
      <a href="/control">检查核心状态</a>
    </section>
    <section
      class="overflow-hidden rounded-xl border border-[var(--border-default)]"
    >
      <header class="flex justify-between p-4">
        <b>网络脉冲 · {{ connected ? "LIVE" : "OFFLINE" }}</b
        ><span>累计流量 {{ formatBytes(totalUp + totalDown) }}</span>
      </header>
      <div class="grid sm:grid-cols-2 xl:grid-cols-4">
        <div
          v-for="m in [
            {
              i: ArrowDown,
              l: '实时下载',
              v: formatSpeed(speedDown),
              h: downloadSpeedHistory,
              c: 'blue',
            },
            {
              i: ArrowUp,
              l: '实时上传',
              v: formatSpeed(speedUp),
              h: uploadSpeedHistory,
              c: 'green',
            },
            {
              i: GitBranch,
              l: '活动连接',
              v: String(connectionCount),
              h: connectionCountHistory,
              c: 'purple',
            },
            {
              i: Cpu,
              l: '内存占用',
              v: memory ? formatBytes(memory) : '--',
              h: memoryHistory,
              c: 'purple',
            },
          ]"
          class="relative min-h-36 border p-4"
        >
          <component :is="m.i" :size="15" />{{ m.l }}
          <div class="text-2xl">{{ m.v }}</div>
          <MiniSparkline
            :data="m.h"
            :color="m.c as any"
            class="absolute inset-x-0 bottom-0"
          />
        </div>
      </div>
    </section>
    <section class="rounded-xl border p-4">
      <h3>实时吞吐</h3>
      <TrafficChart v-if="connected" ref="chart" class="h-[300px]" />
      <div v-else class="h-[300px] grid place-items-center">
        等待实时流量数据
      </div>
    </section>
    <div class="grid gap-4 xl:grid-cols-2">
      <section class="rounded-xl border">
        <header class="flex justify-between p-4">
          <b>活动连接流向</b
          ><button @click="$emit('openConnections')">查看全部</button>
        </header>
        <div v-if="!recent.length" class="p-12 text-center">暂无活动连接</div>
        <div v-for="c in recent" :key="c.id" class="border-t p-3">
          <b>{{ c.metadata.host || c.metadata.destinationIP }}</b
          ><span class="float-right"
            >↓ {{ formatBytes(c.download) }} ↑ {{ formatBytes(c.upload) }}</span
          ><small class="block">{{ c.chains?.join(" → ") || "DIRECT" }}</small>
        </div>
      </section>
      <section class="rounded-xl border p-4">
        <header class="flex justify-between">
          <b>策略组快照</b
          ><button @click="$emit('openProxies')">管理策略</button>
        </header>
        <div
          v-for="g in proxyGroups.slice(0, 6)"
          :key="g.name"
          class="mt-2 flex items-center gap-3 rounded border p-2"
        >
          <ProxyGroupIcon :group="g" /><span
            >{{ g.name
            }}<small class="block">{{ g.now || "未选择" }}</small></span
          >
        </div>
      </section>
    </div>
  </div>
</template>
