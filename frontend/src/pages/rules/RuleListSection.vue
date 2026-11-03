<script setup lang="ts">
import {
  Edit3,
  Eye,
  GripVertical,
  Plus,
  RefreshCw,
  Share2,
  Trash2,
  Upload,
} from "lucide-vue-next";
import AdaptiveActionBar from "@/components/ui/AdaptiveActionBar.vue";
import FlagText from "@/components/ui/FlagText.vue";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";
import { splitDeviceRuleValues } from "./deviceRulePreview";
import { useRuleSort } from "./useRuleSort";
const props = defineProps<{
  rules: RouteRule[];
  subscriptions: RouteRuleSubscription[];
  sorting: boolean;
}>();
const emit = defineEmits<{
  refresh: [];
  addGeo: [];
  addDevice: [];
  add: [];
  preview: [];
  share: [];
  import: [];
  reorder: [number[]];
  toggle: [RouteRule];
  edit: [RouteRule];
  remove: [RouteRule];
  detail: [RouteRule];
}>();
const { tableBody, displayRules, draggingId, start, keydown } = useRuleSort(
  () => props.rules,
  () => props.sorting,
  (ids) => emit("reorder", ids),
);

function isAdBlock(rule: RouteRule) {
  return (
    rule.system_key === "ad_block" ||
    (rule.is_system && rule.name === "广告拦截")
  );
}

function isFinalStrategy(rule: RouteRule) {
  return (
    rule.system_key === "global_direct" ||
    ["fallback", "final"].includes(rule.rule_type) ||
    (rule.is_system && ["全球直连", "最终策略"].includes(rule.name))
  );
}

function typeLabel(rule: RouteRule) {
  if (rule.rule_type === "source_ip_cidr") {
    const { macAddresses, ipCidrs } = splitDeviceRuleValues(rule.values);
    const identity = [
      ...(macAddresses.length ? ["MAC"] : []),
      ...(ipCidrs.length ? ["IP/CIDR"] : []),
    ].join(" 或 ");
    return `设备${identity ? ` ${identity}` : ""} · DNS 跟随`;
  }
  return isFinalStrategy(rule) ? "最终规则" : rule.rule_type;
}

function valueLabel(rule: RouteRule) {
  if (isFinalStrategy(rule)) return "未匹配流量";
  const values = rule.values.join(", ") || "无匹配值";
  if (rule.rule_type !== "source_ip_cidr") return values;
  const { macAddresses, ipCidrs } = splitDeviceRuleValues(rule.values);
  const identity = [
    ...(macAddresses.length ? [`MAC: ${macAddresses.join(", ")}`] : []),
    ...(ipCidrs.length ? [`IP/CIDR: ${ipCidrs.join(", ")}`] : []),
  ].join(" 或 ");
  return [
    identity ? `${identity}（任一命中）` : "无设备匹配值",
    ...(rule.direct_domains?.length
      ? [`直连还需命中域名白名单（${rule.direct_domains.length} 项）`]
      : []),
  ].join(" · ");
}

function statusToggleTitle(rule: RouteRule) {
  if (isAdBlock(rule)) return "系统广告拦截规则为只读，不能切换状态";
  if (isFinalStrategy(rule)) return "最终策略必须保持启用，仅允许编辑出站";
  return "";
}

const actionLabels: Record<string, string> = {
  direct: "直连 direct",
  proxy: "策略 proxy",
  bypass: "内核绕过 bypass",
  block: "阻断 block",
};

function actionLabel(rule: RouteRule) {
  if (isFinalStrategy(rule)) return rule.outbound;
  return actionLabels[rule.outbound] || rule.outbound;
}
</script>
<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <AdaptiveActionBar id="rule-list-actions" label="规则操作">
      <template #title>
        <h3 class="font-semibold">规则列表</h3>
      </template>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('addDevice')"
      >
        <Plus :size="13" />设备规则
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('preview')"
      >
        <Eye :size="13" />预览
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('share')"
      >
        <Share2 :size="13" />分享
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('import')"
      >
        <Upload :size="13" />导入
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('addGeo')"
      >
        <Plus :size="13" />GEO 规则
      </button>
      <button class="aw-action-button aw-action-neutral" @click="$emit('add')">
        <Plus :size="13" />自定义规则
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('refresh')"
      >
        <RefreshCw :size="13" />刷新
      </button>
    </AdaptiveActionBar>
    <p
      v-if="sorting"
      role="status"
      class="mt-2 text-xs text-[var(--text-secondary)]"
    >
      正在保存排序…
    </p>
    <div class="aw-data-table-wrap mt-4" :aria-busy="sorting">
      <table class="aw-data-table min-w-[760px]">
        <thead>
          <tr>
            <th>排序</th>
            <th>名称</th>
            <th>类型</th>
            <th>匹配值</th>
            <th>命中后动作</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody ref="tableBody">
          <tr v-if="!rules.length">
            <td colspan="7" class="py-8 text-center">暂无路由规则</td>
          </tr>
          <tr
            v-for="(r, i) in displayRules"
            :key="r.id"
            :data-rule-id="r.id"
            :class="{ 'rule-row-placeholder': draggingId === r.id }"
          >
            <td>
              <div class="flex items-center gap-1.5">
                <span class="w-6 text-[var(--text-tertiary)]"
                  >#{{ i + 1 }}</span
                >
                <button
                  v-if="!isAdBlock(r) && !isFinalStrategy(r)"
                  type="button"
                  class="aw-action-button aw-action-neutral !h-7 !w-7 touch-none select-none !px-0 cursor-grab active:cursor-grabbing"
                  :disabled="sorting"
                  title="上下拖动排序，也可使用方向键"
                  :aria-label="`调整 ${r.name} 的顺序`"
                  @pointerdown="start($event, r)"
                  @keydown="keydown($event, r)"
                  @dragstart.prevent
                >
                  <GripVertical :size="15" />
                </button>
              </div>
            </td>
            <td>
              <FlagText
                class="font-medium text-[var(--text-primary)]"
                :text="isFinalStrategy(r) ? '最终策略' : r.name"
              /><small
                v-if="r.is_system"
                class="ml-2 rounded-full bg-[var(--button-primary-bg)] px-2 py-0.5 text-[10px] text-[var(--button-primary-text)]"
                >系统默认</small
              >
            </td>
            <td>
              <span
                class="rounded bg-[var(--button-secondary-bg)] px-2 py-1 text-xs"
                >{{ typeLabel(r) }}</span
              >
            </td>
            <td class="max-w-[320px] truncate" :title="valueLabel(r)">
              {{ valueLabel(r) }}
            </td>
            <td>{{ actionLabel(r) }}</td>
            <td>
              <button
                class="aw-action-button"
                :class="r.enabled ? 'aw-action-success' : 'aw-action-neutral'"
                :disabled="isAdBlock(r) || isFinalStrategy(r)"
                :title="statusToggleTitle(r)"
                @click="$emit('toggle', r)"
              >
                {{ r.enabled ? "启用" : "停用" }}
              </button>
            </td>
            <td>
              <div class="flex gap-2">
                <button
                  v-if="!r.is_system || isFinalStrategy(r)"
                  class="aw-action-button aw-action-neutral"
                  @click="$emit('edit', r)"
                >
                  <Edit3 :size="13" />编辑</button
                ><button
                  v-if="!r.is_system"
                  class="aw-action-button aw-action-danger"
                  @click="$emit('remove', r)"
                >
                  <Trash2 :size="13" />删除
                </button>
                <button
                  class="aw-action-button aw-action-neutral"
                  @click="$emit('detail', r)"
                >
                  <Eye :size="13" />查看
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.rule-row-placeholder > td {
  background: var(--button-primary-bg) !important;
  color: transparent !important;
  box-shadow:
    inset 0 1px 0 var(--color-primary),
    inset 0 -1px 0 var(--color-primary);
}
.rule-row-placeholder > td > * {
  visibility: hidden;
}
:global(.rule-sort-ghost) {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 60;
  pointer-events: none;
  overflow: hidden;
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-card);
  outline: 1px solid var(--color-primary);
  opacity: 0.8;
  will-change: transform;
}
:global(body.rule-sort-active),
:global(body.rule-sort-active *) {
  cursor: grabbing !important;
  user-select: none !important;
}
</style>
