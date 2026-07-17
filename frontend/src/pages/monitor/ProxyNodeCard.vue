<script setup lang="ts">
import { computed } from "vue";
import type { ProxyNode } from "@/services/clash";
import ProxyLatencyTag from "./ProxyLatencyTag.vue";
import { latestDelay, proxyNodeDescription } from "./proxyGroupUtils";

const props = withDefaults(
  defineProps<{
    name: string;
    node?: ProxyNode;
    active?: boolean;
    selectable?: boolean;
    testing?: boolean;
  }>(),
  { active: false, selectable: false, testing: false },
);

const emit = defineEmits<{ select: []; test: [] }>();
const delay = computed(() => latestDelay(props.node));
const description = computed(() => proxyNodeDescription(props.node));

function select() {
  if (props.selectable) emit("select");
}
</script>

<template>
  <div
    class="flex min-w-0 flex-col items-start gap-2 rounded-md border p-2 transition-colors"
    :class="[
      active
        ? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--button-danger-text)]'
        : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]',
      selectable ? 'cursor-pointer' : 'cursor-default',
    ]"
    :role="selectable ? 'button' : undefined"
    :tabindex="selectable ? 0 : -1"
    :title="name"
    :data-proxy-name="name"
    @click="select"
    @keydown.enter.prevent="select"
  >
    <span class="w-full truncate text-sm font-medium">{{ name }}</span>
    <span class="flex h-4 w-full items-center justify-between gap-2">
      <small class="truncate text-[10px] tracking-tight uppercase opacity-65">{{
        description
      }}</small>
      <ProxyLatencyTag
        :delay="delay"
        :loading="testing"
        :active="active"
        @test="$emit('test')"
      />
    </span>
  </div>
</template>
