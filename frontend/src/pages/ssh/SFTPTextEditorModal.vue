<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { FilePenLine, RefreshCw } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import type { SSHSFTPTextFile } from "@/services/sshTypes";

const props = defineProps<{
  file: SSHSFTPTextFile | null;
  saving: boolean;
  error: string;
}>();
const emit = defineEmits<{
  close: [];
  reload: [];
  save: [content: string];
}>();

const content = ref("");
const original = ref("");
const confirmClose = ref(false);
const confirmReload = ref(false);
const dirty = computed(() => content.value !== original.value);

watch(
  () => props.file,
  (file) => {
    content.value = file?.content || "";
    original.value = file?.content || "";
    confirmClose.value = false;
    confirmReload.value = false;
  },
  { immediate: true },
);

function requestClose() {
  if (props.saving) return;
  if (dirty.value) confirmClose.value = true;
  else emit("close");
}

function keydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    if (dirty.value && !props.saving) emit("save", content.value);
  }
}

function discardChanges() {
  confirmClose.value = false;
  emit("close");
}

function requestReload() {
  if (props.saving) return;
  if (dirty.value) confirmReload.value = true;
  else emit("reload");
}

function discardAndReload() {
  confirmReload.value = false;
  emit("reload");
}
</script>

<template>
  <Modal
    :open="!!file"
    :title="file ? `编辑文本 · ${file.name}` : '编辑文本'"
    size="xl"
    :closable="!saving"
    @close="requestClose"
  >
    <template #actions>
      <span class="text-xs text-[var(--text-tertiary)]">
        {{ dirty ? "有未保存修改" : "已保存" }}
      </span>
    </template>
    <div
      class="mb-3 flex items-center justify-between gap-3 text-xs text-[var(--text-secondary)]"
    >
      <div class="flex min-w-0 items-center gap-2">
        <FilePenLine :size="15" class="shrink-0 text-[var(--color-primary)]" />
        <span class="truncate font-mono" :title="file?.path">{{
          file?.path
        }}</span>
      </div>
      <span class="shrink-0">{{ content.length.toLocaleString() }} 字符</span>
    </div>
    <div
      v-if="error"
      class="mb-3 flex items-center justify-between gap-3 rounded-[var(--radius-lg)] bg-[var(--color-error-bg)] px-3 py-2 text-xs text-[var(--color-error)]"
    >
      <span>{{ error }}</span>
      <button
        class="inline-flex shrink-0 items-center gap-1 font-medium"
        @click="requestReload"
      >
        <RefreshCw :size="13" />重新加载
      </button>
    </div>
    <textarea
      v-model="content"
      class="aw-input h-[min(68vh,720px)] min-h-[360px] w-full resize-none whitespace-pre p-3 font-mono text-[13px] leading-5"
      spellcheck="false"
      :disabled="saving"
      aria-label="远端文本文件内容"
      @keydown="keydown"
    />
    <template #footer>
      <Button :disabled="saving" @click="requestClose">关闭</Button>
      <Button
        variant="primary"
        :loading="saving"
        :disabled="!dirty"
        @click="$emit('save', content)"
      >
        保存 Ctrl+S
      </Button>
    </template>
  </Modal>
  <ConfirmDialog
    :open="confirmClose"
    title="放弃未保存修改"
    message="当前文本尚未保存，确认关闭编辑器？"
    confirm-text="放弃修改"
    danger
    above-modal
    @confirm="discardChanges"
    @cancel="confirmClose = false"
  />
  <ConfirmDialog
    :open="confirmReload"
    title="重新加载远端文件"
    message="重新加载会放弃当前未保存修改，确认继续？"
    confirm-text="放弃修改并重新加载"
    danger
    above-modal
    @confirm="discardAndReload"
    @cancel="confirmReload = false"
  />
</template>
