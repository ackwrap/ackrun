<script setup lang="ts">
import { Edit, Eye, Plus, Trash2, Zap } from "lucide-vue-next";
import Pagination from "@/components/ui/Pagination.vue";
import { defaultFlag, getFlagImageURL } from "@/utils/nodeFlags";
import { subscriptionFilterLabel } from "./nodeGroupLabels";
import type { FacetItem, NodeGroup } from "./collectionTypes";

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
      : [...new Set([...props.selected, ...props.pagedGroups.map((group) => group.id)])],
  );
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap justify-between gap-2">
      <p class="text-sm text-[var(--text-secondary)]">
        使用关键词自动筛选节点，作为策略组的基础单元
      </p>
      <div class="flex gap-2">
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
    <div class="overflow-x-auto border border-[var(--border-default)]">
      <table class="aw-data-table min-w-[1120px]">
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
          <tr v-for="group in pagedGroups" :key="group.id">
            <td>
              <input
                type="checkbox"
                :checked="selected.includes(group.id)"
                @change="toggleSelected(group.id)"
              />
            </td>
            <td>
              <img
                :src="getFlagImageURL(flags[String(group.id)] || defaultFlag)"
                class="mr-2 inline h-4 w-4"
              />{{ group.name }}
            </td>
            <td>{{ group.type === "urltest" ? "自动" : "手动" }}</td>
            <td>{{ group.filter_protocols || "全部" }}</td>
            <td>
              {{
                subscriptionFilterLabel(
                  group.filter_subscriptions,
                  subscriptions,
                )
              }}
            </td>
            <td>{{ group.filter_include }}</td>
            <td>{{ group.filter_exclude || "-" }}</td>
            <td>{{ group.matched_node_count || 0 }} 个</td>
            <td>{{ group.enabled ? "启用" : "停用" }}</td>
            <td>
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
