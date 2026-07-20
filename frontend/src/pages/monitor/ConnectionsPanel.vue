<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { Eye, Pause, Play, RefreshCw, X } from "lucide-vue-next";
import type { Connection } from "@/services/clash";
import Modal from "@/components/ui/Modal.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { formatBytes, formatSpeed } from "./monitorUtils";
import { displayProxyName } from "./proxyGroupUtils";

interface ConnectionRow {
  connection: Connection;
  downloadSpeed: number;
  uploadSpeed: number;
  closedAt?: number;
}

interface Snapshot {
  connection: Connection;
  observedAt: number;
}

const props = defineProps<{
  connections: Connection[];
  loading: boolean;
  nodeFlags: Record<string, string>;
}>();
const emit = defineEmits<{
  refresh: [];
  closeConnection: [string];
  closeAll: [];
}>();

const tab = ref<"active" | "closed">("active");
const search = ref("");
const source = ref("");
const paused = ref(false);
const detailConnectionID = ref("");
const activeRows = ref<ConnectionRow[]>([]);
const closedRows = ref<ConnectionRow[]>([]);
const detailRow = computed(() =>
  detailConnectionID.value
    ? [...activeRows.value, ...closedRows.value].find(
        (row) => row.connection.id === detailConnectionID.value,
      ) || null
    : null,
);
const previous = new Map<string, Snapshot>();
const columns = [
  { key: "close", label: "操作", width: 72, min: 64, align: "center" },
  { key: "host", label: "主机", width: 220, min: 120, align: "left" },
  { key: "type", label: "类型", width: 180, min: 110, align: "left" },
  { key: "rule", label: "规则", width: 170, min: 100, align: "left" },
  { key: "chain", label: "代理链", width: 520, min: 180, align: "left" },
  { key: "downSpeed", label: "下载速度", width: 110, min: 90, align: "right" },
  { key: "upSpeed", label: "上传速度", width: 110, min: 90, align: "right" },
  { key: "download", label: "下载", width: 90, min: 70, align: "right" },
  { key: "upload", label: "上传", width: 90, min: 70, align: "right" },
  { key: "elapsed", label: "连接时间", width: 110, min: 90, align: "right" },
] as const;
type ColumnKey = (typeof columns)[number]["key"];
const columnWidths = ref<Record<ColumnKey, number>>(
  Object.fromEntries(
    columns.map((column) => [column.key, column.width]),
  ) as Record<ColumnKey, number>,
);
const storedWidths = localStorage.getItem("ackwrap.monitor.connection-columns");
if (storedWidths) {
  try {
    const parsed = JSON.parse(storedWidths) as Partial<
      Record<ColumnKey, number>
    >;
    for (const column of columns) {
      const width = Number(parsed[column.key]);
      if (Number.isFinite(width) && width >= column.min)
        columnWidths.value[column.key] = Math.min(width, 1200);
    }
  } catch {}
}
const tableWidth = computed(() =>
  columns.reduce((total, column) => total + columnWidths.value[column.key], 0),
);
let cleanupResize: (() => void) | undefined;

function startColumnResize(event: PointerEvent, key: ColumnKey, min: number) {
  if (event.button !== 0) return;
  event.preventDefault();
  cleanupResize?.();
  const startX = event.clientX;
  const startWidth = columnWidths.value[key];
  const move = (nextEvent: PointerEvent) => {
    columnWidths.value[key] = Math.max(
      min,
      startWidth + nextEvent.clientX - startX,
    );
  };
  const finish = () => {
    localStorage.setItem(
      "ackwrap.monitor.connection-columns",
      JSON.stringify(columnWidths.value),
    );
    cleanupResize?.();
  };
  cleanupResize = () => {
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", finish);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    cleanupResize = undefined;
  };
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", finish);
}

onBeforeUnmount(() => cleanupResize?.());

function updateRows(connections: Connection[]) {
  const observedAt = Date.now();
  const activeIDs = new Set(connections.map((connection) => connection.id));

  for (const [id, snapshot] of previous) {
    if (!activeIDs.has(id)) {
      const existing = activeRows.value.find((row) => row.connection.id === id);
      closedRows.value = [
        {
          ...(existing || {
            connection: snapshot.connection,
            downloadSpeed: 0,
            uploadSpeed: 0,
          }),
          closedAt: observedAt,
        },
        ...closedRows.value.filter((row) => row.connection.id !== id),
      ].slice(0, 200);
      previous.delete(id);
    }
  }

  activeRows.value = connections.map((connection) => {
    const snapshot = previous.get(connection.id);
    const seconds = snapshot
      ? Math.max(0.001, (observedAt - snapshot.observedAt) / 1000)
      : 0;
    const row = {
      connection,
      downloadSpeed: snapshot
        ? Math.max(
            0,
            (connection.download - snapshot.connection.download) / seconds,
          )
        : 0,
      uploadSpeed: snapshot
        ? Math.max(
            0,
            (connection.upload - snapshot.connection.upload) / seconds,
          )
        : 0,
    };
    previous.set(connection.id, { connection, observedAt });
    return row;
  });
}

watch(
  () => props.connections,
  (connections) => {
    if (!paused.value) updateRows(connections);
  },
  { immediate: true },
);

const sourceOptions = computed(() =>
  Array.from(
    new Set(
      [...activeRows.value, ...closedRows.value]
        .map((row) => row.connection.metadata.sourceIP)
        .filter(Boolean),
    ),
  ).sort(),
);

const rows = computed(() => {
  const candidates =
    tab.value === "active" ? activeRows.value : closedRows.value;
  const keyword = search.value.trim();
  let matcher: (value: string) => boolean = () => true;
  if (keyword) {
    try {
      const regex = new RegExp(keyword, "i");
      matcher = (value) => regex.test(value);
    } catch {
      const normalized = keyword.toLowerCase();
      matcher = (value) => value.toLowerCase().includes(normalized);
    }
  }
  return candidates.filter((row) => {
    const connection = row.connection;
    if (source.value && connection.metadata.sourceIP !== source.value)
      return false;
    return matcher(
      [
        connection.metadata.host,
        connection.metadata.destinationIP,
        connection.metadata.sourceIP,
        connection.rule,
        connection.rulePayload,
        connection.chains?.join(" "),
      ]
        .filter(Boolean)
        .join(" "),
    );
  });
});

function togglePaused() {
  paused.value = !paused.value;
  if (!paused.value) updateRows(props.connections);
}

function target(connection: Connection) {
  const host = connection.metadata.host || connection.metadata.destinationIP;
  return connection.metadata.destinationPort
    ? `${host}:${connection.metadata.destinationPort}`
    : host;
}

function connectionType(connection: Connection) {
  return [connection.metadata.type, connection.metadata.network]
    .filter(Boolean)
    .join(" | ");
}

function ruleDescription(connection: Connection) {
  const description = [connection.rule, connection.rulePayload]
    .filter(Boolean)
    .join(": ");
  return connection.rule === "final"
    ? "final（未命中规则，使用默认出站）"
    : description || "-";
}

function displayChains(connection: Connection) {
  return [...(connection.chains || [])].reverse();
}

function elapsed(connection: Connection, closedAt?: number) {
  const startedAt = new Date(connection.start).getTime();
  if (!Number.isFinite(startedAt)) return "--";
  const seconds = Math.max(
    0,
    Math.floor(((closedAt || Date.now()) - startedAt) / 1000),
  );
  if (seconds < 60) return `${seconds} 秒前`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`;
  return `${Math.floor(seconds / 3600)} 小时前`;
}

function endpoint(address?: string, port?: string) {
  if (!address) return "-";
  return port ? `${address}:${port}` : address;
}

function startedAt(connection: Connection) {
  const value = new Date(connection.start);
  return Number.isNaN(value.getTime()) ? connection.start || "-" : value.toLocaleString();
}

function openDetailsFromKeyboard(event: KeyboardEvent, row: ConnectionRow) {
  if (event.target !== event.currentTarget) return;
  if (event.key === " ") event.preventDefault();
  detailConnectionID.value = row.connection.id;
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3 pb-4">
    <div
      class="flex flex-wrap items-center gap-2 rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-2"
    >
      <div class="flex h-8 shrink-0 rounded-lg bg-[var(--bg-base)] p-0.5">
        <button
          v-for="item in [
            { value: 'active', label: '活跃', count: activeRows.length },
            { value: 'closed', label: '已关闭', count: closedRows.length },
          ] as const"
          :key="item.value"
          type="button"
          class="rounded-md px-3 text-xs font-medium transition"
          :class="
            tab === item.value
              ? 'bg-[var(--color-primary-bg)] text-[var(--color-primary-hover)]'
              : 'text-[var(--text-secondary)]'
          "
          @click="tab = item.value"
        >
          {{ item.label
          }}<span v-if="tab === item.value"> ({{ item.count }})</span>
        </button>
      </div>

      <select
        v-model="source"
        class="h-8 min-w-36 rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] px-3 text-xs"
      >
        <option value="">全部来源</option>
        <option v-for="item in sourceOptions" :key="item" :value="item">
          {{ item }}
        </option>
      </select>

      <div class="relative min-w-48 flex-1">
        <Search
          :size="13"
          class="absolute top-1/2 left-3 -translate-y-1/2 text-[var(--text-tertiary)]"
        />
        <input
          v-model="search"
          type="search"
          class="h-8 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] pr-8 pl-8 text-xs outline-none focus:border-[var(--color-primary)]"
          placeholder="搜索 | Regex"
        />
      </div>

      <div class="ml-auto flex items-center gap-1">
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full hover:bg-[var(--bg-sidebar-hover)]"
          :disabled="loading"
          title="刷新连接"
          @click="emit('refresh')"
        >
          <RefreshCw :size="14" :class="loading ? 'animate-spin' : ''" />
        </button>
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full hover:bg-[var(--bg-sidebar-hover)]"
          :title="paused ? '继续更新' : '暂停更新'"
          @click="togglePaused"
        >
          <Play v-if="paused" :size="14" />
          <Pause v-else :size="14" />
        </button>
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full text-[var(--color-error)] hover:bg-[var(--color-error-bg)]"
          title="关闭所有活动连接"
          @click="emit('closeAll')"
        >
          <X :size="15" />
        </button>
      </div>
    </div>

    <div
      class="min-h-0 flex-1 overflow-auto rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)]"
    >
      <table
        class="table-fixed border-collapse text-xs"
        :style="{ width: `${tableWidth}px`, minWidth: '100%' }"
      >
        <colgroup>
          <col
            v-for="column in columns"
            :key="column.key"
            :style="{ width: `${columnWidths[column.key]}px` }"
          />
        </colgroup>
        <thead
          class="sticky top-0 z-10 bg-[var(--bg-elevated)] text-[var(--text-secondary)]"
        >
          <tr>
            <th
              v-for="column in columns"
              :key="column.key"
              class="relative px-3 py-2"
              :class="{
                'text-left': column.align === 'left',
                'text-center': column.align === 'center',
                'text-right': column.align === 'right',
              }"
            >
              {{ column.label }}
              <span
                class="absolute top-0 right-0 h-full w-2 cursor-col-resize touch-none select-none after:absolute after:top-1/4 after:right-0 after:h-1/2 after:w-px after:bg-[var(--border-default)] hover:after:bg-[var(--color-primary)]"
                @pointerdown="startColumnResize($event, column.key, column.min)"
              />
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!rows.length">
            <td
              colspan="10"
              class="h-72 text-center text-[var(--text-tertiary)]"
            >
              {{
                loading
                  ? "正在加载连接..."
                  : tab === "active"
                    ? "暂无活动连接"
                    : "暂无已关闭连接"
              }}
            </td>
          </tr>
          <tr
            v-for="(row, index) in rows"
            :key="`${tab}-${row.connection.id}`"
            class="cursor-pointer border-t border-[var(--border-light)] transition hover:bg-[var(--color-primary-bg)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--color-primary)]"
            :class="index % 2 ? 'bg-[var(--bg-base)]' : ''"
            tabindex="0"
            @click="detailConnectionID = row.connection.id"
            @keydown.enter="openDetailsFromKeyboard($event, row)"
            @keydown.space="openDetailsFromKeyboard($event, row)"
          >
            <td class="px-2 py-2 text-center">
              <div class="flex items-center justify-center gap-1">
                <button
                  type="button"
                  class="inline-flex size-6 items-center justify-center rounded-full bg-[var(--bg-base)] text-[var(--text-secondary)] hover:bg-[var(--color-primary-bg)] hover:text-[var(--color-primary)]"
                  title="查看连接详情"
                  @click.stop="detailConnectionID = row.connection.id"
                >
                  <Eye :size="12" />
                </button>
                <button
                  v-if="tab === 'active'"
                  type="button"
                  class="inline-flex size-6 items-center justify-center rounded-full bg-[var(--bg-base)] text-[var(--text-secondary)] hover:bg-[var(--color-error-bg)] hover:text-[var(--color-error)]"
                  title="关闭连接"
                  @click.stop="emit('closeConnection', row.connection.id)"
                >
                  <X :size="12" />
                </button>
              </div>
            </td>
            <td
              class="max-w-80 truncate px-3 py-2 font-medium text-[var(--text-primary)]"
            >
              {{ target(row.connection) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2">
              {{ connectionType(row.connection) }}
            </td>
            <td class="max-w-64 truncate px-3 py-2">
              {{ ruleDescription(row.connection) }}
            </td>
            <td class="max-w-96 truncate px-3 py-2">
              <span
                v-if="row.connection.chains?.length"
                class="inline-flex max-w-full items-center gap-1"
                title="入口策略组 → 实际出站节点"
              >
                <template
                  v-for="(chain, index) in displayChains(row.connection)"
                  :key="`${chain}-${index}`"
                >
                  <span v-if="index" class="shrink-0">→</span>
                  <NodeFlagName
                    :name="chain"
                    :flag="nodeFlags[chain]"
                    class="min-w-0"
                    >{{ displayProxyName(chain) }}</NodeFlagName
                  >
                </template>
              </span>
              <template v-else>DIRECT</template>
            </td>
            <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums">
              {{ formatSpeed(row.downloadSpeed) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums">
              {{ formatSpeed(row.uploadSpeed) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums">
              {{ formatBytes(row.connection.download) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums">
              {{ formatBytes(row.connection.upload) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2 text-right">
              {{ elapsed(row.connection, row.closedAt) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal
      :open="!!detailRow"
      title="连接详情"
      size="xl"
      @close="detailConnectionID = ''"
    >
      <div v-if="detailRow" class="grid gap-4 text-sm lg:grid-cols-2">
        <section class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-4">
          <h4 class="font-semibold text-[var(--color-primary)]">基本信息</h4>
          <dl class="mt-3 grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
            <dt class="text-[var(--text-tertiary)]">状态</dt>
            <dd>{{ detailRow.closedAt ? "已关闭" : "活跃" }}</dd>
            <dt class="text-[var(--text-tertiary)]">ID</dt>
            <dd class="break-all font-mono">{{ detailRow.connection.id }}</dd>
            <dt class="text-[var(--text-tertiary)]">类型</dt>
            <dd>{{ connectionType(detailRow.connection) || "-" }}</dd>
            <dt class="text-[var(--text-tertiary)]">开始时间</dt>
            <dd>{{ startedAt(detailRow.connection) }}</dd>
            <dt class="text-[var(--text-tertiary)]">连接时长</dt>
            <dd>{{ elapsed(detailRow.connection, detailRow.closedAt) }}</dd>
          </dl>
        </section>

        <section class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-4">
          <h4 class="font-semibold text-[var(--color-primary)]">来源与目的</h4>
          <dl class="mt-3 grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
            <dt class="text-[var(--text-tertiary)]">来源</dt>
            <dd class="break-all font-mono">
              {{ endpoint(detailRow.connection.metadata.sourceIP, detailRow.connection.metadata.sourcePort) }}
            </dd>
            <dt class="text-[var(--text-tertiary)]">主机</dt>
            <dd class="break-all font-mono">{{ target(detailRow.connection) || "-" }}</dd>
            <dt class="text-[var(--text-tertiary)]">目的地址</dt>
            <dd class="break-all font-mono">
              {{ endpoint(detailRow.connection.metadata.destinationIP, detailRow.connection.metadata.destinationPort) }}
            </dd>
            <dt class="text-[var(--text-tertiary)]">DNS 模式</dt>
            <dd>{{ detailRow.connection.metadata.dnsMode || "-" }}</dd>
          </dl>
        </section>

        <section class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-4">
          <h4 class="font-semibold text-[var(--color-primary)]">规则与代理链</h4>
          <dl class="mt-3 grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
            <dt class="text-[var(--text-tertiary)]">命中规则</dt>
            <dd class="break-all font-mono">{{ ruleDescription(detailRow.connection) }}</dd>
            <dt class="text-[var(--text-tertiary)]">代理链</dt>
            <dd class="flex flex-wrap items-center gap-1">
              <template v-if="detailRow.connection.chains?.length">
                <template
                  v-for="(chain, index) in displayChains(detailRow.connection)"
                  :key="`${chain}-${index}`"
                >
                  <span v-if="index">→</span>
                  <NodeFlagName :name="chain" :flag="nodeFlags[chain]">
                    {{ displayProxyName(chain) }}
                  </NodeFlagName>
                </template>
              </template>
              <template v-else>DIRECT</template>
            </dd>
          </dl>
        </section>

        <section class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-4">
          <h4 class="font-semibold text-[var(--color-primary)]">流量</h4>
          <dl class="mt-3 grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
            <dt class="text-[var(--text-tertiary)]">下载</dt>
            <dd>{{ formatBytes(detailRow.connection.download) }} / {{ formatSpeed(detailRow.downloadSpeed) }}</dd>
            <dt class="text-[var(--text-tertiary)]">上传</dt>
            <dd>{{ formatBytes(detailRow.connection.upload) }} / {{ formatSpeed(detailRow.uploadSpeed) }}</dd>
          </dl>
        </section>

        <details
          class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-4 lg:col-span-2"
        >
          <summary class="cursor-pointer text-xs font-medium text-[var(--text-secondary)]">
            原始 JSON
          </summary>
          <pre
            class="mt-3 max-h-80 overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-5 text-[var(--text-primary)]"
          >{{ JSON.stringify(detailRow.connection, null, 2) }}</pre>
        </details>
      </div>
    </Modal>
  </div>
</template>
