<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { Search, X } from "lucide-vue-next";
import type {
  ClashConnection,
  ConnectionsSnapshot,
  ProxyMap,
} from "@/services/clash";
import {
  activityInitial,
  connectionSource,
  connectionTarget,
  rememberConnections,
} from "@/utils/controlActivity";
import { formatBytes } from "@/utils/traffic";
import ControlProxyGroups from "./ControlProxyGroups.vue";

const props = defineProps<{
  isRunning: boolean;
  snapshot: ConnectionsSnapshot | null;
  proxies: ProxyMap | null;
  connectionError: string;
  proxyError: string;
}>();
const emit = defineEmits<{
  refresh: [];
  notify: [message: string, type: "success" | "error" | "info"];
}>();
type ActivityTab = "connections" | "requests";
const activeTab = ref<ActivityTab>("connections");
const history = ref<ClashConnection[]>([]);
const search = ref("");
const searchOpen = ref(false);
const searchInput = ref<HTMLInputElement | null>(null);
const searchToggle = ref<HTMLButtonElement | null>(null);
const activeIDs = computed(
  () => new Set(props.snapshot?.connections.map((item) => item.id)),
);
const connections = computed(() =>
  activeTab.value === "requests"
    ? history.value
    : props.snapshot?.connections || [],
);
const filteredConnections = computed(() => {
  const keyword = search.value.trim().toLowerCase();
  if (!keyword) return connections.value;
  return connections.value.filter((connection) =>
    [
      connectionSource(connection),
      connection.metadata.processPath,
      connection.metadata.sourceIP,
      connectionTarget(connection),
      connection.metadata.destinationIP,
      ...(connection.chains || []),
      connection.rule,
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase()
      .includes(keyword),
  );
});
const tabs = computed(() => [
  {
    id: "connections" as const,
    label: "实时连接",
    count: props.snapshot?.connections.length ?? "—",
    title: "当前活动连接",
  },
  {
    id: "requests" as const,
    label: "最近请求",
    count: history.value.length,
    title: "本页最近观测到的连接，保留 200 条",
  },
]);
const emptyMessage = computed(() =>
  !props.isRunning
    ? "核心未运行"
    : connections.value.length
      ? "没有匹配的连接"
      : activeTab.value === "requests"
        ? "等待新的连接记录"
        : "暂无活动连接",
);
const timeLabel = (value: string | number) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "—"
    : date.toLocaleTimeString("zh-CN", { hour12: false });
};

function closeSearch() {
  searchOpen.value = false;
  search.value = "";
  searchToggle.value?.focus();
}
async function toggleSearch() {
  if (searchOpen.value) {
    closeSearch();
    return;
  }
  searchOpen.value = true;
  await nextTick();
  searchInput.value?.focus();
}
function navigateTabs(event: KeyboardEvent, id: ActivityTab) {
  const keys = ["ArrowLeft", "ArrowRight", "Home", "End"];
  if (!keys.includes(event.key)) return;
  event.preventDefault();
  const index = tabs.value.findIndex((tab) => tab.id === id);
  const next =
    event.key === "Home"
      ? 0
      : event.key === "End"
        ? tabs.value.length - 1
        : (index + (event.key === "ArrowRight" ? 1 : -1) + tabs.value.length) %
          tabs.value.length;
  activeTab.value = tabs.value[next].id;
  document.getElementById(`control-tab-${activeTab.value}`)?.focus();
}

watch(
  () => props.snapshot,
  (snapshot) => {
    if (snapshot)
      history.value = rememberConnections(history.value, snapshot.connections);
  },
);
watch(
  () => props.isRunning,
  (running) => {
    if (running) history.value = [];
  },
  { immediate: true },
);
</script>

<template>
  <div class="grid items-stretch gap-4 md:grid-cols-2">
    <section
      class="aw-card flex h-[320px] min-w-0 flex-col rounded-[var(--radius-xl)] bg-[var(--bg-surface)] p-4"
      aria-label="连接记录"
    >
      <div class="flex h-8 shrink-0 items-center gap-2">
        <div
          class="flex min-w-0 flex-1 gap-1 overflow-x-auto"
          role="tablist"
          aria-label="连接记录"
        >
          <button
            v-for="tab in tabs"
            :id="`control-tab-${tab.id}`"
            :key="tab.id"
            role="tab"
            :aria-selected="activeTab === tab.id"
            :aria-controls="`control-activity-${tab.id}`"
            :tabindex="activeTab === tab.id ? 0 : -1"
            :title="tab.title"
            class="flex h-8 shrink-0 items-center gap-1.5 rounded-full bg-[var(--bg-inset)] px-3 text-xs font-semibold text-[var(--text-secondary)]"
            :class="
              activeTab === tab.id &&
              '!bg-[var(--button-primary-bg)] !text-[var(--button-primary-text)]'
            "
            @click="activeTab = tab.id"
            @keydown="navigateTabs($event, tab.id)"
          >
            <span class="tabular-nums">{{ tab.count }}</span
            >{{ tab.label }}
          </button>
        </div>
        <input
          v-if="searchOpen"
          id="control-connection-search"
          ref="searchInput"
          v-model="search"
          type="search"
          class="h-8 w-2/5 min-w-0 max-w-48 shrink-0"
          placeholder="搜索连接"
          aria-label="筛选连接记录"
          @keydown.esc.stop="closeSearch"
        />
        <button
          ref="searchToggle"
          type="button"
          class="focus-ring flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-[var(--text-secondary)] transition-colors hover:bg-[var(--button-secondary-hover)]"
          :aria-label="searchOpen ? '收起连接搜索' : '搜索连接'"
          :title="searchOpen ? '收起连接搜索' : '搜索连接'"
          :aria-expanded="searchOpen"
          aria-controls="control-connection-search"
          @click="toggleSearch"
        >
          <X v-if="searchOpen" :size="16" />
          <Search v-else :size="16" />
        </button>
      </div>
      <div
        :id="`control-activity-${activeTab}`"
        :aria-labelledby="`control-tab-${activeTab}`"
        role="tabpanel"
        class="min-h-0 flex-1 overflow-auto"
      >
        <div
          v-if="connectionError"
          role="alert"
          class="mb-2 flex items-center justify-between gap-2 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] p-2 text-xs text-[var(--color-error)]"
        >
          <span>{{ connectionError }}</span
          ><button class="shrink-0 underline" @click="emit('refresh')">
            重试
          </button>
        </div>
        <table
          class="aw-activity-table w-full min-w-[500px] table-fixed text-left text-[11px]"
        >
          <thead>
            <tr>
              <th class="w-[23%]">来源</th>
              <th class="w-[32%]">目标</th>
              <th class="w-[19%]">出口</th>
              <th class="w-[13%] text-right">下载</th>
              <th class="w-[13%] text-right">上传</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="connection in filteredConnections"
              :key="connection.id"
              :class="
                activeTab === 'requests' &&
                !activeIDs.has(connection.id) &&
                'text-[var(--text-tertiary)]'
              "
            >
              <td
                :title="
                  [
                    connection.metadata.processPath,
                    connection.metadata.sourceIP,
                    timeLabel(connection.start),
                  ]
                    .filter(Boolean)
                    .join(' · ')
                "
              >
                <div class="flex min-w-0 items-center gap-2">
                  <span
                    class="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-[var(--button-secondary-bg)] text-[10px] font-bold"
                    >{{ activityInitial(connectionSource(connection)) }}</span
                  ><span class="truncate">{{
                    connectionSource(connection)
                  }}</span>
                </div>
              </td>
              <td :title="connectionTarget(connection)">
                <div class="truncate">{{ connectionTarget(connection) }}</div>
              </td>
              <td
                :title="
                  [...(connection.chains || [])].reverse().join(' › ') ||
                  connection.rule
                "
              >
                <div class="truncate">{{ connection.chains?.[0] || "—" }}</div>
              </td>
              <td class="text-right tabular-nums">
                {{ formatBytes(connection.download || 0) }}
              </td>
              <td class="text-right tabular-nums">
                {{ formatBytes(connection.upload || 0) }}
              </td>
            </tr>
            <tr v-if="!filteredConnections.length">
              <td
                colspan="5"
                class="h-44 text-center text-[var(--text-tertiary)]"
              >
                {{ connectionError ? "连接记录暂不可用" : emptyMessage }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <ControlProxyGroups
      :is-running="isRunning"
      :proxies="proxies"
      :error="proxyError"
      @refresh="emit('refresh')"
      @notify="(message, type) => emit('notify', message, type)"
    />
  </div>
</template>

<style scoped>
.aw-activity-table {
  border-collapse: separate;
  border-spacing: 0 3px;
}
.aw-activity-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--bg-inset);
  color: var(--text-secondary);
  padding: 7px 8px;
  font-weight: 600;
}
.aw-activity-table td {
  padding: 6px 8px;
}
.aw-activity-table tbody tr:nth-child(even) {
  background: var(--bg-inset);
}
.aw-activity-table th:first-child,
.aw-activity-table td:first-child {
  border-radius: 10px 0 0 10px;
}
.aw-activity-table th:last-child,
.aw-activity-table td:last-child {
  border-radius: 0 10px 10px 0;
}
</style>
