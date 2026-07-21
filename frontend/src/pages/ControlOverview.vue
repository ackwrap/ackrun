<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import {
  Activity,
  ArrowUpRight,
  CircleAlert,
  FileCheck2,
  FileCode,
  Globe2,
  Layers,
  ListChecks,
  Network,
  RadioTower,
  RefreshCw,
  Rss,
  ServerCog,
  Settings,
  ShieldCheck,
} from "lucide-vue-next";
import { api } from "@/services/api";
const props = defineProps<{
    refreshKey: number;
    configStatus: {
      has_config: boolean;
      validated: boolean;
      valid: boolean;
    } | null;
    proxyMode: string;
  }>(),
  emit = defineEmits<{
    resourcesChanged: [];
    message: [string, "success" | "error" | "info"];
  }>();
const links = [
  ["/subscriptions", "订阅管理", "同步与流量信息", RadioTower],
  ["/nodes", "节点管理", "可用性与启用状态", Network],
  ["/rules", "规则管理", "分流、规则集与 Geo", ListChecks],
  ["/collections", "策略组管理", "代理策略与节点选择", Layers],
  ["/dns", "DNS 管理", "服务器与解析规则", ServerCog],
  ["/config", "配置生成", "生成与校验配置", FileCode],
  ["/logs", "日志", "核心与系统日志", Activity],
  ["/settings", "设置", "更新与节点过滤", Settings],
] as const;
const summary = ref<any>(null),
  error = ref(""),
  runningAction = ref("");
let mounted = true,
  timer: number;
const value = (r: PromiseSettledResult<any>) =>
  r.status === "fulfilled" ? r.value : null;
async function load() {
  const rs = await Promise.allSettled([
    api.getNodeFacets(),
    api.getNodes({ enabled: true, limit: 1 }),
    api.getNodes({ status: "available", limit: 1 }),
    api.getSubscriptions(),
    api.getRouteRules(),
    api.getRouteRuleSubscriptions(),
    api.getGeoAssets(),
  ]);
  if (!mounted) return;
  const f = value(rs[0]),
    en = value(rs[1]),
    av = value(rs[2]),
    subs = value(rs[3])?.filter((x: any) => x.url !== "manual://local"),
    rules = value(rs[4]),
    ruleSubs = value(rs[5]),
    geo = value(rs[6]),
    resources = [
      ...(ruleSubs?.filter((x: any) => x.enabled) || []),
      ...(geo || []),
    ],
    ready = resources.filter(
      (x: any) => x.cached_updated_at > 0 && x.sync_status !== "failed",
    ),
    old = summary.value || {};
  summary.value = {
    totalNodes: f?.total ?? old.totalNodes ?? 0,
    enabledNodes: en?.total ?? old.enabledNodes ?? 0,
    availableNodes: av?.total ?? old.availableNodes ?? 0,
    subscriptions: subs?.length ?? old.subscriptions ?? 0,
    failedSubscriptions:
      subs?.filter((x: any) => x.sync_status === "failed").length ??
      old.failedSubscriptions ??
      0,
    expiringSubscriptions:
      subs?.filter(
        (x: any) => x.expire_at > 0 && x.expire_at <= Date.now() + 604800000,
      ).length ??
      old.expiringSubscriptions ??
      0,
    enabledRules:
      rules?.filter((x: any) => x.enabled).length ?? old.enabledRules ?? 0,
    ruleResources:
      ruleSubs && geo ? resources.length : (old.ruleResources ?? 0),
    readyRuleResources:
      ruleSubs && geo ? ready.length : (old.readyRuleResources ?? 0),
  };
  const labels = [
    "节点统计",
    "启用节点",
    "可用节点",
    "订阅状态",
    "路由规则",
    "规则订阅",
    "Geo 资源",
  ];
  const failures = rs
    .map((r, i) =>
      r.status === "rejected"
        ? `${labels[i]}: ${r.reason instanceof Error ? r.reason.message : "请求失败"}`
        : "",
    )
    .filter(Boolean);
  error.value = failures.length
    ? `部分状态加载失败：${failures.join("；")}`
    : "";
}
const actions = [
  {
    key: "subscriptions",
    label: "同步节点订阅",
    detail: "拉取全部外部订阅",
    icon: Rss,
    action: api.syncAllSubscriptions,
    bg: true,
  },
  {
    key: "validate",
    label: "校验当前配置",
    detail: "执行 sing-box check",
    icon: FileCheck2,
    action: async () => {
      const s = await api.validateConfig();
      if (!s.valid) throw Error(s.error || "当前配置校验未通过");
    },
  },
  {
    key: "rules",
    label: "更新规则订阅",
    detail: "刷新全部远程规则集",
    icon: ShieldCheck,
    action: api.syncAllRouteRuleSubscriptions,
    bg: true,
  },
  {
    key: "geo",
    label: "更新 Geo 资源",
    detail: "刷新 GeoIP 与 Geosite",
    icon: Globe2,
    action: api.syncAllGeoAssets,
    bg: true,
  },
];
async function run(a: any) {
  runningAction.value = a.key;
  if (a.bg) emit("message", `${a.label}任务启动中`, "info");
  try {
    await a.action();
    if (!a.bg) emit("message", `${a.label}成功`, "success");
  } catch (e: any) {
    emit("message", `${a.label}失败：${e?.message || "请求失败"}`, "error");
  } finally {
    emit("resourcesChanged");
    runningAction.value = "";
  }
}
const warnings = computed(() => {
  const w: any[] = [];
  if (
    props.configStatus &&
    (!props.configStatus.has_config ||
      (props.configStatus.validated && !props.configStatus.valid))
  )
    w.push([
      "配置尚未就绪",
      props.configStatus.has_config
        ? "当前配置校验未通过"
        : "需要先生成可用配置",
      "/config",
    ]);
  const s = summary.value;
  if (s?.enabledNodes === 0)
    w.push(["没有启用节点", `节点池共 ${s.totalNodes} 个节点`, "/nodes"]);
  if (s?.failedSubscriptions)
    w.push([
      `${s.failedSubscriptions} 个订阅同步失败`,
      "查看订阅页中的具体失败原因",
      "/subscriptions",
    ]);
  if (s?.expiringSubscriptions)
    w.push([
      `${s.expiringSubscriptions} 个订阅即将到期`,
      "到期时间不足 7 天",
      "/subscriptions",
    ]);
  if (s && props.proxyMode === "rule" && !s.enabledRules)
    w.push(["规则模式没有启用规则", "流量将只使用默认出站策略", "/rules"]);
  if (s && s.readyRuleResources < s.ruleResources)
    w.push([
      "规则资源未全部就绪",
      `${s.readyRuleResources}/${s.ruleResources} 个规则集或 Geo 资源可用`,
      "/rules",
    ]);
  return w;
});
watch(
  () => props.refreshKey,
  () => load(),
);
onMounted(() => {
  void load();
  timer = window.setInterval(load, 60000);
});
onBeforeUnmount(() => {
  mounted = false;
  clearInterval(timer);
});
</script>
<template>
  <div
    class="order-1 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <h3 class="mb-2 text-sm font-semibold">快捷入口</h3>
    <div
      v-if="error"
      class="mb-1.5 bg-[var(--color-error-bg)] p-2 text-xs text-[var(--color-error)]"
    >
      {{ error }}
    </div>
    <div class="grid gap-1.5 sm:grid-cols-2">
      <RouterLink
        v-for="l in links"
        :key="l[0]"
        :to="l[0]"
        class="group flex items-center gap-2.5 rounded border px-3 py-1.5"
        ><span
          class="flex h-8 w-8 items-center justify-center bg-[var(--color-primary-bg)]"
          ><component :is="l[3]" :size="15" /></span
        ><span class="min-w-0 flex-1"
          ><b class="block text-xs">{{ l[1] }}</b
          ><small>{{ l[2] }}</small></span
        ><ArrowUpRight :size="13"
      /></RouterLink>
    </div>
  </div>
  <div
    class="order-2 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-3 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <h3 class="mb-2 text-sm font-semibold">快捷任务</h3>
    <div class="grid flex-1 grid-rows-4 gap-1.5">
      <button
        v-for="a in actions"
        :key="a.key"
        :disabled="!!runningAction"
        class="flex items-center gap-2.5 rounded border bg-[var(--bg-base)] px-3 py-1.5 text-left"
        @click="run(a)"
      >
        <span class="flex h-8 w-8 items-center justify-center"
          ><RefreshCw
            v-if="runningAction === a.key"
            :size="15"
            class="animate-spin" /><component
            :is="a.icon"
            v-else
            :size="15" /></span
        ><span
          ><b class="block text-xs">{{ a.label }}</b
          ><small>{{ a.detail }}</small></span
        >
      </button>
    </div>
  </div>
  <div
    class="order-6 flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <div class="mb-3 flex justify-between">
      <h3>待处理事项</h3>
      <span>{{ warnings.length ? `${warnings.length} 项` : "状态良好" }}</span>
    </div>
    <div
      v-if="!warnings.length"
      class="bg-[var(--color-success-bg)] p-3 text-center text-xs text-[var(--color-success)]"
    >
      无
    </div>
    <div v-else class="space-y-2">
      <RouterLink
        v-for="w in warnings"
        :key="w[0]"
        :to="w[2]"
        class="flex items-center gap-3 rounded border bg-[var(--bg-base)] p-2"
        ><CircleAlert :size="15" /><span class="flex-1"
          ><b class="block text-xs">{{ w[0] }}</b
          ><small>{{ w[1] }}</small></span
        ><ArrowUpRight :size="13"
      /></RouterLink>
    </div>
  </div>
  <div
    class="order-7 rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)] xl:col-span-4"
  >
    <slot name="installation" />
  </div>
</template>
