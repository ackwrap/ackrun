<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { Copy, Eye, EyeOff, KeyRound, RefreshCw, Save } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import { writeClipboardText } from "@/utils/clipboard";

const props = defineProps<{
  proxyPort: number;
  inboundMode: string;
  disabled: boolean;
}>();
const emit = defineEmits<{
  message: [string, "success" | "error" | "info"];
  busy: [value: boolean];
}>();
const open = ref(false);
const username = ref("");
const password = ref("");
const savedUsername = ref("");
const savedPassword = ref("");
const showPassword = ref(false);
const loading = ref(false);
const loaded = ref(false);
const loadError = ref("");
const saving = ref(false);
let disposed = false;
const mixedEnabled = computed(() => props.inboundMode !== "tun");
const dirty = computed(
  () =>
    username.value !== savedUsername.value ||
    password.value !== savedPassword.value,
);
const authConfigured = computed(
  () => !!savedUsername.value && !!savedPassword.value,
);
const authEnabled = computed(() => mixedEnabled.value && authConfigured.value);
const proxyHost = computed(() => {
  const hostname = window.location.hostname || "127.0.0.1";
  return hostname.includes(":") && !hostname.startsWith("[")
    ? `[${hostname}]`
    : hostname;
});
const proxyURL = computed(() => {
  if (!authEnabled.value) return "";
  return `http://${encodeURIComponent(savedUsername.value)}:${encodeURIComponent(savedPassword.value)}@${proxyHost.value}:${props.proxyPort || 7890}`;
});
const displayedURL = computed(() => {
  if (!proxyURL.value || showPassword.value) return proxyURL.value;
  return `http://${encodeURIComponent(savedUsername.value)}:••••••@${proxyHost.value}:${props.proxyPort || 7890}`;
});

function randomToken(length: number) {
  const alphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
  const values = crypto.getRandomValues(new Uint8Array(length));
  return Array.from(values, (value) => alphabet[value & 63]).join("");
}

function generateCredentials() {
  try {
    username.value = `u${randomToken(6)}`;
    password.value = randomToken(10);
    showPassword.value = true;
    emit("message", "已生成简短随机账户和密码，请保存后使用", "info");
  } catch {
    emit("message", "当前浏览器无法安全生成随机账户", "error");
  }
}

async function load() {
  if (loading.value) return;
  loading.value = true;
  loaded.value = false;
  loadError.value = "";
  try {
    const settings = await api.getMixedInboundSettings();
    if (disposed) return;
    username.value = settings.username || "";
    password.value = settings.password || "";
    savedUsername.value = username.value;
    savedPassword.value = password.value;
    loaded.value = true;
  } catch (cause: any) {
    if (!disposed) loadError.value = `Mixed 代理认证加载失败: ${cause.message}`;
  } finally {
    loading.value = false;
  }
}
function show() {
  if (saving.value || props.disabled) return;
  open.value = true;
  showPassword.value = false;
  void load();
}

async function save() {
  if (
    saving.value ||
    loading.value ||
    !loaded.value ||
    props.disabled ||
    !dirty.value
  )
    return;
  saving.value = true;
  const nextUsername = username.value;
  const nextPassword = password.value;
  try {
    await api.setMixedInboundSettings({
      username: nextUsername,
      password: nextPassword,
    });
    if (disposed) return;
    savedUsername.value = nextUsername.trim();
    username.value = savedUsername.value;
    savedPassword.value = nextPassword;
    emit(
      "message",
      authConfigured.value
        ? mixedEnabled.value
          ? "Mixed 代理认证已保存并应用"
          : "Mixed 代理认证已保存，切换到 Mixed 模式后生效"
        : "Mixed 代理认证已关闭",
      authConfigured.value && !mixedEnabled.value ? "info" : "success",
    );
  } catch (cause: any) {
    emit("message", `Mixed 代理认证保存失败: ${cause.message}`, "error");
  } finally {
    saving.value = false;
  }
}

async function copyURL() {
  if (
    loading.value ||
    saving.value ||
    !loaded.value ||
    !proxyURL.value ||
    dirty.value
  )
    return;
  try {
    await writeClipboardText(proxyURL.value);
    emit("message", "完整 Mixed HTTP 代理地址已复制", "success");
  } catch (cause: any) {
    emit("message", `代理地址复制失败: ${cause.message}`, "error");
  }
}

watch(saving, (value) => emit("busy", value));
onBeforeUnmount(() => {
  disposed = true;
  emit("busy", false);
});
</script>

<template>
  <Button :disabled="disabled || saving" @click="show">
    <KeyRound :size="14" />Mixed 认证
  </Button>
  <Modal
    :open="open"
    title="Mixed 代理认证"
    size="md"
    :closable="!saving"
    @close="open = false"
  >
    <div class="mb-3 flex items-center justify-between gap-3">
      <span class="text-sm text-[var(--text-secondary)]"
        >HTTP / SOCKS 代理认证</span
      >
      <span
        class="text-xs"
        :class="
          authEnabled
            ? 'text-[var(--color-success)]'
            : 'text-[var(--text-tertiary)]'
        "
        >{{
          loading
            ? "读取中…"
            : !loaded
              ? "状态未知"
              : !mixedEnabled
                ? authConfigured
                  ? "已保存 · 当前未生效"
                  : "Mixed 未启用"
                : authEnabled
                  ? "已启用"
                  : "未启用"
        }}</span
      >
    </div>
    <div v-if="loadError" role="alert" class="mb-4 space-y-2">
      <p class="text-sm text-[var(--color-error)]">{{ loadError }}</p>
      <Button size="sm" @click="load">重新加载</Button>
    </div>

    <div class="grid gap-2 sm:grid-cols-2">
      <label class="text-xs text-[var(--text-secondary)]">
        用户名
        <input
          v-model="username"
          class="aw-input mt-1 w-full"
          maxlength="64"
          autocomplete="username"
          :disabled="loading || saving || !loaded"
          placeholder="同时留空可关闭认证"
        />
      </label>
      <label class="text-xs text-[var(--text-secondary)]">
        密码
        <span class="relative mt-1 block">
          <input
            v-model="password"
            class="aw-input w-full pr-9"
            :type="showPassword ? 'text' : 'password'"
            maxlength="128"
            autocomplete="new-password"
            :disabled="loading || saving || !loaded"
            placeholder="请输入代理密码"
          />
          <button
            type="button"
            class="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-[var(--text-secondary)]"
            :title="showPassword ? '隐藏密码' : '显示密码'"
            :disabled="loading || saving || !loaded"
            @click="showPassword = !showPassword"
          >
            <EyeOff v-if="showPassword" :size="15" />
            <Eye v-else :size="15" />
          </button>
        </span>
      </label>
    </div>

    <div class="mt-3 flex flex-wrap gap-2">
      <Button
        size="sm"
        variant="ghost"
        :disabled="loading || saving || !loaded"
        @click="generateCredentials"
      >
        <RefreshCw :size="14" />随机生成
      </Button>
    </div>

    <div class="mt-3 flex items-center gap-2">
      <div
        class="min-w-0 flex-1 truncate rounded-[var(--radius-md)] border border-[var(--border-light)] bg-[var(--bg-base)] px-3 py-2 font-mono text-xs text-[var(--text-secondary)]"
        :title="displayedURL"
      >
        {{
          loading
            ? "正在读取认证设置…"
            : !loaded
              ? "认证设置未加载"
              : !mixedEnabled
                ? "纯 TUN 模式未启用 Mixed 代理"
                : dirty
                  ? "请先保存修改后再复制地址"
                  : displayedURL || "设置账户密码后可复制完整地址"
        }}
      </div>
      <Button
        size="sm"
        variant="ghost"
        title="复制完整 HTTP 代理地址"
        :disabled="loading || saving || !loaded || !proxyURL || dirty"
        @click="copyURL"
      >
        <Copy :size="14" />复制
      </Button>
    </div>
    <template #footer>
      <Button :disabled="saving" @click="open = false">关闭</Button>
      <Button
        variant="primary"
        :loading="saving"
        :disabled="loading || saving || !loaded || !dirty || disabled"
        @click="save"
      >
        <Save :size="14" />保存并应用
      </Button>
    </template>
  </Modal>
</template>
