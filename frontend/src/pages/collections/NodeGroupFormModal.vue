<script setup lang="ts">
import { ref } from "vue";
import Modal from "@/components/ui/Modal.vue";
import ManualNodePickerModal from "./ManualNodePickerModal.vue";
import type {
  FacetItem,
  NodeGroup,
  NodeGroupRequest,
} from "./collectionTypes";

const props = defineProps<{
  group: NodeGroup;
  protocols: FacetItem[];
  subscriptions: FacetItem[];
}>();
const emit = defineEmits<{
  close: [];
  save: [NodeGroupRequest];
  error: [string];
}>();

function parseUIDs(value: string) {
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed)
      ? parsed.filter((item): item is string => typeof item === "string")
      : [];
  } catch {
    return [];
  }
}

function toggle<T>(items: T[], value: T) {
  return items.includes(value)
    ? items.filter((item) => item !== value)
    : [...items, value];
}

const name = ref(props.group.name || "");
const type = ref(props.group.type || "selector");
const selectedProtocols = ref(
  props.group.filter_protocols?.split(",").filter(Boolean) || [],
);
const selectedSubscriptions = ref(
  props.group.filter_subscriptions?.split(",").filter(Boolean) || [],
);
const include = ref(props.group.filter_include || "");
const exclude = ref(props.group.filter_exclude || "");
const nodeUIDs = ref<string[]>(parseUIDs(props.group.node_uids));
const enabled = ref(props.group.enabled ?? true);
const tolerance = ref(props.group.tolerance || 100);
const pickerOpen = ref(false);

function submit() {
  emit("save", {
    name: name.value,
    type: type.value,
    filter_protocols: selectedProtocols.value.join(","),
    filter_subscriptions: selectedSubscriptions.value.join(","),
    filter_include: include.value,
    filter_exclude: exclude.value,
    node_uids: nodeUIDs.value,
    enabled: enabled.value,
    priority: props.group.priority || 0,
    tolerance: tolerance.value,
  });
}
</script>

<template>
  <Modal
    :open="true"
    :title="group.id ? '编辑节点组' : '新增节点组'"
    size="lg"
    @close="emit('close')"
  >
    <div class="grid gap-4 md:grid-cols-2">
      <label class="grid content-start gap-1.5 md:col-span-2">
        <span class="aw-modal-label text-xs font-medium">节点组名称</span>
        <input
          v-model.trim="name"
          class="aw-input"
          placeholder="例如：香港节点、流媒体节点"
        />
      </label>
      <label class="grid content-start gap-1.5">
        <span class="aw-modal-label text-xs font-medium">节点组类型</span>
        <select v-model="type" class="aw-input">
          <option value="selector">手动切换</option>
          <option value="urltest">自动选择（测速）</option>
        </select>
      </label>
      <label
        class="flex h-[34px] cursor-pointer items-center gap-2 self-end rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs text-[var(--text-secondary)]"
      >
        <input v-model="enabled" type="checkbox" class="h-4 w-4" />
        启用此节点组
      </label>
      <div
        class="flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-3 md:col-span-2"
      >
        <div>
          <p class="text-xs font-medium">指定固定节点</p>
          <p class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">
            已手动选择节点时优先使用，下面的动态筛选条件不参与匹配
          </p>
        </div>
        <button class="aw-action-button" @click="pickerOpen = true">
          手动选择节点（已选择 {{ nodeUIDs.length }} 个）
        </button>
      </div>
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
      <fieldset class="grid gap-2 md:col-span-2">
        <div class="flex items-end justify-between gap-2">
          <div>
            <legend class="text-sm font-semibold">协议范围</legend>
            <p class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">
              不选择表示允许全部协议
            </p>
          </div>
          <button
            v-if="selectedProtocols.length"
            type="button"
            class="aw-action-button"
            @click="selectedProtocols = []"
          >
            清空
          </button>
        </div>
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
          <label
            v-for="item in protocols"
            :key="item.value"
            class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border px-3 py-2 text-xs transition-colors"
            :class="
              selectedProtocols.includes(item.value)
                ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]'
            "
          >
            <input
              type="checkbox"
              class="h-4 w-4 shrink-0"
              :checked="selectedProtocols.includes(item.value)"
              @change="selectedProtocols = toggle(selectedProtocols, item.value)"
            />
            <span class="min-w-0 flex-1 truncate uppercase">{{ item.label }}</span>
            <span class="shrink-0 tabular-nums opacity-70">{{ item.count }}</span>
          </label>
        </div>
      </fieldset>
      <fieldset class="grid gap-2 md:col-span-2">
        <div class="flex items-end justify-between gap-2">
          <div>
            <legend class="text-sm font-semibold">订阅范围</legend>
            <p class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">
              不选择表示包含全部订阅
            </p>
          </div>
          <button
            v-if="selectedSubscriptions.length"
            type="button"
            class="aw-action-button"
            @click="selectedSubscriptions = []"
          >
            清空
          </button>
        </div>
        <div
          class="grid max-h-48 gap-2 overflow-y-auto rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-2 sm:grid-cols-2"
        >
          <label
            v-for="item in subscriptions"
            :key="item.value"
            class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border px-3 py-2 text-xs transition-colors"
            :class="
              selectedSubscriptions.includes(item.value)
                ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]'
            "
          >
            <input
              type="checkbox"
              class="h-4 w-4 shrink-0"
              :checked="selectedSubscriptions.includes(item.value)"
              @change="
                selectedSubscriptions = toggle(selectedSubscriptions, item.value)
              "
            />
            <span class="min-w-0 flex-1 truncate">{{ item.label }}</span>
            <span class="shrink-0 tabular-nums opacity-70">{{ item.count }}</span>
          </label>
          <p
            v-if="!subscriptions.length"
            class="py-6 text-center text-xs text-[var(--text-tertiary)] sm:col-span-2"
          >
            暂无可用订阅
          </p>
        </div>
      </fieldset>
      <label class="grid content-start gap-1.5 md:col-span-2">
        <span class="aw-modal-label text-xs font-medium">包含关键词</span>
        <input
          v-model.trim="include"
          class="aw-input"
          placeholder="正则表达式，例如：香港|HK"
        />
      </label>
      <label class="grid content-start gap-1.5 md:col-span-2">
        <span class="aw-modal-label text-xs font-medium">排除关键词</span>
        <input
          v-model.trim="exclude"
          class="aw-input"
          placeholder="正则表达式，例如：过期|流量"
        />
      </label>
    </div>
    <template #footer>
      <button class="aw-action-button" @click="emit('close')">取消</button>
      <button
        class="aw-action-button"
        :disabled="!name.trim()"
        @click="submit"
      >
        保存
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
