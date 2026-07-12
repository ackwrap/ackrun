<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Edit, Eye, Layers, Plus, Trash2, Zap } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import Modal from "@/components/ui/Modal.vue";
import Pagination from "@/components/ui/Pagination.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import NodeGroupDetailModal from "./collections/NodeGroupDetailModal.vue";
import { api } from "@/services/api";
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
  facets = ref<{ protocols: Facet[]; subscriptions: Facet[]; total: number }>({
    protocols: [],
    subscriptions: [],
    total: 0,
  }),
  flags = ref<Record<string, string>>({}),
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
  ngURL = ref("https://www.gstatic.com/generate_204"),
  ngInterval = ref(300),
  ngTolerance = ref(100);
const editingCol = ref<Col | null>(null),
  colName = ref(""),
  colType = ref("selector"),
  colSource = ref<"node_groups" | "manual">("node_groups"),
  colGroups = ref<number[]>([]),
  colUIDs = ref<string[]>([]),
  colRules = ref<number[]>([]),
  colEnabled = ref(true),
  colURL = ref("https://www.gstatic.com/generate_204"),
  colInterval = ref(300),
  colTolerance = ref(100),
  previewCol = ref<Col | null>(null),
  tests = ref<Record<number, CollectionTestResponse>>({}),
  testing = ref(new Set<number>()),
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
  sortedCols = computed(() =>
    [...collections.value].sort((a, b) =>
      a.name === "全球直连" ? -1 : b.name === "全球直连" ? 1 : 0,
    ),
  ),
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
  const r = await fetch(url, init);
  if (!r.ok)
    throw new Error(
      (await r.json().catch(() => null))?.error?.message || r.statusText,
    );
  return r.json().catch(() => null);
}
async function load() {
  try {
    const [g, c, f, r] = await Promise.all([
      json("/api/v1/node-groups"),
      json("/api/v1/collections"),
      api.getNodeFacets(),
      json("/api/v1/rules"),
    ]);
    groups.value = Array.isArray(g) ? g : [];
    collections.value = Array.isArray(c) ? c : [];
    rules.value = Array.isArray(r) ? r : [];
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
  ngURL.value = "https://www.gstatic.com/generate_204";
  ngInterval.value = 300;
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
  ngURL.value = x.test_url || ngURL.value;
  ngInterval.value = x.test_interval || 300;
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
        test_url: ngURL.value,
        test_interval: ngInterval.value,
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
  colEnabled.value = true;
  colURL.value = "https://www.gstatic.com/generate_204";
  colInterval.value = 300;
  colTolerance.value = 100;
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
  colSource.value = x.source_type === "manual" ? "manual" : "node_groups";
  colGroups.value = x.referenced_groups?.map((g) => g.id) || [];
  colUIDs.value = x.node_uids || [];
  colRules.value = x.route_rule_ids || [];
  colEnabled.value = x.enabled;
  colURL.value = x.test_url || colURL.value;
  colInterval.value = x.test_interval || 300;
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
  );
function selectRule(id: number) {
  colRules.value = id ? [id] : [];
  colName.value = rules.value.find((x) => x.id === id)?.name || "";
}
async function saveCol() {
  try {
    const id = editingCol.value?.id,
      p = {
        name: colName.value,
        type: colType.value,
        source_type: colSource.value,
        referenced_group_ids:
          colSource.value === "node_groups" ? colGroups.value : [],
        route_rule_ids: colRules.value,
        node_uids:
          colSource.value === "manual"
            ? colUIDs.value
            : builtin(editingCol.value),
        enabled: colEnabled.value,
        test_url: colURL.value,
        test_interval: colInterval.value,
        tolerance: colTolerance.value,
      };
    await json(id ? `/api/v1/collections/${id}` : "/api/v1/collections", {
      method: id ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(p),
    });
    show(id ? "策略组已更新" : "策略组已创建");
    resetCol();
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
  }
}
async function removeCol(x: Col) {
  await json(`/api/v1/collections/${x.id}`, { method: "DELETE" });
  show("策略组已删除");
  await load();
}
async function testCol(x: Col) {
  testing.value = new Set(testing.value).add(x.id);
  try {
    const r = await api.testProxyCollection(x.id);
    tests.value[x.id] = r;
    show(
      r.available
        ? `测速完成：${r.available}/${r.tested} 个节点可用，最低 ${r.fastest_latency}ms`
        : `测速完成：全部节点不可用`,
      r.available ? "success" : "error",
    );
  } catch (e: any) {
    show(`测速失败: ${e.message}`, "error");
  } finally {
    testing.value.delete(x.id);
    testing.value = new Set(testing.value);
  }
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
              url: previewCol.value.test_url,
              interval: `${previewCol.value.test_interval || 300}s`,
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
    <Toast :message="message" :type="messageType" />
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
              <td>{{ x.filter_subscriptions || "全部" }}</td>
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
                  '名称',
                  '绑定规则',
                  '类型',
                  '引用节点组',
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
            <tr v-for="x in pagedCols" :key="x.id">
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
                  ><button
                    :disabled="testing.has(x.id) || !x.enabled"
                    @click="testCol(x)"
                  >
                    <Zap :size="12" class="inline" />{{
                      testing.has(x.id) ? "测速中" : "测速"
                    }}
                  </button>
                  <button @click="editCol(x)">
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
        <input
          v-model="ngName"
          class="aw-input md:col-span-2"
          placeholder="名称"
        /><select v-model="ngType" class="aw-input">
          <option value="selector">手动切换</option>
          <option value="urltest">自动选择（测速）</option></select
        ><label
          ><input v-model="ngEnabled" type="checkbox" /> 启用此节点组</label
        >
        <div class="md:col-span-2">
          <button class="aw-action-button" @click="picker('group')">
            手动选择节点 ({{ ngUIDs.length }})
          </button>
        </div>
        <template v-if="ngType === 'urltest'"
          ><input v-model="ngURL" class="aw-input md:col-span-2" /><input
            v-model.number="ngInterval"
            type="number"
            class="aw-input" /><input
            v-model.number="ngTolerance"
            type="number"
            class="aw-input"
        /></template>
        <div class="md:col-span-2">
          <b>协议范围</b>
          <div class="flex flex-wrap gap-2">
            <label v-for="x in facets.protocols" :key="x.value"
              ><input
                type="checkbox"
                :checked="ngProtocols.includes(x.value)"
                @change="ngProtocols = toggle(ngProtocols, x.value)"
              />
              {{ x.label }} ({{ x.count }})</label
            >
          </div>
        </div>
        <div class="md:col-span-2">
          <b>订阅范围</b>
          <div class="grid gap-2 md:grid-cols-2">
            <label v-for="x in facets.subscriptions" :key="x.value"
              ><input
                type="checkbox"
                :checked="ngSubscriptions.includes(x.value)"
                @change="ngSubscriptions = toggle(ngSubscriptions, x.value)"
              />
              {{ x.label }}</label
            >
          </div>
        </div>
        <input
          v-model="ngInclude"
          class="aw-input md:col-span-2"
          placeholder="包含关键词，以 | 分隔"
        /><input
          v-model="ngExclude"
          class="aw-input md:col-span-2"
          placeholder="排除关键词，以 | 分隔"
        />
      </div>
      <template #footer
        ><button class="aw-action-button" @click="resetGroup">取消</button
        ><button class="aw-action-button" @click="saveGroup">
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
              <td>{{ x.name }}</td>
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
      <div>
        <b>按订阅筛选</b>
        <div>
          <label v-for="x in facets.subscriptions" :key="x.value"
            ><input
              type="checkbox"
              :checked="quickSubscriptions.includes(x.value)"
              @change="quickSubscriptions = toggle(quickSubscriptions, x.value)"
            />
            {{ x.label }}
          </label>
        </div>
        <b>按协议筛选</b>
        <div>
          <label v-for="x in facets.protocols" :key="x.value"
            ><input
              type="checkbox"
              :checked="quickProtocols.includes(x.value)"
              @change="quickProtocols = toggle(quickProtocols, x.value)"
            />
            {{ x.label }}
          </label>
        </div>
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
        <select
          class="aw-input"
          :value="colRules[0] || 0"
          @change="
            selectRule(Number(($event.target as HTMLSelectElement).value))
          "
        >
          <option :value="0">请选择规则</option>
          <option v-for="x in selectableRules" :key="x.id" :value="x.id">
            {{ x.name }}
          </option></select
        ><select v-model="colType" class="aw-input">
          <option value="selector">手动切换</option>
          <option value="urltest">自动选择（测速）</option></select
        ><select v-model="colSource" class="aw-input">
          <option value="node_groups">引用节点组</option>
          <option value="manual">手动选择节点</option></select
        ><label><input v-model="colEnabled" type="checkbox" /> 启用</label
        ><template v-if="colType === 'urltest'"
          ><input v-model="colURL" class="aw-input md:col-span-2" /><input
            v-model.number="colInterval"
            type="number"
            class="aw-input" /><input
            v-model.number="colTolerance"
            type="number"
            class="aw-input"
        /></template>
        <div v-if="colSource === 'node_groups'" class="md:col-span-2">
          <b>选择引用的节点组</b
          ><label
            v-for="x in groups.filter((g) => g.enabled)"
            :key="x.id"
            class="block"
            ><input
              type="checkbox"
              :checked="colGroups.includes(x.id)"
              @change="colGroups = toggle(colGroups, x.id)"
            />
            {{ x.name }} ({{ x.matched_node_count || 0 }})</label
          >
        </div>
        <div v-else class="md:col-span-2">
          <button class="aw-action-button" @click="picker('collection')">
            手动选择节点 ({{ colUIDs.length }})
          </button>
        </div>
      </div>
      <template #footer
        ><button class="aw-action-button" @click="resetCol">取消</button
        ><button
          class="aw-action-button"
          :disabled="
            !colRules.length ||
            (colSource === 'node_groups' ? !colGroups.length : !colUIDs.length)
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
