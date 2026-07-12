<script setup lang="ts">
import { computed } from "vue";
const p = withDefaults(
  defineProps<{
    variant?: "primary" | "secondary" | "ghost" | "danger" | "link";
    size?: "sm" | "md" | "lg";
    loading?: boolean;
    fullWidth?: boolean;
    disabled?: boolean;
    type?: "button" | "submit" | "reset";
  }>(),
  { variant: "secondary", size: "md", type: "button" },
);
const variants = {
  primary:
    "border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]",
  secondary:
    "bg-[var(--button-secondary-bg)] border border-[var(--border-default)]",
  ghost: "bg-transparent text-[var(--text-secondary)]",
  danger: "bg-[var(--color-error)] text-white",
  link: "bg-transparent text-[var(--color-primary)]",
};
const sizes = {
  sm: "h-7 px-3 text-xs",
  md: "h-8 px-3.5 text-xs",
  lg: "h-9 px-4 text-sm",
};
const classes = computed(() => [
  variants[p.variant],
  sizes[p.size],
  p.fullWidth && "w-full",
]);
</script>
<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    class="btn-press focus-ring inline-flex items-center justify-center gap-1.5 rounded-[var(--radius-lg)] font-medium disabled:opacity-50"
    :class="classes"
  >
    <span
      v-if="loading"
      class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
    /><slot v-else name="icon" /><slot />
  </button>
</template>
