<script setup lang="ts">
import { ref } from "vue";
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  DatabaseZap,
  Eraser,
  FileDown,
  ListX,
  RefreshCw,
  ShieldCheck,
  XCircle,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type { MaintenanceCheckResponse } from "@/services/types";

const props = defineProps<{
  isRunning: boolean;
  platform?: string;
  disabled: boolean;
}>();
const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
  busy: [label: string];
}>();
const running = ref("");
const confirm = ref<"firewall" | "logs" | null>(null);
const report = ref<MaintenanceCheckResponse | null>(null);
const actions = [
  {
    label: "关闭连接",
    icon: ShieldCheck,
    action: api.closeConnections,
    requiresCore: true,
  },
  {
    label: "清理核心 DNS",
    icon: DatabaseZap,
    action: api.flushCoreDNS,
    requiresCore: true,
  },
  {
    label: "清理 FakeIP",
    icon: Eraser,
    action: api.flushFakeIP,
    requiresCore: true,
  },
];
async function run(action: () => Promise<unknown>, label: string) {
  if (running.value || props.disabled) return;
  running.value = label;
  emit("busy", label);
  try {
    await action();
    if (action === checkNetwork && report.value && !report.value.success)
      emit("notify", "网络自检发现需要处理的项目", "info");
    else emit("notify", `${label}完成`);
  } catch (error: any) {
    emit("notify", `${label}失败: ${error.message}`, "error");
  } finally {
    running.value = "";
    emit("busy", "");
  }
}
async function checkNetwork() {
  report.value = await api.networkCheck();
}
async function exportDiagnostics() {
  const data = await api.getDiagnostics();
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(data, null, 2)], {
      type: "application/json;charset=utf-8",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = `ackwrap-diagnostics-${new Date(data.generated_at).toISOString().replace(/[:.]/g, "-")}.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}
function confirmAction() {
  const kind = confirm.value;
  confirm.value = null;
  if (kind === "firewall") void run(api.resetFirewall, "重置防火墙");
  else if (kind === "logs") void run(api.clearCoreLogs, "清理核心日志");
}
</script>

<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <h3 class="mb-4 text-sm font-semibold">网络维护</h3>
    <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
      <Button
        v-for="item in actions"
        :key="item.label"
        size="sm"
        :disabled="disabled || !!running || (item.requiresCore && !isRunning)"
        @click="run(item.action, item.label)"
        ><component :is="item.icon" :size="13" />{{ item.label }}</Button
      >
      <Button
        size="sm"
        :disabled="disabled || !!running"
        @click="run(checkNetwork, '网络自检')"
        ><Activity :size="13" />网络自检</Button
      >
      <Button
        size="sm"
        :disabled="
          disabled ||
          !!running ||
          !['windows', 'linux'].includes(platform || '')
        "
        @click="run(api.flushDNS, '清理系统 DNS')"
        ><RefreshCw :size="13" />清理系统 DNS</Button
      >
      <Button
        size="sm"
        :disabled="disabled || !!running"
        @click="confirm = 'logs'"
        ><ListX :size="13" />清理核心日志</Button
      >
      <Button
        size="sm"
        :disabled="disabled || !!running"
        @click="run(exportDiagnostics, '导出诊断')"
        ><FileDown :size="13" />导出诊断</Button
      >
      <Button
        size="sm"
        :disabled="disabled || !!running || platform !== 'windows'"
        @click="confirm = 'firewall'"
        ><AlertTriangle :size="13" />重置防火墙</Button
      >
    </div>
    <p
      v-if="running"
      role="status"
      class="mt-3 text-xs text-[var(--text-secondary)]"
    >
      {{ running }}中…
    </p>
    <ConfirmDialog
      :open="!!confirm"
      :title="confirm === 'firewall' ? '重置 Windows 防火墙' : '清理核心日志'"
      :message="
        confirm === 'firewall'
          ? '此操作会清除并恢复系统防火墙规则，可能影响其他应用的网络访问。确认继续吗？'
          : '此操作会清空当前内存中的核心日志，且无法恢复。确认继续吗？'
      "
      danger
      @cancel="confirm = null"
      @confirm="confirmAction"
    />
    <Modal
      :open="!!report"
      title="网络自检结果"
      size="md"
      @close="report = null"
    >
      <div class="space-y-2">
        <div
          v-for="check in report?.checks || []"
          :key="check.key"
          class="flex items-start gap-3 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
        >
          <CheckCircle2
            v-if="check.status === 'pass'"
            :size="17"
            class="mt-0.5 shrink-0 text-[var(--color-success)]"
          />
          <AlertCircle
            v-else-if="check.status === 'warn'"
            :size="17"
            class="mt-0.5 shrink-0 text-[var(--color-warning)]"
          />
          <XCircle
            v-else
            :size="17"
            class="mt-0.5 shrink-0 text-[var(--color-error)]"
          />
          <div>
            <div class="font-medium">{{ check.label }}</div>
            <p class="mt-0.5 text-xs text-[var(--text-secondary)]">
              {{ check.message }}
            </p>
          </div>
        </div>
      </div>
    </Modal>
  </section>
</template>
