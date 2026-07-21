<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  CheckCircle2,
  Copy,
  Database,
  Download,
  Eye,
  EyeOff,
  ExternalLink,
  LayoutDashboard,
  RefreshCw,
  Trash2,
} from "lucide-vue-next";
import { api } from "@/services/api";
import type { Dashboard } from "@/services/types";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();

const clashApiPort = ref("9090");
const clashApiSecret = ref("");
const showClashApiSecret = ref(false);
const dashboardID = ref("");
const cacheFileEnabled = ref(true);
const cacheFileStoreFakeIP = ref(true);
const cacheFileStoreDNS = ref(true);
const dashboards = ref<Dashboard[]>([]);
const loading = ref(true);
const checking = ref(false);
const installingID = ref("");
const deletingID = ref("");
const confirmDeleteOpen = ref(false);

const panel =
  "rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]";
const input =
  "aw-input w-full outline-none focus:border-[var(--color-primary)]";

const selectedDashboard = computed(
  () => dashboards.value.find((item) => item.id === dashboardID.value) || null,
);
const dashboardURL = computed(() =>
  dashboardID.value &&
  (dashboardID.value === "custom" || selectedDashboard.value?.installed)
    ? `${window.location.origin}/api/v1/clash/ui/`
    : "",
);

function generateSecret() {
  const alphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
  const values = crypto.getRandomValues(new Uint8Array(32));
  clashApiSecret.value = Array.from(
    values,
    (value) => alphabet[value & 63],
  ).join("");
}

async function copyDashboardURL() {
  try {
    await navigator.clipboard.writeText(dashboardURL.value);
    emit("notify", "面板地址已复制", "success");
  } catch {
    emit("notify", "复制失败，请手动选择面板地址", "error");
  }
}

async function load() {
  loading.value = true;
  try {
    const [settings, dashboardItems] = await Promise.all([
      api.getExperimentalSettings(),
      api.listDashboards(),
    ]);
    clashApiPort.value = settings.clash_api_port || "9090";
    clashApiSecret.value = settings.clash_api_secret || "";
    dashboardID.value = settings.clash_api_dashboard || "";
    cacheFileEnabled.value = settings.cache_file_enabled !== false;
    cacheFileStoreFakeIP.value = settings.cache_file_store_fakeip !== false;
    cacheFileStoreDNS.value = settings.cache_file_store_dns !== false;
    dashboards.value = dashboardItems;
  } catch (cause: any) {
    emit("notify", `加载实验性功能失败: ${cause.message}`, "error");
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (selectedDashboard.value && !selectedDashboard.value.installed) {
    emit("notify", "请先安装所选控制面板", "error");
    return;
  }
  try {
    await api.setExperimentalSettings({
      clash_api_enabled: true,
      clash_api_port: clashApiPort.value,
      clash_api_secret: clashApiSecret.value,
      clash_api_dashboard: dashboardID.value,
      cache_file_enabled: cacheFileEnabled.value,
      cache_file_store_fakeip: cacheFileStoreFakeIP.value,
      cache_file_store_dns: cacheFileStoreDNS.value,
    });
    emit("notify", "实验性功能设置已保存", "success");
    dashboards.value = await api.listDashboards();
  } catch (cause: any) {
    emit("notify", `保存失败: ${cause.message}`, "error");
  }
}

async function checkUpdates(showSuccess = true) {
  checking.value = true;
  try {
    dashboards.value = await api.checkDashboardUpdates();
    if (showSuccess) emit("notify", "控制面板更新检查完成", "success");
  } catch (cause: any) {
    emit("notify", `检查控制面板更新失败: ${cause.message}`, "error");
  } finally {
    checking.value = false;
  }
}

async function installSelected() {
  const dashboard = selectedDashboard.value;
  if (!dashboard) return;
  installingID.value = dashboard.id;
  try {
    await api.installDashboard(dashboard.id);
    emit(
      "notify",
      dashboard.installed ? `${dashboard.name} 已更新` : `${dashboard.name} 已安装`,
      "success",
    );
    dashboards.value = await api.checkDashboardUpdates();
  } catch (cause: any) {
    emit("notify", `控制面板安装失败: ${cause.message}`, "error");
  } finally {
    installingID.value = "";
  }
}

function requestDelete() {
  if (!selectedDashboard.value?.installed) return;
  confirmDeleteOpen.value = true;
}

async function deleteSelected() {
  const dashboard = selectedDashboard.value;
  confirmDeleteOpen.value = false;
  if (!dashboard) return;
  deletingID.value = dashboard.id;
  try {
    await api.deleteDashboard(dashboard.id);
    dashboardID.value = "";
    dashboards.value = await api.listDashboards();
    emit("notify", `${dashboard.name} 已删除`, "success");
  } catch (cause: any) {
    emit("notify", `删除控制面板失败: ${cause.message}`, "error");
  } finally {
    deletingID.value = "";
  }
}

onMounted(async () => {
  await load();
  if (dashboards.value.length) await checkUpdates(false);
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
          >
            实验性
          </span>
        </div>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          Clash API、外部控制面板与运行缓存统一配置。
        </p>
      </div>
      <span class="text-xs text-[var(--color-success)]">● Clash API 已强制启用</span>
    </div>

    <div v-if="loading" class="py-12 text-center text-sm text-[var(--text-secondary)]">
      正在加载设置...
    </div>
    <div v-else class="space-y-6">
      <div class="grid gap-4 lg:grid-cols-3">
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
              :aria-label="showClashApiSecret ? '隐藏 API 密钥' : '显示 API 密钥'"
              :title="showClashApiSecret ? '隐藏 API 密钥' : '显示 API 密钥'"
              @click="showClashApiSecret = !showClashApiSecret"
            >
              <EyeOff v-if="showClashApiSecret" :size="16" />
              <Eye v-else :size="16" />
            </button>
          </div>
        </div>
        <label class="block text-xs">
          控制面板
          <select v-model="dashboardID" :class="input">
            <option value="">不启用外部控制面板</option>
            <option v-if="dashboardID === 'custom'" value="custom">
              现有自定义控制面板
            </option>
            <option v-for="dashboard in dashboards" :key="dashboard.id" :value="dashboard.id">
              {{ dashboard.name }}{{ dashboard.installed ? "" : "（未安装）" }}
            </option>
          </select>
        </label>
      </div>

      <div
        class="border-y border-[var(--border-default)] py-4"
      >
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <div
              class="flex h-9 w-9 items-center justify-center rounded-[var(--radius-lg)] bg-[var(--color-primary-bg)] text-[var(--color-primary)]"
            >
              <LayoutDashboard :size="18" />
            </div>
            <div>
              <h3 class="text-sm font-medium">
                {{ selectedDashboard?.name || "本地控制面板" }}
              </h3>
              <p class="mt-0.5 text-xs text-[var(--text-secondary)]">
                {{
                  selectedDashboard?.description ||
                  "选择 MetaCubeXD、Yacd-meta 或 Zashboard，文件保存在数据目录的 dash 文件夹。"
                }}
              </p>
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button size="sm" :loading="checking" @click="checkUpdates()">
              <template #icon><RefreshCw :size="14" /></template>
              检查面板更新
            </Button>
            <Button
              v-if="selectedDashboard"
              size="sm"
              variant="primary"
              :loading="installingID === selectedDashboard.id"
              @click="installSelected"
            >
              <template #icon><Download :size="14" /></template>
              {{ selectedDashboard.installed ? "更新面板" : "安装面板" }}
            </Button>
            <Button
              v-if="selectedDashboard?.installed"
              size="sm"
              variant="danger"
              :disabled="selectedDashboard.selected"
              :loading="deletingID === selectedDashboard.id"
              @click="requestDelete"
            >
              <template #icon><Trash2 :size="14" /></template>
              删除
            </Button>
          </div>
        </div>

        <div v-if="selectedDashboard" class="mt-3 flex flex-wrap gap-2 text-xs">
          <span
            class="rounded-full px-2.5 py-1"
            :class="
              selectedDashboard.installed
                ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]'
                : 'bg-[var(--bg-sidebar-hover)] text-[var(--text-secondary)]'
            "
          >
            {{ selectedDashboard.installed ? "已安装" : "未安装" }}
          </span>
          <span
            v-if="selectedDashboard.current_version"
            class="rounded-full bg-[var(--bg-sidebar-hover)] px-2.5 py-1 text-[var(--text-secondary)]"
          >
            当前 {{ selectedDashboard.current_version }}
          </span>
          <span
            v-if="selectedDashboard.latest_version"
            class="rounded-full bg-[var(--color-primary-bg)] px-2.5 py-1 text-[var(--color-primary)]"
          >
            最新 {{ selectedDashboard.latest_version }}
          </span>
          <span
            v-if="selectedDashboard.update_available"
            class="rounded-full bg-[var(--color-warning-bg)] px-2.5 py-1 text-[var(--color-warning)]"
          >
            有更新
          </span>
          <span v-if="selectedDashboard.check_error" class="text-[var(--color-error)]">
            {{ selectedDashboard.check_error }}
          </span>
        </div>

        <div
          v-if="dashboardURL"
          class="mt-4 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-page)] p-3"
        >
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <div>
              <p class="text-xs font-medium">完整面板地址</p>
              <p class="mt-0.5 text-xs text-[var(--text-secondary)]">
                保存设置并重新生成配置后，由运行中的 sing-box Clash API 托管。
              </p>
            </div>
            <div class="flex gap-2">
              <Button size="sm" @click="copyDashboardURL">
                <template #icon><Copy :size="14" /></template>
                复制
              </Button>
              <a
                :href="dashboardURL"
                target="_blank"
                rel="noreferrer"
                class="btn-press focus-ring inline-flex h-7 items-center justify-center gap-1.5 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs font-medium"
              >
                <ExternalLink :size="14" />打开面板
              </a>
            </div>
          </div>
          <input
            :value="dashboardURL"
            readonly
            class="aw-input w-full select-all font-mono text-xs"
            aria-label="完整面板地址"
            @focus="($event.target as HTMLInputElement).select()"
          />
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
              <span class="mt-0.5 block text-xs text-[var(--text-secondary)]">
                重启后保留已分配的 FakeIP 映射。
              </span>
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
              <span class="mt-0.5 block text-xs text-[var(--text-secondary)]">
                重启后继续复用完整 DNS 查询结果。
              </span>
            </span>
          </label>
        </div>
      </div>

      <div
        class="flex gap-2 rounded-[var(--radius-md)] bg-[var(--color-success-bg)] p-3 text-xs text-[var(--color-success)]"
      >
        <CheckCircle2 :size="15" class="mt-0.5 shrink-0" />
        控制面板检查与下载使用全局更新代理；选中面板后，下次生成配置会自动写入本地 external_ui 路径。
      </div>

      <Button size="sm" class="self-start" @click="save">保存实验性功能设置</Button>
    </div>
  </section>

  <ConfirmDialog
    :open="confirmDeleteOpen"
    title="删除控制面板"
    :message="`确认删除本地 ${selectedDashboard?.name || ''} 文件？`"
    confirm-text="删除"
    danger
    @confirm="deleteSelected"
    @cancel="confirmDeleteOpen = false"
  />
</template>
