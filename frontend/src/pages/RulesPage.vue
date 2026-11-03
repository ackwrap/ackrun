<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { FileJson2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api, request } from "@/services/api";
import { writeClipboardText } from "@/utils/clipboard";
import type {
  GeoAsset,
  RouteRule,
  RouteRuleRequest,
  RouteRulePreviewResponse,
  RouteRuleSubscription,
  StrategyItem,
  WSEvent,
} from "@/services/types";
import GeoDatabaseSection from "./rules/GeoDatabaseSection.vue";
import RuleListSection from "./rules/RuleListSection.vue";
import { isFixedRouteRule } from "./rules/useRuleSort";
import RouteRuleFormModal from "./rules/RouteRuleFormModal.vue";
import NewRuleStrategyModal from "./rules/NewRuleStrategyModal.vue";
import RouteRuleImportModal from "./rules/RouteRuleImportModal.vue";
import RuleSubscriptionActionsModal from "./rules/RuleSubscriptionActionsModal.vue";
import RuleSubscriptionFormModal from "./rules/RuleSubscriptionFormModal.vue";
import RuleSubscriptionSection from "./rules/RuleSubscriptionSection.vue";
import StrategyFormModal from "./collections/StrategyFormModal.vue";
import type {
  NodeGroup,
  StrategyCollectionRequest,
} from "./collections/collectionTypes";
const ruleStrategy = ref<StrategyItem | null>(null);
const strategyRule = ref<RouteRule | null>(null);
const strategyEnabled = ref(true);
const strategySaving = ref(false);
const strategyConnectivity = ref({ test_url: "", interval_seconds: 300 });
const strategyGroups = ref<NodeGroup[]>([]);
const strategyError = ref("");
const strategyLoadingId = ref<number | null>(null);
const ruleSaving = ref(false);
const newRuleDraft = ref<RouteRuleRequest | null>(null);
const ruleOrderSaving = ref(false);
const rules = ref<RouteRule[]>([]),
  subscriptions = ref<RouteRuleSubscription[]>([]),
  geoAssets = ref<GeoAsset[]>([]),
  loading = ref(true),
  message = ref(""),
  messageType = ref<"success" | "error">("success"),
  pending = ref<RouteRule | null>(null),
  detailRule = ref<RouteRule | null>(null),
  formOpen = ref(false),
  editing = ref<RouteRule | null>(null),
  name = ref(""),
  enabled = ref(true),
  ruleType = ref("domain_suffix"),
  valuesText = ref(""),
  directDomainsText = ref(""),
  outbound = ref("direct"),
  invert = ref(false),
  subEdit = ref<RouteRuleSubscription | null>(null),
  subFormOpen = ref(false),
  sub = ref({
    name: "",
    enabled: true,
    tag: "",
    url: "",
    format: "auto",
    use_proxy: false,
    sync_mode: "daily",
    sync_time: "04:00:00",
    sync_weekday: 0,
  }),
  generate = ref(true),
  referenceOutbound = ref("proxy"),
  actions = ref<RouteRuleSubscription | null>(null),
  preview = ref<RouteRulePreviewResponse | null>(null),
  content = ref<{ title: string; content: string } | null>(null),
  geoSyncing = ref(false),
  importOpen = ref(false),
  importCode = ref(""),
  importing = ref(false);
let poll: number | undefined;
let loadVersion = 0;
const show = (s: string, t: "success" | "error" = "success") => {
    message.value = s;
    messageType.value = t;
  },
  values = computed(() => [
    ...new Set(
      valuesText.value
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean),
    ),
  ]),
  isGeoSyncing = computed(
    () =>
      geoSyncing.value ||
      geoAssets.value.some((x) => x.sync_status === "syncing"),
  ),
  isRuleSubscriptionSyncing = computed(() =>
    subscriptions.value.some((x) => x.sync_status === "syncing"),
  ),
  previewText = computed(
    () =>
      content.value?.content ||
      JSON.stringify(
        {
          final: preview.value?.final,
          rule_set: preview.value?.rule_sets,
          rules: preview.value?.rules,
        },
        null,
        2,
      ),
  );
async function load() {
  const version = ++loadVersion;
  try {
    const result = await Promise.all([
      api.getRouteRules(),
      api.getRouteRuleSubscriptions(),
      api.getGeoAssets(),
    ]);
    if (version !== loadVersion) return;
    if (!ruleOrderSaving.value) rules.value = result[0];
    subscriptions.value = result[1];
    geoAssets.value = result[2];
  } catch (e: any) {
    if (version !== loadVersion) return;
    show(`规则加载失败: ${e.message}`, "error");
  } finally {
    if (version === loadVersion) loading.value = false;
  }
}
function resetRule() {
  editing.value = null;
  name.value = "";
  enabled.value = true;
  ruleType.value = "domain_suffix";
  valuesText.value = "";
  directDomainsText.value = "";
  outbound.value = "direct";
  invert.value = false;
}
function addRule(type = "domain_suffix") {
  resetRule();
  ruleType.value = type;
  if (type === "geosite") {
    name.value = "GeoSite 规则";
    outbound.value = "proxy";
  }
  if (type === "source_ip_cidr") outbound.value = "proxy";
  formOpen.value = true;
}
function isFinalStrategyRule(rule: RouteRule | null) {
  return (
    !!rule &&
    (rule.system_key === "global_direct" ||
      ["fallback", "final"].includes(rule.rule_type))
  );
}
function ruleDisplayName(rule: RouteRule) {
  return isFinalStrategyRule(rule) ? "最终策略" : rule.name;
}
function editRule(r: RouteRule) {
  editing.value = r;
  name.value = ruleDisplayName(r);
  enabled.value = r.enabled;
  ruleType.value = r.rule_type;
  valuesText.value = r.values.join("\n");
  directDomainsText.value = (r.direct_domains || []).join("\n");
  outbound.value = r.outbound;
  invert.value = r.invert;
  formOpen.value = true;
}
async function saveRule() {
  if (ruleSaving.value) return;
  if (!name.value.trim()) return show("请填写规则名称", "error");
  if (!isFinalStrategyRule(editing.value) && !values.value.length)
    return show("请填写匹配值", "error");
  ruleSaving.value = true;
  try {
    const isDevice = ruleType.value === "source_ip_cidr";
    const requestedEnabled = enabled.value;
    const body = {
      name: name.value,
      enabled:
        isDevice && editing.value?.rule_type !== "source_ip_cidr"
          ? false
          : enabled.value,
      priority: editing.value?.priority || 0,
      rule_type: ruleType.value,
      values: values.value,
      direct_domains: isDevice
        ? directDomainsText.value
            .split("\n")
            .map((value) => value.trim())
            .filter(Boolean)
        : [],
      outbound: outbound.value,
      invert: invert.value,
    };
    if (!editing.value && body.outbound === "proxy") {
      beginNewRuleStrategy({ ...body, enabled: requestedEnabled });
      return;
    }
    const saved = editing.value
      ? await api.updateRouteRule(editing.value.id, body)
      : await api.createRouteRule(body);
    show(
      isFinalStrategyRule(editing.value)
        ? "最终策略已更新"
        : editing.value
          ? "规则已更新"
          : "规则已添加",
    );
    formOpen.value = false;
    resetRule();
    await load();
    if (isDevice) {
      show("设备规则已保存，请完成固定节点绑定；新规则在绑定后启用");
      await openRuleStrategy(saved, requestedEnabled);
    }
  } catch (e: any) {
    show(`规则保存失败: ${e.message}`, "error");
  } finally {
    ruleSaving.value = false;
  }
}
function beginNewRuleStrategy(body: RouteRuleRequest) {
  resetRule();
  name.value = body.name;
  enabled.value = body.enabled;
  ruleType.value = body.rule_type;
  valuesText.value = body.values.join("\n");
  directDomainsText.value = body.direct_domains?.join("\n") ?? "";
  outbound.value = body.outbound;
  invert.value = body.invert;
  newRuleDraft.value = body;
  formOpen.value = false;
}
function cancelNewRuleStrategy() {
  newRuleDraft.value = null;
  formOpen.value = true;
}
async function finishNewRule() {
  newRuleDraft.value = null;
  resetRule();
  show("规则与代理策略已添加");
  await load();
}
async function openRuleStrategy(
  rule: RouteRule,
  requestedEnabled = rule.enabled,
) {
  if (strategyLoadingId.value !== null || strategySaving.value) return;
  strategyLoadingId.value = rule.id;
  strategyError.value = "";
  try {
    const [strategies, connectivity, groups] = await Promise.all([
      api.getRouteStrategies(),
      api.getConnectivitySettings(),
      rule.rule_type === "source_ip_cidr"
        ? Promise.resolve<NodeGroup[]>([])
        : request<NodeGroup[]>("/node-groups"),
    ]);
    const strategy = strategies.find(
      (item) => item.rule_id === rule.id && item.kind === "proxy",
    );
    if (!strategy) throw new Error("未找到此规则的代理策略");
    strategyRule.value = rule;
    strategyEnabled.value = requestedEnabled;
    strategyConnectivity.value = connectivity;
    strategyGroups.value = groups ?? [];
    ruleStrategy.value = strategy;
  } catch (e) {
    show(
      `策略加载失败：${e instanceof Error ? e.message : "请求失败"}`,
      "error",
    );
  } finally {
    strategyLoadingId.value = null;
  }
}
function closeRuleStrategy() {
  ruleStrategy.value = null;
  strategyRule.value = null;
  strategyError.value = "";
}
async function saveRuleStrategy(payload: StrategyCollectionRequest) {
  if (!ruleStrategy.value || !strategyRule.value || strategySaving.value)
    return;
  strategySaving.value = true;
  strategyError.value = "";
  try {
    const current = ruleStrategy.value.collection;
    if (current) await api.updateProxyCollection(current.id, payload);
    else
      ruleStrategy.value.collection = await api.createProxyCollection(payload);
    if (strategyRule.value.enabled !== strategyEnabled.value) {
      await api.updateRouteRule(strategyRule.value.id, {
        ...strategyRule.value,
        enabled: strategyEnabled.value,
      });
    }
    closeRuleStrategy();
    show("代理策略已保存");
    await load();
  } catch (e) {
    strategyError.value = `策略保存失败：${e instanceof Error ? e.message : "请求失败"}`;
  } finally {
    strategySaving.value = false;
  }
}
async function toggleRule(r: RouteRule) {
  if (
    r.system_key === "ad_block" ||
    r.system_key === "global_direct" ||
    ["fallback", "final"].includes(r.rule_type) ||
    (r.is_system && ["广告拦截", "全球直连"].includes(r.name))
  )
    return;
  try {
    await api.updateRouteRule(r.id, {
      name: r.name,
      enabled: !r.enabled,
      priority: r.priority,
      rule_type: r.rule_type,
      values: r.values,
      outbound: r.outbound,
      invert: r.invert,
    });
    await load();
  } catch (e: any) {
    show(`规则状态更新失败: ${e.message}`, "error");
  }
}
async function removeRule() {
  if (!pending.value) return;
  try {
    await api.deleteRouteRule(pending.value.id);
    show("规则已删除");
    pending.value = null;
    await load();
  } catch (e: any) {
    show(`规则删除失败: ${e.message}`, "error");
  }
}
async function reorderRules(ids: number[]) {
  if (ruleOrderSaving.value) return;
  const previous = rules.value;
  const byID = new Map(previous.map((rule) => [rule.id, rule]));
  if (
    ids.length !== previous.length ||
    new Set(ids).size !== ids.length ||
    ids.some((id) => !byID.has(id)) ||
    previous.some(
      (rule, index) => isFixedRouteRule(rule) && ids[index] !== rule.id,
    ) ||
    ids.every((id, index) => id === previous[index].id)
  )
    return;
  ruleOrderSaving.value = true;
  rules.value = ids.map((id) => byID.get(id)!);
  ++loadVersion;
  try {
    await api.reorderRouteRules(ids);
    show("规则顺序已保存");
  } catch (e: any) {
    rules.value = previous;
    show(`规则排序失败: ${e.message}`, "error");
  } finally {
    ruleOrderSaving.value = false;
    await load();
  }
}
async function shareRules() {
  try {
    const result = await api.shareRouteRules();
    await writeClipboardText(result.code);
    show(`规则分享码已复制，共 ${result.rule_count} 条自定义规则`);
  } catch (e: any) {
    show(`规则分享失败: ${e.message}`, "error");
  }
}
function openImport() {
  importCode.value = "";
  importOpen.value = true;
}
async function importRules() {
  if (importing.value || !importCode.value.trim()) return;
  importing.value = true;
  try {
    const result = await api.importRouteRules(importCode.value);
    show(`规则导入完成：新增 ${result.created}，更新 ${result.updated}`);
    importOpen.value = false;
    importCode.value = "";
    await load();
  } catch (e: any) {
    show(`规则导入失败: ${e.message}`, "error");
  } finally {
    importing.value = false;
  }
}
function resetSub() {
  subEdit.value = null;
  sub.value = {
    name: "",
    enabled: true,
    tag: "",
    url: "",
    format: "auto",
    use_proxy: false,
    sync_mode: "daily",
    sync_time: "04:00:00",
    sync_weekday: 0,
  };
  generate.value = true;
  referenceOutbound.value = "proxy";
}
function addSub() {
  resetSub();
  subFormOpen.value = true;
}
function closeSubForm() {
  subFormOpen.value = false;
  resetSub();
}
function editSub(x: RouteRuleSubscription) {
  actions.value = null;
  subEdit.value = x;
  sub.value = {
    name: x.name,
    enabled: x.enabled,
    tag: x.tag,
    url: x.url,
    format: x.format,
    use_proxy: x.use_proxy,
    sync_mode: x.sync_mode,
    sync_time: x.sync_time,
    sync_weekday: x.sync_weekday,
  };
  subFormOpen.value = true;
}
async function saveSub() {
  try {
    let generatedRule: RouteRuleRequest | null = null;
    if (subEdit.value)
      await api.updateRouteRuleSubscription(subEdit.value.id, sub.value);
    else {
      const x = await api.createRouteRuleSubscription(sub.value);
      if (generate.value) {
        const body: RouteRuleRequest = {
          name: x.name,
          enabled: x.enabled,
          priority: 0,
          rule_type: "rule_set",
          values: [x.tag],
          outbound: referenceOutbound.value,
          invert: false,
        };
        if (body.outbound === "proxy") generatedRule = body;
        else await api.createRouteRule(body);
      }
    }
    show(subEdit.value ? "规则订阅已更新" : "规则订阅已添加");
    subFormOpen.value = false;
    resetSub();
    await load();
    if (generatedRule) beginNewRuleStrategy(generatedRule);
  } catch (e: any) {
    show(`规则订阅保存失败: ${e.message}`, "error");
  }
}
async function deleteSub(x: RouteRuleSubscription) {
  try {
    await api.deleteRouteRuleSubscription(x.id);
    actions.value = null;
    show("规则订阅已删除");
    await load();
  } catch (e: any) {
    show(`规则订阅删除失败: ${e.message}`, "error");
  }
}
async function toggleSub(x: RouteRuleSubscription) {
  try {
    await api.updateRouteRuleSubscription(x.id, { ...x, enabled: !x.enabled });
    actions.value = null;
    await load();
  } catch (e: any) {
    show(`规则订阅状态更新失败: ${e.message}`, "error");
  }
}
async function syncSub(x: RouteRuleSubscription) {
  try {
    await api.syncRouteRuleSubscription(x.id);
    actions.value = null;
    show(`${x.name} 已开始更新`);
    await load();
  } catch (e: any) {
    show(`规则订阅更新失败: ${e.message}`, "error");
  }
}
async function syncAll() {
  try {
    await api.syncAllRouteRuleSubscriptions();
    show("规则订阅已开始全部更新");
    await load();
  } catch (e: any) {
    show(`规则订阅批量更新失败: ${e.message}`, "error");
  }
}
async function createRef(x: RouteRuleSubscription, o: string) {
  if (
    rules.value.some(
      (r) =>
        r.rule_type === "rule_set" &&
        r.outbound === o &&
        r.values.includes(x.tag),
    )
  )
    return show("引用规则已存在", "error");
  const body: RouteRuleRequest = {
    name: x.name,
    enabled: x.enabled,
    priority: 0,
    rule_type: "rule_set",
    values: [x.tag],
    outbound: o,
    invert: false,
  };
  if (o === "proxy") {
    actions.value = null;
    beginNewRuleStrategy(body);
    return;
  }
  try {
    await api.createRouteRule(body);
    actions.value = null;
    show("引用规则已生成");
    await load();
  } catch (e: any) {
    show(`引用规则创建失败: ${e.message}`, "error");
  }
}
async function previewSub(x: RouteRuleSubscription) {
  try {
    actions.value = null;
    content.value = {
      title: `${x.name} 转换结果`,
      content: JSON.stringify(
        await api.getRouteRuleSubscriptionContent(x.id),
        null,
        2,
      ),
    };
  } catch (e: any) {
    show(`规则订阅预览失败: ${e.message}`, "error");
  }
}
async function openRulePreview() {
  try {
    preview.value = await api.previewRouteRules();
  } catch (e: any) {
    show(`规则预览失败: ${e.message}`, "error");
  }
}
async function syncGeo(x?: GeoAsset) {
  geoSyncing.value = true;
  const targets = x ? [x] : geoAssets.value;
  targets.forEach((item) => {
    item.sync_status = "syncing";
    item.sync_progress = 0;
    item.sync_error = "";
  });
  try {
    x ? await api.syncGeoAsset(x.id) : await api.syncAllGeoAssets();
    show("Geo 数据库已开始更新");
    await load();
  } catch (e: any) {
    show(`Geo 数据库更新失败: ${e.message}`, "error");
  } finally {
    geoSyncing.value = false;
  }
}
useRealtimeSocket((event: WSEvent) => {
  const data: any = event.data;
  if (event.type === "geo.sync") {
    const asset = geoAssets.value.find((item) => item.id === data.id);
    if (asset) {
      asset.sync_status = data.status ?? asset.sync_status;
      asset.sync_progress = data.progress ?? asset.sync_progress;
      asset.sync_error = data.error ?? "";
    }
    if (data.status === "failed") {
      show(`Geo 数据库更新失败: ${data.error || "请求失败"}`, "error");
    }
  } else if (event.type === "route_rule_subscription.sync") {
    const subscription = subscriptions.value.find(
      (item) => item.id === data.id,
    );
    if (subscription) {
      subscription.sync_status = data.status ?? subscription.sync_status;
      subscription.sync_progress = data.progress ?? subscription.sync_progress;
      subscription.sync_error = data.error ?? "";
    }
    if (data.status === "failed") {
      show(`规则订阅更新失败: ${data.error || "请求失败"}`, "error");
    } else if (data.status === "updated") {
      void load();
    }
  } else if (event.type === "geo.sync_all" && data.status === "completed") {
    void load();
    if (data.failed) show(`Geo 数据库更新完成，${data.failed} 项失败`, "error");
  }
});
async function updateGeo(x: GeoAsset, b: any) {
  try {
    await api.updateGeoAsset(x.id, b);
    show(`${x.name} 自动更新设置已保存`);
    await load();
  } catch (e: any) {
    show(`Geo 自动更新设置保存失败: ${e.message}`, "error");
  }
}
onMounted(() => {
  load();
  poll = window.setInterval(() => {
    if (isGeoSyncing.value || isRuleSubscriptionSyncing.value) load();
  }, 2000);
});
onBeforeUnmount(() => clearInterval(poll));
</script>
<template>
  <div class="space-y-4">
    <PageHeader title="规则管理" /><Toast
      :message="message"
      :type="messageType"
      @dismiss="message = ''"
    />
    <div v-if="loading" class="py-20 text-center">加载中...</div>
    <template v-else
      ><GeoDatabaseSection
        :geo-assets="geoAssets"
        :syncing="isGeoSyncing"
        @sync-all="syncGeo()"
        @sync-one="syncGeo"
        @update="updateGeo" /><RuleListSection
        :rules="rules"
        :subscriptions="subscriptions"
        :sorting="ruleOrderSaving"
        :strategy-loading-id="strategyLoadingId"
        @refresh="load"
        @add-geo="addRule('geosite')"
        @add-device="addRule('source_ip_cidr')"
        @add="addRule()"
        @preview="openRulePreview"
        @share="shareRules"
        @import="openImport"
        @reorder="reorderRules"
        @toggle="toggleRule"
        @edit="editRule"
        @edit-strategy="openRuleStrategy"
        @remove="pending = $event"
        @detail="detailRule = $event" />
      <RuleSubscriptionSection
        :subscriptions="subscriptions"
        :syncing="isRuleSubscriptionSyncing"
        @add="addSub"
        @sync-all="syncAll"
        @manage="actions = $event" /></template
    ><RouteRuleFormModal
      v-if="formOpen"
      :editing="editing"
      v-model:name="name"
      v-model:enabled="enabled"
      v-model:rule-type="ruleType"
      v-model:values-text="valuesText"
      v-model:direct-domains-text="directDomainsText"
      v-model:outbound="outbound"
      v-model:invert="invert"
      :saving="ruleSaving"
      :subscriptions="subscriptions"
      @close="
        formOpen = false;
        resetRule();
      "
      @save="saveRule"
    />
    <NewRuleStrategyModal
      v-if="newRuleDraft"
      :rule="newRuleDraft"
      @close="cancelNewRuleStrategy"
      @saved="finishNewRule"
    />
    <StrategyFormModal
      v-if="ruleStrategy"
      :key="ruleStrategy.rule_id"
      :strategy="ruleStrategy"
      :groups="strategyGroups"
      :connectivity="strategyConnectivity"
      :saving="strategySaving"
      :save-error="strategyError"
      @close="closeRuleStrategy"
      @save="saveRuleStrategy"
      @error="strategyError = $event"
    />
    <RouteRuleImportModal
      :open="importOpen"
      :code="importCode"
      :importing="importing"
      @close="importOpen = false"
      @update:code="importCode = $event"
      @import="importRules"
    /><RuleSubscriptionActionsModal
      v-if="actions"
      :item="actions"
      @close="actions = null"
      @create-rule="createRef"
      @preview="previewSub"
      @sync="syncSub"
      @toggle="toggleSub"
      @edit="editSub"
      @remove="deleteSub"
    /><RuleSubscriptionFormModal
      v-if="subFormOpen"
      :editing="!!subEdit"
      v-model:form="sub"
      v-model:generate="generate"
      v-model:reference-outbound="referenceOutbound"
      @close="closeSubForm"
      @save="saveSub"
    />
    <Modal
      :open="!!detailRule"
      :title="
        detailRule ? `规则详情：${ruleDisplayName(detailRule)}` : '规则详情'
      "
      size="lg"
      @close="detailRule = null"
    >
      <div v-if="detailRule" class="grid gap-4 text-sm md:grid-cols-2">
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-[var(--text-tertiary)]">类型</span>
          <p class="mt-1 font-mono">
            {{
              isFinalStrategyRule(detailRule)
                ? "最终规则"
                : detailRule.rule_type
            }}
          </p>
        </div>
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-[var(--text-tertiary)]">命中后动作</span>
          <p class="mt-1 font-mono">{{ detailRule.outbound }}</p>
        </div>
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-[var(--text-tertiary)]">状态</span>
          <p class="mt-1">{{ detailRule.enabled ? "启用" : "停用" }}</p>
        </div>
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-[var(--text-tertiary)]">排序 / 反向</span>
          <p class="mt-1">
            {{ detailRule.priority }} / {{ detailRule.invert ? "是" : "否" }}
          </p>
        </div>
        <div
          class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-base)] p-3 md:col-span-2"
        >
          <span class="text-[var(--text-tertiary)]">匹配值</span>
          <pre
            class="mt-2 max-h-[50vh] overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-5 text-[var(--text-primary)]"
            >{{
              isFinalStrategyRule(detailRule)
                ? "未匹配流量"
                : detailRule.values.join("\n") || "-"
            }}</pre>
        </div>
      </div>
    </Modal>
    <Modal
      :open="!!(preview || content)"
      :title="content?.title || '规则 JSON 预览'"
      size="lg"
      @close="
        preview = null;
        content = null;
      "
    >
      <template #title>
        <span class="flex items-center gap-2">
          <FileJson2 :size="18" />{{ content?.title || "规则 JSON 预览" }}
        </span>
      </template>
      <textarea
        :value="previewText"
        readonly
        rows="22"
        class="min-h-[480px] w-full resize-none font-mono text-xs leading-5"
        spellcheck="false"
      />
    </Modal>
    <ConfirmDialog
      :open="!!pending"
      title="删除规则"
      :message="pending ? `确定要删除规则「${pending.name}」吗？` : ''"
      danger
      @confirm="removeRule"
      @cancel="pending = null"
    />
  </div>
</template>
