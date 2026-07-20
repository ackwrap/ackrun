<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Layers, Zap } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import { authenticatedFetch } from "@/services/apiAuth";
import type {
  CollectionTestResponse,
  StrategyItem,
  WSEvent,
} from "@/services/types";
import type {
  DetailedProxyCollection,
  FacetItem,
  NodeGroup,
  NodeGroupRequest,
  StrategyCollectionRequest,
} from "./collections/collectionTypes";
import NodeGroupDetailModal from "./collections/NodeGroupDetailModal.vue";
import NodeGroupFormModal from "./collections/NodeGroupFormModal.vue";
import NodeGroupSection from "./collections/NodeGroupSection.vue";
import StrategyFormModal from "./collections/StrategyFormModal.vue";
import StrategySection from "./collections/StrategySection.vue";

interface MatchedNode {
  uid: string;
  name: string;
  type: string;
  subscription_id: number;
  subscription_name: string;
  latency_ms: number;
  status: string;
}

const active = ref<"node-groups" | "collections">("node-groups");
const groups = ref<NodeGroup[]>([]);
const strategies = ref<StrategyItem[]>([]);
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
const editingStrategy = ref<StrategyItem | null>(null);
const strategySaving = ref(false);
const previewStrategy = ref<StrategyItem | null>(null);
const connectivitySettings = ref({ test_url: "", interval_seconds: 300 });
const tests = ref<Record<number, CollectionTestResponse>>({});
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
const previewSummary = computed(() => {
  const collection = previewStrategy.value?.collection as
    | DetailedProxyCollection
    | undefined;
  if (!collection) return null;
  const sourceMode = {
    node_groups: "引用节点组",
    node_groups_and_nodes: "引用节点组和组内节点",
    manual: "手动选择节点",
  }[collection.source_type];
  return {
    name: previewStrategy.value?.name || collection.name,
    sourceMode,
    groupNames: (collection.referenced_groups || []).map((group) => group.name),
    selectedNodeCount: (collection.node_uids || []).filter(
      (uid) => uid !== "direct",
    ).length,
    selectionMode: collection.type === "urltest" ? "自动选择（URLTest）" : "手动切换（Selector）",
    testURL: collection.type === "urltest" ? connectivitySettings.value.test_url : "",
    testInterval:
      collection.type === "urltest"
        ? `${connectivitySettings.value.interval_seconds} 秒`
        : "",
    tolerance:
      collection.type === "urltest" ? `${collection.tolerance || 100} ms` : "",
  };
});

function show(
  text: string,
  type: "success" | "error" | "info" = "success",
) {
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
    const [groupItems, strategyItems, nodeFacets, connectivity] =
      await Promise.all([
        json("/api/v1/node-groups"),
        api.getRouteStrategies(),
        api.getNodeFacets(),
        api.getConnectivitySettings(),
      ]);
    groups.value = Array.isArray(groupItems) ? groupItems : [];
    strategies.value = Array.isArray(strategyItems) ? strategyItems : [];
    connectivitySettings.value = connectivity;
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

function configureStrategy(strategy: StrategyItem) {
  if (strategy.kind === "proxy") editingStrategy.value = strategy;
}

async function saveStrategy(payload: StrategyCollectionRequest) {
  if (!editingStrategy.value || strategySaving.value) return;
  strategySaving.value = true;
  try {
    const collection = editingStrategy.value.collection;
    if (collection) await api.updateProxyCollection(collection.id, payload);
    else await api.createProxyCollection(payload);
    show(collection ? "代理策略已更新" : "代理策略已配置");
    editingStrategy.value = null;
    await load();
  } catch (error) {
    show(
      `保存失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    strategySaving.value = false;
  }
}

async function removeStrategy(strategy: StrategyItem) {
  if (!strategy.collection) return;
  await api.deleteProxyCollection(strategy.collection.id);
  show("代理策略配置已删除");
  await load();
}

function requestRemoveStrategy(strategy: StrategyItem) {
  if (!strategy.collection) return;
  confirmDelete(`确定删除代理策略「${strategy.name}」的配置吗？`, () =>
    removeStrategy(strategy),
  );
}

useRealtimeSocket((event: WSEvent) => {
  if (event.type === "collection.test") {
    const result = event.data as CollectionTestResponse;
    tests.value[result.collection_id] = result;
  }
});

watch(groupPages, (pages) => {
  groupPage.value = Math.min(groupPage.value, pages);
});
onMounted(load);
</script>

<template>
  <div class="space-y-4">
    <PageHeader title="策略组管理" />
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <div class="flex gap-2 border-b border-[var(--border-default)]">
      <button
        class="px-4 py-2 text-[var(--text-secondary)]"
        :class="
          active === 'node-groups'
            ? 'border-b-2 border-[var(--color-primary)] text-[var(--text-primary)]'
            : ''
        "
        @click="active = 'node-groups'"
      >
        <Layers :size="16" class="inline" /> 节点组（地域划分）
      </button>
      <button
        class="px-4 py-2 text-[var(--text-secondary)]"
        :class="
          active === 'collections'
            ? 'border-b-2 border-[var(--color-primary)] text-[var(--text-primary)]'
            : ''
        "
        @click="active = 'collections'"
      >
        <Zap :size="16" class="inline" /> 策略组（业务用途）
      </button>
    </div>

    <div v-if="loading" class="py-20 text-center">加载中...</div>
    <NodeGroupSection
      v-else-if="active === 'node-groups'"
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
    <StrategySection
      v-else
      :strategies="strategies"
      :tests="tests"
      @configure="configureStrategy"
      @preview="previewStrategy = $event"
      @remove="requestRemoveStrategy"
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
    <StrategyFormModal
      v-if="editingStrategy"
      :key="editingStrategy.rule_id"
      :strategy="editingStrategy"
      :groups="groups"
      :connectivity="connectivitySettings"
      :saving="strategySaving"
      @close="editingStrategy = null"
      @save="saveStrategy"
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
    <Modal
      :open="!!previewStrategy"
      title="策略配置摘要"
      size="lg"
      @close="previewStrategy = null"
    >
      <div v-if="previewSummary" class="grid gap-4 text-sm">
        <div
          class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-secondary)] p-4"
        >
          <dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
            <div>
              <dt class="text-xs text-[var(--text-tertiary)]">策略名称</dt>
              <dd class="mt-1 font-medium text-[var(--text-primary)]">
                {{ previewSummary.name }}
              </dd>
            </div>
            <div>
              <dt class="text-xs text-[var(--text-tertiary)]">节点来源</dt>
              <dd class="mt-1">{{ previewSummary.sourceMode }}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-xs text-[var(--text-tertiary)]">已选节点组</dt>
              <dd class="mt-1">
                {{ previewSummary.groupNames.join("、") || "无" }}
              </dd>
            </div>
            <div>
              <dt class="text-xs text-[var(--text-tertiary)]">已选节点数</dt>
              <dd class="mt-1">{{ previewSummary.selectedNodeCount }}</dd>
            </div>
            <div>
              <dt class="text-xs text-[var(--text-tertiary)]">选择方式</dt>
              <dd class="mt-1">{{ previewSummary.selectionMode }}</dd>
            </div>
            <template v-if="previewSummary.testURL">
              <div class="sm:col-span-2">
                <dt class="text-xs text-[var(--text-tertiary)]">测速地址</dt>
                <dd class="mt-1 break-all">{{ previewSummary.testURL }}</dd>
              </div>
              <div>
                <dt class="text-xs text-[var(--text-tertiary)]">测速间隔</dt>
                <dd class="mt-1">{{ previewSummary.testInterval }}</dd>
              </div>
              <div>
                <dt class="text-xs text-[var(--text-tertiary)]">切换容差</dt>
                <dd class="mt-1">{{ previewSummary.tolerance }}</dd>
              </div>
            </template>
          </dl>
        </div>
        <p
          class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-3 text-xs text-[var(--text-secondary)]"
        >
          此处仅展示配置摘要。节点组展开、节点名称去重及最终出站标签由服务端生成。
        </p>
      </div>
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
