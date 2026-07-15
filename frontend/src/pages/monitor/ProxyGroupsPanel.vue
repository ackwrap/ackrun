<script setup lang="ts">
import { computed, ref } from "vue";
import {
  Bolt,
  ChevronDown,
  Folder,
  LayoutGrid,
  RefreshCw,
  Search,
  Zap,
} from "lucide-vue-next";
import type { ProxyGroup, ProxyNode } from "@/services/clash";
import ProxyGroupIcon from "./ProxyGroupIcon.vue";

type GroupFilter = "all" | "selector" | "automatic";

const props = defineProps<{
  proxies: Record<string, ProxyGroup | ProxyNode>;
  proxyGroups: ProxyGroup[];
  selectedGroup: string | null;
  loading: boolean;
}>();

const emit = defineEmits<{
  refresh: [];
  selectGroup: [string | null];
  selectProxy: [string, string];
  testDelay: [string];
}>();

const filter = ref<GroupFilter>("all");
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

function nodeDelay(name: string) {
  const history = props.proxies[name]?.history || [];
  return Number(history[history.length - 1]?.delay || 0);
}

function delayClass(delay: number) {
  if (!delay) return "text-[var(--text-tertiary)]";
  if (delay < 200) return "text-emerald-500";
  if (delay < 800) return "text-amber-500";
  return "text-rose-500";
}

function delayBackground(delay: number) {
  if (!delay) return "bg-[var(--text-tertiary)]";
  if (delay < 200) return "bg-emerald-400";
  if (delay < 800) return "bg-amber-400";
  return "bg-rose-400";
}

function availableCount(group: ProxyGroup) {
  return (group.all || []).filter((name) => nodeDelay(name) > 0).length;
}

function visibleNodes(group: ProxyGroup) {
  const keyword = groupSearch.value[group.name]?.trim().toLowerCase();
  if (!keyword) return group.all || [];
  return (group.all || []).filter((name) =>
    name.toLowerCase().includes(keyword),
  );
}

function nodeDescription(name: string) {
  const node = props.proxies[name] as ProxyNode | undefined;
  return [node?.type || "proxy", node?.udp ? "udp" : ""]
    .filter(Boolean)
    .join(" / ");
}

function distribution(group: ProxyGroup) {
  const result = { low: 0, medium: 0, high: 0, unknown: 0 };
  for (const name of group.all || []) {
    const delay = nodeDelay(name);
    if (!delay) result.unknown += 1;
    else if (delay < 200) result.low += 1;
    else if (delay < 800) result.medium += 1;
    else result.high += 1;
  }
  return result;
}

function percentage(count: number, total: number) {
  return total ? `${(count / total) * 100}%` : "0%";
}

function changeFilter(value: GroupFilter) {
  filter.value = value;
  emit("selectGroup", null);
}
</script>

<template>
  <div class="space-y-3 pb-4">
    <div
      class="flex items-center justify-between gap-2 rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-1.5"
    >
      <div class="flex min-w-0 items-center gap-1 overflow-x-auto">
        <button
          v-for="item in filters"
          :key="item.value"
          type="button"
          class="flex h-8 shrink-0 items-center gap-2 rounded-lg border border-transparent px-3 text-xs font-medium transition"
          :class="
            filter === item.value
              ? 'border border-[var(--button-primary-border)] bg-[var(--color-primary-bg)] text-[var(--color-primary-hover)]'
              : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
          "
          @click="changeFilter(item.value)"
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
        class="flex size-8 shrink-0 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
        :disabled="loading"
        title="刷新策略组"
        @click="emit('refresh')"
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

    <div v-else class="grid items-start gap-3 lg:grid-cols-2">
      <div
        v-for="(column, columnIndex) in columns"
        :key="columnIndex"
        class="space-y-3"
      >
        <section
          v-for="group in column"
          :key="group.name"
          class="overflow-hidden rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)]"
        >
          <div
            class="cursor-pointer px-4 py-3.5"
            role="button"
            tabindex="0"
            @click="
              emit(
                'selectGroup',
                selectedGroup === group.name ? null : group.name,
              )
            "
            @keydown.enter="
              emit(
                'selectGroup',
                selectedGroup === group.name ? null : group.name,
              )
            "
          >
            <div class="flex min-w-0 items-center gap-2.5">
              <ProxyGroupIcon :group="group" class="h-5 w-5 shrink-0" />
              <b class="min-w-0 truncate text-sm text-[var(--text-primary)]">{{
                group.name
              }}</b>
              <span
                class="min-w-0 flex-1 truncate text-[10px] font-medium tracking-wider text-[var(--text-tertiary)] uppercase"
              >
                {{ group.type }} · {{ availableCount(group) }}/{{
                  group.all?.length || 0
                }}
              </span>
              <button
                type="button"
                class="flex size-7 shrink-0 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
                title="展开并搜索组内节点"
                @click.stop="
                  emit(
                    'selectGroup',
                    selectedGroup === group.name ? null : group.name,
                  )
                "
              >
                <Search :size="13" />
              </button>
              <button
                type="button"
                class="min-w-10 shrink-0 rounded-lg bg-[var(--bg-secondary)] px-2 py-1 text-[10px] tabular-nums"
                :class="delayClass(nodeDelay(group.now))"
                :disabled="!group.now"
                title="测试当前节点延迟"
                @click.stop="emit('testDelay', group.now)"
              >
                {{ nodeDelay(group.now) || "-" }}
              </button>
              <ChevronDown
                :size="14"
                class="shrink-0 text-[var(--text-tertiary)] transition-transform"
                :class="selectedGroup === group.name ? 'rotate-180' : ''"
              />
            </div>

            <div class="mt-2 flex items-center gap-2 text-xs">
              <span class="text-[var(--text-tertiary)]">◎</span>
              <span
                class="min-w-0 flex-1 truncate font-medium text-[var(--text-primary)]"
              >
                {{ group.now || "未选择节点" }}
              </span>
            </div>

            <div v-if="selectedGroup !== group.name" class="mt-3">
              <div
                v-if="(group.all?.length || 0) <= 48"
                class="flex flex-wrap gap-1.5"
              >
                <button
                  v-for="name in group.all || []"
                  :key="name"
                  type="button"
                  class="flex size-4 items-center justify-center rounded-full transition hover:scale-110"
                  :class="delayBackground(nodeDelay(name))"
                  :title="`${name}${nodeDelay(name) ? ` · ${nodeDelay(name)} ms` : ''}`"
                  @click.stop="
                    group.type === 'Selector' &&
                    emit('selectProxy', group.name, name)
                  "
                >
                  <span
                    v-if="group.now === name"
                    class="size-2 rounded-full bg-white"
                  />
                </button>
              </div>
              <div v-else class="flex h-2 overflow-hidden rounded-full">
                <span
                  class="bg-emerald-400"
                  :style="{
                    width: percentage(
                      distribution(group).low,
                      group.all.length,
                    ),
                  }"
                />
                <span
                  class="bg-amber-400"
                  :style="{
                    width: percentage(
                      distribution(group).medium,
                      group.all.length,
                    ),
                  }"
                />
                <span
                  class="bg-rose-400"
                  :style="{
                    width: percentage(
                      distribution(group).high,
                      group.all.length,
                    ),
                  }"
                />
                <span
                  class="bg-[var(--text-tertiary)]"
                  :style="{
                    width: percentage(
                      distribution(group).unknown,
                      group.all.length,
                    ),
                  }"
                />
              </div>
            </div>
          </div>

          <div
            v-if="selectedGroup === group.name"
            class="border-t border-[var(--border-default)] px-3 pb-3"
          >
            <div class="relative my-3">
              <Search
                :size="13"
                class="absolute top-1/2 left-2.5 -translate-y-1/2 text-[var(--text-tertiary)]"
              />
              <input
                v-model="groupSearch[group.name]"
                type="search"
                class="h-8 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] pr-3 pl-8 text-xs outline-none focus:border-[var(--color-primary)]"
                placeholder="搜索组内节点"
                @click.stop
              />
            </div>
            <div
              class="grid min-w-0 gap-2 [grid-template-columns:repeat(auto-fill,minmax(min(160px,100%),1fr))]"
            >
              <div
                v-for="name in visibleNodes(group)"
                :key="name"
                class="flex min-w-0 flex-col items-start gap-1 rounded-lg p-2 text-left transition"
                :class="
                  group.now === name
                    ? 'bg-[var(--color-primary)] text-white'
                    : 'bg-[var(--bg-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
                "
                role="button"
                tabindex="0"
                @click.stop="
                  group.type === 'Selector' &&
                  emit('selectProxy', group.name, name)
                "
                @keydown.enter="
                  group.type === 'Selector' &&
                  emit('selectProxy', group.name, name)
                "
              >
                <span class="w-full truncate text-xs font-medium">{{
                  name
                }}</span>
                <span
                  class="flex h-4 w-full items-center justify-between gap-2"
                >
                  <small class="truncate text-[10px] uppercase opacity-70">{{
                    nodeDescription(name)
                  }}</small>
                  <span
                    class="shrink-0 rounded-md px-1.5 text-[10px] tabular-nums"
                    :class="
                      group.now === name
                        ? 'bg-black/15 text-white'
                        : [
                            'bg-[var(--bg-surface)]',
                            delayClass(nodeDelay(name)),
                          ]
                    "
                    role="button"
                    title="测试节点延迟"
                    @click.stop="emit('testDelay', name)"
                  >
                    <template v-if="nodeDelay(name)">{{
                      nodeDelay(name)
                    }}</template>
                    <Zap v-else :size="11" />
                  </span>
                </span>
              </div>
            </div>
            <p
              v-if="!visibleNodes(group).length"
              class="py-6 text-center text-xs text-[var(--text-secondary)]"
            >
              没有匹配的节点
            </p>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
