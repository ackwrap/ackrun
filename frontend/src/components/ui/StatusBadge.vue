<script setup lang="ts">
import { computed } from "vue";
type S = "running" | "stopped" | "error" | "pending" | "online" | "offline";
const p = withDefaults(
  defineProps<{
    status: S;
    label?: string;
    pulse?: boolean;
    size?: "sm" | "md";
  }>(),
  { size: "md" },
);
const color: Record<S, string> = {
  running: "bg-[var(--color-success)]",
  online: "bg-[var(--color-success)]",
  stopped: "bg-[var(--text-disabled)]",
  offline: "bg-[var(--text-disabled)]",
  error: "bg-[var(--color-error)]",
  pending: "bg-[var(--color-warning)]",
};
const pulsing = computed(
  () => p.pulse ?? ["running", "online"].includes(p.status),
);
</script>
<template>
  <span class="inline-flex items-center gap-1.5"
    ><span
      class="inline-block rounded-full"
      :class="[
        color[status],
        size === 'sm' ? 'h-1.5 w-1.5' : 'h-2 w-2',
        pulsing && 'animate-status-pulse',
      ]"
    /><span v-if="label" class="text-sm text-[var(--text-secondary)]">{{
      label
    }}</span></span
  >
</template>
