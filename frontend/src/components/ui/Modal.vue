<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from "vue";
const p = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    width?: number;
    size?: "sm" | "md" | "lg" | "xl";
    closable?: boolean;
  }>(),
  { size: "md", closable: true },
);
const emit = defineEmits<{ close: [] }>();
const maxWidth = computed(
  () => p.width || { sm: 420, md: 520, lg: 760, xl: 1120 }[p.size],
);
const key = (e: KeyboardEvent) => {
  if (e.key === "Escape" && p.closable) emit("close");
};
watch(
  () => p.open,
  (v) => {
    document.body.style.overflow = v ? "hidden" : "";
    v
      ? document.addEventListener("keydown", key)
      : document.removeEventListener("keydown", key);
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  document.body.style.overflow = "";
  document.removeEventListener("keydown", key);
});
</script>
<template>
  <Teleport to="body"
    ><div
      v-if="open"
      class="fixed inset-0 z-[var(--z-modal)] flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      @click.self="closable && $emit('close')"
    >
      <div class="absolute inset-0 bg-[var(--bg-overlay)]" />
      <div
        class="relative w-full rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-elevated)] shadow-[var(--shadow-xl)]"
        :style="{ maxWidth: `${maxWidth}px` }"
        @click.stop
      >
        <header
          class="flex items-center justify-between border-b border-[var(--border-light)] px-6 py-4"
        >
          <h3 class="text-lg font-semibold">{{ title }}</h3>
          <button v-if="closable" @click="$emit('close')">×</button>
        </header>
        <div class="px-6 py-4"><slot /></div>
        <footer
          v-if="$slots.footer"
          class="flex justify-end gap-2 border-t border-[var(--border-light)] px-6 py-4"
        >
          <slot name="footer" />
        </footer>
      </div></div
  ></Teleport>
</template>
