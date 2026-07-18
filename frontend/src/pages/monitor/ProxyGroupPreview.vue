<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { ProxyMap } from "./proxyGroupUtils";
import {
  latencyBackgroundClass,
  latencyLevel,
  latestDelay,
} from "./proxyGroupUtils";

const props = withDefaults(
  defineProps<{
    nodes: string[];
    now?: string;
    proxies: ProxyMap;
    selectable?: boolean;
  }>(),
  { now: "", selectable: false },
);

defineEmits<{ select: [string] }>();

const previewRef = ref<HTMLElement | null>(null);
const width = ref(0);
let observer: ResizeObserver | undefined;

const nodesWithDelay = computed(() =>
  props.nodes.map((name) => ({
    name,
    delay: latestDelay(props.proxies[name]),
  })),
);

const showDots = computed(
  () => props.nodes.length > 0 && width.value > props.nodes.length * 20,
);

const distribution = computed(() => {
  const result = { low: 0, medium: 0, high: 0, unknown: 0 };
  for (const node of nodesWithDelay.value) {
    result[latencyLevel(node.delay)] += 1;
  }
  return result;
});

function percentage(count: number) {
  return props.nodes.length ? `${(count / props.nodes.length) * 100}%` : "0%";
}

onMounted(() => {
  if (!previewRef.value) return;
  observer = new ResizeObserver(([entry]) => {
    if (!entry) return;
    width.value = entry.contentRect.width;
  });
  observer.observe(previewRef.value);
});

onBeforeUnmount(() => observer?.disconnect());
</script>

<template>
  <div
    ref="previewRef"
    class="flex flex-wrap"
    :class="showDots ? 'gap-1.5 pt-4' : 'gap-2 pt-5 pb-1'"
  >
    <template v-if="showDots">
      <button
        v-for="node in nodesWithDelay"
        :key="node.name"
        type="button"
        class="flex h-4 w-4 items-center justify-center rounded-full transition hover:scale-110 disabled:cursor-default"
        :class="latencyBackgroundClass(node.delay)"
        :disabled="!selectable"
        :title="`${node.name}${node.delay ? ` · ${node.delay} ms` : ''}`"
        @click.stop="$emit('select', node.name)"
      >
        <span
          v-if="now === node.name"
          class="h-2 w-2 rounded-full bg-[var(--button-danger-text)]"
        />
      </button>
    </template>
    <div
      v-else
      class="flex h-2 flex-1 items-center justify-center overflow-hidden rounded-full"
    >
      <span
        class="h-full bg-[var(--latency-low)]"
        :style="{ width: percentage(distribution.low) }"
      />
      <span
        class="h-full bg-[var(--latency-medium)]"
        :style="{ width: percentage(distribution.medium) }"
      />
      <span
        class="h-full bg-[var(--latency-high)]"
        :style="{ width: percentage(distribution.high) }"
      />
      <span
        class="h-full bg-[var(--latency-unknown)]"
        :style="{ width: percentage(distribution.unknown) }"
      />
    </div>
  </div>
</template>
