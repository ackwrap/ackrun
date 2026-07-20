<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { LoaderCircle, RefreshCw } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import { api } from "@/services/api";
import type {
  NodeItem,
  NodeTracerouteAttempt,
  NodeTracerouteEvent,
  NodeTracerouteHop,
  NodeTracerouteResponse,
  WSEvent,
} from "@/services/types";
import { loadGeoProviderOptions, type GeoProviderOption } from "./geoProviders";

type TracePhase = "waiting" | "starting" | "running" | "completed" | "failed";

const props = withDefaults(
  defineProps<{ node: NodeItem | null; flag?: string }>(),
  { flag: "" },
);
const emit = defineEmits<{ close: [] }>();
const result = ref<NodeTracerouteResponse | null>(null);
const error = ref("");
const phase = ref<TracePhase>("waiting");
const traceID = ref("");
const activeUID = ref("");
const geoProvider = ref("disable-geoip");
const geoProviderOptions = ref<GeoProviderOption[]>([
  { value: "disable-geoip", label: "关闭 Geo 查询" },
]);
let cancelRequested = false;

function handleRealtime(event: WSEvent) {
  if (event.type !== "node.traceroute") return;
  const data = event.data as NodeTracerouteEvent;
  if (!traceID.value || data.trace_id !== traceID.value || !result.value)
    return;

  result.value.node_name = data.node_name || result.value.node_name;
  result.value.target = data.target || result.value.target;
  result.value.resolved_ip = data.resolved_ip || result.value.resolved_ip;
  result.value.protocol = data.protocol || result.value.protocol;
  result.value.ip_version = data.ip_version || result.value.ip_version;
  result.value.reached = data.reached;
  result.value.duration_ms = data.duration_ms;
  result.value.geo_provider = data.geo_provider || result.value.geo_provider;
  if (data.hop) {
    const index = result.value.hops.findIndex(
      (hop) => hop.ttl === data.hop!.ttl,
    );
    if (index >= 0) result.value.hops[index] = data.hop;
    else result.value.hops.push(data.hop);
    result.value.hops.sort((left, right) => left.ttl - right.ttl);
  }

  if (data.status === "started" || data.status === "hop") {
    phase.value = "running";
  } else if (data.status === "completed") {
    phase.value = "completed";
  } else if (data.status === "failed") {
    error.value = data.error || "路由追踪失败";
    phase.value = "failed";
  } else if (data.status === "canceled" && props.node) {
    error.value = "路由追踪已取消";
    phase.value = "failed";
  }
}

const { connected } = useRealtimeSocket(handleRealtime);
const active = computed(
  () => phase.value === "starting" || phase.value === "running",
);
const phaseLabel = computed(() => {
  if (phase.value === "waiting")
    return connected.value ? "准备开始追踪" : "正在连接实时通道";
  if (phase.value === "starting") return "正在启动路由追踪";
  if (phase.value === "running")
    return `已收到 ${result.value?.hops.length || 0} 跳，正在继续追踪`;
  if (phase.value === "completed")
    return result.value?.reached
      ? "追踪完成，已到达目标"
      : "追踪完成，未到达目标";
  return "路由追踪失败";
});

function initialResult(node: NodeItem): NodeTracerouteResponse {
  return {
    uid: node.uid,
    node_name: node.name,
    target: node.server,
    resolved_ip: "",
    protocol: "ICMP",
    ip_version: 0,
    reached: false,
    duration_ms: 0,
    geo_provider: geoProvider.value,
    hops: [],
  };
}

function newTraceID() {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

async function start() {
  const node = props.node;
  if (!node || !connected.value || traceID.value) return;
  const id = newTraceID();
  traceID.value = id;
  activeUID.value = node.uid;
  cancelRequested = false;
  error.value = "";
  result.value = initialResult(node);
  phase.value = "starting";
  try {
    await api.startNodeTraceroute(node.uid, id, geoProvider.value);
    if (traceID.value === id && phase.value === "starting")
      phase.value = "running";
  } catch (cause: any) {
    if (traceID.value === id && phase.value === "starting") {
      error.value = cause?.message || "路由追踪启动失败";
      phase.value = "failed";
    }
  }
}

function cancelActive() {
  const uid = activeUID.value;
  const id = traceID.value;
  if (!uid || !id || !active.value || cancelRequested) return;
  cancelRequested = true;
  api.cancelNodeTraceroute(uid, id).catch(() => undefined);
}

function reset(node: NodeItem | null) {
  traceID.value = "";
  activeUID.value = node?.uid || "";
  cancelRequested = false;
  error.value = "";
  phase.value = "waiting";
  result.value = node ? initialResult(node) : null;
}

async function loadGeoProviders() {
  try {
    const loaded = await loadGeoProviderOptions(true);
    geoProviderOptions.value = loaded.options;
    if (!loaded.options.some((option) => option.value === geoProvider.value)) {
      geoProvider.value = "disable-geoip";
    }
  } catch (cause: any) {
    error.value = cause?.message || "GeoIP Provider 加载失败";
  }
}

function restart() {
  cancelActive();
  reset(props.node);
  void start();
}

function close() {
  cancelActive();
  emit("close");
}

const formatRTT = (value?: number) =>
  value === undefined ? "--" : `${value.toFixed(2)} ms`;
const displayAttempt = (hop: NodeTracerouteHop) =>
  hop.attempts.find((attempt) => attempt.success);
const formatASN = (attempt?: NodeTracerouteAttempt) => {
  if (!attempt?.success) return "--";
  const asn = attempt.geo?.asnumber ? `AS${attempt.geo.asnumber}` : "*";
  return attempt.geo?.whois ? `${asn} [${attempt.geo.whois}]` : asn;
};
const formatOwner = (attempt?: NodeTracerouteAttempt) =>
  attempt?.geo?.owner || attempt?.geo?.isp || attempt?.geo?.domain || "--";
const formatLocation = (attempt?: NodeTracerouteAttempt) =>
  [
    attempt?.geo?.country,
    attempt?.geo?.prov,
    attempt?.geo?.city,
    attempt?.geo?.district,
  ]
    .filter(Boolean)
    .join(" ") || "--";

watch(
  () => props.node?.uid,
  () => {
    cancelActive();
    reset(props.node);
    if (props.node) void loadGeoProviders();
  },
  { immediate: true },
);
watch(connected, (value) => {
  if (!value && active.value) {
    cancelActive();
    error.value = "实时连接已断开，本次追踪已取消";
    phase.value = "failed";
  }
});
onBeforeUnmount(cancelActive);
</script>

<template>
  <Modal :open="!!node" title="路由追踪" :width="1540" @close="close">
    <template #title>
      <span>路由追踪</span>
      <template v-if="node">
        · <NodeFlagName :name="node.name" :flag="flag" />
      </template>
    </template>
    <template v-if="result">
      <div class="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div
          v-for="item in [
            [
              '目标',
              result.resolved_ip
                ? `${result.target} (${result.resolved_ip})`
                : result.target,
            ],
            [
              '协议',
              result.ip_version
                ? `${result.protocol} / IPv${result.ip_version}`
                : `${result.protocol} / 解析中`,
            ],
            ['状态', phaseLabel],
            ['耗时', result.duration_ms ? `${result.duration_ms} ms` : '--'],
          ]"
          :key="item[0]"
          class="min-w-0 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
        >
          <div class="text-xs text-[var(--text-tertiary)]">{{ item[0] }}</div>
          <div class="mt-1 truncate font-medium" :title="String(item[1])">
            {{ item[1] }}
          </div>
        </div>
      </div>

      <div
        class="mb-3 flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-sm"
      >
        <div class="flex min-w-0 items-center gap-2">
          <LoaderCircle
            v-if="active"
            :size="16"
            class="shrink-0 animate-spin"
          />
          <span class="truncate">{{ phaseLabel }}</span>
        </div>
        <div class="flex items-center gap-2">
          <label class="flex items-center gap-2 text-[var(--text-secondary)]">
            Geo API
            <select
              v-model="geoProvider"
              class="aw-input h-9 w-56"
              :disabled="active"
            >
              <option
                v-for="provider in geoProviderOptions"
                :key="provider.value"
                :value="provider.value"
              >
                {{ provider.label }}
              </option>
            </select>
          </label>
          <button
            v-if="phase === 'waiting'"
            class="aw-action-button aw-action-neutral shrink-0"
            :disabled="!connected"
            @click="start"
          >
            <RefreshCw :size="13" />开始追踪
          </button>
          <button
            v-else-if="phase === 'completed' || phase === 'failed'"
            class="aw-action-button aw-action-neutral shrink-0"
            :disabled="!connected"
            @click="restart"
          >
            <RefreshCw :size="13" />重新追踪
          </button>
        </div>
      </div>

      <div
        v-if="error"
        class="mb-3 rounded-[var(--radius-lg)] border border-[var(--color-error)]/35 bg-[var(--color-error)]/10 p-3"
      >
        <p class="font-medium text-[var(--color-error)]">{{ error }}</p>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          ICMP 原始套接字在 Windows 上通常需要以管理员权限运行 Ackwrap。
        </p>
      </div>

      <div class="aw-data-table-wrap max-h-[58vh]">
        <table class="aw-data-table min-w-[1370px] table-fixed">
          <colgroup>
            <col class="w-16" />
            <col class="w-60" />
            <col class="w-[360px]" />
            <col class="w-60" />
            <col class="w-[330px]" />
            <col class="w-36" />
          </colgroup>
          <thead>
            <tr>
              <th>跳</th>
              <th>IP</th>
              <th>ASN / 线路</th>
              <th>地区</th>
              <th>反向 DNS</th>
              <th>RTT</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!result.hops.length">
              <td
                colspan="6"
                class="py-12 text-center text-[var(--text-secondary)]"
              >
                {{
                  !connected
                    ? "正在连接 WebSocket..."
                    : phase === "waiting"
                      ? "选择 Geo API 后开始追踪"
                      : "等待第一跳结果..."
                }}
              </td>
            </tr>
            <tr v-for="hop in result.hops" v-else :key="hop.ttl">
              <td class="text-center font-mono font-semibold">{{ hop.ttl }}</td>
              <td class="min-w-0 font-mono">
                <span
                  v-if="displayAttempt(hop)"
                  class="block truncate"
                  :title="displayAttempt(hop)?.ip"
                  >{{ displayAttempt(hop)?.ip }}</span
                >
                <span v-else class="text-[var(--text-tertiary)]">*</span>
              </td>
              <td class="min-w-0">
                <template v-if="displayAttempt(hop)">
                  <div
                    class="truncate font-medium text-[var(--color-success)]"
                    :title="formatASN(displayAttempt(hop))"
                  >
                    {{ formatASN(displayAttempt(hop)) }}
                  </div>
                  <div
                    class="mt-1 truncate text-xs text-[var(--text-tertiary)]"
                    :title="formatOwner(displayAttempt(hop))"
                  >
                    {{ formatOwner(displayAttempt(hop)) }}
                  </div>
                </template>
                <span v-else class="text-[var(--text-tertiary)]">--</span>
              </td>
              <td class="min-w-0">
                <span
                  class="block truncate"
                  :title="formatLocation(displayAttempt(hop))"
                  >{{ formatLocation(displayAttempt(hop)) }}</span
                >
              </td>
              <td class="min-w-0">
                <span
                  v-if="displayAttempt(hop)"
                  class="block truncate text-[var(--text-secondary)]"
                  :title="
                    displayAttempt(hop)?.hostname ||
                    displayAttempt(hop)?.geo_error ||
                    '无反向 DNS'
                  "
                  >{{ displayAttempt(hop)?.hostname || "无反向 DNS" }}</span
                >
                <span v-else class="text-[var(--text-tertiary)]">--</span>
              </td>
              <td class="font-medium">
                <span v-if="displayAttempt(hop)">{{
                  formatRTT(displayAttempt(hop)?.rtt_ms)
                }}</span>
                <span v-else class="font-mono text-[var(--text-tertiary)]"
                  >*</span
                >
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="mt-3 text-xs text-[var(--text-tertiary)]">
        每跳发送 3 次 ICMP 探测，并使用首个成功响应确定该跳；结果表示 Ackwrap
        主机到节点服务器的路径。
      </p>
      <p class="mt-1 text-xs text-[var(--text-tertiary)]">
        启用第三方 Geo API 会把逐跳
        IP（可能包含节点服务器地址）发送给所选服务；关闭 Geo 查询时仍保留
        IP、反向 DNS 和 RTT。
      </p>
    </template>
  </Modal>
</template>
