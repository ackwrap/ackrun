<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Plus, Save, ServerCog, Trash2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import OrderButtons from "@/components/ui/OrderButtons.vue";
import { authenticatedFetch } from "@/services/apiAuth";
import DNSRuleFormModal from "./dns/DNSRuleFormModal.vue";
import DNSServerFormModal from "./dns/DNSServerFormModal.vue";
interface Server {
  id: number;
  tag: string;
  enabled: boolean;
  server_type: string;
  address: string;
  address_resolver: string;
  address_strategy: string;
  strategy: string;
  detour: string;
  client_subnet: string;
  priority: number;
}
interface Rule {
  id: number;
  enabled: boolean;
  priority: number;
  conditions_json: string;
  server: string;
  disable_cache: boolean;
  rewrite_ttl: number;
  client_subnet: string;
}
interface Col {
  name: string;
  enabled: boolean;
}
const defaults = {
    enabled: true,
    final: "dns_proxy",
    strategy: "prefer_ipv4",
    disable_cache: false,
    disable_expire: false,
    independent_cache: false,
    reverse_mapping: false,
    cache_capacity: 4096,
    client_subnet: "",
    fakeip_enabled: false,
    fakeip_inet4_range: "198.18.0.0/15",
    fakeip_inet6_range: "fdfe:dcba:9876::/48",
  },
  servers = ref<Server[]>([]),
  rules = ref<Rule[]>([]),
  collections = ref<Col[]>([]),
  global = ref({ ...defaults }),
  loading = ref(true),
  message = ref(""),
  messageType = ref<"success" | "error">("success"),
  serverForm = ref<Partial<Server> | null>(null),
  ruleForm = ref<any | null>(null),
  ruleSaving = ref(false),
  ruleOrderPending = ref(false);
const types = [
    "udp",
    "tcp",
    "https",
    "tls",
    "quic",
    "h3",
    "local",
    "hosts",
    "dhcp",
    "rcode",
  ],
  conditions = [
    "domain",
    "domain_suffix",
    "domain_keyword",
    "domain_regex",
    "geosite",
    "query_type",
    "network",
    "protocol",
    "clash_mode",
    "rule_set",
  ],
  strategies = ["prefer_ipv4", "prefer_ipv6", "ipv4_only", "ipv6_only"],
  presets = [
    ["阿里 DoH", "dns_ali", "https", "https://dns.alidns.com/dns-query", ""],
    ["阿里 DNS UDP", "dns_ali_udp", "udp", "223.5.5.5", ""],
    ["阿里 DNS UDP 备用", "dns_ali_udp_2", "udp", "223.6.6.6", ""],
    [
      "腾讯 DNSPod DoH",
      "dns_tencent",
      "https",
      "https://doh.pub/dns-query",
      "",
    ],
    ["腾讯 DNSPod UDP", "dns_tencent_udp", "udp", "119.29.29.29", ""],
    ["腾讯 DNSPod UDP 备用", "dns_tencent_udp_2", "udp", "119.28.28.28", ""],
    [
      "Cloudflare DoH",
      "dns_cloudflare",
      "https",
      "https://cloudflare-dns.com/dns-query",
      "proxy",
    ],
    [
      "Google DoH",
      "dns_google",
      "https",
      "https://dns.google/dns-query",
      "proxy",
    ],
    [
      "Quad9 DoH",
      "dns_quad9",
      "https",
      "https://dns.quad9.net/dns-query",
      "proxy",
    ],
    ["114 DNS UDP", "dns_114", "udp", "114.114.114.114", ""],
    ["百度 DNS UDP", "dns_baidu", "udp", "180.76.76.76", ""],
    ["移动 DNS UDP", "dns_mobile", "udp", "211.136.192.6", ""],
    ["联通 DNS UDP", "dns_unicom", "udp", "123.125.81.6", ""],
    ["电信 DNS UDP", "dns_telecom", "udp", "202.96.128.86", ""],
  ],
  preset = ref(0);
async function request(url: string, init?: RequestInit) {
  const r = await authenticatedFetch(url, init);
  const x = await r.json().catch(() => null);
  if (!r.ok) throw new Error(x?.error?.message || r.statusText);
  return x;
}
const show = (s: string, t: "success" | "error" = "success") => {
  message.value = s;
  messageType.value = t;
};
async function load() {
  try {
    const [a, b, c, d] = await Promise.all([
      request("/api/v1/dns/servers"),
      request("/api/v1/dns/rules"),
      request("/api/v1/dns/global"),
      request("/api/v1/collections").catch(() => []),
    ]);
    servers.value = Array.isArray(a) ? a : [];
    rules.value = Array.isArray(b) ? b : [];
    collections.value = Array.isArray(d) ? d : [];
    Object.assign(global.value, c || {});
  } catch (e: any) {
    show(`加载失败: ${e.message}`, "error");
  } finally {
    loading.value = false;
  }
}
const detours = computed(() => [
    "",
    "direct",
    "proxy",
    ...collections.value.filter((x) => x.enabled).map((x) => x.name),
  ]),
  matchingRules = computed(() =>
    rules.value.filter((rule) => !hasLegacyOutboundCondition(rule)),
  ),
  conditionText = (r: Rule) => {
    try {
      const x = JSON.parse(r.conditions_json);
      return Object.keys(x)
        .map((k) => `${k}: ${JSON.stringify(x[k])}`)
        .join(", ");
    } catch {
      return "(空)";
    }
  };
function hasLegacyOutboundCondition(rule: Rule) {
  try {
    const conditions = JSON.parse(rule.conditions_json || "{}");
    return Object.prototype.hasOwnProperty.call(conditions, "outbound");
  } catch {
    return false;
  }
}
async function saveGlobal() {
  try {
    await request("/api/v1/dns/global", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(global.value),
    });
    show("全局设置已保存");
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
  }
}
function newServer() {
  serverForm.value = {
    enabled: true,
    server_type: "https",
    tag: "",
    address: "",
    address_resolver: "",
    address_strategy: "",
    strategy: "",
    detour: "",
    client_subnet: "",
  };
}
async function saveServer() {
  const s = serverForm.value!;
  try {
    await request(
      s.id ? `/api/v1/dns/servers/${s.id}` : "/api/v1/dns/servers",
      {
        method: s.id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...s, options: {} }),
      },
    );
    show(s.id ? "DNS 服务器已更新" : "DNS 服务器已添加");
    serverForm.value = null;
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
  }
}
async function addPreset() {
  const x = presets[preset.value];
  try {
    await request("/api/v1/dns/servers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        tag: x[1],
        enabled: true,
        server_type: x[2],
        address: x[3],
        detour: x[4],
        options: {},
      }),
    });
    show(`${x[0]} 已加入`);
    await load();
  } catch (e: any) {
    show(`加入内置 Server 失败: ${e.message}`, "error");
  }
}
async function removeServer(s: Server) {
  if (confirm(`确定删除 DNS 服务器 "${s.tag}" 吗？`)) {
    try {
      await request(`/api/v1/dns/servers/${s.id}`, { method: "DELETE" });
      show("DNS 服务器已删除");
      await load();
    } catch (e: any) {
      show(`删除失败: ${e.message}`, "error");
    }
  }
}
async function toggleServer(s: Server) {
  try {
    await request(`/api/v1/dns/servers/${s.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...s, enabled: !s.enabled, options: {} }),
    });
    await load();
  } catch (e: any) {
    show(`状态更新失败: ${e.message}`, "error");
  }
}
function swapped<T>(items: T[], index: number, direction: -1 | 1) {
  const target = index + direction;
  if (target < 0 || target >= items.length) return null;
  const next = [...items];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}
async function moveServer(index: number, direction: -1 | 1) {
  const next = swapped(servers.value, index, direction);
  if (!next) return;
  servers.value = next;
  try {
    await request("/api/v1/dns/servers/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next.map((item) => item.id)),
    });
  } catch (e: any) {
    show(`DNS Server 排序失败: ${e.message}`, "error");
    await load();
  }
}
async function moveRule(index: number, direction: -1 | 1) {
  if (ruleOrderPending.value) return;
  const reordered = swapped(matchingRules.value, index, direction);
  if (!reordered) return;
  let nextRuleIndex = 0;
  const next = rules.value.map((rule) =>
    hasLegacyOutboundCondition(rule) ? rule : reordered[nextRuleIndex++],
  );
  ruleOrderPending.value = true;
  rules.value = next;
  try {
    await request("/api/v1/dns/rules/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next.map((item) => item.id)),
    });
  } catch (e: any) {
    show(`DNS 规则排序失败: ${e.message}`, "error");
    await load();
  } finally {
    ruleOrderPending.value = false;
  }
}
function newRule() {
  ruleForm.value = {
    enabled: true,
    server: "",
    disable_cache: false,
    rewrite_ttl: 0,
    client_subnet: "",
    condition_type: "domain_suffix",
    values: "",
  };
}
function editRule(r: Rule) {
  let type = "domain_suffix",
    values = "";
  try {
    const c = JSON.parse(r.conditions_json),
      k = Object.keys(c)[0];
    type = k;
    values = Array.isArray(c[k]) ? c[k].join("\n") : String(c[k]);
  } catch {}
  ruleForm.value = { ...r, condition_type: type, values };
}
async function saveRule() {
  if (ruleSaving.value) return;
  const f = ruleForm.value,
    v = f.values
      .split("\n")
      .map((x: string) => x.trim())
      .filter(Boolean),
    c: { [k: string]: any } = {
      [f.condition_type]:
        f.condition_type === "clash_mode" ? v[0] || "rule" : v,
    },
    rewriteTTL = Number(f.rewrite_ttl || 0);
  if (!f.server) {
    show("请选择 DNS Server", "error");
    return;
  }
  if (!v.length && f.condition_type !== "clash_mode") {
    show("请填写至少一个匹配值", "error");
    return;
  }
  if (!Number.isInteger(rewriteTTL) || rewriteTTL < 0) {
    show("Rewrite TTL 必须是大于或等于 0 的整数", "error");
    return;
  }
  ruleSaving.value = true;
  try {
    await request(f.id ? `/api/v1/dns/rules/${f.id}` : "/api/v1/dns/rules", {
      method: f.id ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        enabled: f.enabled,
        priority: f.priority || 0,
        rule_type: "default",
        conditions: c,
        server: f.server,
        disable_cache: f.disable_cache,
        rewrite_ttl: rewriteTTL,
        client_subnet: f.client_subnet,
      }),
    });
    show(f.id ? "DNS 规则已更新" : "DNS 规则已添加");
    ruleForm.value = null;
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
  } finally {
    ruleSaving.value = false;
  }
}
async function removeRule(r: Rule) {
  if (confirm("确定删除此 DNS 规则吗？")) {
    try {
      await request(`/api/v1/dns/rules/${r.id}`, { method: "DELETE" });
      show("DNS 规则已删除");
      await load();
    } catch (e: any) {
      show(`删除失败: ${e.message}`, "error");
    }
  }
}
async function toggleRule(r: Rule) {
  try {
    await request(`/api/v1/dns/rules/${r.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...r,
        enabled: !r.enabled,
        conditions: JSON.parse(r.conditions_json || "{}"),
      }),
    });
    await load();
  } catch (e: any) {
    show(`状态更新失败: ${e.message}`, "error");
  }
}
onMounted(load);
</script>
<template>
  <div class="space-y-4">
    <PageHeader title="DNS 管理" /><Toast
      :message="message"
      :type="messageType"
      @dismiss="message = ''"
    />
    <div v-if="loading" class="py-20 text-center">加载中...</div>
    <template v-else
      ><div class="grid gap-4 lg:grid-cols-2">
        <section
        class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <div class="flex justify-between">
          <h3><ServerCog class="inline" /> 全局设置</h3>
          <label
            ><input v-model="global.enabled" type="checkbox" /> 启用 DNS
            管理</label
          >
        </div>
        <div class="mt-4 grid gap-3 md:grid-cols-5">
          <label
            >默认 Server<select v-model="global.final">
              <option value="">请选择</option>
              <option
                v-for="s in servers.filter((x) => x.enabled && x.server_type !== 'fakeip')"
                :value="s.tag"
              >
                {{ s.tag }}
              </option>
            </select></label
          ><label
            >IP 返回策略<select v-model="global.strategy">
              <option v-for="x in strategies">{{ x }}</option>
            </select></label
          ><label
            >缓存容量<input
              v-model.number="global.cache_capacity"
              type="number" /></label
          ><label>Client Subnet<input v-model="global.client_subnet" /></label
          ><button
            class="aw-action-button aw-action-success self-end justify-self-end px-4"
            @click="saveGlobal"
          >
            <Save :size="13" />保存全局设置
          </button>
        </div>
        <div class="mt-3 flex gap-4">
          <label
            v-for="x in [
              'disable_cache',
              'disable_expire',
              'independent_cache',
              'reverse_mapping',
            ]"
            ><input
              v-model="global[x as keyof typeof global]"
              type="checkbox"
            />
            {{ x }}</label
          >
        </div>
        </section>
        <section
        class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <div class="flex justify-between">
          <h3>FakeIP</h3>
          <label class="text-sm text-[var(--text-secondary)]">
            <input :checked="global.fakeip_enabled" type="checkbox" disabled />
            {{ global.fakeip_enabled ? "已随 TUN 启用" : "已随 TUN 停用" }}
          </label>
        </div>
        <p class="mt-2 text-xs text-[var(--text-tertiary)]">
          FakeIP 由运行模式自动管理：TUN / TUN + Mixed 启用，Mixed 停用。
          显式 DNS 规则用于国内和局域网等真实 IP 例外；TUN 未命中的 A/AAAA
          查询使用 FakeIP，其余真实查询统一经过安全 DNS final。
        </p>
        <div class="mt-3 grid gap-3 md:grid-cols-3">
          <input v-model="global.fakeip_inet4_range" /><input
            v-model="global.fakeip_inet6_range"
          /><button @click="saveGlobal">保存 FakeIP</button>
        </div>
        </section>
      </div>
      <section
        class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <header class="flex flex-wrap items-center justify-between gap-3">
          <h3 class="font-semibold">DNS 服务器</h3>
          <div class="flex flex-wrap items-center gap-2">
            <select v-model.number="preset">
              <option v-for="(x, i) in presets" :value="i">
                {{ x[0] }}
              </option>
            </select>
            <button
              class="aw-action-button aw-action-neutral"
              @click="addPreset"
            >
              加入内置 Server
            </button>
            <button
              class="aw-action-button aw-action-neutral"
              @click="newServer"
            >
              <Plus :size="13" />新增 Server
            </button>
          </div>
        </header>
        <div class="aw-data-table-wrap mt-4">
          <table class="aw-data-table min-w-[760px]">
            <thead>
              <tr>
                <th>排序</th>
                <th>Tag</th>
                <th>类型</th>
                <th>地址</th>
                <th>Detour</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!servers.length">
                <td colspan="7" class="py-10 text-center">暂无 DNS 服务器</td>
              </tr>
              <tr v-for="(s, i) in servers" :key="s.id">
                <td>
                  <OrderButtons
                    :up-disabled="i === 0"
                    :down-disabled="i === servers.length - 1"
                    @up="moveServer(i, -1)"
                    @down="moveServer(i, 1)"
                  />
                </td>
                <td class="font-medium text-[var(--text-primary)]">
                  {{ s.tag }}
                </td>
                <td>{{ s.server_type }}</td>
                <td class="max-w-[420px] truncate" :title="s.address">
                  {{ s.address }}
                </td>
                <td>{{ s.detour || "-" }}</td>
                <td>
                  <button
                    class="aw-action-button"
                    :class="
                      s.enabled ? 'aw-action-success' : 'aw-action-neutral'
                    "
                    @click="toggleServer(s)"
                  >
                    {{ s.enabled ? "启用" : "停用" }}
                  </button>
                </td>
                <td>
                  <div class="flex gap-2">
                    <button
                      class="aw-action-button aw-action-neutral"
                      @click="serverForm = { ...s }"
                    >
                      编辑</button
                    ><button
                      class="aw-action-button aw-action-danger"
                      @click="removeServer(s)"
                    >
                      <Trash2 :size="13" />删除
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section
        class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <header class="flex items-center justify-between gap-3">
          <div>
            <h3 class="font-semibold">DNS 规则</h3>
            <p class="mt-1 text-xs text-[var(--text-tertiary)]">
              从上到下匹配，适合配置国内、局域网等需要真实 IP 的例外；TUN
              模式下 FakeIP 位于这些显式规则之后。
            </p>
          </div>
          <button class="aw-action-button aw-action-neutral" @click="newRule">
            <Plus :size="13" />新增规则
          </button>
        </header>
        <div class="aw-data-table-wrap mt-4">
          <table class="aw-data-table min-w-[760px]">
            <thead>
              <tr>
                <th>排序</th>
                <th>匹配条件</th>
                <th>DNS 服务器</th>
                <th>禁用缓存</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!matchingRules.length">
                <td colspan="6" class="py-10 text-center">暂无 DNS 规则</td>
              </tr>
              <tr v-for="(r, i) in matchingRules" :key="r.id">
                <td>
                  <OrderButtons
                    :up-disabled="ruleOrderPending || i === 0"
                    :down-disabled="ruleOrderPending || i === matchingRules.length - 1"
                    @up="moveRule(i, -1)"
                    @down="moveRule(i, 1)"
                  />
                </td>
                <td class="max-w-[520px] truncate" :title="conditionText(r)">
                  {{ conditionText(r) }}
                </td>
                <td>{{ r.server }}</td>
                <td>{{ r.disable_cache ? "是" : "否" }}</td>
                <td>
                  <button
                    class="aw-action-button"
                    :class="
                      r.enabled ? 'aw-action-success' : 'aw-action-neutral'
                    "
                    @click="toggleRule(r)"
                  >
                    {{ r.enabled ? "启用" : "停用" }}
                  </button>
                </td>
                <td>
                  <div class="flex gap-2">
                    <button
                      class="aw-action-button aw-action-neutral"
                      @click="editRule(r)"
                    >
                      编辑</button
                    ><button
                      class="aw-action-button aw-action-danger"
                      @click="removeRule(r)"
                    >
                      删除
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section></template
    >
    <DNSServerFormModal
      v-if="serverForm"
      :form="serverForm"
      :types="types"
      :strategies="strategies"
      :detours="detours"
      @close="serverForm = null"
      @save="saveServer"
    />
    <DNSRuleFormModal
      v-if="ruleForm"
      :form="ruleForm"
      :conditions="conditions"
      :servers="servers.filter((server) => server.enabled && server.server_type !== 'fakeip')"
      :saving="ruleSaving"
      @close="ruleForm = null"
      @save="saveRule"
    />
  </div>
</template>
