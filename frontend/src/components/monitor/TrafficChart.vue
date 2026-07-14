<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import type { ECharts } from "echarts";
const p = withDefaults(defineProps<{ class?: string }>(), { class: "" }),
  el = ref<HTMLDivElement | null>(null),
  data: { time: number; upload: number; download: number }[] = [];
let chart: ECharts | null = null,
  disposed = false;
const speed = (n: number) => {
  if (!n) return "0 B/s";
  const s = ["B/s", "KB/s", "MB/s", "GB/s"],
    i = Math.min(3, Math.floor(Math.log(n) / Math.log(1024)));
  return `${Math.round((n / 1024 ** i) * 100) / 100} ${s[i]}`;
};
function update() {
  chart?.setOption({
    xAxis: { data: data.map((x) => String(x.time)) },
    series: [
      { data: data.map((x) => x.upload) },
      { data: data.map((x) => x.download) },
    ],
  });
}
function addData(upload: number, download: number) {
  data.push({ time: Date.now(), upload, download });
  if (data.length > 60) data.shift();
  update();
}
defineExpose({ addData });
const resize = () => chart?.resize();
onMounted(async () => {
  const e = await import("echarts");
  if (!el.value || disposed) return;
  chart = e.init(el.value);
  chart.setOption({
    backgroundColor: "transparent",
    grid: { left: 8, right: 12, bottom: 8, top: 12, containLabel: true },
    tooltip: {
      trigger: "axis",
      formatter: (x: any[]) =>
        `${x[0].axisValueLabel}<br/>上传: ${speed(x[0].value)}<br/>下载: ${speed(x[1].value)}`,
    },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: [],
      axisTick: { show: false },
      axisLabel: {
        color: "#7B8AA2",
        fontSize: 10,
        formatter: (v: string) => new Date(+v).toLocaleTimeString(),
      },
    },
    yAxis: {
      type: "value",
      axisLabel: { color: "#7B8AA2", fontSize: 10, formatter: speed },
      splitLine: { lineStyle: { color: "rgba(148,163,184,.1)" } },
    },
    series: [
      {
        name: "上传",
        type: "line",
        smooth: true,
        symbol: "none",
        lineStyle: { color: "#10b981" },
        areaStyle: { opacity: 0.15 },
        data: [],
      },
      {
        name: "下载",
        type: "line",
        smooth: true,
        symbol: "none",
        lineStyle: { color: "#3b82f6" },
        areaStyle: { opacity: 0.15 },
        data: [],
      },
    ],
  });
  window.addEventListener("resize", resize);
});
onBeforeUnmount(() => {
  disposed = true;
  window.removeEventListener("resize", resize);
  chart?.dispose();
  chart = null;
});
</script>
<template>
  <div ref="el" :class="['w-full', p.class || 'h-[300px]']" />
</template>
