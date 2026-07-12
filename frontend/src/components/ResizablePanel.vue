<script setup lang="ts">
import { ref } from "vue";
const p = withDefaults(
  defineProps<{
    defaultLeftWidth?: number;
    minLeftWidth?: number;
    maxLeftWidth?: number;
  }>(),
  { defaultLeftWidth: 420, minLeftWidth: 280, maxLeftWidth: 700 },
);
const width = ref(p.defaultLeftWidth);
const dragging = ref(false);
function start(e: MouseEvent) {
  e.preventDefault();
  dragging.value = true;
  const x = e.clientX,
    w = width.value;
  const move = (m: MouseEvent) =>
    (width.value = Math.min(
      p.maxLeftWidth,
      Math.max(p.minLeftWidth, w + m.clientX - x),
    ));
  const up = () => {
    dragging.value = false;
    document.removeEventListener("mousemove", move);
    document.removeEventListener("mouseup", up);
  };
  document.addEventListener("mousemove", move);
  document.addEventListener("mouseup", up);
}
</script>
<template>
  <div class="flex h-full" :style="{ userSelect: dragging ? 'none' : 'auto' }">
    <div class="shrink-0 overflow-y-auto" :style="{ width: `${width}px` }">
      <slot name="left" />
    </div>
    <div
      class="w-1 shrink-0 cursor-col-resize bg-[var(--border-default)] hover:bg-blue-500/60"
      @mousedown="start"
    />
    <div class="min-w-0 flex-1 overflow-y-auto"><slot name="right" /></div>
  </div>
</template>
