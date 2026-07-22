<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import {
  CheckCircle2,
  Download,
  ExternalLink,
  RefreshCw,
  Rocket,
} from "lucide-vue-next";
import { api } from "@/services/api";
import type {
  AppUpdateInstallStatus,
  AppUpdateStatus,
} from "@/services/types";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();

const acceleration = ref("ghproxy");
const customMirror = ref("");
const status = ref<AppUpdateStatus | null>(null);
const checking = ref(false);
const installing = ref(false);
const confirmOpen = ref(false);
const checkError = ref("");
let installPollTimer: number | undefined;
let installPollGeneration = 0;

const panel =
  "rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]";
const input =
  "aw-input w-full outline-none focus:border-[var(--color-primary)]";

const statusTone = computed(() => {
  if (checkError.value || status.value?.update_error) return "error";
  if (!status.value) return "neutral";
  if (installing.value || status.value.updating) return "warning";
  return status.value.update_available ? "warning" : "success";
});

const statusLabel = computed(() => {
  if (checking.value) return "正在检查";
  if (checkError.value || status.value?.update_error) return "安装失败";
  if (!status.value) return "尚未检查";
  if (installing.value || status.value.updating) return "正在安装";
  return status.value.update_available ? "发现新版本" : "已是最新版本";
});

function formatPublishedAt(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleString();
}

async function loadSettings() {
  try {
    const settings = await api.getUpdateSettings();
    acceleration.value = settings.acceleration || "";
    customMirror.value = settings.custom_mirror_url || "";
  } catch (cause: any) {
    emit("notify", `加载更新设置失败: ${cause.message}`, "error");
  }
}

async function saveSettings() {
  try {
    await api.setUpdateSettings({
      acceleration: acceleration.value,
      custom_mirror_url: customMirror.value,
    });
    emit("notify", "更新代理设置已保存", "success");
    await checkUpdate();
  } catch (cause: any) {
    emit("notify", `保存失败: ${cause.message}`, "error");
  }
}

async function checkUpdate() {
  checking.value = true;
  checkError.value = "";
  try {
    status.value = await api.checkAppUpdate();
  } catch (cause: any) {
    checkError.value = cause.message;
    emit("notify", `检查更新失败: ${cause.message}`, "error");
  } finally {
    checking.value = false;
  }
}

async function installUpdate() {
  confirmOpen.value = false;
  installing.value = true;
  checkError.value = "";
  if (status.value) {
    status.value = {
      ...status.value,
      message: "正在准备更新",
      update_error: undefined,
      install_log: undefined,
    };
  }
  const generation = ++installPollGeneration;
  startInstallPolling(generation);
  try {
    const result = await api.installAppUpdate();
    if (generation !== installPollGeneration) return;
    stopInstallPolling();
    emit("notify", result.message, "success");
    await waitForInstalledVersion(result.version, generation);
  } catch (cause: any) {
    if (generation !== installPollGeneration) return;
    stopInstallPolling();
    try {
      const current = await api.getAppUpdateInstallStatus();
      if (generation !== installPollGeneration) return;
      applyInstallStatus(current);
    } catch {
      // Keep the original install error when the API is unavailable.
    }
    emit("notify", `安装更新失败: ${cause.message}`, "error");
    installing.value = false;
  }
}

function startInstallPolling(generation: number) {
  stopInstallPolling();
  installPollTimer = window.setInterval(async () => {
    if (generation !== installPollGeneration) {
      stopInstallPolling();
      return;
    }
    try {
      const current = await api.getAppUpdateInstallStatus();
      if (generation === installPollGeneration) applyInstallStatus(current);
    } catch {
      // Dependency installation and service restart may briefly interrupt the API.
    }
  }, 3000);
}

function applyInstallStatus(current: AppUpdateInstallStatus) {
  if (status.value) {
    status.value = {
      ...status.value,
      current_version: current.current_version,
      message: current.message,
      updating: current.updating,
      update_error: current.update_error,
      install_log: current.install_log,
    };
    return;
  }
  status.value = {
    ...current,
    latest_version: "",
    update_available: false,
    can_install: false,
    platform: "",
    architecture: "",
  };
}

function stopInstallPolling() {
  if (installPollTimer === undefined) return;
  window.clearInterval(installPollTimer);
  installPollTimer = undefined;
}

async function waitForInstalledVersion(version: string, generation: number) {
  const deadline = Date.now() + 90000;
  while (Date.now() < deadline) {
    await new Promise((resolve) => window.setTimeout(resolve, 3000));
    if (generation !== installPollGeneration) return;
    try {
      const current = await api.getAppUpdateInstallStatus();
      if (generation !== installPollGeneration) return;
      applyInstallStatus(current);
      if (current.update_error) {
        installing.value = false;
        emit("notify", `安装更新失败: ${current.update_error}`, "error");
        return;
      }
      if (current.current_version === version) {
        window.location.reload();
        return;
      }
    } catch {
      // The management API is expected to be briefly unavailable while opkg runs.
    }
  }
  installing.value = false;
  emit(
    "notify",
    "更新安装仍在进行，请稍后手动刷新页面查看版本",
    "info",
  );
}

onMounted(async () => {
  await loadSettings();
  await checkUpdate();
});
onUnmounted(() => {
  installPollGeneration++;
  stopInstallPolling();
});
</script>

<template>
  <div class="grid gap-4 lg:grid-cols-2">
    <section :class="panel" class="relative overflow-hidden">
      <div
        class="pointer-events-none absolute -right-16 -top-20 h-56 w-56 rounded-full bg-[var(--color-primary-bg)] blur-3xl"
      />
      <div class="relative">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="flex items-center gap-3">
            <div
              class="flex h-11 w-11 items-center justify-center rounded-[var(--radius-lg)] bg-[var(--color-primary-bg)] text-[var(--color-primary)]"
            >
              <Rocket :size="22" />
            </div>
            <div>
              <h2 class="text-base font-semibold">Ackwrap WebUI 更新</h2>
              <p class="mt-1 text-xs text-[var(--text-secondary)]">
                WebUI 与后端打包为同一个 OpenWrt IPK，更新后自动重启管理服务。
              </p>
            </div>
          </div>
          <span
            class="rounded-full px-3 py-1 text-xs font-medium"
            :class="{
              'bg-[var(--color-success-bg)] text-[var(--color-success)]':
                statusTone === 'success',
              'bg-[var(--color-warning-bg)] text-[var(--color-warning)]':
                statusTone === 'warning',
              'bg-[var(--color-error-bg)] text-[var(--color-error)]':
                statusTone === 'error',
              'bg-[var(--bg-sidebar-hover)] text-[var(--text-secondary)]':
                statusTone === 'neutral',
            }"
          >
            {{ statusLabel }}
          </span>
        </div>

        <div class="mt-6 grid gap-3 sm:grid-cols-2">
          <div
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-page)] p-4"
          >
            <p class="text-xs text-[var(--text-tertiary)]">当前版本</p>
            <p class="mt-2 font-mono text-xl font-semibold">
              v{{ status?.current_version || "--" }}
            </p>
            <p class="mt-1 text-xs text-[var(--text-secondary)]">
              {{ status ? `${status.platform} / ${status.architecture}` : "等待检查" }}
            </p>
          </div>
          <div
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-page)] p-4"
          >
            <p class="text-xs text-[var(--text-tertiary)]">最新版本</p>
            <p class="mt-2 font-mono text-xl font-semibold text-[var(--color-primary)]">
              v{{ status?.latest_version || "--" }}
            </p>
            <p class="mt-1 text-xs text-[var(--text-secondary)]">
              {{ formatPublishedAt(status?.published_at) }}
            </p>
          </div>
        </div>

        <div
          v-if="checkError || status?.update_error || status?.message"
          class="mt-4 rounded-[var(--radius-md)] p-3 text-xs"
          :class="
            checkError || status?.update_error
              ? 'bg-[var(--color-error-bg)] text-[var(--color-error)]'
              : 'bg-[var(--color-primary-bg)] text-[var(--color-primary)]'
          "
        >
          {{ checkError || status?.update_error || status?.message }}
        </div>

        <div
          v-if="status?.install_log"
          class="mt-4 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-page)]"
        >
          <div
            class="border-b border-[var(--border-default)] px-3 py-2 text-xs font-medium text-[var(--text-secondary)]"
          >
            安装日志
          </div>
          <pre
            class="max-h-52 overflow-auto whitespace-pre-wrap break-all p-3 font-mono text-xs leading-5 text-[var(--text-secondary)]"
            >{{ status.install_log }}</pre
          >
        </div>

        <div class="mt-5 flex flex-wrap items-center gap-2">
          <Button :loading="checking" @click="checkUpdate">
            <template #icon><RefreshCw :size="14" /></template>
            重新检查
          </Button>
          <Button
            v-if="status?.update_available && !status?.updating"
            variant="primary"
            :disabled="!status.can_install"
            :loading="installing"
            @click="confirmOpen = true"
          >
            <template #icon><Download :size="14" /></template>
            更新到 v{{ status.latest_version }}
          </Button>
          <a
            v-if="status?.release_url"
            :href="status.release_url"
            target="_blank"
            rel="noreferrer"
            class="focus-ring inline-flex h-8 items-center gap-1.5 rounded-[var(--radius-lg)] px-3 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]"
          >
            <ExternalLink :size="14" />查看发布说明
          </a>
        </div>
      </div>
    </section>

    <section :class="panel" class="flex flex-col">
      <div class="mb-4 flex items-center gap-2">
        <Download :size="18" class="text-[var(--color-primary)]" />
        <h2 class="font-semibold">更新设置</h2>
      </div>
      <div class="flex flex-1 flex-col gap-4">
        <label class="block text-sm">
          下载加速
          <select v-model="acceleration" :class="input">
            <option value="">无加速（直连）</option>
            <option value="ghproxy">https://gh-proxy.com/</option>
            <option value="ghproxy_vip">https://ghproxy.vip/</option>
            <option value="jsdelivr_fastly">https://fastly.jsdelivr.net/</option>
            <option value="jsdelivr_testingcf">https://testingcf.jsdelivr.net/</option>
            <option value="jsdelivr_cdn">https://cdn.jsdelivr.net/</option>
            <option value="custom">自定义镜像</option>
          </select>
        </label>
        <label v-if="acceleration === 'custom'" class="block text-sm">
          自定义镜像 URL
          <input v-model="customMirror" :class="input" />
        </label>
        <div
          class="rounded-[var(--radius-md)] bg-[var(--color-success-bg)] p-3 text-xs text-[var(--color-success)]"
        >
          <div class="flex gap-2">
            <CheckCircle2 :size="15" class="mt-0.5 shrink-0" />
            <span>
              检查版本和下载 IPK 都使用这里配置的代理。启用代理后不会回退直连。
            </span>
          </div>
        </div>
        <Button class="mt-auto self-start" size="sm" @click="saveSettings">
          保存更新设置
        </Button>
      </div>
    </section>
  </div>

  <ConfirmDialog
    :open="confirmOpen"
    title="安装 Ackwrap 更新"
    :message="`将下载并安装 v${status?.latest_version || ''}。管理服务会短暂重启；如果核心当前正在运行，更新后会自动恢复。`"
    confirm-text="开始更新"
    @confirm="installUpdate"
    @cancel="confirmOpen = false"
  />
</template>
