<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Search, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Pagination from "@/components/ui/Pagination.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type { AlertChannel, AlertDelivery } from "@/services/advancedTypes";
import { errorMessage, formatTime } from "./advancedUi";
import { alertChannelLabel, alertEventLabel } from "./alertUi";

const items = ref<AlertDelivery[]>([]);
const channels = ref<AlertChannel[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(25);
const loading = ref(true);
const clearing = ref(false);
const clearOpen = ref(false);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const filters = reactive({ channelID: "", eventType: "", status: "" });
const applied = reactive({ channelID: "", eventType: "", status: "" });
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
let loadVersion = 0;

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  const version = ++loadVersion;
  loading.value = true;
  try {
    const [result, channelItems] = await Promise.all([
      advancedApi.getAlertDeliveries({
        page: page.value,
        page_size: pageSize.value,
        channel_id: applied.channelID ? Number(applied.channelID) : undefined,
        event_type: applied.eventType,
        status: applied.status as "" | "success" | "failed",
      }),
      advancedApi.getAlertChannels(),
    ]);
    if (version !== loadVersion) return;
    items.value = result.items || [];
    total.value = result.total || 0;
    page.value = result.page || page.value;
    pageSize.value = result.page_size || pageSize.value;
    channels.value = channelItems;
  } catch (error) {
    if (version === loadVersion) show(`加载投递记录失败: ${errorMessage(error)}`, "error");
  } finally {
    if (version === loadVersion) loading.value = false;
  }
}

function search() {
  Object.assign(applied, filters);
  page.value = 1;
  void load();
}

function reset() {
  Object.assign(filters, { channelID: "", eventType: "", status: "" });
  search();
}

function changePage(value: number) {
  page.value = value;
  void load();
}

function changePageSize(value: number) {
  pageSize.value = value;
  page.value = 1;
  void load();
}

async function clearLogs() {
  if (clearing.value) return;
  clearing.value = true;
  try {
    await advancedApi.clearAlertDeliveries();
    clearOpen.value = false;
    show("投递记录已清空");
    page.value = 1;
    await load();
  } catch (error) {
    show(`清空投递记录失败: ${errorMessage(error)}`, "error");
  } finally {
    clearing.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="space-y-4">
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div><h2 class="text-base font-semibold">投递记录</h2><p class="mt-1 text-xs text-[var(--text-secondary)]">保留测试和事件通知的投递结果与失败原因</p></div>
      <Button variant="danger" :disabled="loading || !total" @click="clearOpen = true"><template #icon><Trash2 :size="14" /></template>清空记录</Button>
    </div>

    <Card padding="none">
      <form class="grid gap-3 border-b border-[var(--border-light)] p-4 sm:grid-cols-2 xl:grid-cols-[200px_180px_160px_auto]" @submit.prevent="search">
        <label><span class="text-xs text-[var(--text-secondary)]">渠道</span><select v-model="filters.channelID" class="aw-input mt-1 w-full"><option value="">全部渠道</option><option v-for="channel in channels" :key="channel.id" :value="String(channel.id)">{{ channel.name }}</option></select></label>
        <label><span class="text-xs text-[var(--text-secondary)]">事件</span><select v-model="filters.eventType" class="aw-input mt-1 w-full"><option value="">全部事件</option><option value="circuit_open">熔断</option><option value="recovered">恢复</option><option value="subscription_failed">订阅失败</option><option value="test">测试发送</option></select></label>
        <label><span class="text-xs text-[var(--text-secondary)]">状态</span><select v-model="filters.status" class="aw-input mt-1 w-full"><option value="">全部状态</option><option value="success">成功</option><option value="failed">失败</option></select></label>
        <div class="flex items-end gap-2"><Button type="submit" variant="primary"><template #icon><Search :size="14" /></template>筛选</Button><Button @click="reset">重置</Button></div>
      </form>

      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!items.length" class="p-10 text-center text-sm text-[var(--text-tertiary)]">没有符合条件的投递记录</div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[980px]">
          <thead><tr><th>时间</th><th>渠道</th><th>事件</th><th>状态码</th><th>结果</th><th>失败原因</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td class="whitespace-nowrap text-xs">{{ formatTime(item.delivered_at) }}</td>
              <td><p>{{ item.channel_name }}</p><p class="mt-1 text-xs text-[var(--text-tertiary)]">{{ alertChannelLabel(item.channel_type) }}</p></td>
              <td><p>{{ alertEventLabel(item.event_type) }}</p><p class="mt-1 max-w-64 truncate text-xs text-[var(--text-tertiary)]" :title="item.event_title">{{ item.event_title || "--" }}</p></td>
              <td>{{ item.status_code || "--" }}</td>
              <td><StatusBadge :status="item.success ? 'online' : 'error'" :label="item.success ? '成功' : '失败'" size="sm" /></td>
              <td><p class="max-w-80 truncate text-xs" :class="item.error ? 'text-[var(--color-error)]' : 'text-[var(--text-tertiary)]'" :title="item.error">{{ item.error || "--" }}</p></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="px-4 pb-4"><Pagination :total="total" :page="page" :page-size="pageSize" :total-pages="totalPages" @page-change="changePage" @page-size-change="changePageSize" /></div>
    </Card>

    <ConfirmDialog :open="clearOpen" title="清空投递记录" message="确定清空全部告警投递记录吗？渠道状态不会被重置。" confirm-text="清空" danger @confirm="clearLogs" @cancel="clearOpen = false" />
  </div>
</template>
