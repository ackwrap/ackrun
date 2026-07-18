<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Link, Pause, Play, RefreshCw, Search, X } from "lucide-vue-next";
import type { Connection } from "@/services/clash";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { formatBytes, formatSpeed } from "./monitorUtils";

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
const activeRows = ref<ConnectionRow[]>([]);
const closedRows = ref<ConnectionRow[]>([]);
const previous = new Map<string, Snapshot>();

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
  return (
    [connection.rule, connection.rulePayload].filter(Boolean).join(": ") || "-"
  );
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
      <table class="w-full min-w-[1320px] border-collapse text-xs">
        <thead
          class="sticky top-0 z-10 bg-[var(--bg-elevated)] text-[var(--text-secondary)]"
        >
          <tr>
            <th class="w-10 px-2 py-2 text-center">关闭</th>
            <th class="px-3 py-2 text-left">主机</th>
            <th class="px-3 py-2 text-left">类型</th>
            <th class="px-3 py-2 text-left">规则</th>
            <th class="px-3 py-2 text-left">代理链</th>
            <th class="px-3 py-2 text-right">下载速度</th>
            <th class="px-3 py-2 text-right">上传速度</th>
            <th class="px-3 py-2 text-right">下载</th>
            <th class="px-3 py-2 text-right">上传</th>
            <th class="px-3 py-2 text-right">连接时间</th>
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
            class="border-t border-[var(--border-light)] transition hover:bg-[var(--color-primary-bg)]"
            :class="index % 2 ? 'bg-[var(--bg-base)]' : ''"
          >
            <td class="px-2 py-2 text-center">
              <button
                v-if="tab === 'active'"
                type="button"
                class="inline-flex size-5 items-center justify-center rounded-full bg-[var(--bg-base)] text-[var(--text-secondary)] hover:bg-[var(--color-error-bg)] hover:text-[var(--color-error)]"
                title="关闭连接"
                @click="emit('closeConnection', row.connection.id)"
              >
                <X :size="12" />
              </button>
              <span
                v-else
                class="inline-flex size-5 items-center justify-center"
              >
                <Link :size="12" class="opacity-40" />
              </span>
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
              >
                <template
                  v-for="(chain, index) in row.connection.chains"
                  :key="`${chain}-${index}`"
                >
                  <span v-if="index" class="shrink-0">→</span>
                  <NodeFlagName
                    :name="chain"
                    :flag="nodeFlags[chain]"
                    class="min-w-0"
                  />
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
  </div>
</template>
