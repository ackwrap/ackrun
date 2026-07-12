<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import {
  Edit3,
  Eye,
  FileJson,
  Plus,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import SyncScheduleControls from "@/components/ui/SyncScheduleControls.vue";
import { api } from "@/services/api";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import type {
  NodeImportPreviewItem,
  Subscription,
  UserAgentOption,
  WSEvent,
  NodeFilter,
} from "@/services/types";
import { stringify as stringifyYAML } from "yaml";
const manualURL = "manual://local",
  router = useRouter(),
  subscriptions = ref<Subscription[]>([]),
  filters = ref<NodeFilter[]>([]),
  loading = ref(false),
  message = ref(""),
  formOpen = ref(false),
  editing = ref<Subscription | null>(null),
  deleteTarget = ref<Subscription | null>(null);
const name = ref(""),
  url = ref(""),
  userAgent = ref("clash-meta/2.4.0"),
  userAgentPreset = ref("clash-meta/2.4.0"),
  userAgents = ref<UserAgentOption[]>([]),
  syncMode = ref<"off" | "daily" | "weekly" | "monthly">("off"),
  syncTime = ref("00:00:00"),
  syncWeekday = ref(1),
  syncTimeout = ref(60),
  syncErrors = ref<Record<number, string>>({});
const content = ref(""),
  preview = ref<NodeImportPreviewItem[]>([]),
  previewLoading = ref(false),
  previewError = ref(""),
  previewDetail = ref<NodeImportPreviewItem | null>(null),
  detailFormat = ref<"json" | "yaml">("json"),
  lastPreview = ref("");
const editingFilter = ref<NodeFilter | null>(null),
  filterName = ref(""),
  filterTarget = ref("name"),
  filterPattern = ref(""),
  filterEnabled = ref(true);
const remote = computed(() =>
    subscriptions.value.filter((x) => x.url !== manualURL),
  ),
  manual = computed(() => subscriptions.value.find((x) => x.url === manualURL)),
  displayed = computed(() =>
    (manual.value ? [manual.value] : []).concat(remote.value),
  ),
  anySyncing = computed(() =>
    remote.value.some((x) => x.sync_status === "syncing"),
  ),
  toastType = computed(() =>
    /失败|错误/.test(message.value) ? "error" : "success",
  );
const modes = [
    { value: "off", label: "关闭自动同步" },
    { value: "daily", label: "每天" },
    { value: "weekly", label: "每周" },
    { value: "monthly", label: "每月" },
  ],
  weekdays = [1, 2, 3, 4, 5, 6, 7].map((value, i) => ({
    value,
    label: `周${"一二三四五六日"[i]}`,
  })),
  targets = [
    ["all", "全部字段"],
    ["name", "节点名称"],
    ["type", "协议类型"],
    ["server", "服务器地址"],
    ["raw", "原始内容"],
    ["raw_json", "解析 JSON"],
  ];
const show = (s: string) => (message.value = s),
  formatTime = (v: number) => (v > 0 ? new Date(v).toLocaleString() : "--"),
  traffic = (x: Subscription) =>
    x.traffic_total_bytes > 0
      ? `${(x.traffic_used_bytes / 1073741824).toFixed(1)} GB / ${(x.traffic_total_bytes / 1073741824).toFixed(1)} GB`
      : "--";
const schedule = (x: Subscription) =>
  x.sync_mode === "daily"
    ? `每天 ${x.sync_time}`
    : x.sync_mode === "weekly"
      ? `每周${weekdays.find((w) => w.value === x.sync_weekday)?.label || ""} ${x.sync_time}`
      : x.sync_mode === "monthly"
        ? `每月${x.sync_weekday || 1}号 ${x.sync_time}`
        : "关闭";
async function load() {
  loading.value = true;
  try {
    subscriptions.value = await api.getSubscriptions();
  } catch (e: any) {
    show(`加载失败: ${e.message}`);
  } finally {
    loading.value = false;
  }
}
async function loadFilters() {
  try {
    filters.value = await api.getNodeFilters();
  } catch (e: any) {
    show(`过滤规则加载失败: ${e.message}`);
  }
}
function setUA(v: string) {
  userAgentPreset.value =
    v === "clash-meta/2.4.0" || userAgents.value.some((x) => x.value === v)
      ? v
      : "__custom__";
  userAgent.value = v || "clash-meta/2.4.0";
}
function openCreate() {
  editing.value = null;
  name.value = "";
  url.value = "";
  setUA("clash-meta/2.4.0");
  syncMode.value = "off";
  syncTime.value = "00:00:00";
  syncWeekday.value = 1;
  syncTimeout.value = 60;
  formOpen.value = true;
}
function openEdit(x: Subscription) {
  editing.value = x;
  name.value = x.name;
  url.value = x.url;
  setUA(x.user_agent);
  syncMode.value = (
    ["daily", "weekly", "monthly"].includes(x.sync_mode) ? x.sync_mode : "off"
  ) as any;
  syncTime.value = x.sync_time || "00:00:00";
  syncWeekday.value = x.sync_weekday || 1;
  syncTimeout.value = x.sync_timeout_seconds || 60;
  formOpen.value = true;
}
async function save() {
  try {
    const p = {
      name: name.value,
      url: url.value,
      user_agent: userAgent.value,
      sync_mode: syncMode.value,
      sync_time: syncMode.value === "off" ? "" : syncTime.value,
      sync_weekday: ["weekly", "monthly"].includes(syncMode.value)
        ? syncWeekday.value
        : 0,
      sync_timeout_seconds: syncTimeout.value,
    };
    editing.value
      ? await api.updateSubscription(editing.value.id, p)
      : await api.createSubscription(p);
    show(editing.value ? "订阅已更新" : "订阅已添加，正在同步节点");
    formOpen.value = false;
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`);
  }
}
async function syncOne(x: Subscription) {
  x.sync_status = "syncing";
  x.sync_progress = 0;
  delete syncErrors.value[x.id];
  try {
    await api.syncSubscription(x.id);
  } catch (e: any) {
    x.sync_status = "failed";
    syncErrors.value[x.id] = e.message;
    show(`同步失败: ${e.message}`);
  }
}
async function syncAll() {
  remote.value.forEach((x) => {
    x.sync_status = "syncing";
    x.sync_progress = 0;
  });
  syncErrors.value = {};
  try {
    await api.syncAllSubscriptions();
  } catch (e: any) {
    show(`同步失败: ${e.message}`);
    await load();
  }
}
async function parse(silent = false) {
  if (!content.value.trim()) return;
  previewLoading.value = true;
  previewError.value = "";
  try {
    const r = await api.previewImportNodes({ content: content.value });
    preview.value = r.items;
    if (!silent) show(`预览完成：识别到 ${r.count} 个节点`);
  } catch (e: any) {
    preview.value = [];
    silent
      ? (previewError.value = e.message)
      : show(`节点预览失败: ${e.message}`);
  } finally {
    previewLoading.value = false;
  }
}
async function importNodes() {
  try {
    const r = await api.importNodes({ content: content.value });
    show(`手动导入完成：导入 ${r.imported} 个节点`);
    content.value = "";
    preview.value = [];
    await load();
  } catch (e: any) {
    show(`手动导入失败: ${e.message}`);
  }
}
async function remove() {
  if (!deleteTarget.value) return;
  try {
    await api.deleteSubscription(deleteTarget.value.id);
    show(
      deleteTarget.value.url === manualURL
        ? "本地订阅节点已清空"
        : "订阅已删除",
    );
    deleteTarget.value = null;
    await load();
  } catch (e: any) {
    show(`删除失败: ${e.message}`);
  }
}
function resetFilter() {
  editingFilter.value = null;
  filterName.value = "";
  filterTarget.value = "name";
  filterPattern.value = "";
  filterEnabled.value = true;
}
async function saveFilter() {
  try {
    const p = {
      name: filterName.value,
      target: filterTarget.value,
      pattern: filterPattern.value,
      enabled: filterEnabled.value,
    };
    editingFilter.value
      ? await api.updateNodeFilter(editingFilter.value.id, p)
      : await api.createNodeFilter(p);
    show(editingFilter.value ? "过滤规则已更新" : "过滤规则已添加");
    resetFilter();
    await loadFilters();
  } catch (e: any) {
    show(`过滤规则保存失败: ${e.message}`);
  }
}
async function deleteFilter(x: NodeFilter) {
  try {
    await api.deleteNodeFilter(x.id);
    show("过滤规则已删除");
    await loadFilters();
  } catch (e: any) {
    show(`过滤规则删除失败: ${e.message}`);
  }
}
useRealtimeSocket((event: WSEvent) => {
  if (event.type !== "subscription.sync") return;
  const d: any = event.data;
  if (!d.id) return;
  if (d.error || d.warning) syncErrors.value[d.id] = d.error || d.warning;
  else delete syncErrors.value[d.id];
  const x = subscriptions.value.find((i) => i.id === d.id);
  if (x)
    Object.assign(x, {
      sync_status: d.status ?? x.sync_status,
      sync_progress: d.progress ?? x.sync_progress,
      node_count: d.node_count ?? x.node_count,
      traffic_used_bytes: d.traffic_used_bytes ?? x.traffic_used_bytes,
      traffic_total_bytes: d.traffic_total_bytes ?? x.traffic_total_bytes,
      expire_at: d.expire_at ?? x.expire_at,
      last_sync_at: d.last_sync_at ?? x.last_sync_at,
    });
});
let poll: number | undefined, debounce: number | undefined;
watch(anySyncing, (v) => {
  if (poll) clearInterval(poll);
  if (v)
    poll = window.setInterval(
      () =>
        api
          .getSubscriptions()
          .then((x) => (subscriptions.value = x))
          .catch(() => {}),
      1000,
    );
});
watch(content, (v) => {
  clearTimeout(debounce);
  if (!v.trim()) {
    preview.value = [];
    previewError.value = "";
    return;
  }
  debounce = window.setTimeout(() => {
    if (lastPreview.value !== v.trim()) {
      lastPreview.value = v.trim();
      parse(true);
    }
  }, 700);
});
onMounted(async () => {
  await Promise.all([load(), loadFilters()]);
  userAgents.value = await api.getSubscriptionUserAgents().catch(() => []);
});
onUnmounted(() => {
  if (poll) clearInterval(poll);
  clearTimeout(debounce);
});
const pretty = (s: string) => {
    try {
      return JSON.stringify(JSON.parse(s || "{}"), null, 2);
    } catch {
      return s;
    }
  },
  yaml = (s: string) => {
    try {
      return stringifyYAML(JSON.parse(s || "{}"));
    } catch {
      return s;
    }
  };
</script>
<template>
  <div class="space-y-4">
    <PageHeader title="订阅管理" /><Toast
      :message="message"
      :type="toastType"
    />
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
    >
      <div class="mb-4 flex flex-wrap justify-between gap-3">
        <p class="text-sm text-[var(--text-secondary)]">
          管理外部订阅源，用于从第三方订阅同步节点
        </p>
        <div class="flex gap-2">
          <button class="aw-action-button" @click="openCreate">
            <Plus :size="17" /></button
          ><button
            class="aw-action-button"
            :disabled="anySyncing || !remote.length"
            @click="syncAll"
          >
            <RefreshCw :size="15" />同步所有订阅
          </button>
        </div>
      </div>
      <div class="overflow-x-auto border border-[var(--border-default)]">
        <table class="aw-data-table min-w-[1100px]">
          <thead>
            <tr>
              <th
                v-for="c in [
                  '名称',
                  '订阅链接',
                  '节点数',
                  '流量使用',
                  '到期时间',
                  '最后同步',
                  '同步周期',
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
            <tr v-if="!displayed.length">
              <td colspan="9" class="py-14 text-center">
                {{ loading ? "加载中..." : "暂无订阅" }}
              </td>
            </tr>
            <tr v-for="x in displayed" v-else :key="x.id">
              <td>{{ x.url === manualURL ? "本地订阅" : x.name }}</td>
              <td class="max-w-[260px] truncate">
                {{ x.url === manualURL ? "本地节点源" : x.url }}
              </td>
              <td>
                <button
                  :disabled="!x.node_count"
                  @click="router.push(`/nodes?subscription_id=${x.id}`)"
                >
                  {{ x.node_count }}
                </button>
              </td>
              <td>{{ x.url === manualURL ? "--" : traffic(x) }}</td>
              <td>
                {{ x.url === manualURL ? "--" : formatTime(x.expire_at) }}
              </td>
              <td>
                {{ x.url === manualURL ? "--" : formatTime(x.last_sync_at) }}
              </td>
              <td>{{ x.url === manualURL ? "本地" : schedule(x) }}</td>
              <td>
                <span :title="syncErrors[x.id]">{{
                  x.url === manualURL
                    ? "本地"
                    : x.sync_status === "syncing"
                      ? `同步中 ${Math.round(x.sync_progress || 0)}%`
                      : x.sync_status === "failed"
                        ? "失败"
                        : "已更新"
                }}</span>
                <div class="max-w-[180px] truncate text-xs text-red-400">
                  {{ syncErrors[x.id] }}
                </div>
              </td>
              <td>
                <div class="flex gap-2">
                  <button
                    :disabled="
                      x.url === manualURL || x.sync_status === 'syncing'
                    "
                    @click="openEdit(x)"
                  >
                    <Edit3 :size="14" /></button
                  ><button
                    :disabled="
                      x.url === manualURL || x.sync_status === 'syncing'
                    "
                    @click="syncOne(x)"
                  >
                    <RefreshCw :size="14" /></button
                  ><button
                    :disabled="
                      x.sync_status === 'syncing' ||
                      (x.url === manualURL && !x.node_count)
                    "
                    @click="deleteTarget = x"
                  >
                    <Trash2 :size="14" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
    >
      <h2 class="font-semibold">本地订阅导入</h2>
      <p class="mb-4 text-xs text-[var(--text-secondary)]">
        支持 URI List、Clash YAML 和 sing-box JSON；UID 已存在时追加/更新。
      </p>
      <div class="mb-3 flex flex-wrap justify-end gap-2">
        <button
          class="aw-action-button"
          :disabled="!content.trim()"
          @click="
            content = '';
            preview = [];
          "
        >
          <Trash2 :size="14" />清空</button
        ><button
          class="aw-action-button"
          :disabled="!content.trim() || previewLoading"
          @click="parse(false)"
        >
          <FileJson :size="15" />{{
            previewLoading ? "预览中..." : "节点预览"
          }}</button
        ><button
          class="aw-action-button"
          :disabled="!content.trim()"
          @click="importNodes"
        >
          <Upload :size="15" />导入节点
        </button>
      </div>
      <div class="grid gap-4 xl:grid-cols-[3fr_2fr]">
        <textarea
          v-model="content"
          rows="12"
          class="aw-input min-h-[300px] w-full font-mono"
          placeholder="粘贴 URI / Clash YAML / sing-box JSON"
        />
        <aside
          class="min-h-[300px] overflow-auto rounded-md border border-[var(--border-default)] p-4"
        >
          <p class="mb-3 text-xs">
            {{
              preview.length
                ? `已识别 ${preview.length} 个节点`
                : previewError || "粘贴后自动解析"
            }}
          </p>
          <button
            v-for="x in preview.slice(0, 50)"
            :key="x.uid"
            class="mb-2 flex w-full justify-between rounded border border-[var(--border-default)] p-2 text-left"
            @click="previewDetail = x"
          >
            <span class="truncate">{{ x.name }} · {{ x.type }}</span
            ><Eye :size="13" />
          </button>
        </aside>
      </div>
    </section>
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
    >
      <h2 class="font-semibold">节点过滤规则</h2>
      <div class="my-4 grid gap-3 lg:grid-cols-[1fr_160px_1.5fr_110px]">
        <input
          v-model="filterName"
          class="aw-input"
          placeholder="规则名称"
        /><select v-model="filterTarget" class="aw-input">
          <option v-for="x in targets" :key="x[0]" :value="x[0]">
            {{ x[1] }}
          </option></select
        ><input
          v-model="filterPattern"
          class="aw-input font-mono"
          placeholder="Go 正则表达式"
        /><label><input v-model="filterEnabled" type="checkbox" /> 启用</label>
      </div>
      <div class="mb-4 flex gap-2">
        <button class="aw-action-button" @click="saveFilter">
          {{ editingFilter ? "更新规则" : "添加规则" }}</button
        ><button
          v-if="editingFilter"
          class="aw-action-button"
          @click="resetFilter"
        >
          取消编辑
        </button>
      </div>
      <div class="overflow-x-auto">
        <table class="aw-data-table min-w-[680px]">
          <thead>
            <tr>
              <th
                v-for="c in ['名称', '目标字段', '正则', '状态', '操作']"
                :key="c"
              >
                {{ c }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!filters.length">
              <td colspan="5" class="py-10 text-center">暂无过滤规则</td>
            </tr>
            <tr v-for="x in filters" v-else :key="x.id">
              <td>{{ x.name }}</td>
              <td>
                {{ targets.find((t) => t[0] === x.target)?.[1] || x.target }}
              </td>
              <td class="font-mono">{{ x.pattern }}</td>
              <td>
                <button
                  @click="
                    api
                      .updateNodeFilter(x.id, { ...x, enabled: !x.enabled })
                      .then(loadFilters)
                  "
                >
                  {{ x.enabled ? "启用" : "停用" }}
                </button>
              </td>
              <td>
                <button
                  @click="
                    editingFilter = x;
                    filterName = x.name;
                    filterTarget = x.target;
                    filterPattern = x.pattern;
                    filterEnabled = x.enabled;
                  "
                >
                  编辑
                </button>
                <button @click="deleteFilter(x)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <Modal
      :open="formOpen"
      :title="editing ? '编辑订阅' : '添加订阅'"
      @close="formOpen = false"
      ><div class="space-y-3">
        <input
          v-model="name"
          class="aw-input w-full"
          placeholder="名称"
        /><input
          v-model="url"
          class="aw-input w-full"
          placeholder="https://..."
        /><select
          v-model="userAgentPreset"
          class="aw-input w-full"
          @change="
            userAgentPreset !== '__custom__' && (userAgent = userAgentPreset)
          "
        >
          <option v-for="x in userAgents" :key="x.value" :value="x.value">
            {{ x.label }} - {{ x.value }}
          </option>
          <option value="__custom__">自定义</option></select
        ><input
          v-if="userAgentPreset === '__custom__'"
          v-model="userAgent"
          class="aw-input w-full"
        /><SyncScheduleControls
          :value="{
            sync_mode: syncMode,
            sync_time: syncTime,
            sync_weekday: syncWeekday,
          }"
          :sync-modes="modes"
          :weekdays="[]"
          :weekday-options="weekdays"
          @change="
            (p) => {
              if (p.sync_mode !== undefined) syncMode = p.sync_mode;
              if (p.sync_time !== undefined) syncTime = p.sync_time;
              if (p.sync_weekday !== undefined) syncWeekday = p.sync_weekday;
            }
          "
        /><input
          v-model.number="syncTimeout"
          type="number"
          min="5"
          max="300"
          class="aw-input w-full"
        />
      </div>
      <template #footer
        ><button class="aw-action-button" @click="formOpen = false">取消</button
        ><button class="aw-action-button" @click="save">保存</button></template
      ></Modal
    >
    <Modal
      :open="!!previewDetail"
      title="节点配置详情"
      size="xl"
      @close="previewDetail = null"
      ><template v-if="previewDetail"
        ><div class="mb-3 flex gap-2">
          <button @click="detailFormat = 'json'">JSON</button
          ><button @click="detailFormat = 'yaml'">YAML</button>
        </div>
        <pre
          class="max-h-[58vh] overflow-auto rounded bg-[var(--bg-base)] p-4 text-xs"
          >{{
            detailFormat === "json"
              ? pretty(previewDetail.raw_json)
              : yaml(previewDetail.raw_json)
          }}</pre>
      </template></Modal
    ><ConfirmDialog
      :open="!!deleteTarget"
      :title="deleteTarget?.url === manualURL ? '清空本地节点' : '删除订阅'"
      :message="
        deleteTarget?.url === manualURL
          ? `确定清空本地订阅中的 ${deleteTarget?.node_count} 个节点？`
          : `确定删除订阅「${deleteTarget?.name}」？`
      "
      confirm-text="确认"
      @confirm="remove"
      @cancel="deleteTarget = null"
    />
  </div>
</template>
