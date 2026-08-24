<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { Copy, Download, LockKeyhole, Upload } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import { sshApi } from "@/services/sshApi";
import type { SSHHost, SSHHostImportResponse } from "@/services/sshTypes";
import { errorMessage } from "../advanced/advancedUi";

const props = defineProps<{
  open: boolean;
  host: SSHHost | null;
}>();
const emit = defineEmits<{
  close: [];
  imported: [result: SSHHostImportResponse];
}>();

const password = ref("");
const confirmation = ref("");
const code = ref("");
const busy = ref(false);
const error = ref("");
const copied = ref(false);
const codeField = ref<HTMLTextAreaElement | null>(null);
const exportMode = computed(() => props.host !== null);

function clearSensitiveState(clearCode = true) {
  password.value = "";
  confirmation.value = "";
  if (clearCode) code.value = "";
  copied.value = false;
}

watch(
  () => props.open,
  () => {
    clearSensitiveState();
    error.value = "";
  },
);

function validPassword() {
  const length = Array.from(password.value).length;
  if (length < 8 || new TextEncoder().encode(password.value).length > 256) {
    error.value = "分享密码至少需要 8 个字符且不能超过 256 字节";
    return false;
  }
  return true;
}

async function exportHost() {
  if (!props.host || busy.value || !validPassword()) return;
  if (password.value !== confirmation.value) {
    error.value = "两次输入的分享密码不一致";
    return;
  }
  busy.value = true;
  error.value = "";
  copied.value = false;
  try {
    code.value = (await sshApi.shareHost(props.host.id, password.value)).code;
    clearSensitiveState(false);
  } catch (cause) {
    error.value = `生成分享码失败：${errorMessage(cause)}`;
  } finally {
    busy.value = false;
  }
}

async function importHost() {
  if (busy.value || !validPassword()) return;
  if (!code.value.trim()) {
    error.value = "请粘贴 SSH 主机分享码";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    const result = await sshApi.importHost(code.value.trim(), password.value);
    clearSensitiveState();
    emit("imported", result);
  } catch (cause) {
    error.value = `导入失败：${errorMessage(cause)}`;
  } finally {
    busy.value = false;
  }
}

async function copyCode() {
  if (!code.value) return;
  try {
    if (window.isSecureContext && navigator.clipboard) {
      await navigator.clipboard.writeText(code.value);
    } else {
      await nextTick();
      codeField.value?.select();
      if (!document.execCommand("copy")) throw new Error("copy failed");
    }
    copied.value = true;
  } catch {
    error.value = "自动复制失败，请在分享码文本框中手动复制";
  }
}

function close() {
  if (busy.value) return;
  clearSensitiveState();
  emit("close");
}
</script>

<template>
  <Modal
    :open="open"
    :title="exportMode ? '加密分享 SSH 主机' : '导入 SSH 主机'"
    size="lg"
    :closable="!busy"
    @close="close"
  >
    <div class="rounded-[var(--radius-lg)] bg-[var(--color-warning-bg)] p-4 text-sm">
      <div class="flex items-center gap-2 font-medium">
        <LockKeyhole :size="17" class="text-[var(--color-warning)]" />
        {{ exportMode ? host?.name : "密码保护的 SSH 主机分享码" }}
      </div>
      <p class="mt-2 text-xs leading-5 text-[var(--text-secondary)]">
        分享码包含主机地址、用户名和关联凭据，并使用密码加密。请通过不同渠道发送分享码和密码。
        Host Key 信任及本机节点入口不会导出，导入后使用直连并需重新核验 Host Key。
      </p>
    </div>

    <label v-if="!exportMode" class="mt-4 block">
      <span class="aw-modal-label text-xs">SSH 主机分享码</span>
      <textarea
        v-model="code"
        class="aw-input mt-1 min-h-36 w-full resize-y font-mono text-xs"
        maxlength="1048576"
        autocomplete="off"
        placeholder="粘贴 ackwrap-ssh-v1 开头的分享码"
      />
    </label>

    <div class="mt-4 grid gap-4 sm:grid-cols-2">
      <label :class="exportMode ? '' : 'sm:col-span-2'">
        <span class="aw-modal-label text-xs">分享密码</span>
        <input
          v-model="password"
          class="aw-input mt-1 w-full"
          type="password"
          autocomplete="new-password"
          maxlength="256"
          placeholder="至少 8 个字符"
          @keydown.enter="exportMode ? exportHost() : importHost()"
        />
      </label>
      <label v-if="exportMode">
        <span class="aw-modal-label text-xs">确认密码</span>
        <input
          v-model="confirmation"
          class="aw-input mt-1 w-full"
          type="password"
          autocomplete="new-password"
          maxlength="256"
          @keydown.enter="exportHost"
        />
      </label>
    </div>

    <div v-if="exportMode && code" class="mt-4">
      <div class="flex items-center justify-between gap-3">
        <span class="aw-modal-label text-xs">加密分享码</span>
        <Button size="sm" variant="ghost" @click="copyCode">
          <template #icon><Copy :size="13" /></template>
          {{ copied ? "已复制" : "复制" }}
        </Button>
      </div>
      <textarea
        ref="codeField"
        :value="code"
        class="aw-input mt-1 min-h-36 w-full resize-y font-mono text-xs"
        readonly
      />
    </div>

    <p v-if="error" class="mt-4 text-sm text-[var(--color-error)]">{{ error }}</p>

    <template #footer>
      <Button :disabled="busy" @click="close">关闭</Button>
      <Button
        variant="primary"
        :loading="busy"
        @click="exportMode ? exportHost() : importHost()"
      >
        <template #icon>
          <Download v-if="exportMode" :size="14" />
          <Upload v-else :size="14" />
        </template>
        {{ exportMode ? (code ? "重新生成" : "生成分享码") : "解密并导入" }}
      </Button>
    </template>
  </Modal>
</template>
