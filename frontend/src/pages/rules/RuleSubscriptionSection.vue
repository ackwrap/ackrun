<script setup lang="ts">
import { Cloud, Plus, RefreshCw, Settings2 } from "lucide-vue-next";
import AdaptiveActionBar from "@/components/ui/AdaptiveActionBar.vue";
import type { RouteRuleSubscription } from "@/services/types";

defineProps<{
  subscriptions: RouteRuleSubscription[];
  syncing: boolean;
}>();

defineEmits<{
  add: [];
  syncAll: [];
  manage: [RouteRuleSubscription];
}>();

function syncStatusLabel(status: string) {
  return (
    {
      syncing: "同步中",
      updated: "已更新",
      failed: "失败",
      idle: "待同步",
    }[status] ||
    status ||
    "待同步"
  );
}

function scheduleLabel(item: RouteRuleSubscription) {
  if (item.sync_mode === "off") return "关闭";
  if (item.sync_mode === "weekly")
    return `每周 ${item.sync_weekday} / ${item.sync_time}`;
  if (item.sync_mode === "monthly")
    return `每月 ${item.sync_weekday} 日 / ${item.sync_time}`;
  return `每天 / ${item.sync_time}`;
}
</script>

<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <AdaptiveActionBar id="rule-subscription-actions" label="规则订阅操作">
      <template #title>
        <h3 class="flex items-center gap-2 font-semibold">
          <Cloud :size="17" />规则订阅
        </h3>
      </template>
      <button class="aw-action-button aw-action-neutral" @click="$emit('add')">
        <Plus :size="13" />新增订阅
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        :disabled="syncing || !subscriptions.length"
        @click="$emit('syncAll')"
      >
        <RefreshCw :size="13" :class="syncing ? 'animate-spin' : ''" />同步全部
      </button>
    </AdaptiveActionBar>
    <details class="mt-2 text-xs text-[var(--text-secondary)]">
      <summary class="w-fit cursor-pointer">支持的格式</summary>
      <p class="mt-2 leading-5">
        支持 sing-box binary / source 和 Clash Rule Provider YAML。 auto 会按
        .yml / .yaml 识别 Clash 格式并转换为规则集，不支持完整 Clash 节点订阅。
      </p>
    </details>

    <div class="aw-data-table-wrap aw-mobile-table-wrap mt-4">
      <table class="aw-data-table aw-mobile-table min-w-[840px]">
        <thead>
          <tr>
            <th>名称 / 地址</th>
            <th>规则集 Tag</th>
            <th>格式 / 下载</th>
            <th>自动同步</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!subscriptions.length">
            <td colspan="6" class="py-8 text-center">暂无规则订阅</td>
          </tr>
          <tr v-for="item in subscriptions" :key="item.id">
            <td class="aw-mobile-title max-w-[340px]">
              <div class="font-medium text-[var(--text-primary)]">
                {{ item.name }}
              </div>
              <div
                class="mt-1 truncate text-xs text-[var(--text-tertiary)]"
                :title="item.url"
              >
                {{ item.url }}
              </div>
              <div
                v-if="item.sync_error"
                class="mt-1 truncate text-xs text-[var(--color-error)]"
                :title="item.sync_error"
              >
                {{ item.sync_error }}
              </div>
            </td>
            <td data-label="规则集 Tag">
              <span
                class="rounded bg-[var(--button-secondary-bg)] px-2 py-1 font-mono text-xs"
              >
                {{ item.tag }}
              </span>
            </td>
            <td data-label="格式 / 下载">
              <div>{{ item.format }}</div>
              <div class="mt-1 text-xs text-[var(--text-tertiary)]">
                {{ item.use_proxy ? "代理下载" : "直连下载" }}
              </div>
            </td>
            <td data-label="自动同步">{{ scheduleLabel(item) }}</td>
            <td data-label="状态">
              <span
                class="text-xs"
                :class="
                  item.enabled
                    ? 'text-[var(--color-success)]'
                    : 'text-[var(--text-tertiary)]'
                "
              >
                {{ item.enabled ? "启用" : "停用" }}
              </span>
              <span
                class="mt-1 block text-xs"
                :class="
                  item.sync_status === 'failed'
                    ? 'text-[var(--color-error)]'
                    : item.sync_status === 'syncing'
                      ? 'text-[var(--color-primary)]'
                      : 'text-[var(--text-secondary)]'
                "
              >
                {{ syncStatusLabel(item.sync_status) }}
                <template v-if="item.sync_status === 'syncing'">
                  {{ item.sync_progress || 0 }}%
                </template>
              </span>
            </td>
            <td class="aw-mobile-actions">
              <button
                class="aw-action-button aw-action-neutral"
                @click="$emit('manage', item)"
              >
                <Settings2 :size="13" />管理
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
