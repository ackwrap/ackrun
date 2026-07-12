<script setup lang="ts">
withDefaults(defineProps<{ title?: string; lines?: number }>(), { lines: 3 });
</script>
<template>
  <div class="animate-pulse space-y-5">
    <div v-if="title" class="h-7 w-40 rounded bg-white/[0.06]" />
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div
        v-for="i in 4"
        :key="i"
        class="h-44 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-white/[0.03] p-5"
      />
    </div>
    <div
      v-for="i in lines"
      :key="i"
      class="h-32 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-white/[0.03]"
    />
  </div>
</template>
