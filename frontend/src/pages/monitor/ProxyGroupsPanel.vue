<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Bolt, Folder, LayoutGrid, RefreshCw } from "lucide-vue-next";
import type { ProxyGroup } from "@/services/clash";
import {
  connectivityTestTargets,
  connectivityTestTargetValues,
} from "@/utils/connectivityTargets";
import ProxyGroupCard from "./ProxyGroupCard.vue";
import type { ProxyMap } from "./proxyGroupUtils";

type GroupFilter = "all" | "selector" | "automatic";

const props = withDefaults(
  defineProps<{
    proxies: ProxyMap;
    proxyGroups: ProxyGroup[];
    loading: boolean;
    testingNodes?: Set<string>;
    testingGroups?: Set<string>;
    delayTestUrl: string;
    nodeFlags: Record<string, string>;
  }>(),
  {
    testingNodes: () => new Set<string>(),
    testingGroups: () => new Set<string>(),
  },
);

const emit = defineEmits<{
  refresh: [];
  selectProxy: [string, string];
  testDelay: [string];
  testGroup: [string, string[]];
  "update:delayTestUrl": [string];
}>();

const filter = ref<GroupFilter>("all");
const expandedGroups = ref(new Set<string>());
const groupSearch = ref<Record<string, string>>({});
const delayTargetMode = ref(
  connectivityTestTargetValues.has(props.delayTestUrl)
    ? props.delayTestUrl
    : "custom",
);
const customDelayTestURL = ref(
  connectivityTestTargetValues.has(props.delayTestUrl)
    ? ""
    : props.delayTestUrl,
);
watch(
  () => props.delayTestUrl,
  (value) => {
    if (connectivityTestTargetValues.has(value)) delayTargetMode.value = value;
    else {
      delayTargetMode.value = "custom";
      customDelayTestURL.value = value;
    }
  },
);
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

function selectDelayTarget(value: string) {
  delayTargetMode.value = value;
  if (value !== "custom") emit("update:delayTestUrl", value);
}

function saveCustomDelayTarget() {
  emit("update:delayTestUrl", customDelayTestURL.value);
}
</script>

<template>
  <div class="space-y-3.5 pb-5">
    <div
      class="flex flex-wrap items-center justify-between gap-2 rounded-[14px] border border-[var(--proxy-card-border)] bg-[var(--proxy-card-bg)] p-1.5 shadow-[var(--proxy-card-shadow)]"
    >
      <div class="flex min-w-0 items-center gap-1 overflow-x-auto">
        <button
          v-for="item in filters"
          :key="item.value"
          type="button"
          class="flex h-8 shrink-0 items-center gap-2 rounded-[10px] border border-transparent px-3 text-xs font-medium transition-colors"
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
      <div class="flex w-full shrink-0 items-center gap-1.5 sm:w-auto">
        <label class="hidden text-[11px] text-[var(--text-tertiary)] sm:block">
          测速地址
        </label>
        <select
          class="h-8 min-w-0 flex-1 rounded-[10px] border border-[var(--border-light)] bg-[var(--button-secondary-bg)] px-2 text-xs text-[var(--text-secondary)] outline-none focus:border-[var(--color-primary)] sm:max-w-44"
          :value="delayTargetMode"
          :title="delayTestUrl"
          @change="
            selectDelayTarget(($event.target as HTMLSelectElement).value)
          "
        >
          <option
            v-for="target in connectivityTestTargets"
            :key="target.value"
            :value="target.value"
          >
            {{ target.label }}
          </option>
          <option value="custom">自定义地址</option>
        </select>
        <input
          v-if="delayTargetMode === 'custom'"
          v-model="customDelayTestURL"
          class="h-8 min-w-0 flex-1 rounded-[10px] border border-[var(--border-light)] bg-[var(--button-secondary-bg)] px-2 text-xs text-[var(--text-secondary)] outline-none focus:border-[var(--color-primary)] sm:w-48"
          placeholder="https://example.com/generate_204"
          @change="saveCustomDelayTarget"
        />
        <button
          type="button"
          class="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-sidebar-hover)]"
          :disabled="loading"
          title="刷新策略组"
          @click="$emit('refresh')"
        >
          <RefreshCw :size="14" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>
    </div>

    <div
      v-if="!groups.length"
      class="rounded-[14px] border border-[var(--proxy-card-border)] bg-[var(--proxy-card-bg)] p-12 text-center text-[var(--text-secondary)] shadow-[var(--proxy-card-shadow)]"
    >
      {{ loading ? "加载中..." : "暂无策略组" }}
    </div>

    <div v-else class="grid items-start gap-3 lg:grid-cols-2">
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
          :testing-groups="testingGroups"
          :node-flags="nodeFlags"
          @toggle="toggleGroup(group.name)"
          @update:search="groupSearch[group.name] = $event"
          @select-proxy="selectProxy"
          @test-delay="$emit('testDelay', $event)"
          @test-group="(name, nodes) => $emit('testGroup', name, nodes)"
        />
      </div>
    </div>
  </div>
</template>
