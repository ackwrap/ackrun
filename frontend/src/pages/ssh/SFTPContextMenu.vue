<script setup lang="ts">
import {
  ArrowDownToLine,
  ClipboardPaste,
  Copy,
  FilePenLine,
  FolderOpen,
  Pencil,
  Scissors,
  Trash2,
} from "lucide-vue-next";

defineProps<{
  open: boolean;
  x: number;
  y: number;
  isDirectory: boolean;
  selectionCount: number;
  downloadCount: number;
  canPaste: boolean;
  busy: boolean;
}>();
defineEmits<{
  close: [];
  openEntry: [];
  download: [];
  edit: [];
  copy: [];
  cut: [];
  paste: [];
  rename: [];
  delete: [];
}>();
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[100]"
      @pointerdown="$emit('close')"
    >
      <div
        class="fixed w-48 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-elevated)] p-1 shadow-[var(--shadow-xl)]"
        :style="{ left: `${x}px`, top: `${y}px` }"
        role="menu"
        @pointerdown.stop
        @contextmenu.prevent
      >
        <button
          v-if="isDirectory && selectionCount === 1"
          class="sftp-menu-item"
          :disabled="busy"
          @click="$emit('openEntry')"
        >
          <FolderOpen :size="14" />打开目录
        </button>
        <button
          v-if="!isDirectory && selectionCount === 1"
          class="sftp-menu-item"
          :disabled="busy"
          @click="$emit('edit')"
        >
          <FilePenLine :size="14" />编辑文本
        </button>
        <button
          v-if="downloadCount > 0"
          class="sftp-menu-item"
          :disabled="busy"
          @click="$emit('download')"
        >
          <ArrowDownToLine :size="14" />下载文件
        </button>
        <div class="my-1 border-t border-[var(--border-light)]" />
        <button class="sftp-menu-item" :disabled="busy" @click="$emit('copy')">
          <Copy :size="14" />复制
        </button>
        <button class="sftp-menu-item" :disabled="busy" @click="$emit('cut')">
          <Scissors :size="14" />剪切
        </button>
        <button
          class="sftp-menu-item"
          :disabled="busy || !canPaste"
          @click="$emit('paste')"
        >
          <ClipboardPaste :size="14" />粘贴到当前目录
        </button>
        <div class="my-1 border-t border-[var(--border-light)]" />
        <button
          class="sftp-menu-item"
          :disabled="busy || selectionCount !== 1"
          @click="$emit('rename')"
        >
          <Pencil :size="14" />重命名
        </button>
        <button
          class="sftp-menu-item text-[var(--color-error)]"
          :disabled="busy"
          @click="$emit('delete')"
        >
          <Trash2 :size="14" />删除
        </button>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.sftp-menu-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 0.5rem;
  border-radius: var(--radius-md);
  padding: 0.45rem 0.6rem;
  font-size: 0.75rem;
  text-align: left;
}

.sftp-menu-item:hover:not(:disabled) {
  background: var(--bg-sidebar-hover);
}

.sftp-menu-item:disabled {
  cursor: not-allowed;
  opacity: 0.4;
}
</style>
