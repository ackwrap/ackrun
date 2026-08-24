<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Clock3,
  Cpu,
  HardDrive,
  MemoryStick,
  UserRound,
} from "lucide-vue-next";
import { sshApi } from "@/services/sshApi";
import type {
  SSHMonitorDisk,
  SSHMonitorSnapshot,
  SSHSessionCreateResponse,
} from "@/services/sshTypes";

const props = defineProps<{
  session: SSHSessionCreateResponse;
  hostName: string;
  active: boolean;
}>();

const snapshot = ref<SSHMonitorSnapshot | null>(null);
const error = ref("");
const cpuPercent = ref<number | null>(null);
const receiveRate = ref<number | null>(null);
const transmitRate = ref<number | null>(null);
let previous: SSHMonitorSnapshot | null = null;
let timer = 0;
let generation = 0;
let requestController: AbortController | null = null;

const memoryText = computed(() => {
  const current = snapshot.value;
  if (!current) return "--";
  const used = Math.max(
    0,
    current.memory_total_bytes - current.memory_available_bytes,
  );
  return `${formatBytes(used)} / ${formatBytes(current.memory_total_bytes)}`;
});

const identityText = computed(() => {
  const current = snapshot.value;
  if (!current) return "--";
  return `${current.username || "--"} (${current.login_sessions})`;
});

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 0) return "--";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  const digits = unit >= 3 ? 2 : unit > 0 ? 1 : 0;
  return `${amount.toFixed(digits)} ${units[unit]}`;
}

function formatRate(value: number | null) {
  return value === null ? "--" : `${formatBytes(value)}/s`;
}

function formatUptime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return "--";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days) return `${days}天 ${hours}小时`;
  if (hours) return `${hours}小时 ${minutes}分钟`;
  return `${minutes}分钟`;
}

function diskLabel(disk: SSHMonitorDisk) {
  return `${disk.mount_point}: ${disk.usage_percent}%`;
}

function updateRates(next: SSHMonitorSnapshot) {
  if (!previous) {
    cpuPercent.value = null;
    receiveRate.value = null;
    transmitRate.value = null;
    return;
  }
  const elapsed = (next.collected_at - previous.collected_at) / 1000;
  const totalDelta = next.cpu_total - previous.cpu_total;
  const idleDelta = next.cpu_idle - previous.cpu_idle;
  if (totalDelta > 0 && idleDelta >= 0 && idleDelta <= totalDelta) {
    cpuPercent.value = Math.max(
      0,
      Math.min(100, ((totalDelta - idleDelta) / totalDelta) * 100),
    );
  } else {
    cpuPercent.value = null;
  }
  if (
    elapsed > 0 &&
    next.network_received_bytes >= previous.network_received_bytes &&
    next.network_transmitted_bytes >= previous.network_transmitted_bytes
  ) {
    receiveRate.value =
      (next.network_received_bytes - previous.network_received_bytes) / elapsed;
    transmitRate.value =
      (next.network_transmitted_bytes - previous.network_transmitted_bytes) /
      elapsed;
  } else {
    receiveRate.value = null;
    transmitRate.value = null;
  }
}

function schedulePoll(currentGeneration: number, delay: number) {
  if (currentGeneration !== generation || !props.active) return;
  timer = window.setTimeout(() => void poll(currentGeneration), delay);
}

async function poll(currentGeneration: number) {
  if (
    currentGeneration !== generation ||
    !props.active ||
    document.visibilityState === "hidden"
  ) {
    return;
  }
  const controller = new AbortController();
  requestController = controller;
  try {
    const next = await sshApi.getSessionMonitor(
      props.session.session_id,
      props.session.sftp_token,
      controller.signal,
    );
    if (currentGeneration !== generation) return;
    updateRates(next);
    previous = next;
    snapshot.value = next;
    error.value = "";
    schedulePoll(currentGeneration, 3000);
  } catch (cause) {
    if (controller.signal.aborted || currentGeneration !== generation) return;
    error.value = cause instanceof Error ? cause.message : "读取远端监控失败";
    previous = null;
    cpuPercent.value = null;
    receiveRate.value = null;
    transmitRate.value = null;
    schedulePoll(currentGeneration, 5000);
  } finally {
    if (requestController === controller) requestController = null;
  }
}

function restartPolling(resetSnapshot = false) {
  requestController?.abort();
  requestController = null;
  generation += 1;
  window.clearTimeout(timer);
  previous = null;
  cpuPercent.value = null;
  receiveRate.value = null;
  transmitRate.value = null;
  if (resetSnapshot) snapshot.value = null;
  error.value = "";
  if (props.active && document.visibilityState !== "hidden") {
    void poll(generation);
  }
}

function handleVisibilityChange() {
  restartPolling(false);
}

watch(
  [() => props.session.session_id, () => props.active],
  ([sessionID], [previousSessionID]) => {
    restartPolling(sessionID !== previousSessionID);
  },
  { immediate: true },
);
document.addEventListener("visibilitychange", handleVisibilityChange);

onBeforeUnmount(() => {
  requestController?.abort();
  requestController = null;
  generation += 1;
  window.clearTimeout(timer);
  document.removeEventListener("visibilitychange", handleVisibilityChange);
});
</script>

<template>
  <div
    class="flex h-8 shrink-0 items-stretch overflow-x-auto border-t border-[var(--border-default)] bg-[var(--bg-elevated)] text-[11px] text-[var(--text-secondary)]"
    :title="error || '远端 Linux 指标，每 3 秒刷新一次'"
    role="status"
    aria-live="off"
  >
    <div class="ssh-monitor-item font-medium text-[var(--text-primary)]">
      <Activity
        :size="13"
        :class="error ? 'text-[var(--color-error)]' : 'text-[var(--color-success)]'"
      />
      <span class="max-w-40 truncate">{{ snapshot?.hostname || hostName }}</span>
    </div>
    <div v-if="error && !snapshot" class="ssh-monitor-item text-[var(--color-error)]">
      监控不可用
    </div>
    <template v-else>
      <div class="ssh-monitor-item" title="CPU 使用率">
        <Cpu :size="13" class="text-[var(--color-primary)]" />
        {{ cpuPercent === null ? "--" : `${cpuPercent.toFixed(1)}%` }}
      </div>
      <div class="ssh-monitor-item" title="已用内存 / 总内存">
        <MemoryStick :size="13" class="text-[var(--color-success)]" />
        {{ memoryText }}
      </div>
      <div class="ssh-monitor-item" title="实时上传速率">
        <ArrowUp :size="13" class="text-[var(--color-success)]" />
        {{ formatRate(transmitRate) }}
      </div>
      <div class="ssh-monitor-item" title="实时下载速率">
        <ArrowDown :size="13" class="text-[var(--color-primary)]" />
        {{ formatRate(receiveRate) }}
      </div>
      <div class="ssh-monitor-item" title="远端主机运行时间">
        <Clock3 :size="13" class="text-[var(--color-info)]" />
        {{ snapshot ? formatUptime(snapshot.uptime_seconds) : "--" }}
      </div>
      <div class="ssh-monitor-item" title="登录用户和会话数">
        <UserRound :size="13" class="text-[var(--color-warning)]" />
        {{ identityText }}
      </div>
      <div
        v-for="disk in snapshot?.disks || []"
        :key="disk.mount_point"
        class="ssh-monitor-item"
        :title="`${disk.mount_point} 已用 ${formatBytes(disk.used_bytes)} / ${formatBytes(disk.total_bytes)}`"
      >
        <HardDrive :size="13" class="text-[var(--text-tertiary)]" />
        {{ diskLabel(disk) }}
      </div>
    </template>
  </div>
</template>

<style scoped>
.ssh-monitor-item {
  display: flex;
  flex: none;
  align-items: center;
  gap: 0.35rem;
  padding: 0 0.65rem;
  white-space: nowrap;
  border-right: 1px solid var(--border-default);
}
</style>
