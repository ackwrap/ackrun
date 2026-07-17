<script setup lang="ts">
import { computed, ref } from "vue";
import { Bolt, Folder, LayoutGrid, RefreshCw } from "lucide-vue-next";
import type { ProxyGroup } from "@/services/clash";
import ProxyGroupCard from "./ProxyGroupCard.vue";
import type { ProxyMap } from "./proxyGroupUtils";

type GroupFilter = "all" | "selector" | "automatic";

const props = withDefaults(
  defineProps<{
    proxies: ProxyMap;
    proxyGroups: ProxyGroup[];
    loading: boolean;
    testingNodes?: Set<string>;
  }>(),
  { testingNodes: () => new Set<string>() },
);

const emit = defineEmits<{
  refresh: [];
  selectProxy: [string, string];
  testDelay: [string];
}>();

const filter = ref<GroupFilter>("all");
const expandedGroups = ref(new Set<string>());
const groupSearch = ref<Record<string, string>>({});
const publicProxyGroups = computed(() =>
  props.proxyGroups.filter(
    (group) => group.name !== "ackwrap-internal-node-check",
  ),
);

const filters = computed(() => [
  {
    value: "all" as const,
    label: "全部",
    icon: LayoutGrid,
    count: publicProxyGroups.value.length,
  },
  {
    value: "selector" as const,
    label: "手动组",
    icon: Folder,
    count: publicProxyGroups.value.filter((group) => group.type === "Selector")
      .length,
  },
  {
    value: "automatic" as const,
    label: "自动组",
    icon: Bolt,
    count: publicProxyGroups.value.filter((group) => group.type !== "Selector")
      .length,
  },
]);

const groups = computed(() =>
  publicProxyGroups.value.filter(
    (group) =>
      filter.value === "all" ||
      (filter.value === "selector"
        ? group.type === "Selector"
        : group.type !== "Selector"),
  ),
);

const columns = computed(() => [
  groups.value.filter((_, index) => index % 2 === 0),
  groups.value.filter((_, index) => index % 2 === 1),
]);

function toggleGroup(name: string) {
  const next = new Set(expandedGroups.value);
  next.has(name) ? next.delete(name) : next.add(name);
  expandedGroups.value = next;
}

function selectProxy(group: string, node: string) {
  emit("selectProxy", group, node);
}
</script>

<template>
  <div class="space-y-3 pb-4">
    <div
      class="flex items-center justify-between gap-2 rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-1.5 shadow-[var(--shadow-card)]"
    >
      <div class="flex min-w-0 items-center gap-1 overflow-x-auto">
        <button
          v-for="item in filters"
          :key="item.value"
          type="button"
          class="flex h-8 shrink-0 items-center gap-2 rounded-lg border border-transparent px-3 text-xs font-medium transition-colors"
          :class="
            filter === item.value
              ? 'border-[var(--button-primary-border)] bg-[var(--color-primary-bg)] text-[var(--color-primary-hover)]'
              : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
          "
          @click="filter = item.value"
        >
          <component :is="item.icon" :size="14" />
          {{ item.label }}
          <span class="tabular-nums text-[var(--text-tertiary)]">{{
            item.count
          }}</span>
        </button>
      </div>
      <button
        type="button"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
        :disabled="loading"
        title="刷新策略组"
        @click="$emit('refresh')"
      >
        <RefreshCw :size="14" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>

    <div
      v-if="!groups.length"
      class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-12 text-center text-[var(--text-secondary)]"
    >
      {{ loading ? "加载中..." : "暂无策略组" }}
    </div>

    <div
      v-else
      class="grid items-start gap-3 lg:grid-cols-[minmax(0,1.14fr)_minmax(0,1fr)]"
    >
      <div
        v-for="(column, columnIndex) in columns"
        :key="columnIndex"
        class="flex min-w-0 flex-col gap-3"
      >
        <ProxyGroupCard
          v-for="group in column"
          :key="group.name"
          :group="group"
          :proxies="proxies"
          :expanded="expandedGroups.has(group.name)"
          :search="groupSearch[group.name] || ''"
          :testing-nodes="testingNodes"
          @toggle="toggleGroup(group.name)"
          @update:search="groupSearch[group.name] = $event"
          @select-proxy="selectProxy"
          @test-delay="$emit('testDelay', $event)"
        />
      </div>
    </div>
  </div>
</template>
