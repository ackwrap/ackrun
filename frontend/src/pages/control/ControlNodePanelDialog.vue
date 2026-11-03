<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import {
  Copy,
  Download,
  ExternalLink,
  PanelsTopLeft,
  RefreshCw,
  Trash2,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type { Dashboard, ExperimentalSettingsResponse } from "@/services/types";
import { writeClipboardText } from "@/utils/clipboard";

const props = defineProps<{ isRunning: boolean; disabled: boolean }>();
const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
  busy: [value: boolean];
}>();
const open = ref(false);
const loading = ref(false);
const loaded = ref(false);
const loadError = ref("");
const operation = ref("");
const dashboards = ref<Dashboard[]>([]);
const settings = ref<ExperimentalSettingsResponse | null>(null);
const selectedID = ref("");
const confirmDelete = ref(false);
let disposed = false;

const busy = computed(() => loading.value || !!operation.value);
const selected = computed(() =>
  dashboards.value.find((item) => item.id === selectedID.value),
);
const savedID = computed(() => settings.value?.clash_api_dashboard || "");
const panelURL = computed(() => {
  if (
    !settings.value?.clash_api_secret ||
    !savedID.value ||
    savedID.value !== selectedID.value ||
    (savedID.value !== "custom" && !selected.value?.installed)
  )
    return "";
  const url = new URL(window.location.origin);
  url.protocol = "http:";
  url.port = settings.value.clash_api_port || "9090";
  url.pathname = "/ui/";
  url.search = "";
  url.hash = "";
  return url.toString();
});

function launchURL() {
  if (!panelURL.value || !settings.value) return "";
  const params = new URLSearchParams({
    hostname: window.location.hostname,
    port: settings.value.clash_api_port || "9090",
    secret: settings.value.clash_api_secret || "",
  });
  if (savedID.value === "zashboard") {
    params.set("type", "clash");
    return `${panelURL.value}#/setup?${params}`;
  }
  if (["metacubexd", "yacd"].includes(savedID.value))
    return `${panelURL.value}?${params}`;
  return panelURL.value;
}

async function load() {
  if (busy.value) return;
  loading.value = true;
  loaded.value = false;
  loadError.value = "";
  try {
    const [currentSettings, items] = await Promise.all([
      api.getExperimentalSettings(),
      api.listDashboards(),
    ]);
    if (disposed) return;
    settings.value = currentSettings;
    selectedID.value = currentSettings.clash_api_dashboard || "";
    dashboards.value = items;
    loaded.value = true;
  } catch (cause: any) {
    if (!disposed) loadError.value = `节点面板加载失败: ${cause.message}`;
  } finally {
    loading.value = false;
  }
}

function show() {
  if (props.disabled || busy.value) return;
  open.value = true;
  confirmDelete.value = false;
  void load();
}

async function persistSelection() {
  const currentSettings = await api.getExperimentalSettings();
  let secret = currentSettings.clash_api_secret || "";
  if (selectedID.value && !secret) {
    const alphabet =
      "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
    secret = Array.from(
      crypto.getRandomValues(new Uint8Array(32)),
      (value) => alphabet[value & 63],
    ).join("");
  }
  const next = {
    ...currentSettings,
    clash_api_enabled: true,
    clash_api_secret: secret,
    clash_api_dashboard: selectedID.value,
    clash_api_external_ui: selectedID.value
      ? currentSettings.clash_api_external_ui
      : "",
    clash_api_external_ui_download_url: selectedID.value
      ? currentSettings.clash_api_external_ui_download_url
      : "",
  };
  await api.setExperimentalSettings(next);
  if (disposed) return;
  settings.value = next;
  emit(
    "notify",
    selectedID.value ? "节点面板已启用，配置已应用" : "外部面板已停用",
    "success",
  );
  try {
    dashboards.value = await api.listDashboards();
  } catch (cause: any) {
    emit("notify", `面板状态刷新失败: ${cause.message}`, "error");
  }
}

async function selectPanel() {
  confirmDelete.value = false;
  if (busy.value || props.disabled || selectedID.value === savedID.value)
    return;
  if (selected.value && !selected.value.installed) return;
  operation.value = "save";
  try {
    await persistSelection();
  } catch (cause: any) {
    selectedID.value = savedID.value;
    emit("notify", `启用节点面板失败: ${cause.message}`, "error");
  } finally {
    operation.value = "";
  }
}

async function checkUpdates() {
  if (busy.value || props.disabled) return;
  operation.value = "check";
  try {
    const items = await api.checkDashboardUpdates();
    if (disposed) return;
    dashboards.value = items;
    const failed = items.filter((item) => item.check_error).length;
    emit(
      "notify",
      failed ? `${failed} 个面板检查失败，请查看错误信息` : "面板更新检查完成",
      failed ? "error" : "success",
    );
  } catch (cause: any) {
    emit("notify", `检查面板更新失败: ${cause.message}`, "error");
  } finally {
    operation.value = "";
  }
}

async function install() {
  const item = selected.value;
  if (!item || busy.value || props.disabled) return;
  operation.value = "install";
  try {
    const installed = await api.installDashboard(item.id);
    if (disposed) return;
    dashboards.value = dashboards.value.map((entry) =>
      entry.id === item.id ? installed : entry,
    );
    if (savedID.value !== item.id) await persistSelection();
    else emit("notify", `${item.name} 已更新`, "success");
  } catch (cause: any) {
    emit("notify", `面板安装或启用失败: ${cause.message}`, "error");
  } finally {
    operation.value = "";
  }
}

async function remove() {
  const item = selected.value;
  if (!item?.installed || item.selected || busy.value || props.disabled) return;
  operation.value = "delete";
  try {
    await api.deleteDashboard(item.id);
    if (disposed) return;
    confirmDelete.value = false;
    dashboards.value = dashboards.value.map((entry) =>
      entry.id === item.id
        ? {
            ...entry,
            installed: false,
            current_version: undefined,
            update_available: true,
          }
        : entry,
    );
    emit("notify", `${item.name} 已删除`, "success");
  } catch (cause: any) {
    emit("notify", `删除面板失败: ${cause.message}`, "error");
  } finally {
    operation.value = "";
  }
}

async function copyURL() {
  const url = launchURL();
  if (!url) return;
  try {
    await writeClipboardText(url);
    emit("notify", "面板连接地址已复制", "success");
  } catch {
    emit("notify", "复制面板地址失败", "error");
  }
}

function openPanel() {
  const url = launchURL();
  if (url && props.isRunning) window.open(url, "_blank", "noopener,noreferrer");
}

watch(operation, (value) => emit("busy", !!value));
onBeforeUnmount(() => {
  disposed = true;
  emit("busy", false);
});
</script>

<template>
  <Button :disabled="disabled || busy" @click="show">
    <PanelsTopLeft :size="14" />节点面板
  </Button>
  <Modal
    :open="open"
    title="节点面板"
    size="lg"
    :closable="!operation"
    @close="open = false"
  >
    <p
      v-if="loading"
      class="py-8 text-center text-sm text-[var(--text-secondary)]"
    >
      正在加载节点面板…
    </p>
    <div v-else-if="loadError" class="space-y-3">
      <p role="alert" class="text-sm text-[var(--color-error)]">
        {{ loadError }}
      </p>
      <Button @click="load"><RefreshCw :size="14" />重试</Button>
    </div>
    <div v-else-if="loaded" class="space-y-4">
      <label class="block text-sm">
        外部面板
        <select
          v-model="selectedID"
          class="aw-input mt-2 w-full"
          :disabled="!!operation || disabled"
          @change="selectPanel"
        >
          <option value="">不启用外部面板</option>
          <option v-if="savedID === 'custom'" value="custom">
            现有自定义面板
          </option>
          <option v-for="item in dashboards" :key="item.id" :value="item.id">
            {{ item.name }}{{ item.installed ? "" : "（未安装）" }}
          </option>
        </select>
      </label>
      <p class="text-xs text-[var(--text-secondary)]">
        选择已安装的面板后自动启用；未安装时，下载完成后自动启用。
      </p>
      <div
        v-if="selected"
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
      >
        <p class="font-medium">{{ selected.name }}</p>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          {{ selected.description }}
        </p>
        <div class="mt-3 flex flex-wrap gap-3 text-xs">
          <span
            :class="
              selected.installed
                ? 'text-[var(--color-success)]'
                : 'text-[var(--text-secondary)]'
            "
            >{{ selected.installed ? "已安装" : "未安装" }}</span
          >
          <span v-if="selected.selected" class="text-[var(--color-success)]"
            >已启用</span
          >
          <span v-if="selected.current_version"
            >当前 {{ selected.current_version }}</span
          >
          <span v-if="selected.latest_version"
            >最新 {{ selected.latest_version }}</span
          >
          <span
            v-if="selected.update_available"
            class="text-[var(--color-warning)]"
            >有更新</span
          >
        </div>
        <p
          v-if="selected.check_error"
          role="alert"
          class="mt-3 break-words text-xs text-[var(--color-error)]"
        >
          {{ selected.check_error }}
        </p>
        <div class="mt-4 flex flex-wrap gap-2">
          <Button
            variant="primary"
            :disabled="busy || disabled"
            :loading="operation === 'install'"
            @click="install"
            ><Download :size="14" />{{
              selected.installed ? "更新面板" : "下载并安装"
            }}</Button
          >
          <Button
            v-if="selected.installed"
            :disabled="busy || disabled || selected.selected"
            @click="confirmDelete = true"
            ><Trash2 :size="14" />删除面板</Button
          >
        </div>
        <div
          v-if="confirmDelete"
          role="alert"
          class="mt-4 space-y-3 border-t border-[var(--border-default)] pt-3"
        >
          <p class="text-sm">确认删除 {{ selected.name }}？</p>
          <div class="flex flex-wrap gap-2">
            <Button
              variant="danger"
              :disabled="busy || disabled"
              :loading="operation === 'delete'"
              @click="remove"
              >确认删除</Button
            >
            <Button :disabled="busy" @click="confirmDelete = false"
              >取消</Button
            >
          </div>
        </div>
      </div>
      <Button
        :disabled="busy || disabled"
        :loading="operation === 'check'"
        @click="checkUpdates"
        ><RefreshCw :size="14" />检查面板更新</Button
      >
      <div
        v-if="panelURL"
        class="space-y-3 border-t border-[var(--border-default)] pt-4"
      >
        <p class="break-all text-xs text-[var(--text-secondary)]">
          {{ panelURL }}
        </p>
        <div class="flex flex-wrap gap-2">
          <Button
            variant="primary"
            :disabled="busy || !isRunning"
            @click="openPanel"
            ><ExternalLink :size="14" />打开面板</Button
          >
          <Button :disabled="busy" @click="copyURL"
            ><Copy :size="14" />复制连接地址</Button
          >
        </div>
        <p class="text-xs text-[var(--text-tertiary)]">
          {{
            isRunning
              ? "连接地址包含 API 密钥，请妥善保管。"
              : "启动核心后可打开面板。"
          }}
        </p>
      </div>
    </div>
  </Modal>
</template>
