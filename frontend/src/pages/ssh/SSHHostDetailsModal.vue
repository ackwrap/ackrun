<script setup lang="ts">
import { computed, ref } from "vue";
import {
  Clock3,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import type { SSHDeviceDetails, SSHHost } from "@/services/sshTypes";
import SSHSingboxDeployPanel from "./SSHSingboxDeployPanel.vue";

const props = defineProps<{
  host: SSHHost | null;
  details: SSHDeviceDetails | null;
  loading: boolean;
  error: string;
}>();
const emit = defineEmits<{ close: []; retry: [] }>();
const deploymentBusy = ref(false);

const memoryUsed = computed(() =>
  props.details
    ? Math.max(
        0,
        props.details.memory_total_bytes - props.details.memory_available_bytes,
      )
    : 0,
);
const swapUsed = computed(() =>
  props.details
    ? Math.max(
        0,
        props.details.swap_total_bytes - props.details.swap_available_bytes,
      )
    : 0,
);

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 0) return "--";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount.toFixed(unit >= 3 ? 2 : unit ? 1 : 0)} ${units[unit]}`;
}

function formatUptime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return "--";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days) return `${days} 天 ${hours} 小时`;
  if (hours) return `${hours} 小时 ${minutes} 分钟`;
  return `${minutes} 分钟`;
}

function connectionLabel(host: SSHHost) {
  return host.connection_mode === "direct"
    ? "直连"
    : host.node_exposure_name || "节点入口不可用";
}

function valueOrDash(value?: string | number) {
  return value === undefined || value === null || value === "" ? "--" : value;
}
</script>

<template>
  <Modal
    :open="Boolean(host)"
    :title="`设备详情 · ${host?.name || ''}`"
    :width="1480"
    :closable="!deploymentBusy"
    @close="emit('close')"
  >
    <div v-if="host" class="space-y-5">
      <section
        class="grid gap-3 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 sm:grid-cols-2 lg:grid-cols-4"
      >
        <div>
          <p class="text-xs text-[var(--text-tertiary)]">连接地址</p>
          <p class="mt-1 truncate font-mono text-xs" :title="`${host.username}@${host.host}:${host.port}`">
            {{ host.username }}@{{ host.host }}:{{ host.port }}
          </p>
        </div>
        <div>
          <p class="text-xs text-[var(--text-tertiary)]">连接路径</p>
          <p class="mt-1 text-sm">{{ connectionLabel(host) }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--text-tertiary)]">凭据 / 终端</p>
          <p class="mt-1 text-sm">{{ host.credential_name }} · {{ host.terminal_type }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--text-tertiary)]">连接状态</p>
          <div class="mt-1">
            <StatusBadge
              :status="host.last_status === 'available' ? 'online' : host.last_status === 'unavailable' ? 'error' : 'offline'"
              :label="host.last_status === 'available' ? `${host.last_latency_ms} ms` : host.last_status === 'unavailable' ? '不可用' : '未测试'"
              size="sm"
            />
          </div>
        </div>
      </section>

      <div
        v-if="loading"
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] py-14 text-center text-sm text-[var(--text-secondary)]"
      >
        <RefreshCw :size="22" class="mx-auto mb-3 animate-spin text-[var(--color-primary)]" />
        正在通过 SSH 读取设备参数...
      </div>

      <div
        v-else-if="error"
        class="rounded-[var(--radius-lg)] border border-[var(--color-error)] bg-[var(--color-error-bg)] p-5 text-sm"
      >
        <p role="alert" class="text-[var(--color-error)]">{{ error }}</p>
        <Button class="mt-4" size="sm" @click="emit('retry')">
          <template #icon><RefreshCw :size="13" /></template>重新读取
        </Button>
      </div>

      <template v-else-if="details">
        <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="metric-card">
            <Cpu :size="18" class="text-[var(--color-primary)]" />
            <div class="min-w-0"><p>处理器</p><strong :title="details.cpu_model">{{ valueOrDash(details.cpu_model) }}</strong><small>{{ details.cpu_cores || '--' }} 核心</small></div>
          </div>
          <div class="metric-card">
            <MemoryStick :size="18" class="text-[var(--color-success)]" />
            <div><p>内存</p><strong>{{ formatBytes(memoryUsed) }} / {{ formatBytes(details.memory_total_bytes) }}</strong><small>可用 {{ formatBytes(details.memory_available_bytes) }}</small></div>
          </div>
          <div class="metric-card">
            <Clock3 :size="18" class="text-[var(--color-info)]" />
            <div><p>运行时间</p><strong>{{ formatUptime(details.uptime_seconds) }}</strong><small>负载 {{ details.load_average_1.toFixed(2) }} / {{ details.load_average_5.toFixed(2) }} / {{ details.load_average_15.toFixed(2) }}</small></div>
          </div>
          <div class="metric-card">
            <HardDrive :size="18" class="text-[var(--color-warning)]" />
            <div><p>交换空间</p><strong>{{ details.swap_total_bytes ? `${formatBytes(swapUsed)} / ${formatBytes(details.swap_total_bytes)}` : '未配置' }}</strong><small>{{ details.swap_total_bytes ? `可用 ${formatBytes(details.swap_available_bytes)}` : '未启用 Swap' }}</small></div>
          </div>
        </section>
      </template>

      <SSHSingboxDeployPanel
        :host="host"
        :details="details"
        :details-error="error"
        :details-loading="loading"
        @busy="deploymentBusy = $event"
        @refresh="emit('retry')"
      />
    </div>
  </Modal>
</template>

<style scoped>
.metric-card p {
  font-size: 0.7rem;
  color: var(--text-tertiary);
}
.metric-card {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 1rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-surface);
}
.metric-card strong {
  display: block;
  max-width: 100%;
  margin-top: 0.25rem;
  overflow: hidden;
  font-size: 0.8rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-card small {
  display: block;
  margin-top: 0.2rem;
  font-size: 0.65rem;
  color: var(--text-tertiary);
}
</style>
