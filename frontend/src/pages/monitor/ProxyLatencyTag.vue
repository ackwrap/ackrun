<script setup lang="ts">
import { LoaderCircle, Zap } from "lucide-vue-next";
import { latencyTextClass } from "./proxyGroupUtils";

const props = withDefaults(
  defineProps<{
    delay?: number;
    loading?: boolean;
    active?: boolean;
    disabled?: boolean;
  }>(),
  { delay: 0, loading: false, active: false, disabled: false },
);

defineEmits<{ test: [] }>();
</script>

<template>
  <button
    type="button"
    class="flex h-5 min-w-10 shrink-0 items-center justify-center rounded-xl bg-[var(--button-secondary-bg)] px-1.5 text-[10px] tabular-nums transition-colors hover:bg-[var(--button-secondary-hover)] disabled:cursor-default"
    :class="
      props.active
        ? 'bg-[var(--color-primary-active)] text-[var(--button-danger-text)]'
        : latencyTextClass(delay)
    "
    :disabled="disabled || loading"
    title="测试节点延迟"
    @click.stop="$emit('test')"
  >
    <LoaderCircle v-if="loading" :size="12" class="animate-spin" />
    <span v-else-if="delay">{{ delay }}</span>
    <Zap v-else :size="11" />
  </button>
</template>
