<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Edit, Eye, Layers, Plus, Trash2, Zap } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import Modal from "@/components/ui/Modal.vue";
import Pagination from "@/components/ui/Pagination.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import OrderButtons from "@/components/ui/OrderButtons.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import NodeGroupDetailModal from "./collections/NodeGroupDetailModal.vue";
import { subscriptionFilterLabel } from "./collections/nodeGroupLabels";
import {
  findDNSOutboundBinding,
  saveDNSOutboundBinding,
  type DNSBindingRule,
  type DNSBindingServer,
} from "./collections/dnsBinding";
import { api } from "@/services/api";
import { authenticatedFetch } from "@/services/apiAuth";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import type {
  CollectionTestResponse,
  NodeItem,
  WSEvent,
} from "@/services/types";
import { defaultFlag, getFlagImageURL } from "@/utils/nodeFlags";
interface NG {
  id: number;
  name: string;
  type: string;
  filter_protocols: string;
  filter_subscriptions: string;
  filter_include: string;
  filter_exclude: string;
  node_uids: string;
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  priority: number;
  matched_node_count: number;
}
interface Facet {
  value: string;
  label: string;
  count: number;
}
interface Matched {
  uid: string;
  name: string;
  type: string;
  subscription_id: number;
  subscription_name: string;
  latency_ms: number;
  status: string;
}
interface Col {
  id: number;
  name: string;
  type: string;
  source_type: string;
  referenced_group_ids: string;
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  priority: number;
  referenced_groups: NG[];
  route_rule_ids: number[];
  node_uids: string[];
}
interface Rule {
  id: number;
  name: string;
  outbound: string;
  enabled: boolean;
}
const active = ref<"node-groups" | "collections">("node-groups"),
  groups = ref<NG[]>([]),
  collections = ref<Col[]>([]),
  rules = ref<Rule[]>([]),
  dnsServers = ref<DNSBindingServer[]>([]),
  dnsRules = ref<DNSBindingRule[]>([]),
  facets = ref<{ protocols: Facet[]; subscriptions: Facet[]; total: number }>({
    protocols: [],
    subscriptions: [],
    total: 0,
  }),
  flags = ref<Record<string, string>>({}),
  nodeFlags = ref<Record<string, string>>({}),
  loading = ref(true),
  message = ref(""),
  messageType = ref<"success" | "error" | "info">("success");
const selected = ref<number[]>([]),
  groupPage = ref(1),
  groupPageSize = ref(25),
  collectionPage = ref(1),
  collectionPageSize = ref(25),
  editingGroup = ref<NG | null>(null),
  detailGroup = ref<NG | null>(null),
  detailNodes = ref<Matched[]>([]),
  detailLoading = ref(false),
  pickerOpen = ref(false),
  pickerTarget = ref<"group" | "collection">("group"),
  manualNodes = ref<NodeItem[]>([]),
  manualKeyword = ref(""),
  quickOpen = ref(false),
  quickRunning = ref(false),
  quickProtocols = ref<string[]>([]),
  quickSubscriptions = ref<string[]>([]);
const ngName = ref(""),
  ngType = ref("selector"),
  ngProtocols = ref<string[]>([]),
  ngSubscriptions = ref<string[]>([]),
  ngInclude = ref(""),
  ngExclude = ref(""),
  ngUIDs = ref<string[]>([]),
  ngEnabled = ref(true),
  ngTolerance = ref(100);
const editingCol = ref<Col | null>(null),
  colName = ref(""),
  colType = ref("selector"),
  colSource = ref<"node_groups" | "node_groups_and_nodes" | "manual">(
    "node_groups",
  ),
  colGroups = ref<number[]>([]),
  colUIDs = ref<string[]>([]),
  colRules = ref<number[]>([]),
  colDNSServer = ref(""),
  colEnabled = ref(true),
  colTolerance = ref(100),
  colGroupSearch = ref(""),
  connectivitySettings = ref({
    test_url: "",
    interval_seconds: 300,
  }),
  previewCol = ref<Col | null>(null),
  tests = ref<Record<number, CollectionTestResponse>>({}),
  deleteAction = ref<null | (() => Promise<void>)>(null),
  deleteMessage = ref("");
const show = (s: string, t: "success" | "error" | "info" = "success") => {
    message.value = s;
    messageType.value = t;
  },
  groupPages = computed(() =>
    Math.max(1, Math.ceil(groups.value.length / groupPageSize.value)),
  ),
  pagedGroups = computed(() =>
    groups.value.slice(
      (groupPage.value - 1) * groupPageSize.value,
      groupPage.value * groupPageSize.value,
    ),
  ),
  sortedCols = computed(() => collections.value),
  colPages = computed(() =>
    Math.max(1, Math.ceil(sortedCols.value.length / collectionPageSize.value)),
  ),
  pagedCols = computed(() =>
    sortedCols.value.slice(
      (collectionPage.value - 1) * collectionPageSize.value,
      collectionPage.value * collectionPageSize.value,
    ),
  );
const parseUIDs = (s: string) => {
    try {
      const x = JSON.parse(s || "[]");
      return Array.isArray(x) ? x : [];
    } catch {
      return [];
    }
  },
  toggle = <T extends string | number>(a: T[], v: T): T[] =>
    a.includes(v) ? a.filter((x) => x !== v) : [...a, v],
  builtin = (c: Col | null) =>
    c && c.name === "全球直连"
      ? ["direct"]
      : (c?.node_uids || []).filter((x) => x === "direct"),
  system = (c: Col) => c.name === "全球直连";
async function json(url: string, init?: RequestInit) {
  const r = await authenticatedFetch(url, init);
  if (!r.ok)
    throw new Error(
      (await r.json().catch(() => null))?.error?.message || r.statusText,
    );
  return r.json().catch(() => null);
}
async function load() {
  try {
    const [g, c, f, r, ds, dr, connectivity] = await Promise.all([
      json("/api/v1/node-groups"),
      json("/api/v1/collections"),
      api.getNodeFacets(),
      json("/api/v1/rules"),
      json("/api/v1/dns/servers"),
      json("/api/v1/dns/rules"),
      api.getConnectivitySettings(),
    ]);
    groups.value = Array.isArray(g) ? g : [];
    collections.value = Array.isArray(c) ? c : [];
    rules.value = Array.isArray(r) ? r : [];
    dnsServers.value = Array.isArray(ds) ? ds : [];
    dnsRules.value = Array.isArray(dr) ? dr : [];
    connectivitySettings.value = connectivity;
    facets.value = {
      protocols: f.types,
      subscriptions: f.subscriptions,
      total: f.total,
    };
    flags.value = groups.value.length
      ? Object.fromEntries(
          (
            await api.inferNodeFlags(
              groups.value.map((x) => ({
                key: String(x.id),
                name: x.name,
                server: "",
              })),
            )
          ).items.map((x) => [x.key, x.flag]),
        )
      : {};
  } catch (e: any) {
    show(`加载失败: ${e.message}`, "error");
  } finally {
    loading.value = false;
  }
}
function resetGroup() {
  editingGroup.value = null;
  ngName.value = "";
  ngType.value = "selector";
  ngProtocols.value = [];
  ngSubscriptions.value = [];
  ngInclude.value = "";
  ngExclude.value = "";
  ngUIDs.value = [];
  ngEnabled.value = true;
  ngTolerance.value = 100;
}
function createGroup() {
  resetGroup();
  editingGroup.value = {} as NG;
}
function editGroup(x: NG) {
  editingGroup.value = x;
  ngName.value = x.name;
  ngType.value = x.type;
  ngProtocols.value = x.filter_protocols?.split(",").filter(Boolean) || [];
  ngSubscriptions.value =
    x.filter_subscriptions?.split(",").filter(Boolean) || [];
  ngInclude.value = x.filter_include;
  ngExclude.value = x.filter_exclude;
  ngUIDs.value = parseUIDs(x.node_uids);
  ngEnabled.value = x.enabled;
  ngTolerance.value = x.tolerance || 100;
}
async function saveGroup() {
  try {
    const p = {
        name: ngName.value,
        type: ngType.value,
        filter_protocols: ngProtocols.value.join(","),
        filter_subscriptions: ngSubscriptions.value.join(","),
        filter_include: ngInclude.value,
        filter_exclude: ngExclude.value,
        node_uids: ngUIDs.value,
        enabled: ngEnabled.value,
        priority: editingGroup.value?.priority || 0,
        tolerance: ngTolerance.value,
      },
      id = editingGroup.value?.id;
    await json(id ? `/api/v1/node-groups/${id}` : "/api/v1/node-groups", {
      method: id ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(p),
    });
    show(id ? "节点组已更新" : "节点组已创建");
    resetGroup();
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
  }
}
function confirmDelete(msg: string, fn: () => Promise<void>) {
  deleteMessage.value = msg;
  deleteAction.value = fn;
}
async function removeGroup(x: NG) {
  await json(`/api/v1/node-groups/${x.id}`, { method: "DELETE" });
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
async function detail(x: NG) {
  detailGroup.value = x;
  detailLoading.value = true;
  try {
    const p = new URLSearchParams({
      filter_protocols: x.filter_protocols || "",
      filter_subscriptions: x.filter_subscriptions || "",
      filter_include: x.filter_include || "",
      filter_exclude: x.filter_exclude || "",
    });
    detailNodes.value = await json(`/api/v1/node-groups/preview?${p}`);
    const inferred = await api.inferNodeFlags(
      detailNodes.value.map((node) => ({ key: node.uid, name: node.name })),
    );
    nodeFlags.value = {
      ...nodeFlags.value,
      ...Object.fromEntries(
        inferred.items.map((item) => [item.key, item.flag]),
      ),
    };
  } catch (e: any) {
    show(`加载节点组详情失败: ${e.message}`, "error");
  } finally {
    detailLoading.value = false;
  }
}
async function picker(target?: "group" | "collection") {
  if (target) pickerTarget.value = target;
  pickerOpen.value = true;
  try {
    manualNodes.value = (
      await api.getNodes({
        enabled: true,
        limit: 1000,
        keyword: manualKeyword.value,
      })
    ).items;
    const inferred = await api.inferNodeFlags(
      manualNodes.value.map((node) => ({
        key: node.uid,
        name: node.name,
        server: node.server,
      })),
    );
    nodeFlags.value = {
      ...nodeFlags.value,
      ...Object.fromEntries(
        inferred.items.map((item) => [item.key, item.flag]),
      ),
    };
  } catch (e: any) {
    show(`加载节点失败: ${e.message}`, "error");
  }
}
function togglePicked(uid: string) {
  if (pickerTarget.value === "group") ngUIDs.value = toggle(ngUIDs.value, uid);
  else colUIDs.value = toggle(colUIDs.value, uid);
}
async function quick() {
  quickRunning.value = true;
  try {
    await json("/api/v1/node-groups/quick-setup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        filter_subscriptions: quickSubscriptions.value.join(","),
        filter_protocols: quickProtocols.value.join(","),
      }),
    });
    show("节点组已创建");
    quickOpen.value = false;
    await load();
  } catch (e: any) {
    show(`快速配置失败: ${e.message}`, "error");
  } finally {
    quickRunning.value = false;
  }
}
function resetCol() {
  editingCol.value = null;
  colName.value = "";
  colType.value = "selector";
  colSource.value = "node_groups";
  colGroups.value = [];
  colUIDs.value = [];
  colRules.value = [];
  colDNSServer.value = "";
  colEnabled.value = true;
  colTolerance.value = 100;
  colGroupSearch.value = "";
}
function createCol() {
  resetCol();
  editingCol.value = {} as Col;
}
function editCol(x: Col) {
  if (system(x)) return;
  editingCol.value = x;
  colName.value = x.name;
  colType.value = x.type;
  colSource.value =
    x.source_type === "manual"
      ? "manual"
      : x.source_type === "node_groups_and_nodes"
        ? "node_groups_and_nodes"
        : "node_groups";
  colGroups.value = x.referenced_groups?.map((g) => g.id) || [];
  colUIDs.value = x.node_uids || [];
  colRules.value = x.route_rule_ids || [];
  const dnsBinding = findDNSOutboundBinding(dnsRules.value, x.name);
  colDNSServer.value = dnsBinding?.enabled ? dnsBinding.server : "";
  colEnabled.value = x.enabled;
  colTolerance.value = x.tolerance || 100;
}
const occupied = computed(
    () =>
      new Set(
        collections.value
          .filter((x) => x.id !== editingCol.value?.id)
          .flatMap((x) => x.route_rule_ids || []),
      ),
  ),
  selectableRules = computed(() =>
    rules.value.filter(
      (x) => x.outbound === "proxy" && !occupied.value.has(x.id),
    ),
  ),
  selectableColGroups = computed(() => {
    const keyword = colGroupSearch.value.trim().toLowerCase();
    return groups.value.filter(
      (group) =>
        group.enabled &&
        ((group.matched_node_count || 0) > 0 ||
          colGroups.value.includes(group.id)) &&
        (!keyword || group.name.toLowerCase().includes(keyword)),
    );
  }),
  selectableColGroupIDs = computed(() =>
    selectableColGroups.value.map((group) => group.id),
  );
function selectRule(id: number) {
  const previousName = rules.value.find(
    (rule) => rule.id === colRules.value[0],
  )?.name;
  const shouldFillName =
    !colName.value.trim() || colName.value === previousName;
  colRules.value = id ? [id] : [];
  if (shouldFillName) {
    colName.value = rules.value.find((x) => x.id === id)?.name || "";
  }
}
async function saveCol() {
  try {
    const id = editingCol.value?.id,
      previousName = editingCol.value?.name || "",
      previousDNSBinding = previousName
        ? findDNSOutboundBinding(dnsRules.value, previousName)
        : undefined,
      previousDNS = previousDNSBinding?.enabled
        ? previousDNSBinding.server
        : "",
      p = {
        name: colName.value,
        type: colType.value,
        source_type: colSource.value,
        referenced_group_ids:
          colSource.value !== "manual" ? colGroups.value : [],
        route_rule_ids: colRules.value,
        node_uids:
          colSource.value === "manual"
            ? colUIDs.value
            : builtin(editingCol.value),
        enabled: colEnabled.value,
        tolerance: colTolerance.value,
      };
    await json(id ? `/api/v1/collections/${id}` : "/api/v1/collections", {
      method: id ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(p),
    });
    if (previousName !== colName.value || previousDNS !== colDNSServer.value) {
      try {
        await saveDNSOutboundBinding(
          json,
          dnsRules.value,
          previousName,
          colName.value,
          colDNSServer.value,
        );
      } catch (e: any) {
        await load();
        show(`策略组已保存，但 DNS 出口绑定失败: ${e.message}`, "error");
        return;
      }
    }
    show(id ? "策略组已更新" : "策略组已创建");
    resetCol();
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
  }
}
async function removeCol(x: Col) {
  await json(`/api/v1/collections/${x.id}`, { method: "DELETE" });
  const binding = findDNSOutboundBinding(dnsRules.value, x.name);
  if (binding) {
    try {
      await saveDNSOutboundBinding(json, dnsRules.value, x.name, x.name, "");
    } catch (e: any) {
      await load();
      show(`策略组已删除，但 DNS 出口绑定清理失败: ${e.message}`, "error");
      return;
    }
  }
  show("策略组已删除");
  await load();
}
async function moveCollection(index: number, direction: -1 | 1) {
  if (collectionMoveDisabled(index, direction)) return;
  const currentIndex =
    (collectionPage.value - 1) * collectionPageSize.value + index;
  const nextIndex = currentIndex + direction;
  if (nextIndex < 0 || nextIndex >= collections.value.length) return;
  const next = [...collections.value];
  [next[currentIndex], next[nextIndex]] = [next[nextIndex], next[currentIndex]];
  collections.value = next;
  try {
    await json("/api/v1/collections/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next.map((item) => item.id)),
    });
    show("策略组顺序已保存");
  } catch (e: any) {
    show(`策略组排序失败: ${e.message}`, "error");
    await load();
  }
}
function collectionMoveDisabled(index: number, direction: -1 | 1) {
  const currentIndex =
    (collectionPage.value - 1) * collectionPageSize.value + index;
  const nextIndex = currentIndex + direction;
  const current = collections.value[currentIndex];
  const next = collections.value[nextIndex];
  return !current || !next || system(current) || system(next);
}
const preview = computed(() =>
  previewCol.value
    ? {
        tag: previewCol.value.name,
        type: previewCol.value.type,
        outbounds: [
          ...builtin(previewCol.value),
          ...(previewCol.value.referenced_groups || []).map((g) => g.name),
        ],
        ...(previewCol.value.type === "urltest"
          ? {
              url: connectivitySettings.value.test_url,
              interval: `${connectivitySettings.value.interval_seconds}s`,
              tolerance: previewCol.value.tolerance || 100,
            }
          : {}),
      }
    : null,
);
useRealtimeSocket((e: WSEvent) => {
  if (e.type === "collection.test") {
    const r = e.data as CollectionTestResponse;
    tests.value[r.collection_id] = r;
  }
});
watch([groupPages, colPages], ([g, c]) => {
  groupPage.value = Math.min(groupPage.value, g);
  collectionPage.value = Math.min(collectionPage.value, c);
});
onMounted(load);
</script>
<template>
  <div class="space-y-4">
    <PageHeader title="策略组管理" />
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <div class="flex gap-2 border-b border-[var(--border-default)]">
      <button
        :class="[
          'px-4 py-2',
          active === 'node-groups' && 'border-b-2 border-blue-500',
        ]"
        @click="active = 'node-groups'"
      >
        <Layers :size="16" class="inline" /> 节点组（地域划分）</button
      ><button
        :class="[
          'px-4 py-2',
          active === 'collections' && 'border-b-2 border-blue-500',
        ]"
        @click="active = 'collections'"
      >
        <Zap :size="16" class="inline" /> 策略组（业务用途）
      </button>
    </div>
    <div v-if="loading" class="py-20 text-center">加载中...</div>
    <section v-else-if="active === 'node-groups'" class="space-y-4">
      <div class="flex flex-wrap justify-between gap-2">
        <p class="text-sm text-[var(--text-secondary)]">
          使用关键词自动筛选节点，作为策略组的基础单元
        </p>
        <div class="flex gap-2">
          <button
            v-if="selected.length"
            class="aw-action-button"
            @click="
              confirmDelete(
                `确定删除选中的 ${selected.length} 个节点组吗？`,
                batchDelete,
              )
            "
          >
            <Trash2 :size="14" />批量删除</button
          ><button class="aw-action-button" @click="quickOpen = true">
            <Zap :size="14" />智能快速配置</button
          ><button class="aw-action-button" @click="createGroup">
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
                    pagedGroups.every((x) => selected.includes(x.id))
                  "
                  @change="
                    selected = pagedGroups.every((x) => selected.includes(x.id))
                      ? selected.filter(
                          (id) => !pagedGroups.some((x) => x.id === id),
                        )
                      : [
                          ...new Set([
                            ...selected,
                            ...pagedGroups.map((x) => x.id),
                          ]),
                        ]
                  "
                />
              </th>
              <th
                v-for="c in [
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
                :key="c"
              >
                {{ c }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="x in pagedGroups" :key="x.id">
              <td>
                <input
                  type="checkbox"
                  :checked="selected.includes(x.id)"
                  @change="selected = toggle(selected, x.id)"
                />
              </td>
              <td>
                <img
                  :src="getFlagImageURL(flags[String(x.id)] || defaultFlag)"
                  class="mr-2 inline h-4 w-4"
                />{{ x.name }}
              </td>
              <td>{{ x.type === "urltest" ? "自动" : "手动" }}</td>
              <td>{{ x.filter_protocols || "全部" }}</td>
              <td>
                {{
                  subscriptionFilterLabel(
                    x.filter_subscriptions,
                    facets.subscriptions,
                  )
                }}
              </td>
              <td>{{ x.filter_include }}</td>
              <td>{{ x.filter_exclude || "-" }}</td>
              <td>{{ x.matched_node_count || 0 }} 个</td>
              <td>{{ x.enabled ? "启用" : "停用" }}</td>
              <td>
                <button @click="detail(x)">
                  <Eye :size="12" class="inline" />详情
                </button>
                <button @click="editGroup(x)">
                  <Edit :size="12" class="inline" />编辑
                </button>
                <button
                  @click="
                    confirmDelete(`确定删除节点组「${x.name}」吗？`, () =>
                      removeGroup(x),
                    )
                  "
                >
                  <Trash2 :size="12" class="inline" />删除
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
        :total="groups.length"
        :page="groupPage"
        :page-size="groupPageSize"
        :total-pages="groupPages"
        @page-change="groupPage = $event"
        @page-size-change="groupPageSize = $event"
      />
    </section>
    <section v-else class="space-y-4">
      <div class="flex justify-between">
        <p class="text-sm text-[var(--text-secondary)]">
          引用节点组，组合成业务用途的策略组
        </p>
        <button class="aw-action-button" @click="createCol">
          <Plus :size="14" />新增策略组
        </button>
      </div>
      <div class="overflow-x-auto border border-[var(--border-default)]">
        <table class="aw-data-table min-w-[800px]">
          <thead>
            <tr>
              <th
                v-for="c in [
                  '排序',
                  '名称',
                  '绑定规则',
                  '类型',
                  '节点来源',
                  '状态',
                  '操作',
                ]"
                :key="c"
              >
                {{ c }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(x, i) in pagedCols" :key="x.id">
              <td>
                <OrderButtons
                  :up-disabled="collectionMoveDisabled(i, -1)"
                  :down-disabled="collectionMoveDisabled(i, 1)"
                  @up="moveCollection(i, -1)"
                  @down="moveCollection(i, 1)"
                />
              </td>
              <td>{{ x.name }} <small v-if="system(x)">系统默认</small></td>
              <td>
                {{
                  x.route_rule_ids
                    ?.map(
                      (id) => rules.find((r) => r.id === id)?.name || `#${id}`,
                    )
                    .join("、") || "-"
                }}
              </td>
              <td>{{ x.type === "urltest" ? "自动" : "手动" }}</td>
              <td>
                {{
                  [
                    ...builtin(x),
                    ...(x.referenced_groups || []).map((g) => g.name),
                  ].join("、") || "-"
                }}
                <small v-if="x.source_type === 'node_groups_and_nodes'">
                  · 含组内节点
                </small>
              </td>
              <td>
                {{ x.enabled ? "启用" : "停用"
                }}<small v-if="tests[x.id]">
                  · {{ tests[x.id].available }}/{{
                    tests[x.id].tested
                  }}
                  可用</small
                >
              </td>
              <td>
                <button @click="previewCol = x">
                  <Eye :size="12" class="inline" />预览
                </button>
                <template v-if="!system(x)"
                  ><button @click="editCol(x)">
                    <Edit :size="12" class="inline" />编辑
                  </button>
                  <button
                    @click="
                      confirmDelete(`确定删除策略组「${x.name}」吗？`, () =>
                        removeCol(x),
                      )
                    "
                  >
                    <Trash2 :size="12" class="inline" />删除
                  </button></template
                >
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
        :total="collections.length"
        :page="collectionPage"
        :page-size="collectionPageSize"
        :total-pages="colPages"
        @page-change="collectionPage = $event"
        @page-size-change="collectionPageSize = $event"
      />
    </section>
    <NodeGroupDetailModal
      v-if="detailGroup"
      :group="detailGroup"
      :nodes="detailNodes"
      :flags="nodeFlags"
      :loading="detailLoading"
      :subscriptions="facets.subscriptions"
      @close="detailGroup = null"
    />
    <Modal
      :open="!!editingGroup"
      :title="editingGroup?.id ? '编辑节点组' : '新增节点组'"
      size="lg"
      @close="resetGroup"
    >
      <div class="grid gap-4 md:grid-cols-2">
        <label class="grid content-start gap-1.5 md:col-span-2">
          <span class="aw-modal-label text-xs font-medium">节点组名称</span>
          <input
            v-model.trim="ngName"
            class="aw-input"
            placeholder="例如：香港节点、流媒体节点"
          />
        </label>
        <label class="grid content-start gap-1.5">
          <span class="aw-modal-label text-xs font-medium">节点组类型</span>
          <select v-model="ngType" class="aw-input">
            <option value="selector">手动切换</option>
            <option value="urltest">自动选择（测速）</option>
          </select>
        </label>
        <label
          class="flex h-[34px] cursor-pointer items-center gap-2 self-end rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs text-[var(--text-secondary)]"
        >
          <input v-model="ngEnabled" type="checkbox" class="h-4 w-4" />
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
          <button class="aw-action-button" @click="picker('group')">
            手动选择节点（已选择 {{ ngUIDs.length }} 个）
          </button>
        </div>
        <template v-if="ngType === 'urltest'">
          <div
            class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-3 text-xs text-[var(--text-secondary)] md:col-span-2"
          >
            测速地址与间隔统一使用“设置 → 常规功能 → 连通性测速”。
          </div>
          <label class="grid content-start gap-1.5 md:col-span-2">
            <span class="aw-modal-label text-xs font-medium"
              >切换容差（ms）</span
            >
            <input
              v-model.number="ngTolerance"
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
              v-if="ngProtocols.length"
              type="button"
              class="aw-action-button"
              @click="ngProtocols = []"
            >
              清空
            </button>
          </div>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <label
              v-for="x in facets.protocols"
              :key="x.value"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border px-3 py-2 text-xs transition-colors"
              :class="
                ngProtocols.includes(x.value)
                  ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                  : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]'
              "
            >
              <input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="ngProtocols.includes(x.value)"
                @change="ngProtocols = toggle(ngProtocols, x.value)"
              />
              <span class="min-w-0 flex-1 truncate uppercase">{{
                x.label
              }}</span>
              <span class="shrink-0 tabular-nums opacity-70">{{
                x.count
              }}</span>
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
              v-if="ngSubscriptions.length"
              type="button"
              class="aw-action-button"
              @click="ngSubscriptions = []"
            >
              清空
            </button>
          </div>
          <div
            class="grid max-h-48 gap-2 overflow-y-auto rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-2 sm:grid-cols-2"
          >
            <label
              v-for="x in facets.subscriptions"
              :key="x.value"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border px-3 py-2 text-xs transition-colors"
              :class="
                ngSubscriptions.includes(x.value)
                  ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                  : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]'
              "
            >
              <input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="ngSubscriptions.includes(x.value)"
                @change="ngSubscriptions = toggle(ngSubscriptions, x.value)"
              />
              <span class="min-w-0 flex-1 truncate">{{ x.label }}</span>
              <span class="shrink-0 tabular-nums opacity-70">{{
                x.count
              }}</span>
            </label>
            <p
              v-if="!facets.subscriptions.length"
              class="py-6 text-center text-xs text-[var(--text-tertiary)] sm:col-span-2"
            >
              暂无可用订阅
            </p>
          </div>
        </fieldset>
        <label class="grid content-start gap-1.5 md:col-span-2">
          <span class="aw-modal-label text-xs font-medium">包含关键词</span>
          <input
            v-model.trim="ngInclude"
            class="aw-input"
            placeholder="正则表达式，例如：香港|HK"
          />
        </label>
        <label class="grid content-start gap-1.5 md:col-span-2">
          <span class="aw-modal-label text-xs font-medium">排除关键词</span>
          <input
            v-model.trim="ngExclude"
            class="aw-input"
            placeholder="正则表达式，例如：过期|流量"
          />
        </label>
      </div>
      <template #footer
        ><button class="aw-action-button" @click="resetGroup">取消</button
        ><button
          class="aw-action-button"
          :disabled="!ngName.trim()"
          @click="saveGroup"
        >
          保存
        </button></template
      >
    </Modal>
    <Modal
      :open="pickerOpen"
      title="手动选择节点"
      size="xl"
      @close="pickerOpen = false"
    >
      <div class="mb-3 flex gap-2">
        <input
          v-model="manualKeyword"
          class="aw-input flex-1"
          placeholder="搜索节点名称"
          @keyup.enter="picker()"
        /><button class="aw-action-button" @click="picker()">搜索</button>
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
            <tr v-for="x in manualNodes" :key="x.uid">
              <td>
                <input
                  type="checkbox"
                  :checked="
                    pickerTarget === 'group'
                      ? ngUIDs.includes(x.uid)
                      : colUIDs.includes(x.uid)
                  "
                  @change="togglePicked(x.uid)"
                />
              </td>
              <td><NodeFlagName :name="x.name" :flag="nodeFlags[x.uid]" /></td>
              <td>{{ x.type }}</td>
              <td>{{ x.subscription_name }}</td>
              <td>{{ x.latency_ms || "-" }}</td>
              <td>{{ x.status }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </Modal>
    <Modal :open="quickOpen" title="智能快速配置" @close="quickOpen = false">
      <div class="grid gap-5">
        <fieldset class="grid gap-2">
          <legend
            class="mb-1 text-xs font-semibold text-[var(--text-secondary)]"
          >
            按订阅筛选
          </legend>
          <div class="grid gap-2 sm:grid-cols-2">
            <label
              v-for="x in facets.subscriptions"
              :key="x.value"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--button-secondary-bg)] px-3 py-2 text-xs text-[var(--text-secondary)] transition-colors hover:border-[var(--button-primary-border)] hover:bg-[var(--button-secondary-hover)]"
              ><input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="quickSubscriptions.includes(x.value)"
                @change="
                  quickSubscriptions = toggle(quickSubscriptions, x.value)
                "
              />
              <span class="min-w-0 truncate">{{ x.label }}</span>
            </label>
          </div>
        </fieldset>
        <fieldset class="grid gap-2">
          <legend
            class="mb-1 text-xs font-semibold text-[var(--text-secondary)]"
          >
            按协议筛选
          </legend>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <label
              v-for="x in facets.protocols"
              :key="x.value"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--button-secondary-bg)] px-3 py-2 text-xs text-[var(--text-secondary)] transition-colors hover:border-[var(--button-primary-border)] hover:bg-[var(--button-secondary-hover)]"
              ><input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="quickProtocols.includes(x.value)"
                @change="quickProtocols = toggle(quickProtocols, x.value)"
              />
              <span class="min-w-0 truncate">{{ x.label }}</span>
            </label>
          </div>
        </fieldset>
      </div>
      <template #footer
        ><button
          class="aw-action-button"
          :disabled="quickRunning"
          @click="quick"
        >
          {{ quickRunning ? "配置中..." : "开始配置" }}
        </button></template
      >
    </Modal>
    <Modal
      :open="!!editingCol"
      :title="editingCol?.id ? '编辑策略组' : '新增策略组'"
      size="lg"
      @close="resetCol"
    >
      <div class="grid gap-4 md:grid-cols-2">
        <label class="grid content-start gap-1.5 md:col-span-2">
          <span class="aw-modal-label text-xs font-medium">策略组名称</span>
          <input
            v-model.trim="colName"
            class="aw-input"
            placeholder="例如：AI 服务、国外网站"
          />
        </label>
        <label class="grid content-start gap-1.5">
          <span class="aw-modal-label text-xs font-medium">关联代理规则</span>
          <select
            class="aw-input"
            :value="colRules[0] || 0"
            @change="
              selectRule(Number(($event.target as HTMLSelectElement).value))
            "
          >
            <option :value="0">请选择出站为“代理”的规则</option>
            <option v-for="x in selectableRules" :key="x.id" :value="x.id">
              {{ x.name }}
            </option>
          </select>
          <small class="text-[11px] text-[var(--text-tertiary)]">
            该规则命中的流量将交给此策略组处理
          </small>
        </label>
        <label class="grid content-start gap-1.5">
          <span class="aw-modal-label text-xs font-medium">切换方式</span>
          <select v-model="colType" class="aw-input">
            <option value="selector">手动切换</option>
            <option value="urltest">自动选择（测速）</option>
          </select>
        </label>
        <label class="grid content-start gap-1.5">
          <span class="aw-modal-label text-xs font-medium">节点来源</span>
          <select v-model="colSource" class="aw-input">
            <option value="node_groups">引用节点组</option>
            <option value="node_groups_and_nodes">引用节点组和节点</option>
            <option value="manual">手动选择节点</option>
          </select>
        </label>
        <label
          class="flex h-[34px] cursor-pointer items-center gap-2 self-end rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs text-[var(--text-secondary)]"
        >
          <input v-model="colEnabled" type="checkbox" class="h-4 w-4" />
          启用此策略组
        </label>
        <label class="grid content-start gap-1.5 md:col-span-2">
          <span class="aw-modal-label text-xs font-medium">DNS 出口绑定</span>
          <select v-model="colDNSServer" class="aw-input">
            <option value="">不绑定（使用默认 DNS）</option>
            <option
              v-for="server in dnsServers.filter(
                (item) => item.enabled && item.server_type !== 'fakeip',
              )"
              :key="server.tag"
              :value="server.tag"
            >
              {{ server.tag }}{{ server.detour ? ` · ${server.detour}` : "" }}
            </option>
          </select>
          <small class="text-[11px] text-[var(--text-tertiary)]">
            用于该策略组节点出站的域名解析；DNS 本身走代理时会自动使用直连
            bootstrap，避免循环依赖
          </small>
        </label>
        <template v-if="colType === 'urltest'">
          <div
            class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-3 text-xs text-[var(--text-secondary)] md:col-span-2"
          >
            测速地址与间隔统一使用“设置 → 常规功能 → 连通性测速”。
          </div>
          <label class="grid content-start gap-1.5 md:col-span-2">
            <span class="aw-modal-label text-xs font-medium"
              >切换容差（ms）</span
            >
            <input
              v-model.number="colTolerance"
              type="number"
              min="0"
              class="aw-input"
            />
          </label>
        </template>
        <fieldset
          v-if="colSource !== 'manual'"
          class="grid gap-3 md:col-span-2"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <legend class="text-sm font-semibold">选择引用的节点组</legend>
              <p class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">
                <template v-if="colSource === 'node_groups_and_nodes'">
                  策略组将同时包含节点组入口和组内全部匹配节点；
                </template>
                <template v-else> 策略组仅包含节点组入口； </template>
                仅显示已有匹配节点的启用组，编辑时保留已引用项；已选择
                {{ colGroups.length }} 个
              </p>
            </div>
            <div class="flex gap-2">
              <button
                type="button"
                class="aw-action-button"
                @click="colGroups = selectableColGroupIDs"
              >
                全选当前
              </button>
              <button
                type="button"
                class="aw-action-button"
                :disabled="!colGroups.length"
                @click="colGroups = []"
              >
                清空
              </button>
            </div>
          </div>
          <input
            v-model.trim="colGroupSearch"
            class="aw-input"
            placeholder="搜索节点组"
          />
          <div
            class="grid max-h-64 gap-2 overflow-y-auto rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] p-2 sm:grid-cols-2"
          >
            <label
              v-for="x in selectableColGroups"
              :key="x.id"
              class="flex min-w-0 cursor-pointer items-center gap-2 rounded-[var(--radius-md)] border px-3 py-2.5 text-xs transition-colors"
              :class="
                colGroups.includes(x.id)
                  ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
                  : 'border-[var(--border-light)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:border-[var(--border-default)] hover:bg-[var(--button-secondary-hover)]'
              "
            >
              <input
                type="checkbox"
                class="h-4 w-4 shrink-0"
                :checked="colGroups.includes(x.id)"
                @change="colGroups = toggle(colGroups, x.id)"
              />
              <span class="min-w-0 flex-1 truncate">{{ x.name }}</span>
              <span
                class="shrink-0 rounded-full bg-[var(--button-secondary-bg)] px-2 py-0.5 tabular-nums"
              >
                {{ x.matched_node_count || 0 }}
              </span>
            </label>
            <p
              v-if="!selectableColGroups.length"
              class="py-8 text-center text-xs text-[var(--text-tertiary)] sm:col-span-2"
            >
              没有匹配的可用节点组
            </p>
          </div>
        </fieldset>
        <div v-else class="md:col-span-2">
          <button class="aw-action-button" @click="picker('collection')">
            手动选择节点（已选择 {{ colUIDs.length }} 个）
          </button>
        </div>
      </div>
      <template #footer
        ><button class="aw-action-button" @click="resetCol">取消</button
        ><button
          class="aw-action-button"
          :disabled="
            !colName.trim() ||
            !colRules.length ||
            (colSource !== 'manual' ? !colGroups.length : !colUIDs.length)
          "
          @click="saveCol"
        >
          保存
        </button></template
      >
    </Modal>
    <Modal
      :open="!!previewCol"
      title="策略组预览"
      size="lg"
      @close="previewCol = null"
    >
      <pre
        class="max-h-[520px] overflow-auto rounded bg-[var(--bg-base)] p-4 text-xs"
        >{{ JSON.stringify(preview, null, 2) }}
    </pre>
    </Modal>
    <ConfirmDialog
      :open="!!deleteAction"
      title="确认删除"
      :message="deleteMessage"
      confirm-text="删除"
      @cancel="deleteAction = null"
      @confirm="
        async () => {
          const fn = deleteAction;
          deleteAction = null;
          if (fn)
            try {
              await fn();
            } catch (e: any) {
              show(`删除失败: ${e.message}`, 'error');
            }
        }
      "
    />
  </div>
</template>
