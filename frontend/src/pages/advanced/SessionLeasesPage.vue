<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Clock3, Pencil, Plus, RefreshCw, Trash2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type {
  PlatformRoute,
  SessionLease,
  SessionLeaseRequest,
} from "@/services/advancedTypes";
import {
  errorMessage,
  dateTimeInput,
  formatTime,
  healthBadge,
  routeRefFields,
  routeRefKey,
  statusLabel,
  targetRef,
} from "./advancedUi";
import { useAdvancedOptions } from "./useAdvancedOptions";

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });

interface LeaseForm {
  name: string;
  enabled: boolean;
  client_cidr: string;
  inbound_ids: number[];
  platform_route_id: number;
  outboundKey: string;
  fallbackKey: string;
  expires_at: string;
}

const leases = ref<SessionLease[]>([]);
const routes = ref<PlatformRoute[]>([]);
const settingsDuration = ref(60);
const loading = ref(true);
const saving = ref(false);
const renewing = ref(false);
const formOpen = ref(false);
const editing = ref<SessionLease | null>(null);
const renewItem = ref<SessionLease | null>(null);
const renewMinutes = ref(60);
const deleting = ref<SessionLease | null>(null);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const form = reactive<LeaseForm>(emptyForm());
const { nodes, collections, exposures, loadOptions, targetName } =
  useAdvancedOptions();

const counts = computed(() => ({
  active: leases.value.filter((item) => leaseStatus(item) === "active").length,
  expiring: leases.value.filter((item) => leaseStatus(item) === "expiring").length,
  expired: leases.value.filter((item) => leaseStatus(item) === "expired").length,
}));

function emptyForm(): LeaseForm {
  return {
    name: "",
    enabled: true,
    client_cidr: "",
    inbound_ids: [],
    platform_route_id: 0,
    outboundKey: "direct",
    fallbackKey: "direct",
    expires_at: dateTimeInput(Date.now() + settingsDuration.value * 60_000),
  };
}

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  try {
    const [leaseItems, routeItems] = await Promise.all([
      advancedApi.getSessionLeases(),
      advancedApi.getPlatformRoutes(),
      loadOptions(),
    ]);
    leases.value = leaseItems;
    routes.value = routeItems;
  } catch (error) {
    show(`加载会话租约失败: ${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, emptyForm());
  if (exposures.value.length) form.inbound_ids = [exposures.value[0].id];
  if (routes.value.length) form.platform_route_id = routes.value[0].id;
  formOpen.value = true;
}

function openEdit(item: SessionLease) {
  editing.value = item;
  Object.assign(form, {
    name: item.name || "",
    enabled: item.enabled,
    client_cidr: item.client_cidr,
    inbound_ids: [...(item.inbound_exposure_ids || [])],
    platform_route_id: item.platform_route_id || 0,
    outboundKey: routeRefKey(targetRef(item)),
    fallbackKey: routeRefKey(targetRef(item, true)),
    expires_at: dateTimeInput(item.expires_at),
  });
  formOpen.value = true;
}

function closeForm() {
  if (!saving.value) formOpen.value = false;
}

function closeRenew() {
  if (!renewing.value) renewItem.value = null;
}

function payload(): SessionLeaseRequest {
  const target = routeRefFields(form.outboundKey);
  const fallback = routeRefFields(form.fallbackKey);
  return {
    name: form.name.trim(),
    enabled: form.enabled,
    client_cidr: form.client_cidr.trim(),
    inbound_exposure_ids: form.inbound_ids.map(Number),
    platform_route_id: form.platform_route_id || undefined,
    target_type: target.type,
    target_subscription_id: target.subscriptionID,
    target_node_uid: target.nodeUID,
    target_collection_id: target.collectionID,
    fallback_type: fallback.type,
    fallback_subscription_id: fallback.subscriptionID,
    fallback_node_uid: fallback.nodeUID,
    fallback_collection_id: fallback.collectionID,
    expires_at: new Date(form.expires_at).getTime(),
  };
}

async function save() {
  if (saving.value) return;
  if (!form.client_cidr.trim()) return show("请输入来源 CIDR", "error");
  if (!form.inbound_ids.length || !form.platform_route_id) {
    return show("请选择至少一个入口和平台规则", "error");
  }
  if (!form.expires_at || new Date(form.expires_at).getTime() <= Date.now()) {
    return show("到期时间必须晚于当前时间", "error");
  }
  saving.value = true;
  try {
    if (editing.value) await advancedApi.updateSessionLease(editing.value.id, payload());
    else await advancedApi.createSessionLease(payload());
    show(editing.value ? "会话租约已更新" : "会话租约已创建");
    formOpen.value = false;
    await load();
  } catch (error) {
    show(`保存会话租约失败: ${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

function openRenew(item: SessionLease) {
  renewItem.value = item;
  renewMinutes.value = settingsDuration.value;
}

async function renew() {
  if (!renewItem.value || renewing.value) return;
  const minutes = Number(renewMinutes.value);
  if (!Number.isInteger(minutes) || minutes < 1 || minutes > 525600) {
    return show("续租时长必须是 1 到 525600 的整数", "error");
  }
  renewing.value = true;
  try {
    await advancedApi.renewSessionLease(renewItem.value.id, minutes);
    show("会话租约已续期");
    renewItem.value = null;
    await load();
  } catch (error) {
    show(`续租失败: ${errorMessage(error)}`, "error");
  } finally {
    renewing.value = false;
  }
}

async function remove() {
  if (!deleting.value) return;
  const item = deleting.value;
  deleting.value = null;
  try {
    await advancedApi.deleteSessionLease(item.id);
    show(leaseStatus(item) === "expired" ? "过期租约已删除" : "会话租约已撤销");
    await load();
  } catch (error) {
    show(`处理租约失败: ${errorMessage(error)}`, "error");
  }
}

function outboundLabel(item: SessionLease) {
  return targetName(routeRefKey(targetRef(item)));
}

function leaseStatus(item: SessionLease) {
  if (!item.enabled) return "disabled";
  if (item.expires_at <= Date.now()) return "expired";
  if (item.expires_at <= Date.now() + 15 * 60_000) return "expiring";
  return item.status || "active";
}

function inboundLabel(item: SessionLease) {
  const names = (item.inbound_exposure_ids || [])
    .map((id) => exposures.value.find((entry) => entry.id === id)?.name)
    .filter(Boolean);
  return names.length ? names.join("、") : "--";
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <PageHeader title="会话租约" description="将来源网段在租期内稳定绑定到指定入口和出口，并关联业务平台标签。" :embedded="embedded">
      <template #actions><Button variant="primary" :disabled="loading || !exposures.length || !routes.length" @click="openCreate"><template #icon><Plus :size="15" /></template>创建租约</Button></template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <div class="grid gap-3 sm:grid-cols-3">
      <Card v-for="item in [['生效中', counts.active], ['即将到期', counts.expiring], ['已到期', counts.expired]]" :key="String(item[0])" padding="sm"><p class="text-xs text-[var(--text-secondary)]">{{ item[0] }}</p><p class="mt-2 text-2xl font-semibold">{{ item[1] }}</p></Card>
    </div>

    <Card padding="none">
      <div class="border-b border-[var(--border-light)] px-5 py-4"><h2 class="text-sm font-semibold">租约列表</h2><p class="mt-1 text-xs text-[var(--text-tertiary)]">撤销生效租约与删除过期记录都会调用后端删除接口。</p></div>
      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!leases.length" class="p-10 text-center"><Clock3 :size="30" class="mx-auto text-[var(--text-tertiary)]" /><p class="mt-3 text-sm">暂无会话租约</p></div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[980px]">
          <thead><tr><th>来源 / 状态</th><th>入口</th><th>平台规则</th><th>出口</th><th>到期时间</th><th class="text-right">操作</th></tr></thead>
          <tbody><tr v-for="item in leases" :key="item.id">
            <td><p class="font-mono text-xs">{{ item.client_cidr || '--' }}</p><StatusBadge class="mt-1" :status="healthBadge(leaseStatus(item))" :label="statusLabel(leaseStatus(item))" size="sm" /></td>
            <td class="max-w-52 truncate" :title="inboundLabel(item)">{{ inboundLabel(item) }}</td>
            <td>{{ routes.find((route) => route.id === item.platform_route_id)?.name || (item.platform_route_id ? `规则 #${item.platform_route_id}` : '--') }}</td>
            <td>{{ outboundLabel(item) }}</td>
            <td class="text-xs"><p>{{ formatTime(item.expires_at) }}</p><p v-if="item.name" class="mt-1 text-[var(--text-tertiary)]">{{ item.name }}</p></td>
            <td><div class="flex justify-end gap-2"><button class="aw-control-action" @click="openEdit(item)"><Pencil :size="13" />编辑</button><button class="aw-control-action" @click="openRenew(item)"><RefreshCw :size="13" />续租</button><button class="aw-control-action aw-action-danger" @click="deleting = item"><Trash2 :size="13" />{{ leaseStatus(item) === 'expired' ? '删除' : '撤销' }}</button></div></td>
          </tr></tbody>
        </table>
      </div>
    </Card>

    <Modal :open="formOpen" :title="editing ? '编辑会话租约' : '创建会话租约'" size="lg" @close="closeForm">
      <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
        <fieldset class="contents" :disabled="saving">
        <label><span class="aw-modal-label text-xs">来源 CIDR</span><input v-model="form.client_cidr" class="aw-input mt-1 w-full font-mono" placeholder="192.168.1.20/32" /></label>
        <label><span class="aw-modal-label text-xs">备注（可选）</span><input v-model="form.name" class="aw-input mt-1 w-full" maxlength="100" placeholder="办公终端" /></label>
        <fieldset class="sm:col-span-2"><legend class="aw-modal-label text-xs">入口暴露（可多选）</legend><div class="mt-2 grid max-h-36 gap-2 overflow-y-auto rounded-lg border border-[var(--border-default)] p-3 sm:grid-cols-2"><label v-for="entry in exposures" :key="entry.id" class="flex items-center gap-2 text-xs"><input v-model="form.inbound_ids" type="checkbox" :value="entry.id" />{{ entry.name }} · {{ entry.inbound_type.toUpperCase() }}</label></div></fieldset>
        <label class="sm:col-span-2"><span class="aw-modal-label text-xs">平台规则</span><select v-model.number="form.platform_route_id" class="aw-input mt-1 w-full"><option :value="0" disabled>请选择</option><option v-for="route in routes" :key="route.id" :value="route.id">{{ route.name }}</option></select></label>
        <label v-for="kind in ['outboundKey', 'fallbackKey'] as const" :key="kind"><span class="aw-modal-label text-xs">{{ kind === 'outboundKey' ? '固定出口' : '失败回退' }}</span><select v-model="form[kind]" class="aw-input mt-1 w-full"><optgroup label="内置类型"><option value="direct">直连</option></optgroup><optgroup label="节点"><option v-for="node in nodes" :key="node.value" :value="node.value">{{ node.name }} · {{ node.type }}</option></optgroup><optgroup label="策略组"><option v-for="group in collections" :key="group.id" :value="`collection:${group.id}`">{{ group.name }} · {{ group.type }}</option></optgroup></select></label>
        <label class="sm:col-span-2"><span class="aw-modal-label text-xs">到期时间</span><input v-model="form.expires_at" class="aw-input mt-1 w-full" type="datetime-local" /></label>
        <label class="sm:col-span-2 flex items-center gap-2 rounded-lg border border-[var(--border-default)] p-3 text-sm"><input v-model="form.enabled" type="checkbox" />保存后启用该租约</label>
        </fieldset>
      </form>
      <template #footer><Button :disabled="saving" @click="closeForm">取消</Button><Button variant="primary" :loading="saving" @click="save">保存</Button></template>
    </Modal>

    <Modal :open="!!renewItem" title="续租" size="sm" @close="closeRenew"><label><span class="aw-modal-label text-xs">续租时长（分钟）</span><input v-model.number="renewMinutes" class="aw-input mt-1 w-full" type="number" min="1" max="525600" :disabled="renewing" /></label><p class="mt-3 text-xs text-[var(--text-tertiary)]">后端确认成功后才会刷新到期时间。</p><template #footer><Button :disabled="renewing" @click="closeRenew">取消</Button><Button variant="primary" :loading="renewing" @click="renew">确认续租</Button></template></Modal>
    <ConfirmDialog :open="!!deleting" :title="deleting && leaseStatus(deleting) === 'expired' ? '删除过期租约' : '撤销会话租约'" :message="`确定处理来源 ${deleting?.client_cidr || '--'} 的租约吗？`" :confirm-text="deleting && leaseStatus(deleting) === 'expired' ? '删除' : '撤销'" danger @confirm="remove" @cancel="deleting = null" />
  </div>
</template>
