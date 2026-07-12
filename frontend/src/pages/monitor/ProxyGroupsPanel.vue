<script setup lang="ts">
import { computed, ref } from "vue";
import { ChevronDown, RefreshCw, Zap } from "lucide-vue-next";
import type { ProxyGroup, ProxyNode } from "@/services/clash";
import ProxyGroupIcon from "./ProxyGroupIcon.vue";
const p = defineProps<{
    proxies: Record<string, ProxyGroup | ProxyNode>;
    proxyGroups: ProxyGroup[];
    selectedGroup: string | null;
    loading: boolean;
  }>(),
  emit = defineEmits<{
    refresh: [];
    selectGroup: [string | null];
    selectProxy: [string, string];
    testDelay: [string];
  }>(),
  filter = ref<"all" | "selector" | "automatic">("all");
const groups = computed(() =>
    p.proxyGroups.filter(
      (g) =>
        filter.value === "all" ||
        (filter.value === "selector"
          ? g.type === "Selector"
          : g.type !== "Selector"),
    ),
  ),
  columns = computed(() => [
    groups.value.filter((_, i) => i % 2 === 0),
    groups.value.filter((_, i) => i % 2 === 1),
  ]),
  delay = (n: string) => {
    const history = p.proxies[n]?.history || [];
    return Number(history[history.length - 1]?.delay || 0);
  },
  color = (d: number) =>
    d < 200 ? "text-emerald-500" : d < 800 ? "text-amber-500" : "text-rose-500";
function change(v: typeof filter.value) {
  filter.value = v;
  emit("selectGroup", null);
}
</script>
<template>
  <div class="space-y-3 pb-4">
    <div
      class="flex justify-between rounded-xl border border-[var(--border-default)] p-2"
    >
      <div>
        <button
          v-for="x in [
            { v: 'all', l: '全部' },
            { v: 'selector', l: '手动策略' },
            { v: 'automatic', l: '自动选择' },
          ]"
          :key="x.v"
          class="px-3 py-2 text-xs"
          :class="filter === x.v ? 'text-[var(--color-primary)]' : ''"
          @click="change(x.v as any)"
        >
          {{ x.l }}
          {{
            x.v === "all"
              ? proxyGroups.length
              : x.v === "selector"
                ? proxyGroups.filter((g) => g.type === "Selector").length
                : proxyGroups.filter((g) => g.type !== "Selector").length
          }}
        </button>
      </div>
      <button :disabled="loading" @click="$emit('refresh')">
        <RefreshCw :size="14" :class="loading ? 'animate-spin' : ''" />刷新
      </button>
    </div>
    <div
      v-if="loading || !groups.length"
      class="rounded-xl border p-12 text-center"
    >
      {{ loading ? "加载中..." : "暂无策略组" }}
    </div>
    <div v-else class="grid items-start gap-3 lg:grid-cols-2">
      <div v-for="(col, ci) in columns" :key="ci" class="space-y-3">
        <section
          v-for="g in col"
          :key="g.name"
          class="overflow-hidden rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)]"
        >
          <button
            class="block w-full p-4 text-left"
            @click="
              $emit('selectGroup', selectedGroup === g.name ? null : g.name)
            "
          >
            <div class="flex justify-between">
              <div class="flex gap-3">
                <ProxyGroupIcon :group="g" />
                <div>
                  <b>{{ g.name }}</b
                  ><small class="ml-2">{{ g.type }}</small>
                  <div class="text-xs text-[var(--text-secondary)]">
                    {{ g.now || "未选择节点" }}
                  </div>
                </div>
              </div>
              <ChevronDown
                :class="selectedGroup === g.name ? 'rotate-180' : ''"
              />
            </div>
          </button>
          <div
            v-if="selectedGroup === g.name"
            class="grid gap-2 border-t p-3 sm:grid-cols-2"
          >
            <div
              v-for="n in g.all || []"
              :key="n"
              class="flex items-center rounded-lg border border-[var(--border-default)]"
            >
              <button
                class="min-w-0 flex-1 p-3 text-left"
                :disabled="g.type !== 'Selector'"
                @click="$emit('selectProxy', g.name, n)"
              >
                <span
                  class="block truncate"
                  :class="g.now === n ? 'text-[var(--color-primary)]' : ''"
                  >{{ n }}</span
                ><small>{{ p.proxies[n]?.type || "proxy" }}</small></button
              ><button class="mr-2" @click="$emit('testDelay', n)">
                <span v-if="delay(n)" :class="color(delay(n))">{{
                  delay(n)
                }}</span
                ><Zap v-else :size="12" />
              </button>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
