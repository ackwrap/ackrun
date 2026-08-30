<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Pencil, Plus, Send, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type {
  AlertChannel,
  AlertChannelRequest,
} from "@/services/advancedTypes";
import AlertChannelFormModal from "./AlertChannelFormModal.vue";
import { errorMessage, formatTime, healthBadge } from "./advancedUi";
import { alertChannelLabel, deliveryStatusLabel } from "./alertUi";

const channels = ref<AlertChannel[]>([]);
const loading = ref(true);
const saving = ref(false);
const testing = ref(new Set<number>());
const formOpen = ref(false);
const editing = ref<AlertChannel | null>(null);
const deleting = ref<AlertChannel | null>(null);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");

const enabledCount = computed(() => channels.value.filter((item) => item.enabled).length);

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  try {
    channels.value = await advancedApi.getAlertChannels();
  } catch (error) {
    show(`加载告警渠道失败: ${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  formOpen.value = true;
}

function openEdit(item: AlertChannel) {
  editing.value = item;
  formOpen.value = true;
}

async function save(request: AlertChannelRequest) {
  if (saving.value) return;
  saving.value = true;
  try {
    if (editing.value) await advancedApi.updateAlertChannel(editing.value.id, request);
    else await advancedApi.createAlertChannel(request);
    show(editing.value ? "告警渠道已更新" : "告警渠道已创建");
    formOpen.value = false;
    await load();
  } catch (error) {
    show(`保存告警渠道失败: ${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function toggle(item: AlertChannel) {
  try {
    await advancedApi.updateAlertChannel(item.id, {
      name: item.name,
      type: item.type,
      enabled: !item.enabled,
      config: item.config,
      secret: "",
    });
    await load();
  } catch (error) {
    show(`更新渠道状态失败: ${errorMessage(error)}`, "error");
  }
}

async function test(item: AlertChannel) {
  if (testing.value.has(item.id)) return;
  testing.value = new Set(testing.value).add(item.id);
  try {
    await advancedApi.testAlertChannel(item.id);
    show(`「${item.name}」测试发送成功`);
  } catch (error) {
    show(`测试发送失败: ${errorMessage(error)}`, "error");
  } finally {
    const next = new Set(testing.value);
    next.delete(item.id);
    testing.value = next;
    await load();
  }
}

async function remove() {
  if (!deleting.value) return;
  const item = deleting.value;
  deleting.value = null;
  try {
    await advancedApi.deleteAlertChannel(item.id);
    show("告警渠道已删除");
    await load();
  } catch (error) {
    show(`删除告警渠道失败: ${errorMessage(error)}`, "error");
  }
}

onMounted(load);
</script>

<template>
  <div class="space-y-4">
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold">通知渠道</h2>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">{{ enabledCount }} / {{ channels.length }} 个渠道已启用</p>
      </div>
      <Button variant="primary" :disabled="loading" @click="openCreate"><template #icon><Plus :size="14" /></template>新建渠道</Button>
    </div>

    <Card padding="none">
      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!channels.length" class="p-10 text-center text-sm text-[var(--text-tertiary)]">暂无告警渠道</div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[940px]">
          <thead><tr><th>渠道</th><th>类型 / 目标</th><th>投递状态</th><th>最近投递</th><th class="text-right">操作</th></tr></thead>
          <tbody>
            <tr v-for="item in channels" :key="item.id">
              <td><p class="font-medium">{{ item.name }}</p><button class="mt-1 text-xs text-[var(--color-primary)]" @click="toggle(item)">{{ item.enabled ? "停用" : "启用" }}</button></td>
              <td><p>{{ alertChannelLabel(item.type) }}</p><p class="mt-1 max-w-64 truncate text-xs text-[var(--text-tertiary)]" :title="item.destination">{{ item.destination || "--" }}</p></td>
              <td><StatusBadge :status="healthBadge(item.last_status)" :label="deliveryStatusLabel(item.last_status)" size="sm" /><p v-if="item.last_error" class="mt-1 max-w-72 truncate text-xs text-[var(--color-error)]" :title="item.last_error">{{ item.last_error }}</p></td>
              <td class="whitespace-nowrap text-xs">{{ formatTime(item.last_delivered_at) }}</td>
              <td><div class="flex justify-end gap-2"><button class="aw-control-action" :disabled="testing.has(item.id)" @click="test(item)"><Send :size="13" />{{ testing.has(item.id) ? "发送中" : "测试" }}</button><button class="aw-control-action" @click="openEdit(item)"><Pencil :size="13" />编辑</button><button class="aw-control-action aw-action-danger" @click="deleting = item"><Trash2 :size="13" />删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </Card>

    <AlertChannelFormModal :open="formOpen" :editing="editing" :saving="saving" @close="formOpen = false" @save="save" />
    <ConfirmDialog :open="!!deleting" title="删除告警渠道" :message="`确定删除「${deleting?.name || ''}」吗？引用该渠道且没有其他目标的规则会自动停用。`" confirm-text="删除" danger @confirm="remove" @cancel="deleting = null" />
  </div>
</template>
