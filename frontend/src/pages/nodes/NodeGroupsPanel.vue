<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import { authenticatedFetch } from "@/services/apiAuth";
import type { WSEvent } from "@/services/types";
import type {
  FacetItem,
  NodeGroup,
  NodeGroupRequest,
} from "../collections/collectionTypes";
import NodeGroupDetailModal from "../collections/NodeGroupDetailModal.vue";
import NodeGroupFormModal from "../collections/NodeGroupFormModal.vue";
import NodeGroupSection from "../collections/NodeGroupSection.vue";

interface MatchedNode {
  uid: string;
  name: string;
  type: string;
  subscription_id: number;
  subscription_name: string;
  latency_ms: number;
  status: string;
}

const groups = ref<NodeGroup[]>([]);
const facets = ref<{
  protocols: FacetItem[];
  subscriptions: FacetItem[];
  total: number;
}>({ protocols: [], subscriptions: [], total: 0 });
const flags = ref<Record<string, string>>({});
const nodeFlags = ref<Record<string, string>>({});
const loading = ref(true);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const selected = ref<number[]>([]);
const groupPage = ref(1);
const groupPageSize = ref(25);
const editingGroup = ref<NodeGroup | null>(null);
const detailGroup = ref<NodeGroup | null>(null);
const detailNodes = ref<MatchedNode[]>([]);
const detailLoading = ref(false);
const quickOpen = ref(false);
const quickRunning = ref(false);
const quickProtocols = ref<string[]>([]);
const quickSubscriptions = ref<string[]>([]);
const deleteAction = ref<null | (() => Promise<void>)>(null);
const deleteMessage = ref("");

const groupPages = computed(() =>
  Math.max(1, Math.ceil(groups.value.length / groupPageSize.value)),
);
const pagedGroups = computed(() =>
  groups.value.slice(
    (groupPage.value - 1) * groupPageSize.value,
    groupPage.value * groupPageSize.value,
  ),
);

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

function toggle<T extends string | number>(items: T[], value: T): T[] {
  return items.includes(value)
    ? items.filter((item) => item !== value)
    : [...items, value];
}

async function json(url: string, init?: RequestInit) {
  const response = await authenticatedFetch(url, init);
  if (!response.ok) {
    throw new Error(
      (await response.json().catch(() => null))?.error?.message ||
        response.statusText,
    );
  }
  return response.json().catch(() => null);
}

async function load() {
  try {
    const [groupItems, nodeFacets] = await Promise.all([
      json("/api/v1/node-groups"),
      api.getNodeFacets(),
    ]);
    groups.value = Array.isArray(groupItems) ? groupItems : [];
    facets.value = {
      protocols: nodeFacets.types,
      subscriptions: nodeFacets.subscriptions,
      total: nodeFacets.total,
    };
    flags.value = groups.value.length
      ? Object.fromEntries(
          (
            await api.inferNodeFlags(
              groups.value.map((group) => ({
                key: String(group.id),
                name: group.name,
                server: "",
              })),
            )
          ).items.map((item) => [item.key, item.flag]),
        )
      : {};
  } catch (error) {
    show(
      `加载失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    loading.value = false;
  }
}

async function saveGroup(payload: NodeGroupRequest) {
  try {
    const id = editingGroup.value?.id;
    await json(id ? `/api/v1/node-groups/${id}` : "/api/v1/node-groups", {
      method: id ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    show(id ? "节点组已更新" : "节点组已创建");
    editingGroup.value = null;
    await load();
  } catch (error) {
    show(
      `保存失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  }
}

function confirmDelete(messageText: string, action: () => Promise<void>) {
  deleteMessage.value = messageText;
  deleteAction.value = action;
}

async function removeGroup(group: NodeGroup) {
  await json(`/api/v1/node-groups/${group.id}`, { method: "DELETE" });
  show("节点组已删除");
  await load();
}

async function batchDelete() {
  await json("/api/v1/node-groups/batch-delete", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids: selected.value }),
  });
  show(`已删除 ${selected.value.length} 个节点组`);
  selected.value = [];
  await load();
}

async function showGroupDetail(group: NodeGroup) {
  detailGroup.value = group;
  detailLoading.value = true;
  try {
    const params = new URLSearchParams({
      filter_protocols: group.filter_protocols || "",
      filter_subscriptions: group.filter_subscriptions || "",
      filter_include: group.filter_include || "",
      filter_exclude: group.filter_exclude || "",
    });
    detailNodes.value = await json(`/api/v1/node-groups/preview?${params}`);
    const inferred = await api.inferNodeFlags(
      detailNodes.value.map((node) => ({ key: node.uid, name: node.name })),
    );
    nodeFlags.value = {
      ...nodeFlags.value,
      ...Object.fromEntries(
        inferred.items.map((item) => [item.key, item.flag]),
      ),
    };
  } catch (error) {
    show(
      `加载节点组详情失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    detailLoading.value = false;
  }
}

async function quickSetup() {
  quickRunning.value = true;
  try {
    const subscriptionFilter = facets.value.subscriptions.every((item) =>
      quickSubscriptions.value.includes(item.value),
    )
      ? ""
      : quickSubscriptions.value.join(",");
    const protocolFilter = facets.value.protocols.every((item) =>
      quickProtocols.value.includes(item.value),
    )
      ? ""
      : quickProtocols.value.join(",");
    await json("/api/v1/node-groups/quick-setup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        filter_subscriptions: subscriptionFilter,
        filter_protocols: protocolFilter,
      }),
    });
    show("节点组已同步");
    quickOpen.value = false;
    await load();
  } catch (error) {
    show(
      `快速配置失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    quickRunning.value = false;
  }
}

useRealtimeSocket((event: WSEvent) => {
  if (
    event.type === "node_group.updated" ||
    (event.type === "subscription.sync" &&
      (event.data as { status?: string } | undefined)?.status === "updated")
  ) {
    void load();
  }
});

watch(groupPages, (pages) => {
  groupPage.value = Math.min(groupPage.value, pages);
});
onMounted(load);
</script>

<template>
  <div class="space-y-4">
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <div v-if="loading" class="py-20 text-center">加载中...</div>
    <NodeGroupSection
      v-else
      :groups="groups"
      :paged-groups="pagedGroups"
      :selected="selected"
      :subscriptions="facets.subscriptions"
      :flags="flags"
      :page="groupPage"
      :page-size="groupPageSize"
      :total-pages="groupPages"
      @update:selected="selected = $event"
      @update:page="groupPage = $event"
      @update:page-size="groupPageSize = $event"
      @create="editingGroup = {} as NodeGroup"
      @quick="quickOpen = true"
      @batch-delete="
        confirmDelete(
          `确定删除选中的 ${selected.length} 个节点组吗？`,
          batchDelete,
        )
      "
      @detail="showGroupDetail"
      @edit="editingGroup = $event"
      @remove="
        confirmDelete(`确定删除节点组「${$event.name}」吗？`, () =>
          removeGroup($event),
        )
      "
    />

    <NodeGroupDetailModal
      v-if="detailGroup"
      :group="detailGroup"
      :nodes="detailNodes"
      :flags="nodeFlags"
      :loading="detailLoading"
      :subscriptions="facets.subscriptions"
      @close="detailGroup = null"
    />
    <NodeGroupFormModal
      v-if="editingGroup"
      :group="editingGroup"
      :protocols="facets.protocols"
      :subscriptions="facets.subscriptions"
      @close="editingGroup = null"
      @save="saveGroup"
      @error="show($event, 'error')"
    />

    <Modal :open="quickOpen" title="智能快速配置" @close="quickOpen = false">
      <div class="grid gap-5">
        <fieldset class="grid gap-2">
          <legend
            class="mb-1 text-xs font-semibold text-[var(--text-secondary)]"
          >
            按订阅筛选（不选或全选表示全部，包含未来新增订阅）
          </legend>
          <div class="grid gap-2 sm:grid-cols-2">
            <label
              v-for="item in facets.subscriptions"
              :key="item.value"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--button-secondary-bg)] px-3 py-2 text-xs text-[var(--text-secondary)] transition-colors hover:border-[var(--button-primary-border)] hover:bg-[var(--button-secondary-hover)]"
            >
              <input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="quickSubscriptions.includes(item.value)"
                @change="
                  quickSubscriptions = toggle(quickSubscriptions, item.value)
                "
              />
              <span class="min-w-0 truncate">{{ item.label }}</span>
            </label>
          </div>
        </fieldset>
        <fieldset class="grid gap-2">
          <legend
            class="mb-1 text-xs font-semibold text-[var(--text-secondary)]"
          >
            按协议筛选（不选或全选表示全部）
          </legend>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <label
              v-for="item in facets.protocols"
              :key="item.value"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--button-secondary-bg)] px-3 py-2 text-xs text-[var(--text-secondary)] transition-colors hover:border-[var(--button-primary-border)] hover:bg-[var(--button-secondary-hover)]"
            >
              <input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="quickProtocols.includes(item.value)"
                @change="quickProtocols = toggle(quickProtocols, item.value)"
              />
              <span class="min-w-0 truncate">{{ item.label }}</span>
            </label>
          </div>
        </fieldset>
      </div>
      <template #footer>
        <button
          class="aw-action-button"
          :disabled="quickRunning"
          @click="quickSetup"
        >
          {{ quickRunning ? "配置中..." : "开始配置" }}
        </button>
      </template>
    </Modal>
    <ConfirmDialog
      :open="!!deleteAction"
      title="确认删除"
      :message="deleteMessage"
      confirm-text="删除"
      @cancel="deleteAction = null"
      @confirm="
        async () => {
          const action = deleteAction;
          deleteAction = null;
          if (action)
            try {
              await action();
            } catch (error) {
              show(
                `删除失败: ${
                  error instanceof Error ? error.message : '请求失败'
                }`,
                'error',
              );
            }
        }
      "
    />
  </div>
</template>
