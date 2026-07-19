<script setup lang="ts">
import { ref } from "vue";
import { Eye, RefreshCw, Search, ShieldCheck } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import type { Rule } from "@/services/clash";

defineProps<{
  rules: Rule[];
  search: string;
  loading: boolean;
  unavailableReason: string;
}>();

defineEmits<{ searchChange: [string]; refresh: [] }>();

const detailRule = ref<Rule | null>(null);

function openDetailsFromKeyboard(event: KeyboardEvent, rule: Rule) {
  if (event.target !== event.currentTarget) return;
  if (event.key === " ") event.preventDefault();
  detailRule.value = rule;
}

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
      <table v-else class="w-full min-w-[720px] table-fixed border-collapse text-xs">
        <thead
          class="sticky top-0 z-10 bg-[var(--bg-elevated)] text-[var(--text-secondary)]"
        >
          <tr>
            <th class="w-32 px-4 py-3 text-left font-semibold">类型</th>
            <th class="px-4 py-3 text-left font-semibold">匹配值</th>
            <th class="w-56 px-4 py-3 text-left font-semibold">策略</th>
            <th class="w-24 px-3 py-3 text-center font-semibold">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(rule, index) in rules"
            :key="`${rule.type}-${rule.payload}-${rule.proxy}-${index}`"
            class="cursor-pointer border-t border-[var(--border-light)] transition-colors hover:bg-[var(--color-primary-bg)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--color-primary)]"
            :class="index % 2 ? 'bg-[var(--bg-base)]' : ''"
            tabindex="0"
            @click="detailRule = rule"
            @keydown.enter="openDetailsFromKeyboard($event, rule)"
            @keydown.space="openDetailsFromKeyboard($event, rule)"
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
            <td class="px-3 py-3 text-center align-middle">
              <button
                type="button"
                class="inline-flex h-7 items-center gap-1 whitespace-nowrap rounded-md border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-2.5 text-[var(--text-secondary)] transition hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
                title="查看完整规则"
                @click.stop="detailRule = rule"
              >
                <Eye :size="13" />详情
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal
      :open="!!detailRule"
      title="规则详情"
      size="lg"
      @close="detailRule = null"
    >
      <div v-if="detailRule" class="grid gap-4 text-sm md:grid-cols-2">
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-xs text-[var(--text-tertiary)]">类型</span>
          <p class="mt-1 break-all font-mono">{{ detailRule.type || "-" }}</p>
        </div>
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-xs text-[var(--text-tertiary)]">策略</span>
          <p class="mt-1 break-all font-mono">{{ detailRule.proxy || "-" }}</p>
        </div>
        <div
          class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-3 md:col-span-2"
        >
          <span class="text-xs text-[var(--text-tertiary)]">完整匹配值</span>
          <pre
            class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-5 text-[var(--text-primary)]"
          >{{ detailRule.payload || "-" }}</pre>
        </div>
        <details
          class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-3 md:col-span-2"
        >
          <summary class="cursor-pointer text-xs font-medium text-[var(--text-secondary)]">
            原始 JSON
          </summary>
          <pre
            class="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-5 text-[var(--text-primary)]"
          >{{ JSON.stringify(detailRule, null, 2) }}</pre>
        </details>
      </div>
    </Modal>
  </div>
</template>
