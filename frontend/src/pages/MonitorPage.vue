<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import PageHeader from "@/components/layout/PageHeader.vue";
import Toast from "@/components/ui/Toast.vue";
import { getClashClient } from "@/services/clash";
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
const activeTab = ref<MonitorTab>("overview"),
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
  selectedGroup = ref<string | null>(null),
  connections = ref<Connection[]>([]),
  loadingConnections = ref(false),
  connectionSearch = ref(""),
  rules = ref<Rule[]>([]),
  loadingRules = ref(false),
  ruleSearch = ref(""),
  upHistory = ref<number[]>([]),
  downHistory = ref<number[]>([]),
  connectionHistory = ref<number[]>([]),
  memoryHistory = ref<number[]>([]),
  overview = ref<any>(null);
let timer: number | undefined;
const client = getClashClient(),
  show = (s: string, t: "success" | "error" | "info" = "error") => {
    message.value = s;
    messageType.value = t;
    setTimeout(() => (message.value = ""), t === "error" ? 5000 : 3000);
  },
  unavailable = (e: any) => {
    connected.value = false;
    reason.value = e?.message || "Clash API 未连接，请确认核心已启动";
  },
  available = () => {
    connected.value = true;
    reason.value = "";
  },
  ensure = async () => {
    try {
      await client.getConfig();
      available();
      return true;
    } catch (e) {
      unavailable(e);
      return false;
    }
  },
  offline = (e: any) =>
    String(e?.message || e).match(
      /Failed to connect|connectex|connection refused/,
    );
async function loadProxies() {
  if (!connected.value && !(await ensure())) return;
  loadingProxies.value = true;
  try {
    proxies.value = (await client.getProxies()).proxies;
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
  filteredConnections = computed(() =>
    connections.value.filter(
      (c) =>
        !connectionSearch.value ||
        [c.metadata.host, c.metadata.destinationIP, c.chains?.join(",")].some(
          (x) =>
            x?.toLowerCase().includes(connectionSearch.value.toLowerCase()),
        ),
    ),
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
watch(activeTab, async (t) => {
  clearInterval(timer);
  if (t === "proxies") await loadProxies();
  if (t === "rules") await loadRules();
  if (t === "overview" || t === "connections") {
    await loadConnections();
    timer = window.setInterval(
      loadConnections,
      t === "connections" ? 1000 : 3000,
    );
  }
});
onMounted(() => {
  client.connectTraffic(
    (d: TrafficData) => {
      available();
      speedUp.value = d.up;
      speedDown.value = d.down;
      upHistory.value = [...upHistory.value, d.up].slice(-60);
      downHistory.value = [...downHistory.value, d.down].slice(-60);
      overview.value?.$refs?.chart?.addData(d.up, d.down);
    },
    (e) => unavailable(new Error(e)),
  );
  ensure();
  loadProxies();
  loadConnections();
  timer = window.setInterval(loadConnections, 3000);
});
onBeforeUnmount(() => {
  clearInterval(timer);
  client.disconnectTraffic();
});
async function selectProxy(g: string, n: string) {
  try {
    await client.selectProxy(g, n);
    await loadProxies();
  } catch (e: any) {
    show(`切换节点失败: ${e.message}`);
  }
}
async function testDelay(n: string) {
  try {
    await client.delayTest(n);
    await loadProxies();
  } catch (e: any) {
    show(`测速失败: ${e.message}`);
  }
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
</script>
<template>
  <div class="flex h-full flex-col space-y-2">
    <Toast :message="message" :type="messageType" /><PageHeader
      title="仪表盘"
    /><MonitorTabs :active-tab="activeTab" @change="activeTab = $event" />
    <div class="flex-1 overflow-auto">
      <OverviewPanel
        v-if="activeTab === 'overview'"
        ref="overview"
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
        @open-connections="activeTab = 'connections'"
        @open-proxies="activeTab = 'proxies'"
      /><ProxyGroupsPanel
        v-else-if="activeTab === 'proxies'"
        :proxies="proxies"
        :proxy-groups="groups"
        :selected-group="selectedGroup"
        :loading="loadingProxies"
        @refresh="loadProxies"
        @select-group="selectedGroup = $event"
        @select-proxy="selectProxy"
        @test-delay="testDelay"
      /><ConnectionsPanel
        v-else-if="activeTab === 'connections'"
        :connections="filteredConnections"
        :search="connectionSearch"
        :loading="loadingConnections"
        @search-change="connectionSearch = $event"
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
