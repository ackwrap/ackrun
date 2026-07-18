<script setup lang="ts">
import { LoaderCircle, Zap } from "lucide-vue-next";
import { latencyTagClass } from "./proxyGroupUtils";

const props = withDefaults(
  defineProps<{
    delay?: number;
    loading?: boolean;
    active?: boolean;
    disabled?: boolean;
    title?: string;
  }>(),
  {
    delay: 0,
    loading: false,
    active: false,
    disabled: false,
    title: "测试节点延迟",
  },
);

defineEmits<{ test: [] }>();
</script>

<template>
  <button
    type="button"
    class="flex h-6 min-w-11 shrink-0 items-center justify-center rounded-full px-2 text-[11px] font-medium tabular-nums transition-[filter] hover:brightness-110 disabled:cursor-default"
    :class="
      props.active
        ? 'bg-[var(--color-primary-active)] text-[var(--button-danger-text)]'
        : latencyTagClass(delay)
    "
    :disabled="disabled || loading"
    :title="title"
    @click.stop="$emit('test')"
  >
    <LoaderCircle v-if="loading" :size="12" class="animate-spin" />
    <span v-else-if="delay">{{ delay }}</span>
    <Zap v-else :size="11" />
  </button>
</template>
