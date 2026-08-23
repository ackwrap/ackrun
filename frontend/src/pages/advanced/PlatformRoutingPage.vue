<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Pencil, Plus, Route, Trash2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import OrderButtons from "@/components/ui/OrderButtons.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import Toast from "@/components/ui/Toast.vue";
import { advancedApi } from "@/services/advancedApi";
import type { PlatformRoute, PlatformRouteRequest } from "@/services/advancedTypes";
import {
  errorMessage,
  lineText,
  lines,
  routeRefFields,
  routeRefKey,
  targetRef,
} from "./advancedUi";
import { useAdvancedOptions } from "./useAdvancedOptions";

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });

interface RouteForm {
  name: string;
  enabled: boolean;
  platform: string;
  sourceCidrs: string;
  domains: string;
  domainSuffixes: string;
  domainKeywords: string;
  destinationCidrs: string;
  inboundIds: number[];
  targetKey: string;
  fallbackKey: string;
}

const routes = ref<PlatformRoute[]>([]);
const loading = ref(true);
const saving = ref(false);
const ordering = ref(false);
const formOpen = ref(false);
const editing = ref<PlatformRoute | null>(null);
const deleting = ref<PlatformRoute | null>(null);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const platformPattern = /^[\p{L}\p{N}][\p{L}\p{N}_-]*$/u;
const form = reactive<RouteForm>(emptyForm());
const { nodes, collections, exposures, loadOptions, targetName } =
  useAdvancedOptions();

const enabledCount = computed(() => routes.value.filter((item) => item.enabled).length);
const fallbackCount = computed(
  () => routes.value.filter((item) => Boolean(item.fallback_type)).length,
);
const coveredPlatforms = computed(
  () => new Set(routes.value.map((item) => item.platform).filter(Boolean)).size,
);

function emptyForm(): RouteForm {
  return {
    name: "",
    enabled: true,
    platform: "",
    sourceCidrs: "",
    domains: "",
    domainSuffixes: "",
    domainKeywords: "",
    destinationCidrs: "",
    inboundIds: [],
    targetKey: "direct",
    fallbackKey: "direct",
  };
}

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  try {
    const [items] = await Promise.all([advancedApi.getPlatformRoutes(), loadOptions()]);
    routes.value = items;
  } catch (error) {
    show(`加载平台路由失败: ${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, emptyForm());
  formOpen.value = true;
}

function openEdit(item: PlatformRoute) {
  editing.value = item;
  Object.assign(form, {
    name: item.name,
    enabled: item.enabled,
    platform: item.platform,
    sourceCidrs: lineText(item.source_cidrs),
    domains: lineText(item.domains),
    domainSuffixes: lineText(item.domain_suffixes),
    domainKeywords: lineText(item.domain_keywords),
    destinationCidrs: lineText(item.destination_cidrs),
    inboundIds: [...(item.inbound_exposure_ids || [])],
    targetKey: routeRefKey(targetRef(item)),
    fallbackKey: routeRefKey(targetRef(item, true)),
  });
  formOpen.value = true;
}

function closeForm() {
  if (!saving.value) formOpen.value = false;
}

function payload(): PlatformRouteRequest {
  const target = routeRefFields(form.targetKey);
  const fallback = routeRefFields(form.fallbackKey);
  return {
    name: form.name.trim(),
    platform: form.platform.trim(),
    enabled: form.enabled,
    priority: editing.value?.priority || 0,
    inbound_exposure_ids: form.inboundIds.map(Number),
    source_cidrs: lines(form.sourceCidrs),
    domains: lines(form.domains),
    domain_suffixes: lines(form.domainSuffixes),
    domain_keywords: lines(form.domainKeywords),
    destination_cidrs: lines(form.destinationCidrs),
    target_type: target.type,
    target_subscription_id: target.subscriptionID,
    target_node_uid: target.nodeUID,
    target_collection_id: target.collectionID,
    fallback_type: fallback.type,
    fallback_subscription_id: fallback.subscriptionID,
    fallback_node_uid: fallback.nodeUID,
    fallback_collection_id: fallback.collectionID,
  };
}

async function save() {
  if (saving.value) return;
  if (!form.name.trim() || !form.platform.trim()) {
    return show("请输入规则名称和平台标识", "error");
  }
  if (!platformPattern.test(form.platform.trim())) {
    return show("平台标识仅支持字母、数字、下划线和连字符", "error");
  }
  if (Array.from(form.platform.trim()).length > 64) {
    return show("平台标识不能超过 64 个字符", "error");
  }
  if (!form.targetKey) return show("请选择目标出口", "error");
  saving.value = true;
  try {
    if (editing.value) await advancedApi.updatePlatformRoute(editing.value.id, payload());
    else await advancedApi.createPlatformRoute(payload());
    show(editing.value ? "平台路由已更新" : "平台路由已创建");
    formOpen.value = false;
    await load();
  } catch (error) {
    show(`保存平台路由失败: ${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function move(index: number, delta: number) {
  if (ordering.value) return;
  const next = [...routes.value];
  const target = index + delta;
  if (target < 0 || target >= next.length) return;
  [next[index], next[target]] = [next[target], next[index]];
  ordering.value = true;
  let reordered = false;
  try {
    await advancedApi.reorderPlatformRoutes(next.map((item) => item.id));
    reordered = true;
    routes.value = await advancedApi.getPlatformRoutes();
    show("平台路由顺序已更新");
  } catch (error) {
    if (reordered) {
      routes.value = [];
      show(`排序已保存，但刷新优先级失败: ${errorMessage(error)}`, "error");
    } else {
      show(`调整顺序失败: ${errorMessage(error)}`, "error");
    }
  } finally {
    ordering.value = false;
  }
}

async function remove() {
  if (!deleting.value) return;
  const item = deleting.value;
  deleting.value = null;
  try {
    await advancedApi.deletePlatformRoute(item.id);
    show("平台路由已删除");
    await load();
  } catch (error) {
    show(`删除平台路由失败: ${errorMessage(error)}`, "error");
  }
}

function refLabel(item: PlatformRoute, fallback = false) {
  return targetName(routeRefKey(targetRef(item, fallback)));
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <PageHeader title="平台路由" description="按来源、域名、目标网段和入口组合匹配出口，并记录业务平台标签。" :embedded="embedded">
      <template #actions>
        <Button variant="primary" :disabled="loading || ordering" @click="openCreate">
          <template #icon><Plus :size="15" /></template>新增规则
        </Button>
      </template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <div class="grid gap-3 sm:grid-cols-3">
      <Card v-for="item in [
        ['已启用规则', enabledCount],
        ['覆盖平台', coveredPlatforms],
        ['配置回退', fallbackCount],
      ]" :key="String(item[0])" padding="sm">
        <p class="text-xs text-[var(--text-secondary)]">{{ item[0] }}</p>
        <p class="mt-2 text-2xl font-semibold">{{ item[1] }}</p>
      </Card>
    </div>

    <Card padding="none">
      <div class="border-b border-[var(--border-light)] px-5 py-4">
        <h2 class="text-sm font-semibold">优先级路由表</h2>
        <p class="mt-1 text-xs text-[var(--text-tertiary)]">从上到下匹配，首个命中的启用规则生效。</p>
      </div>
      <div v-if="loading" class="p-10 text-center text-sm text-[var(--text-secondary)]">加载中...</div>
      <div v-else-if="!routes.length" class="p-10 text-center">
        <Route :size="30" class="mx-auto text-[var(--text-tertiary)]" />
        <p class="mt-3 text-sm">暂无平台路由规则</p>
      </div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[900px]">
          <thead><tr><th>顺序</th><th>规则 / 状态</th><th>匹配维度</th><th>入口</th><th>目标 / 回退</th><th class="text-right">操作</th></tr></thead>
          <tbody>
            <tr v-for="(item, index) in routes" :key="item.id">
              <td><OrderButtons :up-disabled="ordering || index === 0" :down-disabled="ordering || index === routes.length - 1" @up="move(index, -1)" @down="move(index, 1)" /></td>
              <td><p class="font-medium">{{ item.name }}</p><StatusBadge class="mt-1" :status="item.enabled ? 'online' : 'offline'" :label="item.enabled ? '启用' : '停用'" size="sm" /></td>
              <td class="text-xs text-[var(--text-secondary)]">
                <p>标签 {{ item.platform || '--' }} · 来源 CIDR {{ item.source_cidrs?.length || 0 }}</p>
                <p class="mt-1">域名 {{ (item.domains?.length || 0) + (item.domain_suffixes?.length || 0) + (item.domain_keywords?.length || 0) }} · 目标网段 {{ item.destination_cidrs?.length || 0 }}</p>
              </td>
              <td>{{ item.inbound_exposure_ids?.length ? `${item.inbound_exposure_ids.length} 个入口` : "全部入口" }}</td>
              <td><p>{{ refLabel(item) }}</p><p class="mt-1 text-xs text-[var(--text-tertiary)]">回退：{{ refLabel(item, true) }}</p></td>
              <td><div class="flex justify-end gap-2"><button class="aw-control-action" :disabled="ordering" @click="openEdit(item)"><Pencil :size="13" />编辑</button><button class="aw-control-action aw-action-danger" :disabled="ordering" @click="deleting = item"><Trash2 :size="13" />删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </Card>

    <Modal :open="formOpen" :title="editing ? '编辑平台路由' : '新增平台路由'" size="xl" @close="closeForm">
      <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
        <fieldset class="contents" :disabled="saving">
        <label class="sm:col-span-2"><span class="aw-modal-label text-xs">规则名称</span><input v-model="form.name" class="aw-input mt-1 w-full" maxlength="100" placeholder="例如：移动端业务路由" /></label>
        <label><span class="aw-modal-label text-xs">业务平台标签</span><input v-model="form.platform" class="aw-input mt-1 w-full" placeholder="例如：streaming" /></label>
        <label><span class="aw-modal-label text-xs">来源 CIDR（每行一项）</span><textarea v-model="form.sourceCidrs" class="aw-input mt-1 min-h-24 w-full font-mono" placeholder="192.168.1.0/24" /></label>
        <label><span class="aw-modal-label text-xs">完整域名（每行一项）</span><textarea v-model="form.domains" class="aw-input mt-1 min-h-24 w-full font-mono" placeholder="api.example.com" /></label>
        <label><span class="aw-modal-label text-xs">域名后缀（每行一项）</span><textarea v-model="form.domainSuffixes" class="aw-input mt-1 min-h-24 w-full font-mono" placeholder="example.com" /></label>
        <label><span class="aw-modal-label text-xs">域名关键词（每行一项）</span><textarea v-model="form.domainKeywords" class="aw-input mt-1 min-h-24 w-full font-mono" placeholder="streaming" /></label>
        <label><span class="aw-modal-label text-xs">目标 CIDR（每行一项）</span><textarea v-model="form.destinationCidrs" class="aw-input mt-1 min-h-24 w-full font-mono" placeholder="10.0.0.0/8" /></label>
        <fieldset class="sm:col-span-2">
          <legend class="aw-modal-label text-xs">入口暴露（可多选，不选表示全部）</legend>
          <div class="mt-2 grid max-h-36 gap-2 overflow-y-auto rounded-lg border border-[var(--border-default)] p-3 sm:grid-cols-2">
            <label v-for="entry in exposures" :key="entry.id" class="flex items-center gap-2 text-xs"><input v-model="form.inboundIds" type="checkbox" :value="entry.id" />{{ entry.name }} · {{ entry.inbound_type.toUpperCase() }}</label>
            <span v-if="!exposures.length" class="text-xs text-[var(--text-tertiary)]">暂无可用入口暴露</span>
          </div>
        </fieldset>
        <label v-for="kind in ['targetKey', 'fallbackKey'] as const" :key="kind">
          <span class="aw-modal-label text-xs">{{ kind === 'targetKey' ? '目标出口' : '失败回退' }}</span>
          <select v-model="form[kind]" class="aw-input mt-1 w-full">
            <optgroup label="内置类型"><option value="direct">直连</option></optgroup>
            <optgroup label="节点"><option v-for="node in nodes" :key="node.value" :value="node.value">{{ node.name }} · {{ node.type }}</option></optgroup>
            <optgroup label="策略组"><option v-for="group in collections" :key="group.id" :value="`collection:${group.id}`">{{ group.name }} · {{ group.type }}</option></optgroup>
          </select>
        </label>
        <p class="sm:col-span-2 text-xs leading-5 text-[var(--text-tertiary)]">平台标签仅用于识别和审计，不参与匹配。每个非空匹配维度之间使用“且”；同一输入框内的多行值使用“或”。</p>
        <label class="sm:col-span-2 flex items-center gap-2 rounded-lg border border-[var(--border-default)] p-3 text-sm"><input v-model="form.enabled" type="checkbox" />保存后启用此规则</label>
        </fieldset>
      </form>
      <template #footer><Button :disabled="saving" @click="closeForm">取消</Button><Button variant="primary" :loading="saving" @click="save">保存</Button></template>
    </Modal>

    <ConfirmDialog :open="!!deleting" title="删除平台路由" :message="`确定删除「${deleting?.name || ''}」吗？已建立的租约可能不再匹配该规则。`" confirm-text="删除" danger @confirm="remove" @cancel="deleting = null" />
  </div>
</template>
