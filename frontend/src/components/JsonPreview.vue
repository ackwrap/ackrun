<script setup lang="ts">
import { computed } from "vue";
const p = withDefaults(defineProps<{ data: unknown; maxHeight?: string }>(), {
  maxHeight: "70vh",
});
const content = computed(() => {
  try {
    return JSON.stringify(p.data, null, 2);
  } catch {
    return String(p.data);
  }
});
</script>
<template>
  <div
    class="h-full overflow-auto rounded-lg border border-[var(--border-default)] bg-black/30"
  >
    <pre
      class="whitespace-pre p-4 font-mono text-xs leading-5"
      :style="{ maxHeight }"
      >{{ content }}</pre>
  </div>
</template>
