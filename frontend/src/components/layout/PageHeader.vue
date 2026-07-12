<script setup lang="ts">
defineProps<{ title: string; description?: string; class?: string }>();
</script>
<template>
  <div class="mb-4" :class="$props.class">
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-bold text-[var(--text-primary)]">{{ title }}</h1>
      <div v-if="$slots.actions" class="flex items-center gap-2">
        <slot name="actions" />
      </div>
    </div>
    <p v-if="description" class="mt-1 text-xs text-[var(--text-secondary)]">
      {{ description }}
    </p>
  </div>
</template>
