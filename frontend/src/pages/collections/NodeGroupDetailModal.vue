<script setup lang="ts">
import { computed } from "vue";
import Modal from "@/components/ui/Modal.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { subscriptionFilterLabel } from "./nodeGroupLabels";

interface FacetItem {
  value: string;
  label: string;
  count: number;
}
interface Group {
  id: number;
  name: string;
  filter_protocols: string;
  filter_subscriptions: string;
  filter_include: string;
  filter_exclude: string;
}
interface Node {
  uid: string;
  name: string;
  type: string;
  subscription_id: number;
  subscription_name: string;
  latency_ms: number;
  status: string;
}
const props = defineProps<{
  group: Group;
  nodes: Node[];
  loading: boolean;
  subscriptions: FacetItem[];
  flags: Record<string, string>;
}>();
const emit = defineEmits<{ close: [] }>();
const subscriptionLabel = computed(() =>
  subscriptionFilterLabel(
    props.group.filter_subscriptions,
    props.subscriptions,
  ),
);
</script>

<template>
  <Modal
    :open="true"
    :title="`节点组详情：${group.name}`"
    size="xl"
    @close="emit('close')"
  >
    <p class="mb-4 text-sm text-[var(--text-secondary)]">
      当前匹配
      {{ nodes.length }} 个节点。排除关键词优先，随后包含关键词任意命中即加入。
    </p>
    <div
      class="mb-4 grid gap-3 rounded-md border border-[var(--border-default)] bg-[var(--bg-secondary)] p-3 md:grid-cols-4"
    >
      <div>
        协议：<b class="uppercase text-[var(--text-primary)]">{{
          group.filter_protocols || "全部"
        }}</b>
      </div>
      <div>
        订阅：<b class="text-[var(--text-primary)]">{{ subscriptionLabel }}</b>
      </div>
      <div class="md:col-span-2">
        包含：<b class="font-mono text-[var(--text-primary)]">{{
          group.filter_include || "无"
        }}</b>
      </div>
      <div class="md:col-span-4">
        排除：<b class="font-mono text-[var(--text-primary)]">{{
          group.filter_exclude || "无"
        }}</b>
      </div>
    </div>
    <div class="aw-data-table-wrap max-h-[60vh]">
      <table class="aw-data-table min-w-[820px]">
        <thead>
          <tr>
            <th
              v-for="col in ['节点名称', '协议', '订阅来源', '延迟', '状态']"
              :key="col"
            >
              {{ col }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="5" class="py-10 text-center">加载中...</td>
          </tr>
          <tr v-else-if="!nodes.length">
            <td colspan="5" class="py-10 text-center">没有匹配到节点</td>
          </tr>
          <tr v-for="node in nodes" v-else :key="node.uid">
            <td class="max-w-[420px] truncate font-medium">
              <NodeFlagName :name="node.name" :flag="flags[node.uid]" />
            </td>
            <td class="uppercase">{{ node.type }}</td>
            <td>
              {{ node.subscription_name || `订阅 ${node.subscription_id}` }}
            </td>
            <td>{{ node.latency_ms > 0 ? `${node.latency_ms} ms` : "-" }}</td>
            <td>{{ node.status || "unknown" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </Modal>
</template>
