<script setup lang="ts">
import { Eye, Plus, Trash2 } from "lucide-vue-next";
import OrderButtons from "@/components/ui/OrderButtons.vue";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";
defineProps<{ rules: RouteRule[]; subscriptions: RouteRuleSubscription[] }>();
defineEmits<{
  refresh: [];
  addGeo: [];
  add: [];
  preview: [];
  move: [number, -1 | 1];
  toggle: [RouteRule];
  edit: [RouteRule];
  remove: [RouteRule];
  detail: [RouteRule];
}>();
</script>
<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="font-semibold">规则列表</h3>
        <small class="text-[var(--text-secondary)]"
          >上移/下移会立即保存排序。</small
        >
      </div>
      <div class="flex flex-wrap justify-end gap-2">
        <button
          class="aw-action-button aw-action-neutral"
          @click="$emit('preview')"
        >
          <Eye :size="13" />预览
        </button>
        <button
          class="aw-action-button aw-action-neutral"
          @click="$emit('addGeo')"
        >
          <Plus :size="13" />添加 GEO 规则
        </button>
        <button
          class="aw-action-button aw-action-neutral"
          @click="$emit('add')"
        >
          自定义规则
        </button>
        <button
          class="aw-action-button aw-action-neutral"
          @click="$emit('refresh')"
        >
          刷新
        </button>
      </div>
    </header>
    <div class="aw-data-table-wrap mt-4">
      <table class="aw-data-table min-w-[880px]">
        <thead>
          <tr>
            <th>排序</th>
            <th>名称</th>
            <th>类型</th>
            <th>匹配值</th>
            <th>出站</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!rules.length">
            <td colspan="7" class="py-12 text-center">暂无路由规则</td>
          </tr>
          <tr v-for="(r, i) in rules" :key="r.id">
            <td>
              <div class="flex items-center gap-1.5">
                <span class="w-6 text-[var(--text-tertiary)]"
                  >#{{ i + 1 }}</span
                >
                <OrderButtons
                  :up-disabled="i === 0"
                  :down-disabled="i === rules.length - 1"
                  @up="$emit('move', i, -1)"
                  @down="$emit('move', i, 1)"
                />
              </div>
            </td>
            <td>
              <span class="font-medium text-[var(--text-primary)]">{{
                r.name
              }}</span
              ><small
                v-if="r.is_system"
                class="ml-2 rounded-full bg-[var(--button-primary-bg)] px-2 py-0.5 text-[10px] text-[var(--button-primary-text)]"
                >系统默认</small
              >
            </td>
            <td>
              <span
                class="rounded bg-[var(--button-secondary-bg)] px-2 py-1 text-xs"
                >{{ r.rule_type }}</span
              >
            </td>
            <td class="max-w-[320px] truncate" :title="r.values.join('\n')">
              {{ r.values.join(", ") }}
            </td>
            <td>{{ r.outbound }}</td>
            <td>
              <button
                class="aw-action-button"
                :class="r.enabled ? 'aw-action-success' : 'aw-action-neutral'"
                @click="$emit('toggle', r)"
              >
                {{ r.enabled ? "启用" : "停用" }}
              </button>
            </td>
            <td>
              <div class="flex gap-2">
                <button
                  v-if="!r.is_system"
                  class="aw-action-button aw-action-neutral"
                  @click="$emit('edit', r)"
                >
                  编辑</button
                ><button
                  class="aw-action-button aw-action-danger"
                  :disabled="r.is_system"
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
