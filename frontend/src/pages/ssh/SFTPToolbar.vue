<script setup lang="ts">
import {
  ArrowDownToLine,
  ArrowUp,
  ClipboardPaste,
  Copy,
  FilePenLine,
  FilePlus2,
  Folder,
  FolderPlus,
  Home,
  Pencil,
  RefreshCw,
  Scissors,
  Trash2,
  Upload,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";

defineProps<{
  pathInput: string;
  loading: boolean;
  busy: boolean;
  canGoUp: boolean;
  entryCount: number;
  selectedCount: number;
  selectedFileCount: number;
  canEdit: boolean;
  canRename: boolean;
  canPaste: boolean;
}>();
const emit = defineEmits<{
  "update:pathInput": [value: string];
  go: [];
  home: [];
  parent: [];
  refresh: [];
  upload: [files: File[]];
  download: [];
  createFile: [];
  createDirectory: [];
  edit: [];
  copy: [];
  cut: [];
  paste: [];
  rename: [];
  delete: [];
}>();

function chooseFiles(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  input.value = "";
  if (files.length) emit("upload", files);
}
</script>

<template>
  <header class="shrink-0 border-b border-[var(--border-default)] p-3">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <Folder :size="16" class="text-[var(--color-primary)]" />
        <span class="text-sm font-semibold">SFTP 文件</span>
      </div>
      <div class="flex items-center gap-1">
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="主目录"
          aria-label="主目录"
          :disabled="busy"
          @click="$emit('home')"
        >
          <Home :size="15" />
        </button>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="上一级"
          aria-label="上一级"
          :disabled="busy || !canGoUp"
          @click="$emit('parent')"
        >
          <ArrowUp :size="15" />
        </button>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="刷新文件夹"
          aria-label="刷新文件夹"
          :disabled="loading || busy"
          @click="$emit('refresh')"
        >
          <RefreshCw :size="15" :class="loading && 'animate-spin'" />
        </button>
      </div>
    </div>
    <form class="mt-2 flex gap-2" @submit.prevent="$emit('go')">
      <input
        :value="pathInput"
        class="aw-input min-w-0 flex-1 font-mono text-xs"
        aria-label="远端路径"
        spellcheck="false"
        :disabled="busy"
        @input="
          $emit('update:pathInput', ($event.target as HTMLInputElement).value)
        "
      />
      <Button size="sm" :disabled="loading || busy" type="submit">转到</Button>
    </form>
    <div
      class="mt-2 flex items-center gap-1 overflow-x-auto border-t border-[var(--border-light)] pt-2"
      role="toolbar"
      aria-label="SFTP 文件操作"
    >
      <label
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center"
        :class="busy && 'pointer-events-none opacity-50'"
        title="上传文件"
        aria-label="上传文件"
      >
        <Upload :size="15" />
        <input
          type="file"
          class="hidden"
          multiple
          :disabled="busy"
          @change="chooseFiles"
        />
      </label>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="下载选中文件"
        aria-label="下载选中文件"
        :disabled="busy || !selectedFileCount"
        @click="$emit('download')"
      >
        <ArrowDownToLine :size="15" />
      </button>
      <span class="mx-0.5 h-5 w-px shrink-0 bg-[var(--border-default)]" />
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="新建文件"
        aria-label="新建文件"
        :disabled="busy"
        @click="$emit('createFile')"
      >
        <FilePlus2 :size="15" />
      </button>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="新建目录"
        aria-label="新建目录"
        :disabled="busy"
        @click="$emit('createDirectory')"
      >
        <FolderPlus :size="15" />
      </button>
      <span class="mx-0.5 h-5 w-px shrink-0 bg-[var(--border-default)]" />
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="在线编辑选中文本文件"
        aria-label="在线编辑选中文本文件"
        :disabled="busy || !canEdit"
        @click="$emit('edit')"
      >
        <FilePenLine :size="15" />
      </button>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="复制选中项"
        aria-label="复制选中项"
        :disabled="busy || !selectedCount"
        @click="$emit('copy')"
      >
        <Copy :size="15" />
      </button>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="剪切选中项"
        aria-label="剪切选中项"
        :disabled="busy || !selectedCount"
        @click="$emit('cut')"
      >
        <Scissors :size="15" />
      </button>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="粘贴到当前目录"
        aria-label="粘贴到当前目录"
        :disabled="busy || !canPaste"
        @click="$emit('paste')"
      >
        <ClipboardPaste :size="15" />
      </button>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center"
        title="重命名选中的文件或目录"
        aria-label="重命名选中的文件或目录"
        :disabled="busy || !canRename"
        @click="$emit('rename')"
      >
        <Pencil :size="15" />
      </button>
      <button
        class="aw-modal-close inline-flex h-7 w-7 shrink-0 items-center justify-center text-[var(--color-error)]"
        title="删除选中的文件或目录"
        aria-label="删除选中的文件或目录"
        :disabled="busy || !selectedCount"
        @click="$emit('delete')"
      >
        <Trash2 :size="15" />
      </button>
      <span
        class="ml-auto min-w-0 truncate text-[11px] text-[var(--text-tertiary)]"
      >
        {{ selectedCount ? `已选 ${selectedCount}` : `${entryCount} 项` }}
      </span>
    </div>
  </header>
</template>
