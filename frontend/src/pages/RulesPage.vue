<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Cloud, FileJson2, Link2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { api } from "@/services/api";
import type {
  GeoAsset,
  RouteRule,
  RouteRulePreviewResponse,
  RouteRuleSubscription,
} from "@/services/types";
import GeoDatabaseSection from "./rules/GeoDatabaseSection.vue";
import RuleListSection from "./rules/RuleListSection.vue";
import RouteRuleFormModal from "./rules/RouteRuleFormModal.vue";
import RuleSubscriptionActionsModal from "./rules/RuleSubscriptionActionsModal.vue";
const rules = ref<RouteRule[]>([]),
  subscriptions = ref<RouteRuleSubscription[]>([]),
  geoAssets = ref<GeoAsset[]>([]),
  loading = ref(true),
  message = ref(""),
  messageType = ref<"success" | "error">("success"),
  pending = ref<RouteRule | null>(null),
  formOpen = ref(false),
  editing = ref<RouteRule | null>(null),
  name = ref(""),
  enabled = ref(true),
  ruleType = ref("domain_suffix"),
  valuesText = ref(""),
  outbound = ref("direct"),
  invert = ref(false),
  subEdit = ref<RouteRuleSubscription | null>(null),
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
const show = (s: string, t: "success" | "error" = "success") => {
    message.value = s;
    messageType.value = t;
    setTimeout(() => (message.value = ""), t === "error" ? 5000 : 3000);
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
  try {
    [rules.value, subscriptions.value, geoAssets.value] = await Promise.all([
      api.getRouteRules(),
      api.getRouteRuleSubscriptions(),
      api.getGeoAssets(),
    ]);
  } catch (e: any) {
    show(`规则加载失败: ${e.message}`, "error");
  } finally {
    loading.value = false;
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
    if (isGeoSyncing.value) load();
  }, 2000);
});
onBeforeUnmount(() => clearInterval(poll));
</script>
<template>
  <div class="space-y-4">
    <PageHeader title="规则管理" /><Toast
      :message="message"
      :type="messageType"
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
      />
      <section
        class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <header class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="flex items-center gap-2 font-semibold">
              <Cloud :size="17" />规则订阅
            </h3>
            <p class="mt-1 text-xs text-[var(--text-secondary)]">
              下载远程规则集，并生成 sing-box rule_set 引用。
            </p>
          </div>
          <button class="aw-action-button aw-action-neutral" @click="syncAll">
            同步全部
          </button>
        </header>
        <div class="grid gap-4 xl:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
          <div
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
          >
            <h4 class="mb-3 font-medium">
              {{ subEdit ? "编辑" : "新增" }}订阅
            </h4>
            <div class="grid gap-3 sm:grid-cols-2">
              <label class="text-xs"
                >名称<input v-model="sub.name" placeholder="订阅名称"
              /></label>
              <label class="text-xs"
                >规则集 tag<input v-model="sub.tag" placeholder="例如 private"
              /></label>
              <label class="text-xs sm:col-span-2"
                >下载地址<input v-model="sub.url" placeholder="https://..."
              /></label>
              <label class="text-xs"
                >格式<select v-model="sub.format">
                  <option
                    v-for="x in ['auto', 'binary', 'source', 'clash']"
                    :key="x"
                  >
                    {{ x }}
                  </option>
                </select></label
              >
              <label class="text-xs"
                >同步周期<select v-model="sub.sync_mode">
                  <option
                    v-for="x in ['off', 'daily', 'weekly', 'monthly']"
                    :key="x"
                  >
                    {{ x }}
                  </option>
                </select></label
              >
              <label class="text-xs"
                >同步时间<input v-model="sub.sync_time" type="time" step="1"
              /></label>
              <label class="text-xs"
                >星期/日期<input
                  v-model.number="sub.sync_weekday"
                  type="number"
              /></label>
            </div>
            <div
              class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs"
            >
              <label class="flex items-center gap-2"
                ><input v-model="sub.enabled" type="checkbox" />启用</label
              >
              <label class="flex items-center gap-2"
                ><input
                  v-model="sub.use_proxy"
                  type="checkbox"
                />下载走代理</label
              >
              <label v-if="!subEdit" class="flex items-center gap-2"
                ><input
                  v-model="generate"
                  type="checkbox"
                />同时生成引用规则</label
              >
              <label v-if="!subEdit" class="flex items-center gap-2"
                >引用出站<select
                  v-model="referenceOutbound"
                  class="!mt-0 !w-28"
                >
                  <option v-for="x in ['proxy', 'direct', 'block']" :key="x">
                    {{ x }}
                  </option>
                </select></label
              >
            </div>
            <div class="mt-4 flex justify-end gap-2">
              <button
                v-if="subEdit"
                class="aw-action-button aw-action-neutral"
                @click="resetSub"
              >
                取消编辑
              </button>
              <button
                class="aw-action-button aw-action-success"
                @click="saveSub"
              >
                <Link2 :size="13" />{{ subEdit ? "更新" : "添加" }}订阅
              </button>
            </div>
          </div>
          <div
            class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)]"
          >
            <div
              v-if="!subscriptions.length"
              class="grid min-h-48 place-items-center text-[var(--text-tertiary)]"
            >
              暂无规则订阅
            </div>
            <article
              v-for="x in subscriptions"
              :key="x.id"
              class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border-light)] p-4 last:border-b-0"
            >
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <b>{{ x.name }}</b
                  ><span
                    class="rounded bg-[var(--button-secondary-bg)] px-2 py-0.5 text-[11px]"
                    >{{ x.tag }}</span
                  ><span class="text-[11px] text-[var(--text-tertiary)]">{{
                    x.sync_status
                  }}</span>
                </div>
                <small
                  class="mt-1 block truncate text-[var(--text-tertiary)]"
                  :title="x.url"
                  >{{ x.url }}</small
                >
                <p v-if="x.sync_error" class="mt-1 text-xs text-red-400">
                  {{ x.sync_error }}
                </p>
              </div>
              <button
                class="aw-action-button aw-action-neutral"
                @click="actions = x"
              >
                管理
              </button>
            </article>
          </div>
        </div>
      </section></template
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
    />
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
