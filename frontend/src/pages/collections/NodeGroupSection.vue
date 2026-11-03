<script setup lang="ts">
import { Edit, Eye, Globe, Plus, Trash2, Zap } from "lucide-vue-next";
import Pagination from "@/components/ui/Pagination.vue";
import { defaultFlag, getFlagImageURL } from "@/utils/nodeFlags";
import { subscriptionFilterLabel } from "./nodeGroupLabels";
import type { FacetItem, NodeGroup } from "./collectionTypes";
import { directNodeGroup } from "./collectionSources";

const props = defineProps<{
  groups: NodeGroup[];
  pagedGroups: NodeGroup[];
  selected: number[];
  subscriptions: FacetItem[];
  flags: Record<string, string>;
  page: number;
  pageSize: number;
  totalPages: number;
}>();
const emit = defineEmits<{
  "update:selected": [number[]];
  "update:page": [number];
  "update:pageSize": [number];
  create: [];
  quick: [];
  batchDelete: [];
  detail: [NodeGroup];
  edit: [NodeGroup];
  remove: [NodeGroup];
}>();

function toggleSelected(id: number) {
  emit(
    "update:selected",
    props.selected.includes(id)
      ? props.selected.filter((item) => item !== id)
      : [...props.selected, id],
  );
}

function togglePage() {
  const allSelected =
    props.pagedGroups.length > 0 &&
    props.pagedGroups.every((group) => props.selected.includes(group.id));
  emit(
    "update:selected",
    allSelected
      ? props.selected.filter(
          (id) => !props.pagedGroups.some((group) => group.id === id),
        )
      : [
          ...new Set([
            ...props.selected,
            ...props.pagedGroups.map((group) => group.id),
          ]),
        ],
  );
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap justify-between gap-2">
      <p class="text-sm text-[var(--text-secondary)]">
        默认按地域自动分组，新增节点、导入和订阅更新后自动归组；支持自定义节点组
      </p>
      <div class="flex flex-wrap gap-2">
        <button
          v-if="selected.length"
          class="aw-action-button"
          @click="emit('batchDelete')"
        >
          <Trash2 :size="14" />批量删除
        </button>
        <button class="aw-action-button" @click="emit('quick')">
          <Zap :size="14" />智能快速配置
        </button>
        <button class="aw-action-button" @click="emit('create')">
          <Plus :size="14" />新增节点组
        </button>
      </div>
    </div>
    <label class="flex items-center gap-2 text-xs sm:hidden">
      <input
        type="checkbox"
        :checked="
          pagedGroups.length > 0 &&
          pagedGroups.every((group) => selected.includes(group.id))
        "
        @change="togglePage"
      />选择当前页
    </label>
    <div
      class="aw-mobile-table-wrap overflow-x-auto border border-[var(--border-default)]"
    >
      <table class="aw-data-table aw-mobile-table min-w-[1120px]">
        <thead>
          <tr>
            <th>
              <input
                type="checkbox"
                :checked="
                  pagedGroups.length > 0 &&
                  pagedGroups.every((group) => selected.includes(group.id))
                "
                @change="togglePage"
              />
            </th>
            <th
              v-for="column in [
                '名称',
                '类型',
                '协议限制',
                '订阅限制',
                '包含关键词',
                '排除关键词',
                '匹配节点',
                '状态',
                '操作',
              ]"
              :key="column"
            >
              {{ column }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td class="aw-mobile-selection">
              <input
                type="checkbox"
                disabled
                aria-label="全球直连为固定内置组，不能批量删除"
              />
            </td>
            <td class="aw-mobile-title">
              <Globe :size="16" class="mr-2 inline" />{{ directNodeGroup.name }}
              <small class="ml-2 text-[var(--text-tertiary)]">内置</small>
            </td>
            <td data-label="类型">直连</td>
            <td
              data-label="节点筛选"
              colspan="4"
              class="text-[var(--text-secondary)]"
            >
              固定 DIRECT 出站，不经过代理，不参与节点筛选或自动测速
            </td>
            <td data-label="匹配节点">—</td>
            <td data-label="状态">始终启用</td>
            <td class="aw-mobile-actions text-[var(--text-tertiary)]">
              固定内置组
            </td>
          </tr>
          <tr v-for="group in pagedGroups" :key="group.id">
            <td class="aw-mobile-selection">
              <input
                type="checkbox"
                :checked="selected.includes(group.id)"
                :aria-label="`选择 ${group.name}`"
                @change="toggleSelected(group.id)"
              />
            </td>
            <td class="aw-mobile-title">
              <img
                :src="getFlagImageURL(flags[String(group.id)] || defaultFlag)"
                class="mr-2 inline h-4 w-4"
              />{{ group.name }}
            </td>
            <td data-label="类型">
              {{ group.type === "urltest" ? "自动" : "手动" }}
            </td>
            <td data-label="协议限制">
              {{ group.filter_protocols || "全部" }}
            </td>
            <td data-label="订阅限制">
              {{
                subscriptionFilterLabel(
                  group.filter_subscriptions,
                  subscriptions,
                )
              }}
            </td>
            <td data-label="包含关键词">{{ group.filter_include || "-" }}</td>
            <td data-label="排除关键词">{{ group.filter_exclude || "-" }}</td>
            <td data-label="匹配节点">
              {{ group.matched_node_count || 0 }} 个
            </td>
            <td data-label="状态">{{ group.enabled ? "启用" : "停用" }}</td>
            <td class="aw-mobile-actions">
              <div class="flex flex-wrap gap-2">
                <button @click="emit('detail', group)">
                  <Eye :size="12" class="inline" />详情
                </button>
                <button @click="emit('edit', group)">
                  <Edit :size="12" class="inline" />编辑
                </button>
                <button @click="emit('remove', group)">
                  <Trash2 :size="12" class="inline" />删除
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <Pagination
      :total="groups.length"
      :page="page"
      :page-size="pageSize"
      :total-pages="totalPages"
      @page-change="emit('update:page', $event)"
      @page-size-change="emit('update:pageSize', $event)"
    />
  </section>
</template>
