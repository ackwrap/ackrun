<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
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
const panel = ref<HTMLElement | null>(null);
let previousFocus: HTMLElement | null = null;
const maxWidth = computed(
  () => p.width || { sm: 420, md: 520, lg: 760, xl: 1120 }[p.size],
);
const focusableSelector =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
const key = (e: KeyboardEvent) => {
  if (e.key === "Escape" && p.closable) {
    emit("close");
    return;
  }
  if (e.key !== "Tab" || !panel.value) return;
  const focusable = Array.from(
    panel.value.querySelectorAll<HTMLElement>(focusableSelector),
  );
  if (!focusable.length) {
    e.preventDefault();
    panel.value.focus();
    return;
  }
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault();
    first.focus();
  }
};
watch(
  () => p.open,
  (v) => {
    document.body.style.overflow = v ? "hidden" : "";
    if (v) {
      previousFocus = document.activeElement as HTMLElement | null;
      document.addEventListener("keydown", key);
      void nextTick(() => {
        const first = panel.value?.querySelector<HTMLElement>(focusableSelector);
        (first || panel.value)?.focus();
      });
    } else {
      document.removeEventListener("keydown", key);
      previousFocus?.focus();
      previousFocus = null;
    }
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  document.body.style.overflow = "";
  document.removeEventListener("keydown", key);
  previousFocus?.focus();
});
</script>
<template>
  <Teleport to="body"
    ><div
      v-if="open"
      class="fixed inset-0 z-[var(--z-modal)] flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      @click.self="closable && $emit('close')"
    >
      <div class="absolute inset-0 bg-[var(--bg-overlay)]" />
      <div
        ref="panel"
        tabindex="-1"
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
