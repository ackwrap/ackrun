<script setup lang="ts">
withDefaults(
  defineProps<{ padding?: "none" | "sm" | "md" | "lg"; hoverable?: boolean }>(),
  { padding: "md" },
);
const pads = { none: "", sm: "p-3", md: "p-5", lg: "p-6" };
</script>
<template>
  <div
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-card)]"
    :class="[
      pads[padding],
      hoverable && 'transition hover:border-[var(--border-strong)]',
    ]"
  >
    <slot />
  </div>
</template>
