<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ArrowLeft, Pencil, Plus, ShieldCheck, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { authenticatedFetch } from "@/services/apiAuth";

interface FakeIPSettings {
  final: string;
  fakeip_enabled: boolean;
  fakeip_inet4_range: string;
  fakeip_inet6_range: string;
}

interface DNSServer {
  tag: string;
  enabled: boolean;
  server_type: string;
}

interface DNSRule {
  id: number;
  enabled: boolean;
  priority: number;
  rule_type?: string;
  conditions_json: string;
  server: string;
  disable_cache: boolean;
  rewrite_ttl: number;
  client_subnet: string;
}

interface ExceptionItem {
  rule: DNSRule;
  conditionType: "domain" | "domain_suffix";
  values: string[];
}

interface ExceptionForm {
  id?: number;
  conditionType: "domain" | "domain_suffix";
  values: string;
  server: string;
  enabled: boolean;
  preset: boolean;
}

const settings = defineModel<FakeIPSettings>({ required: true });
const props = defineProps<{ rules: DNSRule[]; servers: DNSServer[] }>();
const emit = defineEmits<{
  save: [];
  changed: [];
  notify: [message: string, type: "success" | "error"];
}>();

const commonRealIPDomainSuffixPreset = [
  "localhost",
  "lan",
  "local",
  "internal",
  "intranet",
  "corp",
  "routerlogin.com",
  "router.asus.com",
  "miwifi.com",
  "mi.wifi",
  "tplogin.cn",
  "tendawifi.com",
  "my.router",
  "wifi.cmcc",
];

const localRules = ref<DNSRule[]>([]);
const managerOpen = ref(false);
const view = ref<"list" | "form">("list");
const form = ref<ExceptionForm | null>(null);
const saving = ref(false);
const pendingRuleID = ref<number | null>(null);
const deleteTarget = ref<ExceptionItem | null>(null);
const managerClosable = computed(
  () => !saving.value && pendingRuleID.value === null && !deleteTarget.value,
);

watch(
  () => props.rules,
  (rules) => {
    localRules.value = rules.map((rule) => ({ ...rule }));
  },
  { immediate: true, deep: true },
);

function parseConditions(rule: DNSRule): Record<string, unknown> {
  try {
    const conditions = JSON.parse(rule.conditions_json || "{}");
    return conditions && typeof conditions === "object" ? conditions : {};
  } catch {
    return {};
  }
}

function conditionValues(value: unknown): string[] {
  if (typeof value === "string") return value ? [value] : [];
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string" && !!item);
}

const exceptions = computed<ExceptionItem[]>(() =>
  localRules.value.flatMap((rule) => {
    if (!isRealServerRule(rule)) return [];
    const conditions = parseConditions(rule);
    const keys = Object.keys(conditions);
    if (keys.length !== 1 || (keys[0] !== "domain" && keys[0] !== "domain_suffix")) {
      return [];
    }
    const values = conditionValues(conditions[keys[0]]);
    if (!values.length) return [];
    return [
      {
        rule,
        conditionType: keys[0] as "domain" | "domain_suffix",
        values,
      },
    ];
  }),
);

const exceptionValueCount = computed(() =>
  exceptions.value.reduce((total, item) => total + item.values.length, 0),
);
const realServers = computed(() =>
  props.servers.filter((server) => server.server_type !== "fakeip"),
);
const enabledServers = computed(() =>
  realServers.value.filter((server) => server.enabled),
);

function isRealServerRule(rule: DNSRule) {
  const server = props.servers.find((item) => item.tag === rule.server);
  return !!server && server.server_type !== "fakeip";
}

function preferredServer(): string {
  const local = enabledServers.value.find((server) =>
    ["local", "dhcp", "hosts"].includes(server.server_type),
  );
  if (local) return local.tag;
  if (enabledServers.value.some((server) => server.tag === settings.value.final)) {
    return settings.value.final;
  }
  return enabledServers.value[0]?.tag || "";
}

async function request(url: string, init?: RequestInit) {
  const response = await authenticatedFetch(url, init);
  const data = await response.json().catch(() => null);
  if (!response.ok) throw new Error(data?.error?.message || response.statusText);
  return data;
}

function openManager() {
  localRules.value = props.rules.map((rule) => ({ ...rule }));
  view.value = "list";
  form.value = null;
  managerOpen.value = true;
}

function closeManager() {
  if (!managerClosable.value) return;
  managerOpen.value = false;
  form.value = null;
}

function ensureServerAvailable(): string | null {
  const server = preferredServer();
  if (server) return server;
  emit("notify", "请先启用一个非 FakeIP DNS Server", "error");
  return null;
}

function openNew() {
  const server = ensureServerAvailable();
  if (!server) return;
  form.value = {
    conditionType: "domain_suffix",
    values: "",
    server,
    enabled: true,
    preset: false,
  };
  view.value = "form";
}

function openPreset() {
  const server = ensureServerAvailable();
  if (!server) return;
  form.value = {
    conditionType: "domain_suffix",
    values: commonRealIPDomainSuffixPreset.join("\n"),
    server,
    enabled: true,
    preset: true,
  };
  view.value = "form";
}

function openEdit(item: ExceptionItem) {
  form.value = {
    id: item.rule.id,
    conditionType: item.conditionType,
    values: item.values.join("\n"),
    server: item.rule.server,
    enabled: item.rule.enabled,
    preset: false,
  };
  view.value = "form";
}

function normalizeValue(raw: string, conditionType: "domain" | "domain_suffix") {
  let value = raw.trim().toLowerCase().replace(/\.$/, "");
  const hasSuffixPrefix = value.startsWith("+.") || value.startsWith("*.");
  if (conditionType === "domain_suffix") {
    if (hasSuffixPrefix) value = value.slice(2);
    else if (value.startsWith(".")) value = value.slice(1);
  } else if (hasSuffixPrefix || value.startsWith(".") || value.includes("*")) {
    throw new Error(`精确域名不支持通配符: ${raw.trim()}`);
  }
  if (!value || value.includes("*") || !/^[a-z0-9_-]+(?:\.[a-z0-9_-]+)*$/.test(value)) {
    throw new Error(`域名格式无效: ${raw.trim()}`);
  }
  return value;
}

function normalizeValues(raw: string, conditionType: "domain" | "domain_suffix") {
  const values = raw
    .split("\n")
    .map((rawLine) => rawLine.trim())
    .filter(Boolean)
    .map((rawLine) => normalizeValue(rawLine, conditionType));
  const uniqueValues = [...new Set(values)];
  if (!uniqueValues.length) throw new Error("请填写至少一个域名");
  return uniqueValues;
}

function nextPriority() {
  return localRules.value.reduce((max, rule) => Math.max(max, rule.priority), -1) + 1;
}

async function saveException() {
  if (!form.value || saving.value) return;
  const current = form.value;
  const server = realServers.value.find((item) => item.tag === current.server);
  if (!server || !server.enabled) {
    emit("notify", "请选择一个已启用的非 FakeIP DNS Server", "error");
    return;
  }
  let values: string[];
  try {
    values = normalizeValues(current.values, current.conditionType);
  } catch (error) {
    emit("notify", error instanceof Error ? error.message : "域名格式无效", "error");
    return;
  }
  const duplicates = new Set(
    exceptions.value
      .filter(
        (item) =>
          item.rule.id !== current.id &&
          item.rule.server === current.server &&
          item.conditionType === current.conditionType,
      )
      .flatMap((item) =>
        item.values.flatMap((value) => {
          try {
            return [normalizeValue(value, item.conditionType)];
          } catch {
            return [];
          }
        }),
      ),
  );
  values = values.filter((value) => !duplicates.has(value));
  if (!values.length) {
    emit("notify", "这些域名已存在于相同 DNS Server 的例外规则中", "error");
    return;
  }

  const existing = current.id
    ? localRules.value.find((rule) => rule.id === current.id)
    : undefined;
  const conditions = { [current.conditionType]: values };
  const payload = {
    enabled: current.enabled,
    priority: existing?.priority ?? nextPriority(),
    rule_type: existing?.rule_type || "default",
    conditions,
    server: current.server,
    disable_cache: existing?.disable_cache || false,
    rewrite_ttl: existing?.rewrite_ttl || 0,
    client_subnet: existing?.client_subnet || "",
  };

  saving.value = true;
  try {
    if (current.id) {
      await request(`/api/v1/dns/rules/${current.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const index = localRules.value.findIndex((rule) => rule.id === current.id);
      if (index >= 0) {
        localRules.value[index] = {
          ...localRules.value[index],
          ...payload,
          conditions_json: JSON.stringify(conditions),
        };
      }
      emit("notify", "真实 IP 例外已更新", "success");
    } else {
      const created = await request("/api/v1/dns/rules", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      localRules.value.push(created);
      emit("notify", current.preset ? "常见真实 IP 例外预设已加入" : "真实 IP 例外已添加", "success");
    }
    view.value = "list";
    form.value = null;
    emit("changed");
  } catch (error) {
    emit(
      "notify",
      `保存失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    saving.value = false;
  }
}

function rulePayload(rule: DNSRule, enabled: boolean) {
  return {
    enabled,
    priority: rule.priority,
    rule_type: rule.rule_type || "default",
    conditions: parseConditions(rule),
    server: rule.server,
    disable_cache: rule.disable_cache,
    rewrite_ttl: rule.rewrite_ttl,
    client_subnet: rule.client_subnet,
  };
}

async function toggleException(item: ExceptionItem) {
  if (pendingRuleID.value !== null) return;
  pendingRuleID.value = item.rule.id;
  try {
    await request(`/api/v1/dns/rules/${item.rule.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(rulePayload(item.rule, !item.rule.enabled)),
    });
    item.rule.enabled = !item.rule.enabled;
    emit("changed");
  } catch (error) {
    emit(
      "notify",
      `状态更新失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    pendingRuleID.value = null;
  }
}

async function confirmDelete() {
  const target = deleteTarget.value;
  if (!target || pendingRuleID.value !== null) return;
  pendingRuleID.value = target.rule.id;
  try {
    await request(`/api/v1/dns/rules/${target.rule.id}`, { method: "DELETE" });
    localRules.value = localRules.value.filter((rule) => rule.id !== target.rule.id);
    deleteTarget.value = null;
    emit("changed");
    emit("notify", "真实 IP 例外已删除", "success");
  } catch (error) {
    emit(
      "notify",
      `删除失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    pendingRuleID.value = null;
  }
}

function conditionLabel(type: ExceptionItem["conditionType"]) {
  return type === "domain" ? "精确域名" : "域名后缀";
}
</script>

<template>
  <section
    class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <div class="flex justify-between">
      <h3>FakeIP</h3>
      <label class="text-sm text-[var(--text-secondary)]">
        <input :checked="settings.fakeip_enabled" type="checkbox" disabled />
        {{ settings.fakeip_enabled ? "已随 TUN 启用" : "已随 TUN 停用" }}
      </label>
    </div>
    <p class="mt-2 text-xs text-[var(--text-tertiary)]">
      FakeIP 由运行模式自动管理：TUN / TUN + Mixed 启用，Mixed 停用。显式 DNS
      规则用于国内和局域网等真实 IP 例外；启用时，所有未命中显式规则的 A/AAAA
      查询使用 FakeIP，其余查询统一经过安全 DNS final。
    </p>
    <div
      class="mt-3 grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]"
    >
      <input v-model="settings.fakeip_inet4_range" class="aw-input" />
      <input v-model="settings.fakeip_inet6_range" class="aw-input" />
      <Button @click="$emit('save')">保存范围</Button>
    </div>
    <div class="mt-5 border-t border-[var(--border-light)] pt-4">
      <h4 class="text-sm font-medium text-[var(--text-primary)]">其他功能</h4>
      <p class="mt-1 text-xs text-[var(--text-tertiary)]">
        管理不使用 FakeIP、需要返回真实地址的域名规则。
      </p>
      <Button class="mt-4" variant="primary" @click="openManager">
        <template #icon><ShieldCheck :size="14" /></template>
        真实 IP 例外<span
          v-if="exceptionValueCount"
          class="rounded-full bg-[var(--button-secondary-bg)] px-1.5 text-[10px]"
          >{{ exceptionValueCount }}</span
        >
      </Button>
    </div>
  </section>

  <Modal
    :open="managerOpen"
    :title="view === 'form' ? (form?.id ? '编辑真实 IP 例外' : form?.preset ? '加入常见例外预设' : '新增真实 IP 例外') : '真实 IP 例外'"
    size="xl"
    :closable="managerClosable"
    @close="closeManager"
  >
    <template v-if="view === 'list'">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <p class="max-w-2xl text-sm text-[var(--text-secondary)]">
          这些域名使用所选 DNS Server 返回真实地址，并在系统 FakeIP 兜底规则之前匹配。
          局域网域名应选择 local、dhcp、hosts 或实际可访问的 LAN DNS。
        </p>
        <div class="flex flex-wrap gap-2">
          <Button :disabled="!enabledServers.length" @click="openPreset">
            加入常见例外预设
          </Button>
          <Button variant="primary" :disabled="!enabledServers.length" @click="openNew">
            <template #icon><Plus :size="14" /></template>新增例外
          </Button>
        </div>
      </div>

      <div
        v-if="!enabledServers.length"
        class="mt-4 rounded-[var(--radius-lg)] border border-[var(--color-warning)]/40 bg-[var(--color-warning-bg)] px-4 py-3 text-sm text-[var(--text-secondary)]"
      >
        请先在 DNS 服务器区域启用一个非 FakeIP Server。
      </div>

      <div class="aw-data-table-wrap mt-4">
        <table class="aw-data-table min-w-[760px]">
          <thead>
            <tr>
              <th>类型</th>
              <th>域名</th>
              <th>DNS Server</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!exceptions.length">
              <td colspan="5" class="py-10 text-center text-[var(--text-tertiary)]">
                暂无真实 IP 例外
              </td>
            </tr>
            <tr v-for="item in exceptions" :key="item.rule.id">
              <td>{{ conditionLabel(item.conditionType) }}</td>
              <td class="max-w-[440px] truncate" :title="item.values.join('\n')">
                {{ item.values.join("、") }}
              </td>
              <td>
                {{ item.rule.server }}
                <span
                  v-if="!realServers.some((server) => server.tag === item.rule.server && server.enabled)"
                  class="ml-1 text-xs text-[var(--color-warning)]"
                  >不可用</span
                >
              </td>
              <td>
                <button
                  class="aw-action-button"
                  :class="item.rule.enabled ? 'aw-action-success' : 'aw-action-neutral'"
                  :disabled="pendingRuleID !== null"
                  @click="toggleException(item)"
                >
                  {{ item.rule.enabled ? "启用" : "停用" }}
                </button>
              </td>
              <td>
                <div class="flex gap-2">
                  <button
                    class="aw-action-button aw-action-neutral"
                    :disabled="pendingRuleID !== null"
                    @click="openEdit(item)"
                  >
                    <Pencil :size="13" />编辑
                  </button>
                  <button
                    class="aw-action-button aw-action-danger"
                    :disabled="pendingRuleID !== null"
                    @click="deleteTarget = item"
                  >
                    <Trash2 :size="13" />删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <template v-else-if="form">
      <Button variant="ghost" size="sm" :disabled="saving" @click="view = 'list'">
        <template #icon><ArrowLeft :size="14" /></template>返回列表
      </Button>
      <div class="mt-4 grid gap-4 md:grid-cols-2">
        <label class="text-sm text-[var(--text-secondary)]">
          匹配方式
          <select
            v-model="form.conditionType"
            class="aw-input mt-1 w-full"
            :disabled="form.preset"
          >
            <option value="domain">精确域名</option>
            <option value="domain_suffix">域名后缀</option>
          </select>
        </label>
        <label class="text-sm text-[var(--text-secondary)]">
          DNS Server
          <select v-model="form.server" class="aw-input mt-1 w-full">
            <option value="">请选择 Server</option>
            <option
              v-for="server in realServers"
              :key="server.tag"
              :value="server.tag"
              :disabled="!server.enabled"
            >
              {{ server.tag }} · {{ server.server_type }}{{ server.enabled ? "" : " · 已停用" }}
            </option>
          </select>
          <span v-if="form.preset" class="mt-1 block text-xs text-[var(--color-warning)]">
            此预设只让这些域名绕过 FakeIP，默认使用普通 DNS 获取真实结果；它不负责
            LAN 主机名解析。TUN/DNS 劫持模式下不要选择回指 Ackwrap 的 local 或
            127.0.0.1 DNS，否则可能形成查询回环。
          </span>
        </label>
        <label class="text-sm text-[var(--text-secondary)] md:col-span-2">
          域名列表
          <textarea
            v-model="form.values"
            rows="10"
            class="aw-input mt-1 w-full resize-y font-mono"
            placeholder="每行一个域名，例如 wpad.lan"
          />
          <span class="mt-1 block text-xs text-[var(--text-tertiary)]">
            每行一个；域名后缀支持输入 *.lan、+.lan 或 .lan，保存时统一规范化为 lan。
            不支持 localhost.* 形式的尾部通配符。
          </span>
        </label>
      </div>
      <label class="mt-4 inline-flex items-center gap-2 text-sm">
        <input v-model="form.enabled" type="checkbox" />保存后立即启用
      </label>
    </template>

    <template #footer>
      <template v-if="view === 'list'">
        <Button :disabled="!managerClosable" @click="closeManager">关闭</Button>
      </template>
      <template v-else>
        <Button :disabled="saving" @click="view = 'list'">取消</Button>
        <Button variant="primary" :loading="saving" @click="saveException">保存</Button>
      </template>
    </template>
  </Modal>

  <ConfirmDialog
    :open="!!deleteTarget"
    style="z-index: calc(var(--z-modal) + 1)"
    title="删除真实 IP 例外"
    :message="`确定删除这条包含 ${deleteTarget?.values.length || 0} 个域名的例外规则吗？`"
    confirm-text="删除"
    danger
    @confirm="confirmDelete"
    @cancel="deleteTarget = null"
  />
</template>
