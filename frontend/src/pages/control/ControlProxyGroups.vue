<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Check, ChevronRight, Layers } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import { getClashClient, type ProxyMap } from "@/services/clash";
import { isProxyGroup, resolveProxyName } from "@/utils/controlActivity";

const props = defineProps<{
  isRunning: boolean;
  proxies: ProxyMap | null;
  error: string;
}>();
const emit = defineEmits<{
  refresh: [];
  notify: [message: string, type: "success" | "error" | "info"];
}>();
const groups = computed(() =>
  Object.values(props.proxies || {}).filter(isProxyGroup),
);
const selectedName = ref("");
const search = ref("");
const saving = ref("");
const saveError = ref("");
const selectedGroup = computed(() =>
  groups.value.find((group) => group.name === selectedName.value),
);
const manual = computed(
  () => selectedGroup.value?.type.toLowerCase() === "selector",
);
const options = computed(() =>
  (selectedGroup.value?.all || []).filter((name) =>
    `${name} ${resolveProxyName(name, props.proxies || {})}`
      .toLowerCase()
      .includes(search.value.toLowerCase()),
  ),
);
const currentNode = (name: string) =>
  resolveProxyName(name, props.proxies || {});
function openGroup(name: string) {
  selectedName.value = name;
  search.value = "";
  saveError.value = "";
}
async function selectNode(name: string) {
  const group = selectedGroup.value;
  if (!group || !manual.value || saving.value || !props.isRunning) return;
  if (group.now === name) {
    selectedName.value = "";
    return;
  }
  saving.value = name;
  saveError.value = "";
  try {
    await getClashClient().selectProxy(group.name, name);
    selectedName.value = "";
    emit("notify", `${group.name} 已切换节点`, "success");
    emit("refresh");
  } catch (error: any) {
    saveError.value = `切换失败: ${error.message}`;
  } finally {
    saving.value = "";
  }
}
watch(
  () => props.isRunning,
  (running) => {
    if (!running) selectedName.value = "";
  },
);
</script>

<template>
  <section
    class="aw-card flex h-[320px] min-w-0 flex-col rounded-[var(--radius-xl)] bg-[var(--bg-surface)] p-4"
    aria-label="策略组"
  >
    <div class="mb-3 flex h-8 shrink-0 items-center justify-between gap-2">
      <h2 class="flex items-center gap-2 text-xs font-semibold">
        <Layers :size="16" class="text-[var(--text-secondary)]" />策略组<span
          class="rounded-full bg-[var(--bg-inset)] px-2 py-0.5 text-[11px] tabular-nums text-[var(--text-secondary)]"
          >{{ proxies ? groups.length : "—" }}</span
        >
      </h2>
      <button
        v-if="error"
        class="text-xs text-[var(--color-primary)]"
        @click="emit('refresh')"
      >
        重试
      </button>
    </div>
    <div
      v-if="error"
      role="alert"
      class="mb-2 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] p-2 text-xs text-[var(--color-error)]"
    >
      {{ error }}
    </div>
    <div class="min-h-0 flex-1 space-y-2 overflow-y-auto">
      <button
        v-for="group in groups"
        :key="group.name"
        class="focus-ring flex w-full min-w-0 items-center gap-3 rounded-[var(--radius-lg)] bg-[var(--bg-inset)] px-3 py-2.5 text-left transition-colors hover:bg-[var(--button-secondary-hover)]"
        :aria-label="`查看策略组 ${group.name}`"
        @click="openGroup(group.name)"
      >
        <span
          class="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--radius-md)] bg-[var(--button-secondary-bg)] text-xs font-bold text-[var(--text-secondary)]"
          >{{ group.name.slice(0, 1).toUpperCase() }}</span
        >
        <span class="min-w-0 flex-1"
          ><span class="block truncate text-sm font-semibold">{{
            group.name
          }}</span
          ><span
            class="mt-0.5 block truncate text-xs text-[var(--text-secondary)]"
            :title="currentNode(group.now)"
            >{{ currentNode(group.now) || "尚未选择节点" }}</span
          ></span
        >
        <ChevronRight :size="15" class="shrink-0 text-[var(--text-tertiary)]" />
      </button>
      <div
        v-if="!groups.length"
        class="flex h-full min-h-36 items-center justify-center text-xs text-[var(--text-tertiary)]"
      >
        {{
          !isRunning
            ? "核心未运行"
            : error
              ? "策略组暂不可用"
              : proxies
                ? "当前配置暂无策略组"
                : "正在读取策略组…"
        }}
      </div>
    </div>
    <Modal
      :open="!!selectedName"
      :title="selectedName"
      size="md"
      :closable="!saving"
      :close-on-backdrop="!saving"
      @close="selectedName = ''"
    >
      <input
        v-model="search"
        type="search"
        class="mb-3 w-full"
        placeholder="搜索节点"
        aria-label="搜索策略组节点"
      />
      <p v-if="!manual" class="mb-3 text-xs text-[var(--text-secondary)]">
        该策略组自动选择节点。
      </p>
      <div
        v-if="saveError"
        role="alert"
        class="mb-3 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] p-3 text-xs text-[var(--color-error)]"
      >
        {{ saveError }}
      </div>
      <div class="max-h-96 space-y-2 overflow-y-auto">
        <div
          v-for="name in options"
          :key="name"
          class="flex min-w-0 items-center gap-3 rounded-[var(--radius-md)] bg-[var(--bg-inset)] p-3"
        >
          <Check
            v-if="selectedGroup?.now === name"
            :size="16"
            class="shrink-0 text-[var(--color-primary)]"
          />
          <span v-else class="w-4 shrink-0" />
          <span class="min-w-0 flex-1"
            ><span class="block truncate text-sm" :title="name">{{ name }}</span
            ><span
              v-if="currentNode(name) !== name"
              class="mt-1 block truncate text-xs text-[var(--text-secondary)]"
              >{{ currentNode(name) }}</span
            ></span
          >
          <button
            v-if="manual"
            class="aw-action-button shrink-0"
            :disabled="!!saving || !isRunning"
            @click="selectNode(name)"
          >
            {{
              saving === name
                ? "切换中…"
                : selectedGroup?.now === name
                  ? "当前"
                  : "选择"
            }}
          </button>
        </div>
        <p
          v-if="!options.length"
          class="py-8 text-center text-xs text-[var(--text-tertiary)]"
        >
          {{ selectedGroup ? "没有匹配的节点" : "该策略组已不在当前配置中" }}
        </p>
      </div>
    </Modal>
  </section>
</template>
