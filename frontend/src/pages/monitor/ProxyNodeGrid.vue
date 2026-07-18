<script setup lang="ts">
withDefaults(defineProps<{ minWidth?: number }>(), { minWidth: 160 });
</script>

<template>
  <div
    class="grid min-w-0 gap-2.5"
    :style="`grid-template-columns: repeat(auto-fill, minmax(min(${minWidth}px, 100%), 1fr))`"
  >
    <slot />
  </div>
</template>
