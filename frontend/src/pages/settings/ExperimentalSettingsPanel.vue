<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { Database, Eye, EyeOff } from "lucide-vue-next";
import { api } from "@/services/api";
import Button from "@/components/ui/Button.vue";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();

const clashApiPort = ref("9090");
const clashApiSecret = ref("");
const showClashApiSecret = ref(false);
const cacheFileEnabled = ref(true);
const cacheFileStoreFakeIP = ref(true);
const cacheFileStoreDNS = ref(true);
const loading = ref(true);
const loaded = ref(false);
const loadError = ref("");
const saving = ref(false);
let disposed = false;

const panel =
  "rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]";
const input =
  "aw-input w-full outline-none focus:border-[var(--color-primary)]";

function generateSecret() {
  const alphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
  const values = crypto.getRandomValues(new Uint8Array(32));
  clashApiSecret.value = Array.from(
    values,
    (value) => alphabet[value & 63],
  ).join("");
}

async function load() {
  loading.value = true;
  loaded.value = false;
  loadError.value = "";
  try {
    const settings = await api.getExperimentalSettings();
    if (disposed) return;
    clashApiPort.value = settings.clash_api_port || "9090";
    clashApiSecret.value = settings.clash_api_secret || "";
    cacheFileEnabled.value = settings.cache_file_enabled !== false;
    cacheFileStoreFakeIP.value = settings.cache_file_store_fakeip !== false;
    cacheFileStoreDNS.value = settings.cache_file_store_dns !== false;
    loaded.value = true;
  } catch (cause: any) {
    if (!disposed) loadError.value = `加载实验性功能失败: ${cause.message}`;
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (saving.value || loading.value || !loaded.value) return;
  saving.value = true;
  try {
    await api.setExperimentalSettings({
      clash_api_enabled: true,
      clash_api_port: clashApiPort.value,
      clash_api_secret: clashApiSecret.value,
      cache_file_enabled: cacheFileEnabled.value,
      cache_file_store_fakeip: cacheFileStoreFakeIP.value,
      cache_file_store_dns: cacheFileStoreDNS.value,
    });
    if (!disposed) emit("notify", "实验性功能设置已保存", "success");
  } catch (cause: any) {
    if (!disposed) emit("notify", `保存失败: ${cause.message}`, "error");
  } finally {
    saving.value = false;
  }
}

onMounted(load);
onBeforeUnmount(() => {
  disposed = true;
});
</script>

<template>
  <section :class="panel" class="flex flex-col" role="tabpanel">
    <div class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex items-center gap-2">
          <h2 class="font-semibold">实验性功能</h2>
          <span
            class="rounded-[var(--radius-sm)] bg-[var(--color-warning-bg)] px-2 py-0.5 text-xs text-[var(--color-warning)]"
            >实验性</span
          >
        </div>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          Clash API 与运行缓存配置。
        </p>
      </div>
      <span class="text-xs text-[var(--color-success)]"
        >● Clash API 已强制启用</span
      >
    </div>

    <div
      v-if="loading"
      role="status"
      class="py-12 text-center text-sm text-[var(--text-secondary)]"
    >
      正在加载设置...
    </div>
    <div v-else-if="loadError" role="alert" class="space-y-3">
      <p class="text-sm text-[var(--color-error)]">{{ loadError }}</p>
      <Button size="sm" @click="load">重新加载</Button>
    </div>
    <fieldset v-else :disabled="saving || !loaded" class="space-y-6">
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="block text-xs">
          Clash API 端口
          <input v-model="clashApiPort" :class="input" placeholder="9090" />
        </label>
        <div class="block text-xs">
          <div class="mb-1 flex items-center justify-between gap-2">
            <label for="clash-api-secret">API 密钥（可选）</label>
            <button
              type="button"
              class="focus-ring rounded px-1.5 py-0.5 text-[var(--color-primary)] hover:bg-[var(--color-primary-bg)]"
              @click="generateSecret"
            >
              随机生成
            </button>
          </div>
          <div class="relative">
            <input
              id="clash-api-secret"
              v-model="clashApiSecret"
              :type="showClashApiSecret ? 'text' : 'password'"
              autocomplete="new-password"
              :class="[input, 'pr-10']"
            />
            <button
              type="button"
              class="focus-ring absolute right-2 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-[var(--radius-md)] text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--text-primary)]"
              :aria-label="
                showClashApiSecret ? '隐藏 API 密钥' : '显示 API 密钥'
              "
              :title="showClashApiSecret ? '隐藏 API 密钥' : '显示 API 密钥'"
              @click="showClashApiSecret = !showClashApiSecret"
            >
              <EyeOff v-if="showClashApiSecret" :size="16" />
              <Eye v-else :size="16" />
            </button>
          </div>
        </div>
      </div>

      <div>
        <div class="mb-3 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <Database :size="17" class="text-[var(--color-primary)]" />
            <div>
              <h3 class="text-sm font-medium">缓存文件</h3>
              <p class="text-xs text-[var(--text-secondary)]">
                缓存 FakeIP、规则集和 DNS 数据，提高启动与查询性能。
              </p>
            </div>
          </div>
          <label class="flex cursor-pointer items-center gap-2 text-xs">
            <input
              v-model="cacheFileEnabled"
              type="checkbox"
              class="h-4 w-4 accent-[var(--color-primary)]"
            />
            启用缓存文件
          </label>
        </div>
        <div v-if="cacheFileEnabled" class="grid gap-3 sm:grid-cols-2">
          <label
            class="flex cursor-pointer items-start gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-page)] p-3"
          >
            <input
              v-model="cacheFileStoreFakeIP"
              type="checkbox"
              class="mt-0.5 h-4 w-4 shrink-0 accent-[var(--color-primary)]"
            />
            <span>
              <span class="block text-xs font-medium">缓存 FakeIP</span>
              <span class="mt-0.5 block text-xs text-[var(--text-secondary)]"
                >重启后保留已分配的 FakeIP 映射。</span
              >
            </span>
          </label>
          <label
            class="flex cursor-pointer items-start gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-page)] p-3"
          >
            <input
              v-model="cacheFileStoreDNS"
              type="checkbox"
              class="mt-0.5 h-4 w-4 shrink-0 accent-[var(--color-primary)]"
            />
            <span>
              <span class="block text-xs font-medium">持久化完整 DNS 缓存</span>
              <span class="mt-0.5 block text-xs text-[var(--text-secondary)]"
                >重启后继续复用完整 DNS 查询结果。</span
              >
            </span>
          </label>
        </div>
      </div>

      <Button
        size="sm"
        class="self-start"
        :loading="saving"
        :disabled="!loaded"
        @click="save"
      >
        保存实验性功能设置
      </Button>
    </fieldset>
  </section>
</template>
