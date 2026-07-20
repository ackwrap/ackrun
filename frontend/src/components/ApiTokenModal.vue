<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import { apiTokenRequired, establishAPISession } from "@/services/apiAuth";

const token = ref("");
const error = ref("");
const submitting = ref(false);
const input = ref<HTMLInputElement | null>(null);

watch(apiTokenRequired, async (required) => {
  if (!required) return;
  error.value = "";
  await nextTick();
  input.value?.focus();
});

async function submit() {
  const value = token.value.trim();
  if (!value) {
    error.value = "请输入 API Token";
    return;
  }
  submitting.value = true;
  error.value = "";
  try {
    await establishAPISession(value);
    apiTokenRequired.value = false;
    window.location.reload();
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "API Token 验证失败";
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <Modal :open="apiTokenRequired" title="API 认证" size="sm" :closable="false">
    <form class="space-y-4" @submit.prevent="submit">
      <p class="text-sm leading-6 text-[var(--text-secondary)]">
        当前 Ackwrap 服务启用了远程访问保护，请输入启动时配置的 API Token。
      </p>
      <label class="block text-xs text-[var(--text-secondary)]">
        API Token
        <input
          ref="input"
          v-model="token"
          class="aw-input mt-1 w-full"
          type="password"
          autocomplete="current-password"
          placeholder="输入 API Token"
        />
      </label>
      <p v-if="error" class="text-xs text-[var(--color-error)]" role="alert">
        {{ error }}
      </p>
      <Button
        type="submit"
        variant="primary"
        size="lg"
        full-width
        :loading="submitting"
      >
        验证并进入
      </Button>
    </form>
  </Modal>
</template>
