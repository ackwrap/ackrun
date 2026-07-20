<script setup lang="ts">
import { ref, watch } from "vue";
import Modal from "@/components/ui/Modal.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { api } from "@/services/api";
import type { NodeItem } from "@/services/types";

const props = defineProps<{ open: boolean; selected: string[] }>();
const emit = defineEmits<{
  close: [];
  "update:selected": [string[]];
  error: [string];
}>();
const nodes = ref<NodeItem[]>([]);
const keyword = ref("");
const loading = ref(false);
const flags = ref<Record<string, string>>({});

async function load() {
  loading.value = true;
  try {
    nodes.value = (
      await api.getNodes({ enabled: true, limit: 1000, keyword: keyword.value })
    ).items;
    const inferred = await api.inferNodeFlags(
      nodes.value.map((node) => ({
        key: node.uid,
        name: node.name,
        server: node.server,
      })),
    );
    flags.value = Object.fromEntries(
      inferred.items.map((item) => [item.key, item.flag]),
    );
  } catch (error) {
    emit(
      "error",
      `加载节点失败: ${error instanceof Error ? error.message : "请求失败"}`,
    );
  } finally {
    loading.value = false;
  }
}

function toggle(uid: string) {
  emit(
    "update:selected",
    props.selected.includes(uid)
      ? props.selected.filter((item) => item !== uid)
      : [...props.selected, uid],
  );
}

watch(
  () => props.open,
  (open) => {
    if (open) void load();
  },
  { immediate: true },
);
</script>

<template>
  <Modal :open="open" title="手动选择节点" size="xl" @close="emit('close')">
    <div class="mb-3 flex gap-2">
      <input
        v-model="keyword"
        class="aw-input flex-1"
        placeholder="搜索节点名称"
        @keyup.enter="load"
      />
      <button class="aw-action-button" @click="load">搜索</button>
    </div>
    <div class="max-h-[55vh] overflow-auto">
      <table class="aw-data-table min-w-[820px]">
        <thead>
          <tr>
            <th>选择</th>
            <th>节点名称</th>
            <th>协议</th>
            <th>订阅来源</th>
            <th>延迟</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="6" class="py-10 text-center">加载中...</td>
          </tr>
          <tr v-else-if="!nodes.length">
            <td colspan="6" class="py-10 text-center">没有匹配的节点</td>
          </tr>
          <tr v-for="node in nodes" v-else :key="node.uid">
            <td>
              <input
                type="checkbox"
                :checked="selected.includes(node.uid)"
                @change="toggle(node.uid)"
              />
            </td>
            <td><NodeFlagName :name="node.name" :flag="flags[node.uid]" /></td>
            <td>{{ node.type }}</td>
            <td>{{ node.subscription_name }}</td>
            <td>{{ node.latency_ms || "-" }}</td>
            <td>{{ node.status }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </Modal>
</template>
