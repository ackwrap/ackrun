<script setup lang="ts">
import { AlertTriangle, Check, Info } from "lucide-vue-next";
import { computed } from "vue";
const p = withDefaults(
  defineProps<{ message: string; type?: "success" | "error" | "info" }>(),
  { type: "info" },
);
const icon = computed(() =>
  p.type === "success" ? Check : p.type === "error" ? AlertTriangle : Info,
);
</script>
<template>
  <div
    v-if="message"
    class="pointer-events-none fixed bottom-[13vh] left-1/2 z-[70] w-full max-w-xl -translate-x-1/2 px-4"
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
</template>
