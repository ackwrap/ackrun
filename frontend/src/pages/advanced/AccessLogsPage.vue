<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Search, ShieldCheck, Trash2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Pagination from "@/components/ui/Pagination.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type { AdvancedAccessLog } from "@/services/advancedTypes";
import {
  errorMessage,
  formatTime,
  healthBadge,
  statusLabel,
} from "./advancedUi";

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });

const items = ref<AdvancedAccessLog[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(25);
const loading = ref(true);
const clearing = ref(false);
const clearOpen = ref(false);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const filters = reactive({ platform: "", decision: "", keyword: "" });
const applied = reactive({ platform: "", decision: "", keyword: "" });
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
    const result = await advancedApi.getAccessLogs({
      page: page.value,
      page_size: pageSize.value,
      platform: applied.platform,
      decision: applied.decision,
      keyword: applied.keyword,
    });
    if (version !== loadVersion) return;
    items.value = result.items || [];
    total.value = result.total || 0;
    page.value = result.page || page.value;
    pageSize.value = result.page_size || pageSize.value;
  } catch (error) {
    if (version !== loadVersion) return;
    show(`加载访问日志失败: ${errorMessage(error)}`, "error");
  } finally {
    if (version === loadVersion) loading.value = false;
  }
}

function search() {
  Object.assign(applied, {
    platform: filters.platform.trim(),
    decision: filters.decision,
    keyword: filters.keyword.trim(),
  });
  page.value = 1;
  void load();
}

function reset() {
  Object.assign(filters, { platform: "", decision: "", keyword: "" });
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
    await advancedApi.clearAccessLogs();
    clearOpen.value = false;
    show("访问日志已清空");
    page.value = 1;
    await load();
  } catch (error) {
    show(`清空访问日志失败: ${errorMessage(error)}`, "error");
  } finally {
    clearing.value = false;
  }
}

function logStatus(item: AdvancedAccessLog) {
  if (item.error_summary) return "error";
  return item.decision || "unknown";
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <PageHeader title="访问审计" description="审计高级入口的路由决策，不展示节点连接参数。" :embedded="embedded">
      <template #actions><Button variant="danger" :disabled="loading || !total" @click="clearOpen = true"><template #icon><Trash2 :size="14" /></template>清空日志</Button></template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <Card>
      <div class="flex gap-3"><ShieldCheck :size="20" class="shrink-0 text-[var(--color-success)]" /><div><h2 class="text-sm font-semibold">隐私保护</h2><p class="mt-1 text-xs leading-5 text-[var(--text-secondary)]">本页仅显示业务平台、来源哈希、目标摘要、入口、命中规则和出口名称等审计元数据；不会读取或展示 server、端口、UUID、密码、密钥及原始节点配置。字段被隐私策略移除时统一显示“--”。</p></div></div>
    </Card>

    <Card padding="none">
      <form class="grid gap-3 border-b border-[var(--border-light)] p-4 sm:grid-cols-2 xl:grid-cols-[180px_180px_minmax(220px,1fr)_auto]" @submit.prevent="search">
        <label><span class="text-xs text-[var(--text-secondary)]">平台</span><input v-model="filters.platform" class="aw-input mt-1 w-full" placeholder="全部平台" /></label>
        <label><span class="text-xs text-[var(--text-secondary)]">决策</span><select v-model="filters.decision" class="aw-input mt-1 w-full"><option value="">全部决策</option><option value="route">平台路由</option><option value="route_fallback">平台路由回退</option><option value="route_failed">平台路由失败</option><option value="lease">租约路由</option><option value="lease_fallback">租约路由回退</option><option value="lease_failed">租约路由失败</option><option value="node_exposure">入口固定路由</option><option value="node_exposure_failed">入口固定路由失败</option></select></label>
        <label><span class="text-xs text-[var(--text-secondary)]">关键词</span><input v-model="filters.keyword" class="aw-input mt-1 w-full" placeholder="来源、入口、规则或目标名称" /></label>
        <div class="flex items-end gap-2"><Button type="submit" variant="primary"><template #icon><Search :size="14" /></template>筛选</Button><Button @click="reset">重置</Button></div>
      </form>

      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!items.length" class="p-10 text-center text-sm text-[var(--text-tertiary)]">没有符合条件的访问日志</div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[1040px]">
          <thead><tr><th>时间</th><th>平台 / 来源摘要</th><th>网络 / 入口</th><th>目标摘要</th><th>决策 / 出口</th><th>状态 / 错误</th></tr></thead>
          <tbody><tr v-for="item in items" :key="item.id"><td class="whitespace-nowrap text-xs">{{ formatTime(item.event_time) }}</td><td><p>{{ item.platform || '--' }}</p><p class="mt-1 font-mono text-xs text-[var(--text-tertiary)]">{{ item.source_hash || '--' }}</p></td><td><p>{{ item.network || '--' }} · {{ item.inbound || '--' }}</p><p class="mt-1 text-xs text-[var(--text-tertiary)]">路由 #{{ item.platform_route_id || '--' }} · 租约 #{{ item.session_lease_id || '--' }}</p></td><td><p class="max-w-60 truncate" :title="item.domain_summary || ''">{{ item.domain_summary || '--' }}</p><p class="mt-1 max-w-60 truncate text-xs text-[var(--text-tertiary)]" :title="item.destination_summary || ''">{{ item.destination_summary || '--' }}</p></td><td><p>{{ statusLabel(item.decision || '') }}</p><p class="mt-1 text-xs text-[var(--text-tertiary)]">{{ item.outbound_label || '--' }}</p></td><td><StatusBadge :status="healthBadge(logStatus(item))" :label="statusLabel(logStatus(item))" size="sm" /><p v-if="item.error_summary" class="mt-1 max-w-64 truncate text-xs text-[var(--color-error)]" :title="item.error_summary">{{ item.error_summary }}</p><p v-else class="mt-1 text-xs text-[var(--text-tertiary)]">--</p></td></tr></tbody>
        </table>
      </div>
      <div class="px-4 pb-4"><Pagination :total="total" :page="page" :page-size="pageSize" :total-pages="totalPages" @page-change="changePage" @page-size-change="changePageSize" /></div>
    </Card>

    <ConfirmDialog :open="clearOpen" title="清空访问日志" message="确定清空全部高级访问日志吗？此操作不可撤销。" confirm-text="清空" danger @confirm="clearLogs" @cancel="clearOpen = false" />
  </div>
</template>
