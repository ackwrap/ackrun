<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import PageHeader from "@/components/layout/PageHeader.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Toast from "@/components/ui/Toast.vue";
import { useHashTab } from "@/composables/useHashTab";
import { api } from "@/services/api";
import { getClashClient } from "@/services/clash";
import type { ConnectivityTarget } from "@/services/types";
import type {
  Connection,
  ProxyGroup,
  ProxyNode,
  Rule,
  TrafficData,
} from "@/services/clash";
import MonitorTabs from "./monitor/MonitorTabs.vue";
import OverviewPanel from "./monitor/OverviewPanel.vue";
import ProxyGroupsPanel from "./monitor/ProxyGroupsPanel.vue";
import ConnectionsPanel from "./monitor/ConnectionsPanel.vue";
import RulesPanel from "./monitor/RulesPanel.vue";
import type { MonitorTab } from "./monitor/monitorUtils";
const router = useRouter();
const { activeTab, selectTab } = useHashTab<MonitorTab>(
  ["overview", "proxies", "connections", "rules"],
  "overview",
);
const runtimeChecking = ref(true),
  runtimeBlocked = ref(false),
  runtimeBlockedMessage = ref("sing-box 核心未运行，请先在控制台启动核心。"),
  totalUp = ref(0),
  totalDown = ref(0),
  speedUp = ref(0),
  speedDown = ref(0),
  memory = ref(0),
  connected = ref(false),
  reason = ref(""),
  message = ref(""),
  messageType = ref<"success" | "error" | "info">("info"),
  proxies = ref<Record<string, ProxyGroup | ProxyNode>>({}),
  loadingProxies = ref(false),
  testingNodes = ref(new Set<string>()),
  testingGroups = ref(new Set<string>()),
  proxyFlags = ref<Record<string, string>>({}),
  delayTestURL = ref(""),
  connectivityTargets = ref<ConnectivityTarget[]>([]),
  connections = ref<Connection[]>([]),
  loadingConnections = ref(false),
  rules = ref<Rule[]>([]),
  loadingRules = ref(false),
  ruleSearch = ref(""),
  upHistory = ref<number[]>([]),
  downHistory = ref<number[]>([]),
  connectionHistory = ref<number[]>([]),
  memoryHistory = ref<number[]>([]);
let timer: number | undefined;
let ensurePromise: Promise<boolean> | null = null;
let runtimeCheckPromise: Promise<boolean> | null = null;
let monitorSuspended = false;
let selectionSequence = 0;
const pendingSelections = new Map<string, number>();
const client = getClashClient(),
  show = (s: string, t: "success" | "error" | "info" = "error") => {
    message.value = s;
    messageType.value = t;
  },
  unavailable = (e: any) => {
    connected.value = false;
    reason.value = e?.message || "Clash API 未连接，请确认核心已启动";
  },
  available = () => {
    connected.value = true;
    reason.value = "";
  },
  ensure = () => {
    if (monitorSuspended) return Promise.resolve(false);
    if (ensurePromise) return ensurePromise;
    ensurePromise = (async () => {
      try {
        await client.getConfig();
        available();
        return true;
      } catch (e) {
        unavailable(e);
        await suspendWhenCoreStops();
        return false;
      } finally {
        ensurePromise = null;
      }
    })();
    return ensurePromise;
  },
  offline = (e: any) =>
    String(e?.message || e).match(
      /Failed to connect|connectex|connection refused/,
    );
function stopMonitor(message?: string) {
  monitorSuspended = true;
  clearInterval(timer);
  timer = undefined;
  client.disconnectTraffic();
  connected.value = false;
  speedUp.value = 0;
  speedDown.value = 0;
  if (message) reason.value = message;
}
async function suspendWhenCoreStops() {
  if (monitorSuspended) return true;
  if (runtimeCheckPromise) return runtimeCheckPromise;
  runtimeCheckPromise = (async () => {
    try {
      const runtime = await api.getRuntime();
      if (runtime.status === "running") return false;
      const message =
        runtime.status === "not_installed"
          ? "sing-box 核心未安装，仪表盘实时请求已停止。"
          : runtime.status === "no_config"
            ? "sing-box 没有可用配置，仪表盘实时请求已停止。"
            : "sing-box 核心已停止，仪表盘实时请求已停止。";
      stopMonitor(message);
      return true;
    } catch {
      return false;
    } finally {
      runtimeCheckPromise = null;
    }
  })();
  return runtimeCheckPromise;
}
async function loadProxies() {
  if (!connected.value && !(await ensure())) return;
  loadingProxies.value = true;
  try {
    proxies.value = (await client.getProxies()).proxies;
    const inferred = await api.inferNodeFlags(
      Object.keys(proxies.value).map((name) => ({ key: name, name })),
    );
    proxyFlags.value = Object.fromEntries(
      inferred.items.map((item) => [item.key, item.flag]),
    );
  } catch (e: any) {
    offline(e) ? unavailable(e) : show(`加载策略组失败: ${e.message}`);
  } finally {
    loadingProxies.value = false;
  }
}
async function loadConnections() {
  if (!connected.value && !(await ensure())) return;
  loadingConnections.value = true;
  try {
    const r = await client.getConnections();
    connections.value = r.connections;
    totalUp.value = r.uploadTotal;
    totalDown.value = r.downloadTotal;
    memory.value = r.memory || 0;
    connectionHistory.value = [
      ...connectionHistory.value,
      r.connections.length,
    ].slice(-60);
    memoryHistory.value = [...memoryHistory.value, r.memory || 0].slice(-60);
  } catch (e: any) {
    offline(e) ? unavailable(e) : show(`加载连接失败: ${e.message}`);
  } finally {
    loadingConnections.value = false;
  }
}
async function loadRules() {
  if (!connected.value && !(await ensure())) return;
  loadingRules.value = true;
  try {
    rules.value = (await client.getRules()).rules;
  } catch (e: any) {
    offline(e) ? unavailable(e) : show(`加载规则失败: ${e.message}`);
  } finally {
    loadingRules.value = false;
  }
}
const groups = computed(() =>
    Object.entries(proxies.value)
      .filter(([, x]) => x.type === "Selector" || x.type === "URLTest")
      .map(([name, x]) => ({ ...x, name }) as ProxyGroup),
  ),
  filteredRules = computed(() =>
    rules.value.filter(
      (r) =>
        !ruleSearch.value ||
        [r.type, r.payload, r.proxy].some((x) =>
          x?.toLowerCase().includes(ruleSearch.value.toLowerCase()),
        ),
    ),
  );
async function activateTab(t: MonitorTab) {
  clearInterval(timer);
  if (monitorSuspended) return;
  if (t === "proxies") await loadProxies();
  if (t === "rules") await loadRules();
  if (t === "overview" || t === "connections") {
    await loadConnections();
    timer = window.setInterval(
      loadConnections,
      t === "connections" ? 1000 : 3000,
    );
  }
}
watch(activeTab, activateTab);
async function waitForClashAPI() {
  for (let attempt = 0; attempt < 10; attempt += 1) {
    if (await ensure()) return true;
    if (monitorSuspended) return false;
    await new Promise((resolve) => window.setTimeout(resolve, 500));
  }
  return false;
}
async function startMonitor() {
  monitorSuspended = false;
  if (!(await waitForClashAPI())) {
    runtimeBlockedMessage.value = monitorSuspended
      ? reason.value
      : "sing-box 进程已启动，但 Clash API 尚未就绪，请返回控制台检查核心日志。";
    runtimeBlocked.value = true;
    return;
  }
  client.connectTraffic(
    (d: TrafficData) => {
      available();
      speedUp.value = d.up;
      speedDown.value = d.down;
      upHistory.value = [...upHistory.value, d.up].slice(-60);
      downHistory.value = [...downHistory.value, d.down].slice(-60);
    },
    (e) => {
      unavailable(new Error(e));
      void suspendWhenCoreStops();
    },
  );
  const initialLoads: Promise<void>[] = [activateTab(activeTab.value)];
  if (activeTab.value !== "proxies") initialLoads.push(loadProxies());
  await Promise.all(initialLoads);
}
onMounted(async () => {
  try {
    const [settings, targets] = await Promise.all([
      api.getConnectivitySettings(),
      api.getConnectivityTargets(),
    ]);
    connectivityTargets.value = targets.filter((target) => target.enabled);
    const saved = localStorage.getItem("ackwrap.monitor.delayTestURL") || "";
    delayTestURL.value = connectivityTargets.value.some(
      (target) => target.url === saved,
    )
      ? saved
      : settings.test_url;
  } catch (error: any) {
    show(`加载测速地址失败: ${error?.message || "请求失败"}`, "error");
  }
  try {
    const runtime = await api.getRuntime();
    if (runtime.status !== "running") {
      runtimeBlockedMessage.value =
        runtime.status === "not_installed"
          ? "尚未安装 sing-box 核心，请先在控制台完成安装。"
          : runtime.status === "no_config"
            ? "sing-box 尚无可用配置，请先在控制台生成配置。"
            : "sing-box 核心未运行，请先在控制台启动核心。";
      runtimeBlocked.value = true;
      return;
    }
    await startMonitor();
  } catch (error: any) {
    runtimeBlockedMessage.value = `无法确认 sing-box 运行状态：${error?.message || "请求失败"}`;
    runtimeBlocked.value = true;
  } finally {
    runtimeChecking.value = false;
  }
});
onBeforeUnmount(() => {
  stopMonitor();
});
async function selectProxy(g: string, n: string) {
  if (pendingSelections.has(g)) return;
  const current = proxies.value[g] as ProxyGroup | undefined;
  if (!current || current.type !== "Selector" || current.now === n) return;
  const previous = current.now;
  const requestID = ++selectionSequence;
  pendingSelections.set(g, requestID);
  proxies.value = {
    ...proxies.value,
    [g]: { ...current, now: n },
  };
  try {
    await client.selectProxy(g, n);
  } catch (e: any) {
    if (pendingSelections.get(g) === requestID) {
      proxies.value = {
        ...proxies.value,
        [g]: { ...current, now: previous },
      };
    }
    show(`切换节点失败: ${e.message}`);
  } finally {
    if (pendingSelections.get(g) === requestID) pendingSelections.delete(g);
  }
}
async function testDelay(n: string) {
  if (!n || testingNodes.value.has(n)) return;
  testingNodes.value = new Set(testingNodes.value).add(n);
  try {
    await measureDelay(n, delayTestURL.value);
  } catch (e: any) {
    show(`测速失败: ${e.message}`);
  } finally {
    const next = new Set(testingNodes.value);
    next.delete(n);
    testingNodes.value = next;
  }
}

function appendDelayHistory(name: string, delay: number) {
  const current = proxies.value[name];
  if (!current) return;
  proxies.value = {
    ...proxies.value,
    [name]: {
      ...current,
      history: [
        ...(current.history || []),
        { time: new Date().toISOString(), delay },
      ],
    },
  };
}

async function measureDelay(name: string, testURL: string) {
  const result = await client.delayTest(name, testURL);
  appendDelayHistory(name, result.delay);
}

async function testGroup(groupName: string, nodes: string[]) {
  if (!groupName || testingGroups.value.has(groupName)) return;
  const candidates = [...new Set(nodes.filter(Boolean))];
  const testNodes = candidates.filter((name) => !testingNodes.value.has(name));
  if (!testNodes.length) return;

  testingGroups.value = new Set(testingGroups.value).add(groupName);
  testingNodes.value = new Set([...testingNodes.value, ...testNodes]);

  let nextIndex = 0;
  let succeeded = 0;
  let failed = 0;
  let firstError = "";
  const testURL = delayTestURL.value;
  const worker = async () => {
    while (nextIndex < testNodes.length) {
      const name = testNodes[nextIndex++];
      if (!name) continue;
      try {
        await measureDelay(name, testURL);
        succeeded += 1;
      } catch (e: any) {
        failed += 1;
        firstError ||= e?.message || String(e);
        appendDelayHistory(name, 0);
      } finally {
        const nextNodes = new Set(testingNodes.value);
        nextNodes.delete(name);
        testingNodes.value = nextNodes;
      }
    }
  };

  try {
    await Promise.allSettled(
      Array.from({ length: Math.min(5, testNodes.length) }, worker),
    );
  } finally {
    const nextGroups = new Set(testingGroups.value);
    nextGroups.delete(groupName);
    testingGroups.value = nextGroups;
    // Also clean up nodes left behind if a worker exits unexpectedly.
    const nextNodes = new Set(testingNodes.value);
    testNodes.forEach((name) => nextNodes.delete(name));
    testingNodes.value = nextNodes;
  }

  const skipped = candidates.length - testNodes.length;
  const summary = `分组测速完成：成功 ${succeeded}，失败 ${failed}${skipped ? `，跳过 ${skipped}` : ""}`;
  if (!failed) {
    show(summary, "success");
  } else {
    show(
      `${summary}；首个失败原因：${firstError}`,
      succeeded ? "info" : "error",
    );
  }
}
function updateDelayTestURL(value: string) {
  const next = value.trim();
  if (!connectivityTargets.value.some((target) => target.url === next)) {
    show("测速地址未启用，请先在设置中添加或启用", "error");
    return;
  }
  delayTestURL.value = next;
  localStorage.setItem("ackwrap.monitor.delayTestURL", next);
  show("手动测速地址已更新", "success");
}
async function closeConnection(id: string) {
  try {
    await client.closeConnection(id);
    await loadConnections();
  } catch (e: any) {
    show(`关闭连接失败: ${e.message}`);
  }
}
async function closeAll() {
  if (confirm("确定关闭所有连接？")) {
    try {
      await client.closeAllConnections();
      await loadConnections();
    } catch (e: any) {
      show(`关闭所有连接失败: ${e.message}`);
    }
  }
}
function returnToControl() {
  runtimeBlocked.value = false;
  void router.replace("/");
}
</script>
<template>
  <ConfirmDialog
    :open="runtimeBlocked"
    title="仪表盘暂不可用"
    :message="runtimeBlockedMessage"
    confirm-text="返回控制台"
    :show-cancel="false"
    @confirm="returnToControl"
    @cancel="returnToControl"
  />
  <div
    v-if="runtimeChecking"
    class="grid h-full place-items-center text-sm text-[var(--text-secondary)]"
  >
    正在检查 sing-box 运行状态...
  </div>
  <div v-else-if="!runtimeBlocked" class="flex h-full flex-col space-y-2">
    <Toast
      :message="message"
      :type="messageType"
      @dismiss="message = ''"
    /><PageHeader title="仪表盘" /><MonitorTabs
      :active-tab="activeTab"
      @change="selectTab"
    />
    <div class="flex-1 overflow-auto">
      <OverviewPanel
        v-if="activeTab === 'overview'"
        v-bind="{
          connected,
          unavailableReason: reason,
          totalUp,
          totalDown,
          speedUp,
          speedDown,
          memory,
          connectionCount: connections.length,
          connections,
          proxyGroups: groups,
          uploadSpeedHistory: upHistory,
          downloadSpeedHistory: downHistory,
          connectionCountHistory: connectionHistory,
          memoryHistory,
        }"
        @open-connections="selectTab('connections')"
        @open-proxies="selectTab('proxies')"
      /><ProxyGroupsPanel
        v-else-if="activeTab === 'proxies'"
        :proxies="proxies"
        :proxy-groups="groups"
        :loading="loadingProxies"
        :testing-nodes="testingNodes"
        :testing-groups="testingGroups"
        :node-flags="proxyFlags"
        :delay-test-url="delayTestURL"
        :connectivity-targets="connectivityTargets"
        @refresh="loadProxies"
        @update:delay-test-url="updateDelayTestURL"
        @select-proxy="selectProxy"
        @test-delay="testDelay"
        @test-group="testGroup"
      /><ConnectionsPanel
        v-else-if="activeTab === 'connections'"
        :connections="connections"
        :loading="loadingConnections"
        :node-flags="proxyFlags"
        @refresh="loadConnections"
        @close-connection="closeConnection"
        @close-all="closeAll"
      /><RulesPanel
        v-else
        :rules="filteredRules"
        :search="ruleSearch"
        :loading="loadingRules"
        :unavailable-reason="connected ? '' : reason"
        @search-change="ruleSearch = $event"
        @refresh="loadRules"
      />
    </div>
  </div>
</template>
