<script setup lang="ts">
import { computed, ref } from "vue";
import Modal from "@/components/ui/Modal.vue";
import type { StrategyItem } from "@/services/types";
import ManualNodePickerModal from "./ManualNodePickerModal.vue";
import type {
  CollectionSourceType,
  DetailedProxyCollection,
  NodeGroup,
  StrategyCollectionRequest,
} from "./collectionTypes";

const props = defineProps<{
  strategy: StrategyItem;
  groups: NodeGroup[];
  connectivity: { test_url: string; interval_seconds: number };
  saving: boolean;
}>();
const emit = defineEmits<{
  close: [];
  save: [StrategyCollectionRequest];
  error: [string];
}>();

const collection = props.strategy.collection as
  | DetailedProxyCollection
  | undefined;

function referencedGroupIDs() {
  if (collection?.referenced_groups?.length) {
    return collection.referenced_groups.map((group) => group.id);
  }
  try {
    const parsed = JSON.parse(collection?.referenced_group_ids || "[]");
    return Array.isArray(parsed)
      ? parsed.filter(
          (item): item is number => Number.isInteger(item) && item > 0,
        )
      : [];
  } catch {
    return [];
  }
}

const type = ref<"selector" | "urltest">(collection?.type || "selector");
const source = ref<CollectionSourceType>(
  collection?.source_type || "node_groups",
);
const groupIDs = ref<number[]>(referencedGroupIDs());
const nodeUIDs = ref(
  (collection?.node_uids || []).filter((uid) => uid !== "direct"),
);
const enabled = ref(collection?.enabled ?? true);
const tolerance = ref(collection?.tolerance || 100);
const groupSearch = ref("");
const pickerOpen = ref(false);

const selectableGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase();
  return props.groups.filter(
    (group) =>
      group.enabled &&
      ((group.matched_node_count || 0) > 0 || groupIDs.value.includes(group.id)) &&
      (!keyword || group.name.toLowerCase().includes(keyword)),
  );
});
const selectableGroupIDs = computed(() =>
  selectableGroups.value.map((group) => group.id),
);
const canSave = computed(
  () =>
    !props.saving &&
    (source.value === "manual"
      ? nodeUIDs.value.length > 0
      : groupIDs.value.length > 0),
);

function toggleGroup(id: number) {
  groupIDs.value = groupIDs.value.includes(id)
    ? groupIDs.value.filter((item) => item !== id)
    : [...groupIDs.value, id];
}

function submit() {
  if (!canSave.value) return;
  emit("save", {
    name: props.strategy.name,
    type: type.value,
    source_type: source.value,
    referenced_group_ids: source.value === "manual" ? [] : groupIDs.value,
    route_rule_id: props.strategy.rule_id,
    route_rule_ids: [props.strategy.rule_id],
    node_uids: source.value === "manual" ? nodeUIDs.value : [],
    enabled: enabled.value,
    tolerance: tolerance.value,
    test_url: props.connectivity.test_url,
    test_interval: props.connectivity.interval_seconds,
  });
}
</script>

<template>
  <Modal
    :open="true"
    :title="strategy.collection ? '编辑代理策略' : '配置代理策略'"
    size="lg"
    @close="emit('close')"
  >
    <div class="grid gap-4 md:grid-cols-2">
      <div class="grid content-start gap-1.5 md:col-span-2">
        <span class="aw-modal-label text-xs font-medium">业务规则 / 策略组名称</span>
        <div
          class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)]"
        >
          {{ strategy.name }}
        </div>
        <small class="text-[11px] text-[var(--text-tertiary)]">
          名称与规则绑定固定，保存时由后端再次校正
        </small>
      </div>
      <label class="grid content-start gap-1.5">
        <span class="aw-modal-label text-xs font-medium">切换方式</span>
        <select v-model="type" class="aw-input">
          <option value="selector">手动切换</option>
          <option value="urltest">自动选择（测速）</option>
        </select>
      </label>
      <label class="grid content-start gap-1.5">
        <span class="aw-modal-label text-xs font-medium">节点来源</span>
        <select v-model="source" class="aw-input">
          <option value="node_groups">引用节点组</option>
          <option value="node_groups_and_nodes">引用节点组和节点</option>
          <option value="manual">手动选择节点</option>
        </select>
      </label>
      <label
        class="flex h-[34px] cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs text-[var(--text-secondary)]"
      >
        <input v-model="enabled" type="checkbox" class="h-4 w-4" />
        启用此代理策略
      </label>
      <template v-if="type === 'urltest'">
        <div
          class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-3 text-xs text-[var(--text-secondary)] md:col-span-2"
        >
          测速地址与间隔统一使用“设置 → 常规功能 → 连通性测速”。
        </div>
        <label class="grid content-start gap-1.5 md:col-span-2">
          <span class="aw-modal-label text-xs font-medium">切换容差（ms）</span>
          <input
            v-model.number="tolerance"
            type="number"
            min="0"
            class="aw-input"
          />
        </label>
      </template>
      <fieldset v-if="source !== 'manual'" class="grid gap-3 md:col-span-2">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <legend class="text-sm font-semibold">选择引用的节点组</legend>
            <p class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">
              <template v-if="source === 'node_groups_and_nodes'">
                同时包含节点组入口和组内全部匹配节点；
              </template>
              <template v-else>仅包含节点组入口；</template>
              已选择 {{ groupIDs.length }} 个
            </p>
          </div>
          <div class="flex gap-2">
            <button
              type="button"
              class="aw-action-button"
              @click="groupIDs = selectableGroupIDs"
            >
              全选当前
            </button>
            <button
              type="button"
              class="aw-action-button"
              :disabled="!groupIDs.length"
              @click="groupIDs = []"
            >
              清空
            </button>
          </div>
        </div>
        <input
          v-model.trim="groupSearch"
          class="aw-input"
          placeholder="搜索节点组"
        />
        <div
          class="grid max-h-64 gap-2 overflow-y-auto rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-2 sm:grid-cols-2"
        >
          <label
            v-for="group in selectableGroups"
            :key="group.id"
            class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border px-3 py-2.5 text-xs transition-colors"
            :class="
              groupIDs.includes(group.id)
                ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]'
            "
          >
            <input
              type="checkbox"
              class="h-4 w-4 shrink-0"
              :checked="groupIDs.includes(group.id)"
              @change="toggleGroup(group.id)"
            />
            <span class="min-w-0 flex-1 truncate">{{ group.name }}</span>
            <span
              class="shrink-0 rounded-full bg-[var(--button-secondary-bg)] px-2 py-0.5 tabular-nums"
            >
              {{ group.matched_node_count || 0 }}
            </span>
          </label>
          <p
            v-if="!selectableGroups.length"
            class="py-8 text-center text-xs text-[var(--text-tertiary)] sm:col-span-2"
          >
            没有匹配的可用节点组
          </p>
        </div>
      </fieldset>
      <div v-else class="md:col-span-2">
        <button class="aw-action-button" @click="pickerOpen = true">
          手动选择节点（已选择 {{ nodeUIDs.length }} 个）
        </button>
      </div>
    </div>
    <template #footer>
      <button class="aw-action-button" :disabled="saving" @click="emit('close')">
        取消
      </button>
      <button class="aw-action-button" :disabled="!canSave" @click="submit">
        {{ saving ? "保存中..." : "保存" }}
      </button>
    </template>
  </Modal>
  <ManualNodePickerModal
    :open="pickerOpen"
    :selected="nodeUIDs"
    @close="pickerOpen = false"
    @update:selected="nodeUIDs = $event"
    @error="emit('error', $event)"
  />
</template>
