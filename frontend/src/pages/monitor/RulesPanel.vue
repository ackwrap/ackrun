<script setup lang="ts">
import { RefreshCw, Search, ShieldCheck } from "lucide-vue-next";
import type { Rule } from "@/services/clash";

defineProps<{
  rules: Rule[];
  search: string;
  loading: boolean;
  unavailableReason: string;
}>();

defineEmits<{ searchChange: [string]; refresh: [] }>();

function proxyTone(proxy: string) {
  const value = proxy.toLowerCase();
  if (value.includes("reject") || value.includes("block"))
    return "bg-[var(--color-error-bg)] text-[var(--color-error)]";
  if (value.includes("direct"))
    return "bg-[var(--color-success-bg)] text-[var(--color-success)]";
  return "bg-[var(--color-primary-bg)] text-[var(--button-primary-text)]";
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3 pb-4">
    <div
      class="flex flex-wrap items-center gap-3 rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)]"
    >
      <div class="flex min-w-40 items-center gap-3">
        <span
          class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-bg)] text-[var(--color-primary)]"
        >
          <ShieldCheck :size="18" />
        </span>
        <div>
          <b class="block text-sm">规则列表</b>
          <p class="text-xs text-[var(--text-secondary)]">
            共 {{ rules.length }} 条规则
          </p>
        </div>
      </div>

      <div class="relative ml-auto min-w-52 max-w-md flex-1">
        <Search
          :size="14"
          class="absolute top-1/2 left-3 -translate-y-1/2 text-[var(--text-tertiary)]"
        />
        <input
          :value="search"
          type="search"
          class="!mt-0 !h-9 w-full !rounded-lg !bg-[var(--bg-base)] pr-3 pl-9"
          placeholder="搜索类型、匹配值或策略"
          @input="
            $emit('searchChange', ($event.target as HTMLInputElement).value)
          "
        />
      </div>

      <button
        type="button"
        class="flex size-9 shrink-0 items-center justify-center rounded-full text-[var(--text-secondary)] transition hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--color-primary)]"
        :disabled="loading"
        title="刷新规则"
        @click="$emit('refresh')"
      >
        <RefreshCw :size="15" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>

    <div
      class="min-h-0 flex-1 overflow-auto rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]"
    >
      <div
        v-if="loading"
        class="grid min-h-72 place-items-center text-[var(--text-tertiary)]"
      >
        正在加载规则...
      </div>
      <div
        v-else-if="unavailableReason"
        class="grid min-h-72 place-items-center px-6 text-center text-[var(--color-error)]"
      >
        <div>
          <b class="block">Clash API 未连接</b>
          <p class="mt-1 text-xs">{{ unavailableReason }}</p>
        </div>
      </div>
      <div
        v-else-if="!rules.length"
        class="grid min-h-72 place-items-center text-[var(--text-tertiary)]"
      >
        {{ search ? "没有匹配的规则" : "暂无规则" }}
      </div>
      <table v-else class="w-full min-w-[760px] border-collapse text-xs">
        <thead
          class="sticky top-0 z-10 bg-[var(--bg-elevated)] text-[var(--text-secondary)]"
        >
          <tr>
            <th class="w-44 px-4 py-3 text-left font-semibold">类型</th>
            <th class="px-4 py-3 text-left font-semibold">匹配值</th>
            <th class="w-64 px-4 py-3 text-left font-semibold">策略</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(rule, index) in rules"
            :key="`${rule.type}-${rule.payload}-${rule.proxy}-${index}`"
            class="border-t border-[var(--border-light)] transition-colors hover:bg-[var(--color-primary-bg)]"
            :class="index % 2 ? 'bg-[var(--bg-base)]' : ''"
          >
            <td class="px-4 py-3 align-middle">
              <span
                class="inline-flex max-w-full rounded-md bg-[var(--button-secondary-bg)] px-2 py-1 font-medium text-[var(--text-primary)]"
                :title="rule.type"
              >
                {{ rule.type || "-" }}
              </span>
            </td>
            <td
              class="max-w-0 truncate px-4 py-3 align-middle text-[var(--text-secondary)]"
              :title="rule.payload || '-'"
            >
              {{ rule.payload || "-" }}
            </td>
            <td class="px-4 py-3 align-middle">
              <span
                class="inline-flex max-w-full truncate rounded-md px-2 py-1 font-medium"
                :class="proxyTone(rule.proxy)"
                :title="rule.proxy"
              >
                {{ rule.proxy || "-" }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
