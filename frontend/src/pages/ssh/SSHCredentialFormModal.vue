<script setup lang="ts">
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import type {
  SSHCredential,
  SSHCredentialAuthType,
} from "@/services/sshTypes";

defineProps<{
  open: boolean;
  editing: SSHCredential | null;
  saving: boolean;
}>();
defineEmits<{ close: []; save: [] }>();

const name = defineModel<string>("name", { required: true });
const authType = defineModel<SSHCredentialAuthType>("authType", {
  required: true,
});
const secret = defineModel<string>("secret", { required: true });
const passphrase = defineModel<string>("passphrase", { required: true });
</script>

<template>
  <Modal
    :open="open"
    :title="editing ? '编辑 SSH 凭据' : '新增 SSH 凭据'"
    size="lg"
    :closable="!saving"
    @close="$emit('close')"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <label>
        <span class="aw-modal-label text-xs">凭据名称</span>
        <input v-model="name" class="aw-input mt-1 w-full" maxlength="128" />
      </label>
      <label>
        <span class="aw-modal-label text-xs">认证类型</span>
        <select v-model="authType" class="aw-input mt-1 w-full">
          <option value="password">密码</option>
          <option value="private_key">私钥</option>
        </select>
      </label>
      <label class="sm:col-span-2">
        <span class="aw-modal-label text-xs">
          {{ authType === "password" ? "密码" : "OpenSSH 私钥" }}
        </span>
        <textarea
          v-if="authType === 'private_key'"
          v-model="secret"
          class="aw-input mt-1 min-h-52 w-full resize-y font-mono text-xs"
          autocomplete="off"
          :placeholder="editing ? '留空保留现有私钥' : '粘贴私钥内容'"
        />
        <input
          v-else
          v-model="secret"
          class="aw-input mt-1 w-full"
          type="password"
          autocomplete="new-password"
          :placeholder="editing ? '留空保留现有密码' : '输入密码'"
        />
      </label>
      <label v-if="authType === 'private_key'" class="sm:col-span-2">
        <span class="aw-modal-label text-xs">私钥口令（如有）</span>
        <input
          v-model="passphrase"
          class="aw-input mt-1 w-full"
          type="password"
          autocomplete="new-password"
        />
      </label>
    </div>
    <p class="mt-4 text-xs text-[var(--text-tertiary)]">
      秘密仅在本次请求中提交；保存后 API 只返回“已配置”和公钥指纹。
    </p>
    <template #footer>
      <Button :disabled="saving" @click="$emit('close')">取消</Button>
      <Button variant="primary" :loading="saving" @click="$emit('save')">
        保存
      </Button>
    </template>
  </Modal>
</template>
