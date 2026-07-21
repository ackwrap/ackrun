<script setup lang="ts">
import { Cloud, Plus, RefreshCw } from "lucide-vue-next";
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
    }[status] || status || "待同步"
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
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="flex items-center gap-2 font-semibold">
          <Cloud :size="17" />规则订阅
        </h3>
        <small class="text-[var(--text-secondary)]">
          支持 sing-box binary/source 和 Clash Rule Provider YAML；选择 clash 或由 auto 按 .yml/.yaml 自动识别，转换后生成 sing-box rule_set 引用。不支持完整 Clash 节点订阅配置。
        </small>
      </div>
      <div class="flex flex-wrap justify-end gap-2">
        <button
          class="aw-action-button aw-action-neutral"
          @click="$emit('add')"
        >
          <Plus :size="13" />新增订阅
        </button>
        <button
          class="aw-action-button aw-action-neutral"
          :disabled="syncing || !subscriptions.length"
          @click="$emit('syncAll')"
        >
          <RefreshCw :size="13" :class="syncing ? 'animate-spin' : ''" />同步全部
        </button>
      </div>
    </header>

    <div class="aw-data-table-wrap mt-4">
      <table class="aw-data-table min-w-[1080px]">
        <thead>
          <tr>
            <th>名称 / 地址</th>
            <th>规则集 Tag</th>
            <th>格式</th>
            <th>下载</th>
            <th>自动同步</th>
            <th>同步状态</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!subscriptions.length">
            <td colspan="8" class="py-12 text-center">暂无规则订阅</td>
          </tr>
          <tr v-for="item in subscriptions" :key="item.id">
            <td class="max-w-[340px]">
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
            <td>
              <span
                class="rounded bg-[var(--button-secondary-bg)] px-2 py-1 font-mono text-xs"
              >
                {{ item.tag }}
              </span>
            </td>
            <td>{{ item.format }}</td>
            <td>{{ item.use_proxy ? "代理" : "直连" }}</td>
            <td>{{ scheduleLabel(item) }}</td>
            <td>
              <span
                class="text-xs"
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
            <td>
              <span
                class="rounded-full px-2 py-1 text-xs"
                :class="
                  item.enabled
                    ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]'
                    : 'bg-[var(--button-secondary-bg)] text-[var(--text-tertiary)]'
                "
              >
                {{ item.enabled ? "启用" : "停用" }}
              </span>
            </td>
            <td>
              <button
                class="aw-action-button aw-action-neutral"
                @click="$emit('manage', item)"
              >
                管理
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
