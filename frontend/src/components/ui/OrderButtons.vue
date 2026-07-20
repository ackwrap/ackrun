<script setup lang="ts">
import { ArrowDown, ArrowUp } from "lucide-vue-next";

defineProps<{ upDisabled?: boolean; downDisabled?: boolean }>();
defineEmits<{ up: []; down: [] }>();
</script>

<template>
  <div class="flex items-center gap-1.5">
    <button
      type="button"
      class="aw-action-button aw-action-neutral !h-7 !w-7 !px-0"
      :disabled="upDisabled"
      title="上移"
      @click="$emit('up')"
    >
      <ArrowUp :size="13" />
    </button>
    <button
      type="button"
      class="aw-action-button aw-action-neutral !h-7 !w-7 !px-0"
      :disabled="downDisabled"
      title="下移"
      @click="$emit('down')"
    >
      <ArrowDown :size="13" />
    </button>
  </div>
</template>
