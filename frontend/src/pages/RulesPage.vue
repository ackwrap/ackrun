<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { FileJson2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type {
  GeoAsset,
  RouteRule,
  RouteRulePreviewResponse,
  RouteRuleSubscription,
  WSEvent,
} from "@/services/types";
import GeoDatabaseSection from "./rules/GeoDatabaseSection.vue";
import RuleListSection from "./rules/RuleListSection.vue";
import RouteRuleFormModal from "./rules/RouteRuleFormModal.vue";
import RuleSubscriptionActionsModal from "./rules/RuleSubscriptionActionsModal.vue";
import RuleSubscriptionFormModal from "./rules/RuleSubscriptionFormModal.vue";
import RuleSubscriptionSection from "./rules/RuleSubscriptionSection.vue";
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
  geoSyncing = ref(false);
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
        { rule_set: preview.value?.rule_sets, rules: preview.value?.rules },
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
    [rules.value, subscriptions.value, geoAssets.value] = result;
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
  formOpen.value = true;
}
function editRule(r: RouteRule) {
  editing.value = r;
  name.value = r.name;
  enabled.value = r.enabled;
  ruleType.value = r.rule_type;
  valuesText.value = r.values.join("\n");
  outbound.value = r.outbound;
  invert.value = r.invert;
  formOpen.value = true;
}
async function saveRule() {
  if (!values.value.length) return show("请填写匹配值", "error");
  try {
    const body = {
      name: name.value,
      enabled: enabled.value,
      priority: editing.value?.priority || 0,
      rule_type: ruleType.value,
      values: values.value,
      outbound: outbound.value,
      invert: invert.value,
    };
    editing.value
      ? await api.updateRouteRule(editing.value.id, body)
      : await api.createRouteRule(body);
    show(editing.value ? "规则已更新" : "规则已添加");
    formOpen.value = false;
    resetRule();
    await load();
  } catch (e: any) {
    show(`规则保存失败: ${e.message}`, "error");
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
async function move(i: number, d: -1 | 1) {
  const n = i + d;
  if (n < 0 || n >= rules.value.length) return;
  const protectedRule = (rule: RouteRule) =>
    rule.system_key === "ad_block" ||
    rule.system_key === "global_direct" ||
    ["fallback", "final"].includes(rule.rule_type) ||
    (rule.is_system && ["广告拦截", "全球直连"].includes(rule.name));
  if (protectedRule(rules.value[i]) || protectedRule(rules.value[n])) return;
  const next = [...rules.value];
  [next[i], next[n]] = [next[n], next[i]];
  rules.value = next;
  try {
    await api.reorderRouteRules(next.map((x) => x.id));
    await load();
  } catch (e: any) {
    show(`规则排序失败: ${e.message}`, "error");
    await load();
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
    if (subEdit.value)
      await api.updateRouteRuleSubscription(subEdit.value.id, sub.value);
    else {
      const x = await api.createRouteRuleSubscription(sub.value);
      if (generate.value)
        await api.createRouteRule({
          name: x.name,
          enabled: x.enabled,
          priority: 0,
          rule_type: "rule_set",
          values: [x.tag],
          outbound: referenceOutbound.value,
          invert: false,
        });
    }
    show(subEdit.value ? "规则订阅已更新" : "规则订阅已添加");
    subFormOpen.value = false;
    resetSub();
    await load();
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
  try {
    await api.createRouteRule({
      name: x.name,
      enabled: x.enabled,
      priority: 0,
      rule_type: "rule_set",
      values: [x.tag],
      outbound: o,
      invert: false,
    });
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
    const subscription = subscriptions.value.find((item) => item.id === data.id);
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
        @update="updateGeo"
      /><RuleListSection
        :rules="rules"
        :subscriptions="subscriptions"
        @refresh="load"
        @add-geo="addRule('geosite')"
        @add="addRule()"
        @preview="async () => (preview = await api.previewRouteRules())"
        @move="move"
        @toggle="toggleRule"
        @edit="editRule"
        @remove="pending = $event"
        @detail="detailRule = $event"
      />
      <RuleSubscriptionSection
        :subscriptions="subscriptions"
        :syncing="isRuleSubscriptionSyncing"
        @add="addSub"
        @sync-all="syncAll"
        @manage="actions = $event"
      /></template
    ><RouteRuleFormModal
      v-if="formOpen"
      :editing="editing"
      v-model:name="name"
      v-model:enabled="enabled"
      v-model:rule-type="ruleType"
      v-model:values-text="valuesText"
      v-model:outbound="outbound"
      v-model:invert="invert"
      :subscriptions="subscriptions"
      @close="
        formOpen = false;
        resetRule();
      "
      @save="saveRule"
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
      :title="detailRule ? `规则详情：${detailRule.name}` : '规则详情'"
      size="lg"
      @close="detailRule = null"
    >
      <div v-if="detailRule" class="grid gap-4 text-sm md:grid-cols-2">
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-[var(--text-tertiary)]">类型</span>
          <p class="mt-1 font-mono">{{ detailRule.rule_type }}</p>
        </div>
        <div class="rounded-lg bg-[var(--bg-base)] p-3">
          <span class="text-[var(--text-tertiary)]">出站</span>
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
          <span class="text-[var(--text-tertiary)]"
            >匹配值（{{ detailRule.values.length }}）</span
          >
          <pre
            class="mt-2 max-h-[50vh] overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-5 text-[var(--text-primary)]"
          >{{ detailRule.values.join("\n") || "-" }}</pre>
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
