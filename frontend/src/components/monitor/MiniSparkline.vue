<script setup lang="ts">
import { computed, useId } from "vue";
const p = withDefaults(
  defineProps<{
    data: number[];
    color?: "green" | "blue" | "purple";
    class?: string;
  }>(),
  { color: "blue", class: "" },
);
const id = useId().replace(/:/g, ""),
  colors = { green: "#10b981", blue: "#3b82f6", purple: "#a855f7" };
const paths = computed(() => {
  if (p.data.length < 2) return null;
  const max = Math.max(1, ...p.data),
    line = p.data
      .map(
        (v, i) =>
          `${i ? "L" : "M"} ${((i / (p.data.length - 1)) * 100).toFixed(2)} ${(56 - Math.max(2, (v / max) * 51)).toFixed(2)}`,
      )
      .join(" ");
  return { line, area: `${line} L 100 56 L 0 56 Z` };
});
</script>
<template>
  <div v-if="!paths" :class="['h-14 w-full', p.class]" />
  <svg
    v-else
    :class="['h-14 w-full', p.class]"
    viewBox="0 0 100 56"
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    <defs>
      <linearGradient :id="id" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" :stop-color="colors[color]" stop-opacity=".32" />
        <stop offset="100%" :stop-color="colors[color]" stop-opacity="0" />
      </linearGradient>
    </defs>
    <path :d="paths.area" :fill="`url(#${id})`" />
    <path
      :d="paths.line"
      fill="none"
      :stroke="colors[color]"
      stroke-width="1.6"
      vector-effect="non-scaling-stroke"
    />
  </svg>
</template>
