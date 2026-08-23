<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string;
    description?: string;
    class?: string;
    embedded?: boolean;
  }>(),
  { embedded: false },
);
</script>
<template>
  <div class="mb-4" :class="$props.class">
    <div class="flex items-center justify-between">
      <component
        :is="embedded ? 'h2' : 'h1'"
        class="text-lg font-bold text-[var(--text-primary)]"
        >{{ title }}</component
      >
      <div v-if="$slots.actions" class="flex items-center gap-2">
        <slot name="actions" />
      </div>
    </div>
    <p v-if="description" class="mt-1 text-xs text-[var(--text-secondary)]">
      {{ description }}
    </p>
  </div>
</template>
