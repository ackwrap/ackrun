<script setup lang="ts">
import { onMounted, ref } from "vue";
import { Plus, ShieldOff, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type {
  TrafficBypassRule,
  TrafficBypassRuleType,
} from "@/services/types";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const rules = ref<TrafficBypassRule[]>([]);
const loading = ref(true);
const saving = ref(false);
const addOpen = ref(false);
const draft = ref<TrafficBypassRule>({
  type: "process_name",
  value: "",
});
const types: Array<{
  value: TrafficBypassRuleType;
  label: string;
  placeholder: string;
}> = [
  { value: "process_name", label: "进程名称", placeholder: "easytier-core" },
  { value: "interface", label: "网络接口", placeholder: "easytier-tun" },
  { value: "ip_cidr", label: "目标 IP/CIDR", placeholder: "10.0.0.0/8" },
  {
    value: "source_ip_cidr",
    label: "来源 IP/CIDR",
    placeholder: "192.168.1.0/24",
  },
  {
    value: "domain_suffix",
    label: "域名后缀",
    placeholder: "example.com",
  },
];

function placeholder(type: TrafficBypassRuleType) {
  return types.find((item) => item.value === type)?.placeholder || "";
}

function typeLabel(type: TrafficBypassRuleType) {
  return types.find((item) => item.value === type)?.label || type;
}

async function load() {
  loading.value = true;
  try {
    const result = await api.getTrafficBypassSettings();
    rules.value = result.rules.map((rule) => ({ ...rule }));
  } catch (error: any) {
    emit("notify", `加载流量排除设置失败: ${error.message}`, "error");
  } finally {
    loading.value = false;
  }
}

function openAdd() {
  draft.value = { type: "process_name", value: "" };
  addOpen.value = true;
}

function addRule() {
  const rule = {
    type: draft.value.type,
    value: draft.value.value.trim(),
  };
  if (!rule.value) {
    emit("notify", "请输入排除项的匹配值", "error");
    return;
  }
  if (
    rules.value.some(
      (item) => item.type === rule.type && item.value === rule.value,
    )
  ) {
    emit("notify", "该排除项已存在", "info");
    return;
  }
  rules.value.push(rule);
  addOpen.value = false;
}

function removeRule(index: number) {
  rules.value.splice(index, 1);
}

async function save() {
  saving.value = true;
  try {
    await api.setTrafficBypassSettings({ rules: rules.value });
    await load();
    emit("notify", "流量排除设置已保存，配置将自动更新");
  } catch (error: any) {
    emit("notify", `保存流量排除设置失败: ${error.message}`, "error");
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
    role="tabpanel"
  >
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex items-center gap-2">
          <ShieldOff :size="18" class="text-[var(--color-primary)]" />
          <h2 class="font-semibold">流量排除</h2>
        </div>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          匹配项优先直连并绕过透明代理；接口和目标 CIDR 同时从 TUN 自动路由中排除。
        </p>
      </div>
      <Button size="sm" variant="secondary" @click="openAdd">
        <Plus :size="14" />添加排除项
      </Button>
    </div>

    <div class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)]">
      <div
        class="hidden grid-cols-[180px_minmax(0,1fr)_44px] gap-3 border-b border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2 text-xs text-[var(--text-secondary)] sm:grid"
      >
        <span>排除类型</span><span>匹配值</span><span></span>
      </div>
      <div v-if="loading" class="py-10 text-center text-sm text-[var(--text-secondary)]">
        加载中...
      </div>
      <div
        v-else-if="!rules.length"
        class="py-10 text-center text-sm text-[var(--text-secondary)]"
      >
        暂无排除项，所有透明代理流量按现有规则处理。
      </div>
      <div
        v-for="(rule, index) in rules"
        v-else
        :key="index"
        class="grid gap-2 border-b border-[var(--border-default)] p-3 last:border-b-0 sm:grid-cols-[180px_minmax(0,1fr)_44px] sm:gap-3"
      >
        <span class="self-center text-xs text-[var(--text-secondary)]">
          {{ typeLabel(rule.type) }}
        </span>
        <span class="min-w-0 self-center truncate font-mono text-xs" :title="rule.value">
          {{ rule.value }}
        </span>
        <button
          type="button"
          class="aw-action-button aw-action-danger justify-self-end"
          title="删除排除项"
          @click="removeRule(index)"
        >
          <Trash2 :size="14" />
        </button>
      </div>
    </div>

    <div class="mt-4 flex justify-end">
      <Button size="sm" :loading="saving" @click="save">保存流量排除设置</Button>
    </div>

    <Modal :open="addOpen" title="添加排除项" size="sm" @close="addOpen = false">
      <form class="space-y-4" @submit.prevent="addRule">
        <label class="block space-y-1.5">
          <span class="text-xs text-[var(--text-secondary)]">排除类型</span>
          <select v-model="draft.type" class="aw-input w-full">
            <option v-for="item in types" :key="item.value" :value="item.value">
              {{ item.label }}
            </option>
          </select>
        </label>
        <label class="block space-y-1.5">
          <span class="text-xs text-[var(--text-secondary)]">匹配值</span>
          <input
            v-model="draft.value"
            class="aw-input w-full"
            :placeholder="placeholder(draft.type)"
            autofocus
          />
        </label>
      </form>
      <template #footer>
        <Button @click="addOpen = false">取消</Button>
        <Button variant="primary" @click="addRule">添加</Button>
      </template>
    </Modal>
  </section>
</template>
