<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Pencil, Plus, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type {
  AlertChannel,
  AlertRule,
  AlertRuleRequest,
} from "@/services/advancedTypes";
import AlertRuleFormModal from "./AlertRuleFormModal.vue";
import { errorMessage } from "./advancedUi";
import { alertEventLabel } from "./alertUi";

const rules = ref<AlertRule[]>([]);
const channels = ref<AlertChannel[]>([]);
const loading = ref(true);
const saving = ref(false);
const formOpen = ref(false);
const editing = ref<AlertRule | null>(null);
const deleting = ref<AlertRule | null>(null);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");

const channelByID = computed(() => new Map(channels.value.map((item) => [item.id, item])));

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  try {
    [rules.value, channels.value] = await Promise.all([
      advancedApi.getAlertRules(),
      advancedApi.getAlertChannels(),
    ]);
  } catch (error) {
    show(`加载告警规则失败: ${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  formOpen.value = true;
}

function openEdit(item: AlertRule) {
  editing.value = item;
  formOpen.value = true;
}

async function save(request: AlertRuleRequest) {
  if (saving.value) return;
  saving.value = true;
  try {
    if (editing.value) await advancedApi.updateAlertRule(editing.value.id, request);
    else await advancedApi.createAlertRule(request);
    show(editing.value ? "告警规则已更新" : "告警规则已创建");
    formOpen.value = false;
    await load();
  } catch (error) {
    show(`保存告警规则失败: ${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function toggle(item: AlertRule) {
  try {
    await advancedApi.updateAlertRule(item.id, {
      name: item.name,
      enabled: !item.enabled,
      event_types: item.event_types,
      channel_ids: item.channel_ids,
      cooldown_minutes: item.cooldown_minutes,
    });
    await load();
  } catch (error) {
    show(`更新规则状态失败: ${errorMessage(error)}`, "error");
  }
}

async function remove() {
  if (!deleting.value) return;
  const item = deleting.value;
  deleting.value = null;
  try {
    await advancedApi.deleteAlertRule(item.id);
    show("告警规则已删除");
    await load();
  } catch (error) {
    show(`删除告警规则失败: ${errorMessage(error)}`, "error");
  }
}

function channelNames(item: AlertRule) {
  return item.channel_ids.map((id) => channelByID.value.get(id)?.name || `已删除渠道 #${id}`);
}

onMounted(load);
</script>

<template>
  <div class="space-y-4">
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div><h2 class="text-base font-semibold">事件规则</h2><p class="mt-1 text-xs text-[var(--text-secondary)]">定义哪些事件推送给哪些通知渠道</p></div>
      <Button variant="primary" :disabled="loading || !channels.length" @click="openCreate"><template #icon><Plus :size="14" /></template>新建规则</Button>
    </div>

    <Card padding="none">
      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!rules.length" class="p-10 text-center text-sm text-[var(--text-tertiary)]">{{ channels.length ? "暂无告警规则" : "请先在“渠道”中创建通知渠道" }}</div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[900px]">
          <thead><tr><th>规则 / 状态</th><th>事件</th><th>目标渠道</th><th>冷却期</th><th class="text-right">操作</th></tr></thead>
          <tbody>
            <tr v-for="item in rules" :key="item.id">
              <td><p class="font-medium">{{ item.name }}</p><button class="mt-1" @click="toggle(item)"><StatusBadge :status="item.enabled ? 'online' : 'offline'" :label="item.enabled ? '已启用' : '已停用'" size="sm" /></button></td>
              <td><div class="flex max-w-72 flex-wrap gap-1"><span v-for="event in item.event_types" :key="event" class="aw-filter-chip">{{ alertEventLabel(event) }}</span></div></td>
              <td><p class="max-w-80 text-sm">{{ channelNames(item).join("、") || "--" }}</p></td>
              <td>{{ item.cooldown_minutes ? `${item.cooldown_minutes} 分钟` : "不冷却" }}</td>
              <td><div class="flex justify-end gap-2"><button class="aw-control-action" @click="openEdit(item)"><Pencil :size="13" />编辑</button><button class="aw-control-action aw-action-danger" @click="deleting = item"><Trash2 :size="13" />删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </Card>

    <AlertRuleFormModal :open="formOpen" :editing="editing" :channels="channels" :saving="saving" @close="formOpen = false" @save="save" />
    <ConfirmDialog :open="!!deleting" title="删除告警规则" :message="`确定删除「${deleting?.name || ''}」吗？此规则的冷却状态也会清除。`" confirm-text="删除" danger @confirm="remove" @cancel="deleting = null" />
  </div>
</template>
