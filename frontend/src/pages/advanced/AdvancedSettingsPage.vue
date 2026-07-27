<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { Activity, KeyRound, Route, Save, ScrollText, ShieldCheck } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type { AdvancedSettings } from "@/services/advancedTypes";
import { errorMessage } from "./advancedUi";

const safeDefaults: AdvancedSettings = {
  routing_enabled: false,
  leases_enabled: false,
  health_enabled: false,
  access_logs_enabled: false,
  health_interval_seconds: 60,
  health_timeout_seconds: 5,
  failure_threshold: 3,
  recovery_threshold: 2,
  circuit_open_seconds: 300,
  access_log_retention_days: 7,
  access_log_max_entries: 10000,
  access_log_privacy_mode: "strict",
};

const form = reactive<AdvancedSettings>({ ...safeDefaults });
const loading = ref(true);
const loaded = ref(false);
const saving = ref(false);
const saveError = ref("");
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  loaded.value = false;
  try {
    Object.assign(form, await advancedApi.getSettings());
    loaded.value = true;
  } catch (error) {
    show(`加载高级设置失败: ${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function validate() {
  const inRange = (value: number, minimum: number, maximum: number) =>
    Number.isInteger(value) && value >= minimum && value <= maximum;
  if (!inRange(form.health_interval_seconds, 1, 86400)) return "探测周期必须是 1 到 86400 的整数";
  if (!inRange(form.health_timeout_seconds, 1, 300)) return "探测超时必须是 1 到 300 的整数";
  if (!inRange(form.failure_threshold, 1, 100) || !inRange(form.recovery_threshold, 1, 100)) {
    return "健康失败和恢复阈值必须是 1 到 100 的整数";
  }
  if (!inRange(form.circuit_open_seconds, 1, 86400)) return "熔断时长必须是 1 到 86400 的整数";
  if (!inRange(form.access_log_retention_days, 1, 3650)) return "日志保留天数必须是 1 到 3650 的整数";
  if (!inRange(form.access_log_max_entries, 1, 1_000_000)) return "日志最大条数必须是 1 到 1000000 的整数";
  return "";
}

async function save() {
  if (!loaded.value || saving.value) return;
  saveError.value = validate();
  if (saveError.value) return show(saveError.value, "error");
  saving.value = true;
  try {
    const updated = await advancedApi.updateSettings({ ...form });
    Object.assign(form, updated);
    saveError.value = "";
    show("高级设置已保存");
  } catch (error) {
    saveError.value = `保存失败: ${errorMessage(error)}`;
    show(saveError.value, "error");
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <PageHeader title="高级设置" description="集中配置平台路由、租约、健康调度和访问审计。">
      <template #actions><Button variant="primary" :loading="saving" :disabled="loading || !loaded" @click="save"><template #icon><Save :size="14" /></template>保存设置</Button></template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <Card>
      <div class="flex gap-3"><ShieldCheck :size="20" class="shrink-0 text-[var(--color-success)]" /><div><h2 class="text-sm font-semibold">安全默认值</h2><p class="mt-1 text-xs leading-5 text-[var(--text-secondary)]">首次启用前四项能力默认关闭；探测使用有限超时和连续失败阈值；访问日志默认严格隐私并仅保留 7 天。页面加载失败时禁止保存，避免用本地默认值覆盖服务端配置。</p></div></div>
    </Card>

    <div v-if="saveError" role="alert" class="rounded-[var(--radius-lg)] border border-[var(--color-error)] bg-[var(--bg-surface)] p-3 text-sm text-[var(--color-error)]">{{ saveError }}</div>
    <div v-if="loading" class="py-12 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
    <template v-else>
      <div class="grid gap-4 xl:grid-cols-2">
        <Card>
          <div class="flex items-center gap-2"><Route :size="18" class="text-[var(--color-primary)]" /><h2 class="text-sm font-semibold">平台路由</h2></div>
           <label class="mt-4 flex items-start gap-3 rounded-lg border border-[var(--border-default)] p-3"><input v-model="form.routing_enabled" class="mt-0.5" type="checkbox" :disabled="saving" /><span><span class="block text-sm font-medium">启用平台路由</span><span class="mt-1 block text-xs text-[var(--text-secondary)]">关闭时不执行高级多维路由规则。</span></span></label>
        </Card>

        <Card>
          <div class="flex items-center gap-2"><KeyRound :size="18" class="text-[var(--color-primary)]" /><h2 class="text-sm font-semibold">会话租约</h2></div>
          <label class="mt-4 flex items-start gap-3 rounded-lg border border-[var(--border-default)] p-3"><input v-model="form.leases_enabled" class="mt-0.5" type="checkbox" :disabled="saving" /><span><span class="block text-sm font-medium">启用会话租约</span><span class="mt-1 block text-xs text-[var(--text-secondary)]">关闭后已保存租约不参与运行时出口选择。</span></span></label>
        </Card>

        <Card>
          <div class="flex items-center gap-2"><Activity :size="18" class="text-[var(--color-primary)]" /><h2 class="text-sm font-semibold">健康调度</h2></div>
          <label class="mt-4 flex items-start gap-3 rounded-lg border border-[var(--border-default)] p-3"><input v-model="form.health_enabled" class="mt-0.5" type="checkbox" :disabled="saving" /><span><span class="block text-sm font-medium">启用定时健康探测</span><span class="mt-1 block text-xs text-[var(--text-secondary)]">达到连续阈值后才切换健康状态，减少瞬时波动。</span></span></label>
          <div class="mt-4 grid gap-3 sm:grid-cols-2"><label><span class="text-xs text-[var(--text-secondary)]">探测周期（秒）</span><input v-model.number="form.health_interval_seconds" class="aw-input mt-1 w-full" type="number" min="1" max="86400" :disabled="saving" /></label><label><span class="text-xs text-[var(--text-secondary)]">超时（秒）</span><input v-model.number="form.health_timeout_seconds" class="aw-input mt-1 w-full" type="number" min="1" max="300" :disabled="saving" /></label><label><span class="text-xs text-[var(--text-secondary)]">失败阈值</span><input v-model.number="form.failure_threshold" class="aw-input mt-1 w-full" type="number" min="1" max="100" :disabled="saving" /></label><label><span class="text-xs text-[var(--text-secondary)]">恢复阈值</span><input v-model.number="form.recovery_threshold" class="aw-input mt-1 w-full" type="number" min="1" max="100" :disabled="saving" /></label><label class="sm:col-span-2"><span class="text-xs text-[var(--text-secondary)]">熔断时长（秒）</span><input v-model.number="form.circuit_open_seconds" class="aw-input mt-1 w-full" type="number" min="1" max="86400" :disabled="saving" /></label></div>
        </Card>

        <Card>
          <div class="flex items-center gap-2"><ScrollText :size="18" class="text-[var(--color-primary)]" /><h2 class="text-sm font-semibold">访问日志</h2></div>
          <label class="mt-4 flex items-start gap-3 rounded-lg border border-[var(--border-default)] p-3"><input v-model="form.access_logs_enabled" class="mt-0.5" type="checkbox" :disabled="saving" /><span><span class="block text-sm font-medium">启用访问审计</span><span class="mt-1 block text-xs text-[var(--text-secondary)]">关闭时核心停止采集并清空内存事件；严格模式在跨进程传输前移除来源、目标和域名。</span></span></label>
          <div class="mt-4 grid gap-3 sm:grid-cols-2"><label><span class="text-xs text-[var(--text-secondary)]">保留天数</span><input v-model.number="form.access_log_retention_days" class="aw-input mt-1 w-full" type="number" min="1" max="3650" :disabled="saving" /></label><label><span class="text-xs text-[var(--text-secondary)]">最大条数</span><input v-model.number="form.access_log_max_entries" class="aw-input mt-1 w-full" type="number" min="1" max="1000000" :disabled="saving" /></label><label class="sm:col-span-2"><span class="text-xs text-[var(--text-secondary)]">隐私模式</span><select v-model="form.access_log_privacy_mode" class="aw-input mt-1 w-full" :disabled="saving"><option value="strict">严格（推荐）</option><option value="balanced">平衡</option></select></label></div>
        </Card>
      </div>
      <div class="flex justify-end"><Button variant="primary" :loading="saving" :disabled="!loaded || saving" @click="save"><template #icon><Save :size="14" /></template>保存设置</Button></div>
    </template>
  </div>
</template>
