<script setup lang="ts">
import { computed } from "vue";
import { ChevronDown, CircleArrowRight, Search } from "lucide-vue-next";
import type { ProxyGroup, ProxyNode } from "@/services/clash";
import ProxyGroupIcon from "./ProxyGroupIcon.vue";
import ProxyGroupPreview from "./ProxyGroupPreview.vue";
import ProxyLatencyTag from "./ProxyLatencyTag.vue";
import ProxyNodeCard from "./ProxyNodeCard.vue";
import ProxyNodeGrid from "./ProxyNodeGrid.vue";
import type { ProxyMap } from "./proxyGroupUtils";
import { availableProxyCount, latestDelay } from "./proxyGroupUtils";

const props = withDefaults(
  defineProps<{
    group: ProxyGroup;
    proxies: ProxyMap;
    expanded?: boolean;
    search?: string;
    testingNodes?: Set<string>;
  }>(),
  { expanded: false, search: "", testingNodes: () => new Set<string>() },
);

const emit = defineEmits<{
  toggle: [];
  "update:search": [string];
  selectProxy: [string, string];
  testDelay: [string];
}>();

const selectable = computed(() => props.group.type === "Selector");
const visibleNodes = computed(() => {
  const keyword = props.search.trim().toLowerCase();
  if (!keyword) return props.group.all || [];
  return (props.group.all || []).filter((name) =>
    name.toLowerCase().includes(keyword),
  );
});

function node(name: string) {
  return props.proxies[name] as ProxyNode | undefined;
}

function selectProxy(name: string) {
  if (selectable.value) emit("selectProxy", props.group.name, name);
}

function updateSearch(event: Event) {
  emit("update:search", (event.target as HTMLInputElement).value);
}
</script>

<template>
  <section
    class="overflow-hidden rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]"
    :data-group-name="group.name"
  >
    <div
      class="cursor-pointer px-4 py-3.5 outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
      role="button"
      tabindex="0"
      @click="$emit('toggle')"
      @keydown.enter.prevent="$emit('toggle')"
    >
      <div class="flex min-w-0 items-center gap-2.5">
        <ProxyGroupIcon :group="group" class="h-5 w-5 shrink-0" />
        <b class="min-w-0 truncate text-sm text-[var(--text-primary)]">{{
          group.name
        }}</b>
        <span
          class="min-w-0 flex-1 truncate text-[10px] font-medium tracking-wider text-[var(--text-tertiary)] uppercase tabular-nums"
        >
          {{ group.type }} · {{ availableProxyCount(group, proxies) }}/{{
            group.all?.length || 0
          }}
        </span>
        <button
          type="button"
          class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
          title="展开并搜索组内节点"
          @click.stop="$emit('toggle')"
        >
          <Search :size="13" />
        </button>
        <ProxyLatencyTag
          :delay="latestDelay(proxies[group.now])"
          :loading="testingNodes.has(group.now)"
          :disabled="!group.now"
          @test="$emit('testDelay', group.now)"
        />
        <ChevronDown
          :size="14"
          class="shrink-0 text-[var(--text-tertiary)] transition-transform"
          :class="expanded ? 'rotate-180' : ''"
        />
      </div>

      <div class="mt-2 flex items-center gap-2 text-xs">
        <CircleArrowRight
          :size="14"
          class="shrink-0 text-[var(--text-tertiary)]"
        />
        <span
          class="min-w-0 flex-1 truncate font-medium text-[var(--text-primary)]"
        >
          {{ group.now || "未选择节点" }}
        </span>
      </div>

      <ProxyGroupPreview
        v-if="!expanded"
        :nodes="group.all || []"
        :now="group.now"
        :proxies="proxies"
        :selectable="selectable"
        @select="selectProxy"
      />
    </div>

    <div
      v-if="expanded"
      class="border-t border-[var(--border-default)] px-3 pb-3"
      @click.stop
    >
      <div class="relative my-3">
        <Search
          :size="13"
          class="absolute top-1/2 left-2.5 -translate-y-1/2 text-[var(--text-tertiary)]"
        />
        <input
          type="search"
          class="h-8 w-full rounded-lg border border-[var(--border-default)] bg-[var(--button-secondary-bg)] pr-3 pl-8 text-xs outline-none focus:border-[var(--color-primary)]"
          :value="search"
          placeholder="搜索组内节点"
          @input="updateSearch"
        />
      </div>
      <div>
        <ProxyNodeGrid>
          <ProxyNodeCard
            v-for="name in visibleNodes"
            :key="name"
            :name="name"
            :node="node(name)"
            :active="group.now === name"
            :selectable="selectable"
            :testing="testingNodes.has(name)"
            @select="selectProxy(name)"
            @test="$emit('testDelay', name)"
          />
        </ProxyNodeGrid>
        <p
          v-if="!visibleNodes.length"
          class="py-6 text-center text-xs text-[var(--text-secondary)]"
        >
          没有匹配的节点
        </p>
      </div>
    </div>
  </section>
</template>
