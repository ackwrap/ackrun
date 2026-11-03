<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import Modal from "@/components/ui/Modal.vue";
import { api, request } from "@/services/api";
import type {
  RouteRule,
  RouteRuleRequest,
  StrategyItem,
} from "@/services/types";
import StrategyFormModal from "../collections/StrategyFormModal.vue";
import type {
  NodeGroup,
  StrategyCollectionRequest,
} from "../collections/collectionTypes";

const props = defineProps<{ rule: RouteRuleRequest }>();
const emit = defineEmits<{ close: []; saved: [RouteRule] }>();
const groups = ref<NodeGroup[]>([]);
const connectivity = ref({ test_url: "", interval_seconds: 300 });
const loading = ref(true);
const ready = ref(false);
const saving = ref(false);
const error = ref("");
const strategy = computed<StrategyItem>(() => ({
  rule_id: 0,
  name: props.rule.name,
  priority: props.rule.priority,
  kind: "proxy",
  enabled: props.rule.enabled,
  read_only: false,
  outbound_tag: "proxy",
  device_policy: props.rule.rule_type === "source_ip_cidr",
}));

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [items, settings] = await Promise.all([
      request<NodeGroup[]>("/node-groups"),
      api.getConnectivitySettings(),
    ]);
    groups.value = items ?? [];
    connectivity.value = settings;
    ready.value = true;
  } catch (e) {
    error.value = `策略配置加载失败：${e instanceof Error ? e.message : "请求失败"}`;
  } finally {
    loading.value = false;
  }
}

async function save(payload: StrategyCollectionRequest) {
  if (saving.value) return;
  saving.value = true;
  error.value = "";
  try {
    const saved = await api.createRouteRule({
      ...props.rule,
      strategy: payload,
    });
    emit("saved", saved);
  } catch (e) {
    error.value = `规则与策略保存失败：${e instanceof Error ? e.message : "请求失败"}`;
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <StrategyFormModal
    v-if="ready"
    :strategy="strategy"
    :groups="groups"
    :connectivity="connectivity"
    :saving="saving"
    :save-error="error"
    @close="emit('close')"
    @save="save"
    @error="error = $event"
  />
  <Modal
    v-else
    :open="true"
    title="配置代理策略"
    size="lg"
    @close="emit('close')"
  >
    <p
      v-if="loading"
      role="status"
      class="py-8 text-center text-sm text-[var(--text-secondary)]"
    >
      正在加载…
    </p>
    <p v-else role="alert" class="text-sm text-[var(--color-error)]">
      {{ error }}
    </p>
    <template #footer>
      <button class="aw-action-button" @click="emit('close')">取消</button>
      <button v-if="!loading" class="aw-action-button" @click="load">
        重试
      </button>
    </template>
  </Modal>
</template>
