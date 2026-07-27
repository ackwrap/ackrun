<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Activity, Play, ShieldAlert } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type {
  AdvancedHealthResponse,
  AdvancedHealthEvent,
  AdvancedHealthState,
  AdvancedSettings,
} from "@/services/advancedTypes";
import {
  errorMessage,
  formatTime,
  healthBadge,
  statusLabel,
} from "./advancedUi";

const states = ref<AdvancedHealthState[]>([]);
const events = ref<AdvancedHealthEvent[]>([]);
const settings = ref<Partial<AdvancedSettings>>({});
const loading = ref(true);
const running = ref(false);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
let pollVersion = 0;

const healthy = computed(() => states.value.filter((item) => item.status === "healthy").length);
const unhealthy = computed(
  () =>
    states.value.filter((item) =>
      ["unhealthy", "circuit_open"].includes(item.status),
    ).length,
);
const unknown = computed(() => states.value.length - healthy.value - unhealthy.value);

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

function applySnapshot(response: AdvancedHealthResponse) {
  states.value = response.states || [];
  events.value = response.events || [];
  settings.value = response.settings || {};
  running.value = Boolean(response.running);
}

function wait(milliseconds: number) {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

async function pollUntilSettled(version: number, startedAt = 0) {
  while (version === pollVersion) {
    await wait(1000);
    if (version !== pollVersion) return;
    const response = await advancedApi.getHealthScheduling();
    if (version !== pollVersion) return;
    applySnapshot(response);
    if (!response.running) {
      const failed = response.events?.some(
        (event) =>
          event.target_key === "scheduler" &&
          event.event_type === "probe_failed" &&
          event.created_at >= startedAt,
      );
      if (startedAt) {
        show(failed ? "健康探测执行失败，请查看状态事件" : "健康探测已完成", failed ? "error" : "success");
      }
      return;
    }
  }
}

async function load() {
  loading.value = true;
  try {
    const response = await advancedApi.getHealthScheduling();
    applySnapshot(response);
    if (response.running) {
      const version = ++pollVersion;
      void pollUntilSettled(version).catch((error) => {
        if (version === pollVersion) {
          running.value = false;
          show(`刷新健康调度失败: ${errorMessage(error)}`, "error");
        }
      });
    }
  } catch (error) {
    show(`加载健康调度失败: ${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

async function runNow() {
  if (running.value) return;
  running.value = true;
  const startedAt = Date.now();
  const version = ++pollVersion;
  let started = false;
  try {
    await advancedApi.runHealthScheduling();
    started = true;
    show("健康探测已启动", "info");
    await pollUntilSettled(version, startedAt);
  } catch (error) {
    show(`${started ? "健康探测已启动，但刷新状态失败" : "立即探测失败"}: ${errorMessage(error)}`, "error");
  } finally {
    if (version === pollVersion) running.value = false;
  }
}

onMounted(load);
onBeforeUnmount(() => {
  pollVersion++;
});
</script>

<template>
  <div class="space-y-5">
    <PageHeader title="健康调度" description="查看出口健康状态、状态迁移事件并触发一次真实探测。">
      <template #actions><Button variant="primary" :loading="running" :disabled="loading || !settings.health_enabled" @click="runNow"><template #icon><Play :size="14" /></template>立即探测</Button></template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Card padding="sm"><p class="text-xs text-[var(--text-secondary)]">调度状态</p><StatusBadge class="mt-3" :status="settings.health_enabled ? 'online' : 'offline'" :label="settings.health_enabled ? '已启用' : '已停用'" /></Card>
      <Card padding="sm"><p class="text-xs text-[var(--text-secondary)]">探测周期 / 超时</p><p class="mt-2 text-lg font-semibold">{{ settings.health_interval_seconds ?? '--' }}s / {{ settings.health_timeout_seconds ?? '--' }}s</p></Card>
      <Card padding="sm"><p class="text-xs text-[var(--text-secondary)]">失败 / 恢复阈值</p><p class="mt-2 text-lg font-semibold">{{ settings.failure_threshold ?? '--' }} / {{ settings.recovery_threshold ?? '--' }}</p></Card>
      <Card padding="sm"><p class="text-xs text-[var(--text-secondary)]">健康 / 异常 / 待定</p><p class="mt-2 text-lg font-semibold">{{ healthy }} / {{ unhealthy }} / {{ unknown }}</p></Card>
    </div>

    <Card padding="none">
      <div class="border-b border-[var(--border-light)] px-5 py-4"><h2 class="text-sm font-semibold">Health state</h2><p class="mt-1 text-xs text-[var(--text-tertiary)]">连续成功和失败次数由后端调度器累计。</p></div>
      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!states.length" class="p-10 text-center"><Activity :size="30" class="mx-auto text-[var(--text-tertiary)]" /><p class="mt-3 text-sm">暂无健康状态</p></div>
      <div v-else class="aw-data-table-wrap rounded-none border-0"><table class="aw-data-table min-w-[860px]"><thead><tr><th>对象</th><th>状态</th><th>延迟</th><th>连续失败 / 成功</th><th>最近探测 / 熔断至</th><th>错误</th></tr></thead><tbody><tr v-for="item in states" :key="item.target_key"><td><p class="font-medium">{{ item.display_name || '--' }}</p><p class="mt-1 text-xs text-[var(--text-tertiary)]">{{ item.target_type || 'unknown' }}</p></td><td><StatusBadge :status="healthBadge(item.status)" :label="statusLabel(item.status)" size="sm" /></td><td>{{ item.latency_ms > 0 ? `${item.latency_ms} ms` : '--' }}</td><td>{{ item.consecutive_failures ?? 0 }} / {{ item.consecutive_successes ?? 0 }}</td><td class="text-xs"><p>{{ formatTime(item.last_checked_at) }}</p><p class="mt-1 text-[var(--text-tertiary)]">{{ item.circuit_open_until ? formatTime(item.circuit_open_until) : '--' }}</p></td><td class="max-w-64 text-xs text-[var(--color-error)]"><span :title="item.last_error || ''">{{ item.last_error || '--' }}</span></td></tr></tbody></table></div>
    </Card>

    <Card>
      <div class="flex items-center gap-2"><ShieldAlert :size="17" class="text-[var(--color-warning)]" /><h2 class="text-sm font-semibold">状态事件时间线</h2></div>
      <div v-if="!loading && !events.length" class="py-8 text-center text-sm text-[var(--text-tertiary)]">暂无状态迁移事件</div>
      <ol v-else class="mt-4 space-y-0">
        <li v-for="event in events" :key="event.id" class="relative border-l border-[var(--border-default)] pb-5 pl-5 last:pb-0"><span class="absolute -left-1.5 top-1 h-3 w-3 rounded-full border-2 border-[var(--bg-card)] bg-[var(--color-primary)]" /><div class="flex flex-wrap items-center justify-between gap-2"><p class="text-sm font-medium">{{ event.display_name || '--' }}</p><time class="text-xs text-[var(--text-tertiary)]">{{ formatTime(event.created_at) }}</time></div><p class="mt-1 text-xs text-[var(--text-secondary)]">{{ statusLabel(event.event_type) }}<span v-if="event.latency_ms > 0"> · {{ event.latency_ms }} ms</span><span v-if="event.message"> · {{ event.message }}</span></p></li>
      </ol>
    </Card>
  </div>
</template>
