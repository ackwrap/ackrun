<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  Edit3,
  Eye,
  RefreshCw,
  Smile,
  Star,
  Tags,
  Trash2,
  Zap,
} from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Pagination from "@/components/ui/Pagination.vue";
import { api } from "@/services/api";
import type { NodeFacetItem, NodeItem, Subscription } from "@/services/types";
import { defaultFlag, getFlagImageURL } from "@/utils/nodeFlags";

const route = useRoute(),
  router = useRouter(),
  subscriptions = ref<Subscription[]>([]),
  nodes = ref<NodeItem[]>([]),
  flags = ref<Record<string, string>>({}),
  facetsTotal = ref(0),
  typeFacets = ref<NodeFacetItem[]>([]),
  subscriptionFacets = ref<NodeFacetItem[]>([]);
const total = ref(0),
  page = ref(1),
  pageSize = ref(50),
  loading = ref(false),
  message = ref(""),
  keyword = ref(""),
  subscriptionID = ref(String(route.query.subscription_id || "")),
  typeFilter = ref(""),
  statusFilter = ref(""),
  enabledFilter = ref(""),
  preferredFilter = ref("");
const detail = ref<NodeItem | null>(null),
  selected = ref(new Set<string>()),
  tcping = ref(new Set<string>()),
  tcpingLoading = ref(false),
  renameOpen = ref(false),
  renameMode = ref<"lines" | "replace" | "prefix" | "suffix">("prefix"),
  renameText = ref(""),
  findText = ref(""),
  replaceText = ref(""),
  deleteOpen = ref(false);
const toastType = computed(() =>
    /失败|错误/.test(message.value) ? "error" : "success",
  ),
  selectedNodes = computed(() =>
    nodes.value.filter((n) => selected.value.has(n.uid)),
  ),
  totalPages = computed(() =>
    Math.max(1, Math.ceil(total.value / pageSize.value)),
  );
const show = (s: string) => (message.value = s);
async function loadFacets() {
  try {
    const r = await api.getNodeFacets();
    facetsTotal.value = r.total;
    typeFacets.value = r.types;
    subscriptionFacets.value = r.subscriptions;
  } catch (e: any) {
    show(`筛选统计加载失败: ${e.message}`);
  }
}
async function loadNodes() {
  loading.value = true;
  try {
    const r = await api.getNodes({
      subscription_id: subscriptionID.value
        ? Number(subscriptionID.value)
        : undefined,
      keyword: keyword.value,
      type: typeFilter.value,
      status: statusFilter.value,
      enabled:
        enabledFilter.value === "" ? undefined : enabledFilter.value === "true",
      preferred:
        preferredFilter.value === ""
          ? undefined
          : preferredFilter.value === "true",
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
    });
    nodes.value = r.items;
    total.value = r.total;
    selected.value = new Set();
    flags.value = r.items.length
      ? Object.fromEntries(
          (
            await api.inferNodeFlags(
              r.items.map((n) => ({
                key: n.uid,
                name: n.name,
                server: n.server,
              })),
            )
          ).items.map((i) => [i.key, i.flag]),
        )
      : {};
    await router.replace({
      query: {
        ...route.query,
        subscription_id: subscriptionID.value || undefined,
      },
    });
  } catch (e: any) {
    show(`节点加载失败: ${e.message}`);
  } finally {
    loading.value = false;
  }
}
function filter(target: "type" | "subscription", v: string) {
  page.value = 1;
  if (target === "type") typeFilter.value = v;
  else subscriptionID.value = v;
}
function toggle(uid: string) {
  const n = new Set(selected.value);
  n.has(uid) ? n.delete(uid) : n.add(uid);
  selected.value = n;
}
function toggleAll() {
  selected.value =
    selected.value.size === nodes.value.length
      ? new Set()
      : new Set(nodes.value.map((n) => n.uid));
}
async function enabled(n: NodeItem, v = !n.enabled) {
  try {
    await api.setNodeEnabled(n.uid, v);
    n.enabled = v;
  } catch (e: any) {
    show(`更新启用状态失败: ${e.message}`);
  }
}
async function preferred(n: NodeItem, v = !n.preferred) {
  try {
    await api.setNodePreferred(n.uid, v);
    n.preferred = v;
  } catch (e: any) {
    show(`更新首选状态失败: ${e.message}`);
  }
}
async function batchEnabled(v: boolean) {
  try {
    await Promise.all(
      selectedNodes.value.map((n) => api.setNodeEnabled(n.uid, v)),
    );
    selectedNodes.value.forEach((n) => (n.enabled = v));
    show(`已${v ? "启用" : "禁用"} ${selectedNodes.value.length} 个节点`);
  } catch (e: any) {
    show(`批量更新失败: ${e.message}`);
  }
}
async function batchPreferred() {
  try {
    await Promise.all(
      selectedNodes.value.map((n) => api.setNodePreferred(n.uid, true)),
    );
    selectedNodes.value.forEach((n) => (n.preferred = true));
    show(`已将 ${selectedNodes.value.length} 个节点标记为首选`);
  } catch (e: any) {
    show(`批量首选失败: ${e.message}`);
  }
}
async function ping(
  uids = selectedNodes.value.length
    ? selectedNodes.value.map((n) => n.uid)
    : nodes.value.map((n) => n.uid),
) {
  if (!uids.length) return;
  tcpingLoading.value = true;
  tcping.value = new Set([...tcping.value, ...uids]);
  try {
    const r = await api.tcpingNodes(uids),
      m = new Map(r.map((x) => [x.uid, x]));
    nodes.value.forEach((n) => {
      const x = m.get(n.uid);
      if (x) {
        n.latency_ms = x.success ? x.latency_ms : 0;
        n.status = x.success ? "available" : "unavailable";
      }
    });
    show(
      `TCPing 完成：${r.filter((x) => x.success).length}/${r.length} 个节点可连通`,
    );
  } catch (e: any) {
    show(`TCPing 失败: ${e.message}`);
  } finally {
    tcpingLoading.value = false;
    uids.forEach((x) => tcping.value.delete(x));
    tcping.value = new Set(tcping.value);
  }
}
async function emoji() {
  try {
    const r = await api.addNodeEmoji(selectedNodes.value.map((n) => n.uid));
    show(`添加 emoji 完成：成功 ${r.success}，失败/跳过 ${r.failed}`);
    await loadNodes();
  } catch (e: any) {
    show(`添加 emoji 失败: ${e.message}`);
  }
}
function openRename() {
  renameText.value =
    renameMode.value === "lines"
      ? selectedNodes.value.map((n) => n.name).join("\n")
      : "";
  findText.value = "";
  replaceText.value = "";
  renameOpen.value = true;
}
async function saveRename() {
  try {
    const uids = selectedNodes.value.map((n) => n.uid);
    const payload: any = { uids, mode: renameMode.value };
    if (renameMode.value === "lines")
      payload.names = renameText.value.split("\n");
    else if (renameMode.value === "replace") {
      payload.find = findText.value;
      payload.replace = replaceText.value;
    } else payload[renameMode.value] = renameText.value;
    const r = await api.batchRenameNodes(payload);
    show(`修改名称完成：成功 ${r.success}，失败 ${r.failed}`);
    renameOpen.value = false;
    await loadNodes();
  } catch (e: any) {
    show(`修改名称失败: ${e.message}`);
  }
}
async function remove() {
  try {
    const r = await api.batchDeleteNodes(selectedNodes.value.map((n) => n.uid));
    show(`删除完成：成功 ${r.success}，失败 ${r.failed}`);
    deleteOpen.value = false;
    await Promise.all([loadNodes(), loadFacets()]);
  } catch (e: any) {
    show(`删除失败: ${e.message}`);
  }
}
const address = (n: NodeItem) => `${n.server}:${n.server_port}`,
  pretty = (s: string) => {
    try {
      return JSON.stringify(JSON.parse(s || "{}"), null, 2);
    } catch {
      return s || "{}";
    }
  };
watch(
  [
    keyword,
    subscriptionID,
    typeFilter,
    statusFilter,
    enabledFilter,
    preferredFilter,
    page,
    pageSize,
  ],
  loadNodes,
);
watch(totalPages, (v) => {
  if (page.value > v) page.value = v;
});
onMounted(async () => {
  try {
    subscriptions.value = await api.getSubscriptions();
  } catch (e: any) {
    show(`订阅加载失败: ${e.message}`);
  }
  await Promise.all([loadFacets(), loadNodes()]);
});
</script>
<template>
  <div class="space-y-4">
    <PageHeader title="节点管理" /><Toast
      :message="message"
      :type="toastType"
    />
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
    >
      <div class="mb-4 flex flex-wrap justify-between gap-3">
        <div>
          <h2 class="font-semibold text-[var(--text-primary)]">
            节点列表 ({{ total }})
          </h2>
          <p class="text-xs text-[var(--text-secondary)]">
            管理订阅解析后的节点，控制是否参与后续配置生成。
          </p>
        </div>
        <div class="flex flex-wrap justify-end gap-2">
          <button
            class="aw-action-button aw-action-neutral"
            :disabled="tcpingLoading || !nodes.length"
            @click="ping()"
          >
            <Zap :size="14" />节点测速</button
          ><button
            class="aw-action-button aw-action-neutral"
            @click="
              api.syncAllSubscriptions().then(() => show('已触发外部订阅同步'))
            "
          >
            <RefreshCw :size="14" />同步外部订阅</button
          ><button
            class="aw-action-button aw-action-neutral"
            :disabled="!selectedNodes.length"
            @click="emoji"
          >
            <Smile :size="14" />添加 emoji</button
          ><button
            class="aw-action-button aw-action-neutral"
            :disabled="!selectedNodes.length"
            @click="openRename"
          >
            <Edit3 :size="14" />修改名称</button
          ><button
            class="aw-action-button aw-action-neutral"
            :disabled="!selectedNodes.length"
            @click="batchPreferred"
          >
            <Tags :size="14" />管理首选</button
          ><button
            class="aw-action-button aw-action-neutral"
            :disabled="!selectedNodes.length"
            @click="batchEnabled(false)"
          >
            批量禁用</button
          ><button
            class="aw-action-button aw-action-danger"
            :disabled="!selectedNodes.length"
            @click="deleteOpen = true"
          >
            <Trash2 :size="14" />批量删除</button
          ><button
            class="aw-action-button aw-action-success"
            :disabled="!selectedNodes.length"
            @click="batchEnabled(true)"
          >
            批量启用
          </button>
        </div>
      </div>
      <div
        class="mb-4 space-y-3 rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
      >
        <div
          v-for="set in [
            {
              label: '按协议筛选',
              items: typeFacets,
              current: typeFilter,
              target: 'type' as const,
            },
            {
              label: '按订阅筛选',
              items: subscriptionFacets,
              current: subscriptionID,
              target: 'subscription' as const,
            },
          ]"
          :key="set.label"
        >
          <div class="mb-2 text-xs text-[var(--text-tertiary)]">
            {{ set.label }}
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              :class="['aw-filter-chip', !set.current && 'active']"
              @click="filter(set.target, '')"
            >
              全部 ({{ facetsTotal }})</button
            ><button
              v-for="x in set.items"
              :key="x.value"
              :class="['aw-filter-chip', set.current === x.value && 'active']"
              @click="filter(set.target, x.value)"
            >
              {{ x.label }} ({{ x.count }})
            </button>
          </div>
        </div>
      </div>
      <div class="mb-4 grid gap-3 md:grid-cols-2 xl:grid-cols-6">
        <input
          v-model="keyword"
          placeholder="搜索名称 / 地址 / UID"
          class="aw-input xl:col-span-2"
        /><select v-model="subscriptionID" class="aw-input">
          <option value="">全部订阅</option>
          <option v-for="s in subscriptions" :key="s.id" :value="s.id">
            {{ s.name }}
          </option></select
        ><select v-model="typeFilter" class="aw-input">
          <option value="">全部协议</option>
          <option v-for="x in typeFacets" :key="x.value" :value="x.value">
            {{ x.label }}
          </option></select
        ><select v-model="statusFilter" class="aw-input">
          <option value="">全部状态</option>
          <option value="unknown">未知</option>
          <option value="available">可用</option>
          <option value="unavailable">不可用</option>
        </select>
        <div class="grid grid-cols-2 gap-2">
          <select v-model="enabledFilter" class="aw-input">
            <option value="">启用状态</option>
            <option value="true">已启用</option>
            <option value="false">已禁用</option></select
          ><select v-model="preferredFilter" class="aw-input">
            <option value="">首选状态</option>
            <option value="true">首选</option>
            <option value="false">非首选</option>
          </select>
        </div>
      </div>
      <div class="aw-data-table-wrap">
        <table class="aw-data-table min-w-[1120px]">
          <thead>
            <tr>
              <th>
                <input
                  type="checkbox"
                  :checked="nodes.length > 0 && selected.size === nodes.length"
                  @change="toggleAll"
                />
              </th>
              <th
                v-for="c in [
                  '名称',
                  '协议',
                  '地址',
                  '订阅',
                  'UID',
                  '延迟',
                  '状态',
                  '启用',
                  '首选',
                  '更新时间',
                  '操作',
                ]"
                :key="c"
              >
                {{ c }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!nodes.length">
              <td colspan="12" class="py-14 text-center">
                {{ loading ? "加载中..." : "暂无节点，请先同步订阅。" }}
              </td>
            </tr>
            <tr v-for="n in nodes" v-else :key="n.uid">
              <td>
                <input
                  type="checkbox"
                  :checked="selected.has(n.uid)"
                  @change="toggle(n.uid)"
                />
              </td>
              <td class="max-w-[240px] truncate">
                <img
                  :src="getFlagImageURL(flags[n.uid] || defaultFlag)"
                  alt=""
                  class="mr-2 inline h-4 w-4"
                />{{ n.name }}
              </td>
              <td>{{ n.type }}</td>
              <td class="max-w-[220px] truncate">{{ address(n) }}</td>
              <td>{{ n.subscription_name || n.subscription_id }}</td>
              <td class="font-mono">{{ n.uid.slice(0, 12) }}…</td>
              <td>
                <button @click="ping([n.uid])">
                  {{
                    tcping.has(n.uid)
                      ? "测速中..."
                      : n.latency_ms > 0
                        ? `${n.latency_ms} ms`
                        : "--"
                  }}
                </button>
              </td>
              <td>{{ n.status }}</td>
              <td>
                <button @click="enabled(n)">
                  {{ n.enabled ? "启用" : "禁用" }}
                </button>
              </td>
              <td>
                <button @click="preferred(n)">
                  <Star :size="12" class="inline" />{{
                    n.preferred ? "首选" : "普通"
                  }}
                </button>
              </td>
              <td>
                {{
                  n.updated_at > 0
                    ? new Date(n.updated_at).toLocaleString()
                    : "--"
                }}
              </td>
              <td>
                <button @click="detail = n">
                  <Eye :size="13" class="inline" />详情
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
        :total="total"
        :page="page"
        :page-size="pageSize"
        :total-pages="totalPages"
        @page-change="page = $event"
        @page-size-change="
          pageSize = $event;
          page = 1;
        "
      />
    </section>
    <Modal :open="!!detail" title="节点详情" size="lg" @close="detail = null"
      ><template v-if="detail"
        ><div class="grid gap-3 md:grid-cols-2">
          <div>UID：{{ detail.uid }}</div>
          <div>地址：{{ address(detail) }}</div>
          <div>协议：{{ detail.type }}</div>
          <div>
            订阅：{{ detail.subscription_name || detail.subscription_id }}
          </div>
        </div>
        <pre
          class="mt-4 max-h-[50vh] overflow-auto rounded-md bg-[var(--bg-base)] p-4 text-xs"
          >{{ pretty(detail.raw_json) }}</pre>
      </template></Modal
    >
    <Modal
      :open="renameOpen"
      :title="`批量修改名称 (${selectedNodes.length})`"
      @close="renameOpen = false"
      ><select v-model="renameMode" class="aw-input w-full">
        <option value="prefix">添加前缀</option>
        <option value="suffix">添加后缀</option>
        <option value="replace">查找替换</option>
        <option value="lines">按行改名</option>
      </select>
      <div
        v-if="renameMode === 'replace'"
        class="mt-3 grid gap-3 md:grid-cols-2"
      >
        <input
          v-model="findText"
          class="aw-input"
          placeholder="查找文本"
        /><input v-model="replaceText" class="aw-input" placeholder="替换为" />
      </div>
      <textarea
        v-else-if="renameMode === 'lines'"
        v-model="renameText"
        rows="8"
        class="aw-input mt-3 w-full"
      /><input
        v-else
        v-model="renameText"
        class="aw-input mt-3 w-full"
      /><template #footer
        ><button class="aw-action-button" @click="renameOpen = false">
          取消</button
        ><button class="aw-action-button" @click="saveRename">
          保存
        </button></template
      ></Modal
    >
    <ConfirmDialog
      :open="deleteOpen"
      title="批量删除节点"
      :message="`确定删除选中的 ${selectedNodes.length} 个节点吗？此操作不可恢复。`"
      confirm-text="删除"
      @confirm="remove"
      @cancel="deleteOpen = false"
    />
  </div>
</template>
