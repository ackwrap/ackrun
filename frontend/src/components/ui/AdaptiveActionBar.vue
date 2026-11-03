<script setup lang="ts">
import { ref } from "vue";
import { ChevronDown, MoreHorizontal } from "lucide-vue-next";
import { useAdaptiveActions } from "@/composables/useAdaptiveActions";

const props = defineProps<{ id: string; label: string }>();
const headerRow = ref<HTMLElement | null>(null);
const summaryGroup = ref<HTMLElement | null>(null);
const actionsRow = ref<HTMLElement | null>(null);
const { actionsCollapsed, actionsOpen, chooseAction } = useAdaptiveActions(
  headerRow,
  summaryGroup,
  actionsRow,
  props.id,
);
</script>

<template>
  <header
    ref="headerRow"
    class="relative flex items-center justify-between gap-3"
  >
    <div ref="summaryGroup" class="min-w-0 max-w-[calc(100%-2.75rem)] shrink-0">
      <div class="flex w-max max-w-full items-center gap-2">
        <slot name="title" />
      </div>
    </div>
    <button
      v-if="actionsCollapsed"
      class="aw-action-button w-8 !px-0"
      :aria-expanded="actionsOpen"
      :aria-controls="id"
      :aria-label="`${actionsOpen ? '收起' : '展开'}${label}`"
      :title="`${actionsOpen ? '收起' : '展开'}${label}`"
      @click="actionsOpen = !actionsOpen"
    >
      <ChevronDown v-if="actionsOpen" :size="16" />
      <MoreHorizontal v-else :size="16" />
    </button>
    <div
      :id="id"
      ref="actionsRow"
      class="gap-2 [&>button]:shrink-0"
      :class="
        actionsCollapsed
          ? actionsOpen
            ? 'absolute right-0 top-full z-20 mt-2 flex max-h-[70vh] w-max max-w-full flex-col items-stretch overflow-y-auto rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-elevated)] p-2 shadow-[var(--shadow-card)] [&>button]:justify-start'
            : 'invisible pointer-events-none absolute inset-x-0 top-0 flex h-0 w-full flex-wrap overflow-hidden'
          : 'flex flex-nowrap'
      "
      :inert="actionsCollapsed && !actionsOpen"
      :aria-hidden="actionsCollapsed && !actionsOpen ? true : undefined"
      role="group"
      :aria-label="label"
      @click="chooseAction"
    >
      <slot />
    </div>
  </header>
</template>
