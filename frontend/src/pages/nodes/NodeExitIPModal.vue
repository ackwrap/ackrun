<script setup lang="ts">
import { ref, watch } from "vue";
import {
  CircleAlert,
  CircleCheck,
  LoaderCircle,
  RefreshCw,
} from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import NodeFlagName from "@/components/NodeFlagName.vue";
import { api } from "@/services/api";
import type { NodeExitIPResponse, NodeItem } from "@/services/types";

const props = withDefaults(
  defineProps<{ node: NodeItem | null; flag?: string }>(),
  { flag: "" },
);
const emit = defineEmits<{ close: [] }>();
const loading = ref(false);
const error = ref("");
const result = ref<NodeExitIPResponse | null>(null);
let requestID = 0;

async function check() {
  const node = props.node;
  if (!node || loading.value) return;
  const currentRequest = ++requestID;
  loading.value = true;
  error.value = "";
  result.value = null;
  try {
    const response = await api.checkNodeExitIP(node.uid);
    if (currentRequest === requestID) result.value = response;
  } catch (cause: any) {
    if (currentRequest === requestID)
      error.value = cause?.message || "出口 IP 检测失败";
  } finally {
    if (currentRequest === requestID) loading.value = false;
  }
}

function close() {
  requestID++;
  emit("close");
}

watch(
  () => props.node?.uid,
  (uid) => {
    requestID++;
    loading.value = false;
    error.value = "";
    result.value = null;
    if (uid) void check();
  },
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
