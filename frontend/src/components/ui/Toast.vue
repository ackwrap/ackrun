<script setup lang="ts">
import { AlertTriangle, Check, Info } from "lucide-vue-next";
import { computed, onBeforeUnmount, watch } from "vue";
const p = withDefaults(
  defineProps<{
    message: string;
    type?: "success" | "error" | "info";
    duration?: number;
  }>(),
  { type: "info" },
);
const emit = defineEmits<{ dismiss: [] }>();
const icon = computed(() =>
  p.type === "success" ? Check : p.type === "error" ? AlertTriangle : Info,
);
const dismissAfter = computed(
  () => p.duration ?? (p.type === "error" ? 5000 : 3000),
);
let timer: number | undefined;
watch(
  () => [p.message, p.type, p.duration] as const,
  ([message]) => {
    window.clearTimeout(timer);
    if (message && dismissAfter.value > 0)
      timer = window.setTimeout(() => emit("dismiss"), dismissAfter.value);
  },
  { immediate: true },
);
onBeforeUnmount(() => window.clearTimeout(timer));
</script>
<template>
  <Teleport to="body">
    <div
      v-if="message"
      class="pointer-events-none fixed bottom-[13vh] left-1/2 z-[var(--z-toast)] w-full max-w-xl -translate-x-1/2 px-4"
    >
      <div
        class="aw-toast"
        :class="`aw-toast-${type}`"
        :role="type === 'error' ? 'alert' : 'status'"
      >
        <span class="aw-toast-icon"><component :is="icon" :size="19" /></span
        ><span
          ><span class="aw-toast-label">{{
            type === "success" ? "成功" : type === "error" ? "失败" : "提示"
          }}</span
          ><span class="aw-toast-message">{{ message }}</span></span
        >
      </div>
    </div>
  </Teleport>
</template>
