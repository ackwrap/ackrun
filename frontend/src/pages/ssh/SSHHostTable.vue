<script setup lang="ts">
import { computed } from "vue";
import {
  Info,
  Pencil,
  Play,
  Route,
  ServerCog,
  Share2,
  SquareTerminal,
  Trash2,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import type { SSHHost } from "@/services/sshTypes";
import { formatTime } from "../advanced/advancedUi";

const props = defineProps<{
  hosts: SSHHost[];
  loading: boolean;
  testingId: number;
  openingShellId: number;
  loadingDetailsId: number;
  detailsMode: "details" | "singbox" | null;
  shellOpen: boolean;
  selectedIds: number[];
}>();
const emit = defineEmits<{
  "update:selectedIds": [ids: number[]];
  test: [host: SSHHost];
  shell: [host: SSHHost];
  terminal: [host: SSHHost];
  details: [host: SSHHost];
  deploySingbox: [host: SSHHost];
  edit: [host: SSHHost];
  share: [host: SSHHost];
  delete: [host: SSHHost];
  showHostKey: [host: SSHHost];
}>();

const selected = computed(() => new Set(props.selectedIds));
const allSelected = computed(
  () => props.hosts.length > 0 && props.hosts.every((host) => selected.value.has(host.id)),
);
const partiallySelected = computed(
  () => !allSelected.value && props.hosts.some((host) => selected.value.has(host.id)),
);

function toggleAll() {
  emit("update:selectedIds", allSelected.value ? [] : props.hosts.map((host) => host.id));
}

function toggleHost(host: SSHHost) {
  const next = new Set(selected.value);
  if (next.has(host.id)) next.delete(host.id);
  else next.add(host.id);
  emit("update:selectedIds", [...next]);
}

function hostStatus(host: SSHHost) {
  if (!host.enabled) return { status: "offline" as const, label: "已停用" };
  if (host.last_status === "available") {
    return { status: "online" as const, label: `${host.last_latency_ms} ms` };
  }
  if (host.last_status === "host_key_pending") {
    return { status: "pending" as const, label: "待信任" };
  }
  if (host.last_status === "unavailable") {
    return { status: "error" as const, label: "不可用" };
  }
  return { status: "offline" as const, label: "未测试" };
}

function connectionLabel(host: SSHHost) {
  return host.connection_mode === "direct"
    ? "直连"
    : host.node_exposure_name || "节点入口不可用";
}
</script>

<template>
  <Card padding="none">
    <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">
      加载中...
    </div>
    <div
      v-else-if="!hosts.length"
      class="p-12 text-center text-sm text-[var(--text-secondary)]"
    >
      <SquareTerminal :size="34" class="mx-auto mb-3 text-[var(--text-tertiary)]" />
      暂无 SSH 主机。新增主机时可直接创建并选择登录凭据。
    </div>
    <div v-else class="aw-data-table-wrap rounded-none border-0">
      <table class="aw-data-table min-w-[1280px]">
        <thead>
          <tr>
            <th class="w-10">
              <input
                type="checkbox"
                class="h-3.5 w-3.5 accent-[var(--color-primary)]"
                :checked="allSelected"
                :indeterminate="partiallySelected"
                aria-label="选择全部 SSH 主机"
                @change="toggleAll"
              />
            </th>
            <th>主机</th>
            <th>连接路径</th>
            <th>凭据 / Host Key</th>
            <th>状态</th>
            <th>最近检查</th>
            <th class="text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="host in hosts" :key="host.id">
            <td @click.stop>
              <input
                type="checkbox"
                class="h-3.5 w-3.5 accent-[var(--color-primary)]"
                :checked="selected.has(host.id)"
                :aria-label="`选择 ${host.name}`"
                @change="toggleHost(host)"
              />
            </td>
            <td>
              <p class="font-medium">{{ host.name }}</p>
              <p class="mt-1 font-mono text-xs text-[var(--text-tertiary)]">
                {{ host.username }}@{{ host.host }}:{{ host.port }}
              </p>
              <div v-if="host.tags?.length" class="mt-2 flex flex-wrap gap-1">
                <span
                  v-for="tag in host.tags"
                  :key="tag"
                  class="rounded-full bg-[var(--color-primary-bg)] px-2 py-0.5 text-[10px] text-[var(--color-primary-hover)]"
                >{{ tag }}</span>
              </div>
            </td>
            <td>
              <div class="flex items-center gap-2 text-sm">
                <Route :size="14" class="text-[var(--text-tertiary)]" />
                {{ connectionLabel(host) }}
              </div>
            </td>
            <td>
              <p class="text-sm">{{ host.credential_name }}</p>
              <button
                v-if="host.host_key_status === 'trusted'"
                class="mt-1 text-xs text-[var(--color-success)] hover:underline"
                @click="$emit('showHostKey', host)"
              >
                Host Key 已信任
              </button>
              <p v-else class="mt-1 text-xs text-[var(--color-warning)]">Host Key 待确认</p>
            </td>
            <td>
              <StatusBadge
                :status="hostStatus(host).status"
                :label="hostStatus(host).label"
                size="sm"
              />
              <p
                v-if="host.last_error_message"
                class="mt-2 max-w-56 truncate text-xs text-[var(--color-error)]"
                :title="host.last_error_message"
              >
                {{ host.last_error_message }}
              </p>
            </td>
            <td class="text-xs text-[var(--text-secondary)]">
              {{ formatTime(host.last_checked_at) }}
            </td>
            <td>
              <div class="flex justify-end gap-1">
                <Button
                  size="sm"
                  :loading="detailsMode === 'details' && loadingDetailsId === host.id"
                  :disabled="
                    !host.enabled ||
                    testingId > 0 ||
                    loadingDetailsId > 0 ||
                    openingShellId > 0
                  "
                  title="查看设备详情"
                  @click="$emit('details', host)"
                >
                  <template #icon><Info :size="13" /></template>详情
                </Button>
                <Button
                  size="sm"
                  :loading="detailsMode === 'singbox' && loadingDetailsId === host.id"
                  :disabled="
                    !host.enabled ||
                    testingId > 0 ||
                    loadingDetailsId > 0 ||
                    openingShellId > 0
                  "
                  title="打开 sing-box 服务端部署"
                  @click="$emit('deploySingbox', host)"
                >
                  <template #icon><ServerCog :size="13" /></template>sing-box
                </Button>
                <Button
                  size="sm"
                  :loading="testingId === host.id"
                  :disabled="
                    !host.enabled || openingShellId > 0 || loadingDetailsId > 0
                  "
                  title="测试连接"
                  @click="$emit('test', host)"
                >
                  <template #icon><Play :size="13" /></template>测试
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  :disabled="
                    !host.enabled ||
                    testingId > 0 ||
                    openingShellId > 0 ||
                    loadingDetailsId > 0 ||
                    shellOpen
                  "
                  :loading="openingShellId === host.id"
                  title="在当前页面打开临时 Shell"
                  @click="$emit('shell', host)"
                >
                  <template #icon><SquareTerminal :size="13" /></template>Shell
                </Button>
                <Button
                  size="sm"
                  :disabled="
                    !host.enabled ||
                    testingId > 0 ||
                    openingShellId > 0 ||
                    loadingDetailsId > 0
                  "
                  title="在新标签页打开 SSH 与 SFTP 工作台"
                  @click="$emit('terminal', host)"
                >
                  <template #icon><SquareTerminal :size="13" /></template>工作台
                </Button>
                <Button size="sm" variant="ghost" @click="$emit('edit', host)">
                  <template #icon><Pencil :size="13" /></template>
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  title="加密分享主机"
                  @click="$emit('share', host)"
                >
                  <template #icon><Share2 :size="13" /></template>
                </Button>
                <Button size="sm" variant="ghost" @click="$emit('delete', host)">
                  <template #icon><Trash2 :size="13" /></template>
                </Button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </Card>
</template>
