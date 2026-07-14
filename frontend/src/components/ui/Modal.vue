<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from "vue";
import { X } from "lucide-vue-next";
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
        class="relative flex max-h-[calc(100vh-2rem)] w-full flex-col overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-elevated)] shadow-[var(--shadow-xl)]"
        :style="{ maxWidth: `${maxWidth}px` }"
        @click.stop
      >
        <header
          class="flex shrink-0 items-center justify-between border-b border-[var(--border-light)] px-6 py-4"
        >
          <h3 class="text-base font-semibold">
            <slot name="title">{{ title }}</slot>
          </h3>
          <button
            v-if="closable"
            class="aw-modal-close inline-flex h-8 w-8 items-center justify-center p-0"
            aria-label="关闭"
            @click="$emit('close')"
          >
            <X :size="16" />
          </button>
        </header>
        <div class="min-h-0 overflow-y-auto px-6 py-4"><slot /></div>
        <footer
          v-if="$slots.footer"
          class="flex shrink-0 justify-end gap-2 border-t border-[var(--border-light)] px-6 py-4"
        >
          <slot name="footer" />
        </footer>
      </div></div
  ></Teleport>
</template>
