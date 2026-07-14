<script setup lang="ts">
import { onErrorCaptured, ref } from "vue";

const error = ref<Error | null>(null);

onErrorCaptured((captured) => {
  error.value =
    captured instanceof Error ? captured : new Error(String(captured));
  return false;
});
</script>

<template>
  <div
    v-if="error"
    class="flex min-h-[50vh] flex-col items-center justify-center px-6 text-center"
  >
    <div class="mb-4 text-5xl">!</div>
    <h2 class="mb-2 text-xl font-semibold text-[var(--text-primary)]">
      页面出了点问题
    </h2>
    <p class="mb-6 max-w-md text-[var(--text-secondary)]">
      {{ error.message || "发生了未知错误，请重试。" }}
    </p>
    <button class="aw-action-button aw-action-neutral" @click="error = null">
      重试
    </button>
  </div>
  <slot v-else />
</template>
