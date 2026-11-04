<script setup lang="ts">
import {
  Edit3,
  Eye,
  GripVertical,
  LockKeyhole,
  Plus,
  RefreshCw,
  Settings2,
  Share2,
  Trash2,
  Upload,
} from "lucide-vue-next";
import AdaptiveActionBar from "@/components/ui/AdaptiveActionBar.vue";
import { vSpotlight } from "@/directives/spotlight";
import FlagText from "@/components/ui/FlagText.vue";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";
import { splitDeviceRuleValues } from "./deviceRulePreview";
import { useRuleSort } from "./useRuleSort";
const props = defineProps<{
  rules: RouteRule[];
  subscriptions: RouteRuleSubscription[];
  sorting: boolean;
  strategyLoadingId: number | null;
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
  editStrategy: [RouteRule];
  remove: [RouteRule];
  detail: [RouteRule];
}>();
const { tableBody, displayRules, draggingId, start, keydown } = useRuleSort(
  () => props.rules,
  () => props.sorting,
  (ids) => emit("reorder", ids),
);

function setTableBody(element: unknown) {
  tableBody.value = element instanceof HTMLTableSectionElement ? element : null;
}

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
    v-spotlight
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
    <div class="rule-list-wrap aw-data-table-wrap mt-4" :aria-busy="sorting">
      <table
        class="rule-list-table aw-data-table sm:min-w-[1040px] sm:table-fixed"
      >
        <colgroup class="rule-list-columns">
          <col class="w-24" />
          <col class="w-44" />
          <col class="w-36" />
          <col class="w-72" />
          <col class="w-32" />
          <col class="w-20" />
          <col class="w-72" />
        </colgroup>
        <thead class="rule-list-head">
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
        <tbody :ref="setTableBody" class="rule-list-body">
          <tr v-if="!rules.length" class="rule-empty-row">
            <td colspan="7" class="py-8 text-center">暂无路由规则</td>
          </tr>
          <tr
            v-for="(r, i) in displayRules"
            :key="r.id"
            :data-sort-id="r.id"
            class="rule-list-row"
            :class="{ 'aw-sort-placeholder': draggingId === r.id }"
          >
            <td class="rule-sort-cell">
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
                <span
                  v-else
                  role="img"
                  class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-[var(--border-light)] text-[var(--text-tertiary)]"
                  :aria-label="isAdBlock(r) ? '固定在首位' : '固定在末位'"
                  :title="isAdBlock(r) ? '固定在首位' : '固定在末位'"
                >
                  <LockKeyhole :size="13" aria-hidden="true" />
                </span>
              </div>
            </td>
            <td class="rule-name-cell">
              <div class="flex min-w-0 flex-wrap items-center gap-1.5">
                <FlagText
                  class="min-w-0 font-medium text-[var(--text-primary)]"
                  :text="isFinalStrategy(r) ? '最终策略' : r.name"
                /><small
                  v-if="r.is_system"
                  class="inline-flex shrink-0 whitespace-nowrap rounded-full bg-[var(--button-primary-bg)] px-2 py-0.5 text-[10px] text-[var(--button-primary-text)]"
                  >系统默认</small
                >
              </div>
            </td>
            <td class="rule-type-cell">
              <span
                class="inline-block max-w-full truncate whitespace-nowrap rounded bg-[var(--button-secondary-bg)] px-2 py-1 text-xs"
                :title="typeLabel(r)"
                >{{ typeLabel(r) }}</span
              >
            </td>
            <td class="rule-value-cell" :title="valueLabel(r)">
              <span class="rule-cell-label">匹配</span>
              <span class="block truncate">{{ valueLabel(r) }}</span>
            </td>
            <td class="rule-action-cell">
              <span class="rule-cell-label">动作</span>
              <span>{{ actionLabel(r) }}</span>
            </td>
            <td class="rule-status-cell">
              <span class="rule-cell-label">状态</span>
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
            <td class="rule-operations-cell">
              <div class="flex flex-wrap gap-2 sm:flex-nowrap">
                <button
                  v-if="!r.is_system || isFinalStrategy(r)"
                  class="aw-action-button aw-action-neutral"
                  @click="$emit('edit', r)"
                >
                  <Edit3 :size="13" />编辑
                </button>
                <button
                  v-if="
                    r.outbound === 'proxy' &&
                    !r.is_system &&
                    !isFinalStrategy(r)
                  "
                  class="aw-action-button aw-action-neutral"
                  :disabled="strategyLoadingId !== null"
                  @click="$emit('editStrategy', r)"
                >
                  <RefreshCw
                    v-if="strategyLoadingId === r.id"
                    :size="13"
                    class="animate-spin"
                  />
                  <Settings2 v-else :size="13" />编辑策略
                </button>
                <button
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
@media (max-width: 639px) {
  .rule-list-wrap {
    overflow: visible;
    background: transparent;
  }
  .rule-list-table,
  .rule-list-body {
    display: block;
    width: 100%;
  }
  .rule-list-columns,
  .rule-list-head {
    display: none;
  }
  .rule-list-body {
    display: grid;
    gap: 0.75rem;
  }
  .rule-list-row {
    display: grid;
    grid-template-columns: 2rem minmax(0, 1fr) auto;
    grid-template-areas:
      "sort name type"
      "sort value value"
      "sort action status"
      "operations operations operations";
    gap: 0.55rem 0.75rem;
    align-items: center;
    padding: 0.75rem;
    border: 1px solid var(--border-light);
    border-radius: var(--radius-lg);
    background: var(--bg-inset);
  }
  .rule-list-row > td {
    min-width: 0;
    padding: 0 !important;
    border-bottom: 0 !important;
    background: transparent !important;
  }
  .rule-sort-cell {
    grid-area: sort;
    align-self: start;
  }
  .rule-sort-cell > div {
    flex-direction: column;
    gap: 0.35rem;
  }
  .rule-sort-cell span {
    width: auto;
  }
  .rule-name-cell {
    grid-area: name;
  }
  .rule-type-cell {
    grid-area: type;
    max-width: 8rem;
    text-align: right;
  }
  .rule-value-cell {
    grid-area: value;
  }
  .rule-action-cell {
    grid-area: action;
  }
  .rule-status-cell {
    grid-area: status;
    justify-self: end;
  }
  .rule-operations-cell {
    grid-area: operations;
    padding-top: 0.2rem !important;
    border-top: 1px solid var(--border-light) !important;
  }
  .rule-value-cell,
  .rule-action-cell,
  .rule-status-cell {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 0.45rem;
  }
  .rule-cell-label {
    flex: none;
    font-size: 10px;
    color: var(--text-tertiary);
  }
  .rule-empty-row {
    display: block;
    border: 1px solid var(--border-light);
    border-radius: var(--radius-lg);
  }
  .rule-empty-row > td {
    display: block;
  }
}
.rule-cell-label {
  display: none;
}
@media (max-width: 639px) {
  .rule-cell-label {
    display: inline;
  }
}
</style>
