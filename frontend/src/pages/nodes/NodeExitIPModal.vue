<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  ChevronDown,
  CircleAlert,
  CircleCheck,
  LoaderCircle,
  MapPin,
  RefreshCw,
} from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { api } from "@/services/api";
import type { NodeExitIPResponse, NodeItem } from "@/services/types";
import { loadGeoProviderOptions, type GeoProviderOption } from "./geoProviders";

const props = withDefaults(
  defineProps<{ node: NodeItem | null; flag?: string }>(),
  { flag: "" },
);
const emit = defineEmits<{ close: [] }>();
const loading = ref(false);
const error = ref("");
const result = ref<NodeExitIPResponse | null>(null);
const geoExpanded = ref(false);
const geoLoading = ref(false);
const geoError = ref("");
const geoProviderStorageKey = "ackwrap.node.exitIPGeoProvider";
const selectableGeoProviderOptions = ref<GeoProviderOption[]>([]);
const geoProvider = ref("");
let requestID = 0;
let geoRequestID = 0;

async function loadGeoProviders() {
  try {
    const loaded = await loadGeoProviderOptions(false);
    selectableGeoProviderOptions.value = loaded.options;
    let saved = "";
    try {
      saved = localStorage.getItem(geoProviderStorageKey) || "";
    } catch {
      // Storage can be unavailable in restricted browser contexts.
    }
    geoProvider.value = loaded.options.some((option) => option.value === saved)
      ? saved
      : loaded.defaultProvider;
  } catch (cause: any) {
    selectableGeoProviderOptions.value = [];
    geoProvider.value = "";
    geoError.value = cause?.message || "GeoIP Provider 加载失败";
  }
}

function persistGeoProvider(value: string) {
  if (!value) return;
  try {
    localStorage.setItem(geoProviderStorageKey, value);
  } catch {
    // Keep the current in-memory selection when storage is unavailable.
  }
}

const geoDetailItems = computed(() => {
  const geo = result.value?.geo;
  if (!geo) return [];
  const location = [geo.country, geo.prov, geo.city, geo.district]
    .filter(Boolean)
    .join(" · ");
  const coordinates =
    geo.lat !== undefined && geo.lng !== undefined
      ? `${geo.lat.toFixed(4)}, ${geo.lng.toFixed(4)}`
      : "--";
  return [
    ["国家 / 地区", location || "--"],
    ["ASN", geo.asnumber ? `AS${geo.asnumber}` : "--"],
    ["运营商 / 组织", geo.owner || geo.isp || geo.domain || "--"],
    ["网段 / Whois", geo.prefix || geo.whois || "--"],
    ["坐标", coordinates],
    ["数据来源", geo.source || result.value?.geo_provider || "--"],
  ];
});

watch(geoProvider, (value) => {
  persistGeoProvider(value);
  if (value && geoExpanded.value && result.value) void checkGeo();
});

async function check() {
  const node = props.node;
  if (!node || loading.value) return;
  const currentRequest = ++requestID;
  geoRequestID++;
  loading.value = true;
  geoLoading.value = false;
  error.value = "";
  geoError.value = "";
  result.value = null;
  try {
    const response = await api.checkNodeExitIP(node.uid, "disable-geoip");
    if (currentRequest === requestID) {
      result.value = response;
      if (geoExpanded.value) void checkGeo();
    }
  } catch (cause: any) {
    if (currentRequest === requestID)
      error.value = cause?.message || "出口 IP 检测失败";
  } finally {
    if (currentRequest === requestID) loading.value = false;
  }
}

async function checkGeo() {
  const node = props.node;
  if (!node || !result.value || geoLoading.value) return;
  if (!geoProvider.value) {
    geoError.value = "没有可用的 GeoIP Provider，请先在设置中添加或启用";
    return;
  }
  const currentRequest = ++geoRequestID;
  geoLoading.value = true;
  geoError.value = "";
  try {
    const response = await api.checkNodeExitIP(node.uid, geoProvider.value);
    if (currentRequest === geoRequestID) result.value = response;
  } catch (cause: any) {
    if (currentRequest === geoRequestID)
      geoError.value = cause?.message || "GeoIP 查询失败";
  } finally {
    if (currentRequest === geoRequestID) geoLoading.value = false;
  }
}

function toggleGeoDetails() {
  geoExpanded.value = !geoExpanded.value;
  if (
    geoExpanded.value &&
    result.value &&
    (result.value.geo_provider !== geoProvider.value ||
      (!result.value.geo && !result.value.geo_error))
  ) {
    void checkGeo();
  }
}

function close() {
  requestID++;
  geoRequestID++;
  emit("close");
}

watch(
  () => props.node?.uid,
  (uid) => {
    requestID++;
    geoRequestID++;
    loading.value = false;
    geoLoading.value = false;
    error.value = "";
    geoError.value = "";
    result.value = null;
    geoExpanded.value = false;
    if (uid) {
      void loadGeoProviders();
      void check();
    }
  },
  { immediate: true },
);
</script>

<template>
  <Modal :open="!!node" title="出口 IP" size="lg" @close="close">
    <template #title>
      <span>出口 IP</span>
      <template v-if="node">
        · <NodeFlagName :name="node.name" :flag="flag" />
      </template>
    </template>
    <div
      v-if="loading"
      class="flex min-h-48 flex-col items-center justify-center gap-3 text-[var(--text-secondary)]"
    >
      <LoaderCircle :size="24" class="animate-spin" />
      <span>正在通过该节点访问出口 IP 服务...</span>
    </div>

    <div
      v-else-if="error"
      class="rounded-[var(--radius-lg)] border border-[var(--color-error)]/35 bg-[var(--color-error)]/10 p-4"
    >
      <div class="flex items-start gap-3">
        <CircleAlert
          :size="20"
          class="mt-0.5 shrink-0 text-[var(--color-error)]"
        />
        <div class="min-w-0">
          <p class="font-medium text-[var(--color-error)]">检测失败</p>
          <p class="mt-1 break-words text-sm text-[var(--text-secondary)]">
            {{ error }}
          </p>
        </div>
      </div>
      <button class="aw-action-button aw-action-neutral mt-4" @click="check">
        <RefreshCw :size="13" />重新检测
      </button>
    </div>

    <template v-else-if="result">
      <div
        class="flex items-start gap-3 rounded-[var(--radius-lg)] border p-4"
        :class="
          result.matched
            ? 'border-[var(--color-success)]/35 bg-[var(--color-success)]/10'
            : 'border-[var(--color-warning)]/35 bg-[var(--color-warning)]/10'
        "
      >
        <CircleCheck
          v-if="result.matched"
          :size="22"
          class="mt-0.5 shrink-0 text-[var(--color-success)]"
        />
        <CircleAlert
          v-else
          :size="22"
          class="mt-0.5 shrink-0 text-[var(--color-warning)]"
        />
        <div>
          <p class="font-semibold">
            {{
              result.matched
                ? "入口 IP 与出口 IP 一致"
                : "入口 IP 与出口 IP 不一致"
            }}
          </p>
          <p class="mt-1 text-sm text-[var(--text-secondary)]">
            {{
              result.matched
                ? "该节点对外访问使用节点服务器地址。"
                : "该节点的入口和对外出口不同，可能存在中转、NAT 或独立出口。"
            }}
          </p>
        </div>
      </div>

      <div class="mt-4 grid gap-3 sm:grid-cols-2">
        <div
          v-for="item in [
            ['节点 IP', result.node_ip],
            ['实际出口 IP', result.exit_ip],
            [
              '节点地址解析',
              result.resolution === 'alidns_doh' ? 'AliDNS DoH' : 'IP 字面量',
            ],
            ['检测路径', '核心直接通过当前节点 → Cloudflare Trace'],
          ]"
          :key="item[0]"
          class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
        >
          <div class="text-xs text-[var(--text-tertiary)]">{{ item[0] }}</div>
          <div class="mt-1 break-all font-mono font-medium">{{ item[1] }}</div>
        </div>
      </div>

      <section
        class="mt-4 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)]"
      >
        <button
          type="button"
          class="flex w-full items-center justify-between gap-3 p-4 text-left"
          :aria-expanded="geoExpanded"
          @click="toggleGeoDetails"
        >
          <span class="flex min-w-0 items-center gap-2 font-semibold">
            <MapPin :size="17" class="text-[var(--color-primary)]" />出口 IP
            归属详情
            <span
              class="rounded-full bg-[var(--bg-surface)] px-2 py-0.5 text-[10px] font-normal text-[var(--text-tertiary)]"
              >可选</span
            >
          </span>
          <span
            class="flex shrink-0 items-center gap-2 text-xs font-normal text-[var(--text-tertiary)]"
          >
            <span v-if="!geoExpanded">展开后查询</span>
            <ChevronDown
              :size="16"
              class="transition-transform"
              :class="geoExpanded ? 'rotate-180' : ''"
            />
          </span>
        </button>

        <div
          v-if="geoExpanded"
          class="border-t border-[var(--border-light)] p-4"
        >
          <label
            class="grid gap-2 sm:grid-cols-[120px_minmax(0,1fr)] sm:items-center"
          >
            <span class="text-sm text-[var(--text-secondary)]">Geo API</span>
            <select
              v-model="geoProvider"
              class="aw-input"
              :disabled="geoLoading"
            >
              <option
                v-for="provider in selectableGeoProviderOptions"
                :key="provider.value"
                :value="provider.value"
              >
                {{ provider.label }}
              </option>
            </select>
          </label>
          <p class="mt-2 text-xs text-[var(--text-tertiary)]">
            仅展开此区域后查询；检测到的出口 IP
            会发送给所选服务。在线查询失败时自动回退已同步的本地
            geoip.db，Provider 选择保存在当前浏览器。
          </p>

          <div
            v-if="geoLoading"
            class="mt-3 flex items-center gap-2 rounded-[var(--radius-md)] bg-[var(--bg-surface)] p-3 text-sm text-[var(--text-secondary)]"
          >
            <LoaderCircle :size="16" class="animate-spin" />正在查询 GeoIP
            归属...
          </div>
          <div
            v-else-if="geoError || result.geo_error"
            class="mt-3 rounded-[var(--radius-md)] bg-[var(--color-warning)]/10 p-3 text-sm text-[var(--color-warning)]"
          >
            <p>Geo API 查询失败：{{ geoError || result.geo_error }}</p>
            <button
              class="aw-action-button aw-action-neutral mt-3"
              @click="checkGeo"
            >
              <RefreshCw :size="13" />重新查询
            </button>
          </div>
          <div v-else-if="result.geo" class="mt-3 grid gap-3 sm:grid-cols-2">
            <div
              v-for="item in geoDetailItems"
              :key="item[0]"
              class="rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-surface)] p-3"
            >
              <div class="text-xs text-[var(--text-tertiary)]">
                {{ item[0] }}
              </div>
              <div class="mt-1 break-all font-medium">{{ item[1] }}</div>
            </div>
          </div>
        </div>
      </section>

      <p class="mt-4 text-xs text-[var(--text-tertiary)]">
        核心直接使用该节点发起独立请求，不切换任何 selector
        或当前策略组；Cloudflare 会看到本次请求的出口
        IP。结果不一致不一定表示异常，也可能由中转、NAT、Anycast
        或独立落地出口造成。
      </p>
    </template>

    <template #footer>
      <button
        v-if="result"
        class="aw-action-button aw-action-neutral"
        :disabled="loading"
        @click="check"
      >
        <RefreshCw :size="13" />重新检测
      </button>
      <button class="aw-action-button aw-action-neutral" @click="close">
        关闭
      </button>
    </template>
  </Modal>
</template>
