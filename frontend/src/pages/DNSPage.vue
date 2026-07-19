<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Plus, Save, ServerCog, Trash2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import OrderButtons from "@/components/ui/OrderButtons.vue";
import { authenticatedFetch } from "@/services/apiAuth";
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
    fakeip_inet4_range: "198.19.0.0/16",
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
  bindings = ref<Record<string, string>>({}),
  outboundOrder = ref<string[]>([]),
  preview = ref("");
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
    "fakeip",
    "rcode",
  ],
  conditions = [
    "domain",
    "domain_suffix",
    "domain_keyword",
    "domain_regex",
    "geosite",
    "outbound",
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
    const [a, b, c, d, e] = await Promise.all([
      request("/api/v1/dns/servers"),
      request("/api/v1/dns/rules"),
      request("/api/v1/dns/global"),
      request("/api/v1/collections").catch(() => []),
      request("/api/v1/dns/outbound-bindings/order").catch(() => ({
        outbounds: [],
      })),
    ]);
    servers.value = Array.isArray(a) ? a : [];
    rules.value = Array.isArray(b) ? b : [];
    collections.value = Array.isArray(d) ? d : [];
    outboundOrder.value = Array.isArray(e?.outbounds) ? e.outbounds : [];
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
  targets = computed(() => {
    const available = detours.value.filter(Boolean);
    return [
      ...outboundOrder.value.filter((item) => available.includes(item)),
      ...available.filter((item) => !outboundOrder.value.includes(item)),
    ];
  }),
  outbounds = (r: Rule) => {
    try {
      const x = JSON.parse(r.conditions_json || "{}").outbound;
      return Array.isArray(x) ? x : typeof x === "string" ? [x] : [];
    } catch {
      return [];
    }
  },
  bindingRule = (x: string) =>
    rules.value.find((r) => outbounds(r).includes(x)),
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
async function moveBinding(index: number, direction: -1 | 1) {
  const next = swapped(targets.value, index, direction);
  if (!next) return;
  outboundOrder.value = next;
  try {
    await request("/api/v1/dns/outbound-bindings/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ outbounds: next }),
    });
  } catch (e: any) {
    show(`DNS 出口绑定排序失败: ${e.message}`, "error");
    await load();
  }
}
async function saveBinding(x: string) {
  const old = bindingRule(x),
    server = bindings.value[x] ?? old?.server ?? "";
  try {
    if (!server) {
      if (old)
        await request(`/api/v1/dns/rules/${old.id}`, { method: "DELETE" });
    } else
      await request(old ? `/api/v1/dns/rules/${old.id}` : "/api/v1/dns/rules", {
        method: old ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          enabled: true,
          priority: old?.priority || 0,
          rule_type: "default",
          conditions: { outbound: [x] },
          server,
          disable_cache: old?.disable_cache || false,
          rewrite_ttl: old?.rewrite_ttl || 0,
          client_subnet: old?.client_subnet || "",
        }),
      });
    show(`${x} 的 DNS 出口绑定已保存`);
    await load();
  } catch (e: any) {
    show(`保存 DNS 出口绑定失败: ${e.message}`, "error");
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
  const f = ruleForm.value,
    v = f.values
      .split("\n")
      .map((x: string) => x.trim())
      .filter(Boolean),
    c: { [k: string]: any } = {
      [f.condition_type]:
        f.condition_type === "clash_mode" ? v[0] || "rule" : v,
    };
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
        rewrite_ttl: +f.rewrite_ttl || 0,
        client_subnet: f.client_subnet,
      }),
    });
    show(f.id ? "DNS 规则已更新" : "DNS 规则已添加");
    ruleForm.value = null;
    await load();
  } catch (e: any) {
    show(`保存失败: ${e.message}`, "error");
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
      ><section
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
                v-for="s in servers.filter((x) => x.enabled)"
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
          <label
            ><input
              v-model="global.fakeip_enabled"
              type="checkbox"
            />启用</label
          >
        </div>
        <div class="mt-3 grid gap-3 md:grid-cols-3">
          <input v-model="global.fakeip_inet4_range" /><input
            v-model="global.fakeip_inet6_range"
          /><button @click="saveGlobal">保存 FakeIP</button>
        </div>
      </section>
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
        <h3 class="font-semibold">DNS 出口绑定</h3>
        <div class="aw-data-table-wrap mt-4">
          <table class="aw-data-table min-w-[760px]">
            <thead>
              <tr>
                <th>排序</th>
                <th>出站</th>
                <th>DNS 服务器</th>
                <th>绑定状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(x, i) in targets" :key="x">
                <td>
                  <OrderButtons
                    :up-disabled="i === 0"
                    :down-disabled="i === targets.length - 1"
                    @up="moveBinding(i, -1)"
                    @down="moveBinding(i, 1)"
                  />
                </td>
                <td>{{ x }}</td>
                <td>
                  <select
                    :value="bindings[x] ?? bindingRule(x)?.server ?? ''"
                    @change="
                      bindings[x] = ($event.target as HTMLSelectElement).value
                    "
                  >
                    <option value="">不绑定</option>
                    <option
                      v-for="s in servers.filter(
                        (s) => s.enabled && s.server_type !== 'fakeip',
                      )"
                      :value="s.tag"
                    >
                      {{ s.tag }}
                    </option>
                  </select>
                </td>
                <td>
                  {{
                    bindingRule(x)
                      ? `domain_resolver 绑定 #${bindingRule(x)?.id}`
                      : "未生成"
                  }}
                </td>
                <td>
                  <div class="flex gap-2">
                    <button
                      class="aw-action-button aw-action-neutral"
                      @click="
                        preview = JSON.stringify(
                          {
                            tag: x,
                            domain_resolver:
                              (bindings[x] ?? bindingRule(x)?.server)
                                ? {
                                    server:
                                      bindings[x] ?? bindingRule(x)?.server,
                                  }
                                : '(未绑定)',
                          },
                          null,
                          2,
                        )
                      "
                    >
                      预览</button
                    ><button
                      class="aw-action-button aw-action-success"
                      @click="saveBinding(x)"
                    >
                      保存绑定
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
          <h3 class="font-semibold">DNS 规则</h3>
          <button class="aw-action-button aw-action-neutral" @click="newRule">
            <Plus :size="13" />新增规则
          </button>
        </header>
        <div class="aw-data-table-wrap mt-4">
          <table class="aw-data-table min-w-[760px]">
            <thead>
              <tr>
                <th>匹配条件</th>
                <th>DNS 服务器</th>
                <th>禁用缓存</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!rules.length">
                <td colspan="5" class="py-10 text-center">暂无 DNS 规则</td>
              </tr>
              <tr v-for="r in rules" :key="r.id">
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
    <div v-if="ruleForm" class="aw-modal-backdrop">
      <div class="aw-modal-panel max-w-4xl p-5">
        <h3>{{ ruleForm.id ? "编辑" : "新增" }} DNS 规则</h3>
        <select v-model="ruleForm.condition_type">
          <option v-for="x in conditions">{{ x }}</option></select
        ><textarea v-model="ruleForm.values" rows="4" /><select
          v-model="ruleForm.server"
        >
          <option value="">请选择 Server</option>
          <option v-for="s in servers.filter((x) => x.enabled)" :value="s.tag">
            {{ s.tag }}
          </option>
          <option v-if="global.fakeip_enabled">fakeip</option></select
        ><input v-model.number="ruleForm.rewrite_ttl" type="number" /><input
          v-model="ruleForm.client_subnet"
        /><label
          ><input
            v-model="ruleForm.disable_cache"
            type="checkbox"
          />禁用缓存</label
        ><label><input v-model="ruleForm.enabled" type="checkbox" />启用</label
        ><button @click="ruleForm = null">取消</button
        ><button @click="saveRule">保存</button>
      </div>
    </div>
    <div v-if="preview" class="aw-modal-backdrop" @click="preview = ''">
      <pre class="aw-modal-panel max-w-xl p-5">{{ preview }}</pre>
    </div>
  </div>
</template>
