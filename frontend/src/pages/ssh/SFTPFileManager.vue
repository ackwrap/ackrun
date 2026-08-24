<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  ArrowDownToLine,
  ArrowUp,
  File as FileIcon,
  FilePlus2,
  Folder,
  FolderPlus,
  Home,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { sshApi } from "@/services/sshApi";
import type {
  SSHSessionCreateResponse,
  SSHSFTPEntry,
} from "@/services/sshTypes";

const props = defineProps<{ session: SSHSessionCreateResponse }>();

const path = ref(".");
const pathInput = ref(".");
const home = ref(".");
const parent = ref(".");
const entries = ref<SSHSFTPEntry[]>([]);
const selected = ref<SSHSFTPEntry | null>(null);
const loading = ref(true);
const error = ref("");
const message = ref("");
const fileInput = ref<HTMLInputElement | null>(null);
const uploading = ref(false);
const uploadProgress = ref(0);
const uploadName = ref("");
const action = ref<"create" | "mkdir" | "rename" | null>(null);
const actionName = ref("");
const actionSaving = ref(false);
const deleting = ref<SSHSFTPEntry | null>(null);
const pendingOverwrite = ref<File[]>([]);

const canGoUp = computed(
  () => path.value !== "/" && parent.value !== path.value,
);

function remoteJoin(base: string, name: string) {
  return base === "/" ? `/${name}` : `${base.replace(/\/+$/, "")}/${name}`;
}

function validName(value: string) {
  return (
    value.length > 0 &&
    value !== "." &&
    value !== ".." &&
    !/[\\/]/.test(value) &&
    !/[\0\r\n]/.test(value)
  );
}

function formatSize(size: number) {
  if (size < 1024) return `${size} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = size / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}

function formatTime(value: number) {
  if (!value) return "--";
  return new Date(value).toLocaleString(undefined, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

async function load(requestedPath = pathInput.value) {
  if (loading.value && entries.value.length) return;
  loading.value = true;
  error.value = "";
  try {
    const result = await sshApi.listSFTP(
      props.session.session_id,
      props.session.sftp_token,
      requestedPath,
    );
    path.value = result.path;
    pathInput.value = result.path;
    home.value = result.home;
    parent.value = result.parent;
    entries.value = result.entries;
    selected.value = null;
  } catch (cause: any) {
    error.value = cause?.message || "SFTP 目录读取失败";
  } finally {
    loading.value = false;
  }
}

function openEntry(entry: SSHSFTPEntry) {
  if (uploading.value) return;
  selected.value = entry;
  if (entry.is_dir) void load(entry.path);
  else void download(entry);
}

function openAction(next: "create" | "mkdir" | "rename") {
  if (uploading.value) return;
  action.value = next;
  actionName.value = next === "rename" ? selected.value?.name || "" : "";
}

async function saveAction() {
  if (uploading.value || actionSaving.value) return;
  const name = actionName.value.trim();
  if (!validName(name)) {
    message.value = "名称不能为空，且不能包含斜杠或换行";
    return;
  }
  actionSaving.value = true;
  message.value = "";
  try {
    if (action.value === "create") {
      await sshApi.createSFTPFile(
        props.session.session_id,
        props.session.sftp_token,
        remoteJoin(path.value, name),
      );
      message.value = "文件已创建";
    } else if (action.value === "mkdir") {
      await sshApi.createSFTPDirectory(
        props.session.session_id,
        props.session.sftp_token,
        remoteJoin(path.value, name),
      );
      message.value = "目录已创建";
    } else if (action.value === "rename" && selected.value) {
      await sshApi.renameSFTP(
        props.session.session_id,
        props.session.sftp_token,
        selected.value.path,
        remoteJoin(path.value, name),
      );
      message.value = "名称已更新";
    }
    action.value = null;
    await load(path.value);
  } catch (cause: any) {
    message.value = cause?.message || "SFTP 操作失败";
  } finally {
    actionSaving.value = false;
  }
}

async function removeEntry() {
  if (uploading.value) return;
  const entry = deleting.value;
  deleting.value = null;
  if (!entry) return;
  try {
    await sshApi.deleteSFTP(
      props.session.session_id,
      props.session.sftp_token,
      entry.path,
      entry.is_dir,
    );
    message.value = `${entry.name} 已删除`;
    await load(path.value);
  } catch (cause: any) {
    message.value = cause?.message || "删除失败";
  }
}

async function download(entry: SSHSFTPEntry) {
  if (uploading.value || entry.is_dir) return;
  message.value = `正在下载 ${entry.name}`;
  try {
    const blob = await sshApi.downloadSFTP(
      props.session.session_id,
      props.session.sftp_token,
      entry.path,
    );
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = entry.name;
    link.click();
    URL.revokeObjectURL(url);
    message.value = `${entry.name} 下载已开始`;
  } catch (cause: any) {
    message.value = cause?.message || "下载失败";
  }
}

function uploadFiles(files: File[]) {
  if (!files.length || uploading.value) return;
  const existingNames = new Set(entries.value.map((entry) => entry.name));
  if (files.some((file) => existingNames.has(file.name))) {
    pendingOverwrite.value = files;
    return;
  }
  void performUploads(files, false);
}

async function performUploads(files: File[], overwrite: boolean) {
  const targetPath = path.value;
  let completed = 0;
  let failure = "";
  action.value = null;
  deleting.value = null;
  uploading.value = true;
  message.value = "";
  try {
    for (const file of files) {
      if (file.size > 512 * 1024 * 1024) {
        throw new Error(`${file.name} 超过 512 MiB 上传限制`);
      }
      uploadName.value = file.name;
      uploadProgress.value = 0;
      await sshApi.uploadSFTP(
        props.session.session_id,
        props.session.sftp_token,
        remoteJoin(targetPath, file.name),
        file,
        overwrite,
        (progress) => (uploadProgress.value = progress),
      );
      completed++;
    }
  } catch (cause: any) {
    failure = cause?.message || "上传失败";
  } finally {
    uploading.value = false;
    uploadName.value = "";
    uploadProgress.value = 0;
    if (fileInput.value) fileInput.value.value = "";
    await load(targetPath);
    message.value = failure
      ? `已完成 ${completed}/${files.length}：${failure}`
      : `${files.length} 个文件上传完成`;
  }
}

function confirmOverwrite() {
  const files = pendingOverwrite.value;
  pendingOverwrite.value = [];
  void performUploads(files, true);
}

function chooseFiles(event: Event) {
  const input = event.target as HTMLInputElement;
  uploadFiles(Array.from(input.files || []));
}

function dropFiles(event: DragEvent) {
  uploadFiles(Array.from(event.dataTransfer?.files || []));
}

onMounted(() => load("."));
</script>

<template>
  <section
    class="flex h-full min-h-0 flex-col bg-[var(--bg-surface)]"
    @dragover.prevent
    @drop.prevent="dropFiles"
  >
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
            :disabled="uploading"
            @click="load(home)"
          >
            <Home :size="15" />
          </button>
          <button
            class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
            title="上一级"
            aria-label="上一级"
            :disabled="uploading || !canGoUp"
            @click="load(parent)"
          >
            <ArrowUp :size="15" />
          </button>
          <button
            class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
            title="刷新文件夹"
            aria-label="刷新文件夹"
            :disabled="loading || uploading"
            @click="load(path)"
          >
            <RefreshCw :size="15" :class="loading && 'animate-spin'" />
          </button>
        </div>
      </div>
      <form class="mt-2 flex gap-2" @submit.prevent="load(pathInput)">
        <input
          v-model="pathInput"
          class="aw-input min-w-0 flex-1 font-mono text-xs"
          aria-label="远端路径"
          spellcheck="false"
          :disabled="uploading"
        />
        <Button size="sm" :disabled="loading || uploading" type="submit"
          >转到</Button
        >
      </form>
      <div
        class="mt-2 flex items-center gap-1 border-t border-[var(--border-light)] pt-2"
        role="toolbar"
        aria-label="SFTP 文件操作"
      >
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="上传文件"
          aria-label="上传文件"
          :disabled="uploading"
          @click="fileInput?.click()"
        >
          <Upload :size="15" />
        </button>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="下载选中文件"
          aria-label="下载选中文件"
          :disabled="uploading || !selected || selected.is_dir"
          @click="selected && download(selected)"
        >
          <ArrowDownToLine :size="15" />
        </button>
        <span class="mx-0.5 h-5 w-px bg-[var(--border-default)]" />
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="新建文件"
          aria-label="新建文件"
          :disabled="uploading"
          @click="openAction('create')"
        >
          <FilePlus2 :size="15" />
        </button>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="新建目录"
          aria-label="新建目录"
          :disabled="uploading"
          @click="openAction('mkdir')"
        >
          <FolderPlus :size="15" />
        </button>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="重命名选中的文件或目录"
          aria-label="重命名选中的文件或目录"
          :disabled="uploading || !selected"
          @click="openAction('rename')"
        >
          <Pencil :size="15" />
        </button>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center text-[var(--color-error)]"
          title="删除选中的文件或目录"
          aria-label="删除选中的文件或目录"
          :disabled="uploading || !selected"
          @click="deleting = selected"
        >
          <Trash2 :size="15" />
        </button>
        <span
          class="ml-auto min-w-0 truncate text-[11px] text-[var(--text-tertiary)]"
        >
          {{ selected ? selected.name : `${entries.length} 项` }}
        </span>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          multiple
          @change="chooseFiles"
        />
      </div>
    </header>

    <div
      v-if="uploading"
      class="shrink-0 border-b border-[var(--border-light)] px-3 py-2"
    >
      <div
        class="flex justify-between gap-3 text-[11px] text-[var(--text-secondary)]"
      >
        <span class="truncate">上传 {{ uploadName }}</span>
        <span>{{ uploadProgress }}%</span>
      </div>
      <div class="mt-1 h-1 overflow-hidden rounded-full bg-[var(--bg-base)]">
        <div
          class="h-full bg-[var(--color-primary)] transition-[width]"
          :style="{ width: `${uploadProgress}%` }"
        />
      </div>
    </div>

    <div
      v-if="error"
      class="m-3 rounded-[var(--radius-lg)] bg-[var(--color-error-bg)] p-3 text-xs"
    >
      <p class="text-[var(--color-error)]">{{ error }}</p>
      <button
        class="mt-2 text-[var(--color-primary)] hover:underline disabled:opacity-50"
        :disabled="uploading"
        @click="load(pathInput)"
      >
        重新连接 SFTP
      </button>
    </div>

    <div v-else class="min-h-0 flex-1 overflow-auto">
      <table class="w-full table-fixed text-left text-xs">
        <thead
          class="sticky top-0 z-10 bg-[var(--bg-elevated)] text-[var(--text-tertiary)]"
        >
          <tr class="border-b border-[var(--border-default)]">
            <th class="w-[48%] px-3 py-2 font-medium">名称</th>
            <th class="w-[22%] px-2 py-2 font-medium">大小</th>
            <th class="px-2 py-2 font-medium">修改时间</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in entries"
            :key="entry.path"
            class="cursor-default border-b border-[var(--border-light)] transition-colors hover:bg-[var(--bg-sidebar-hover)]"
            :class="
              selected?.path === entry.path && 'bg-[var(--color-primary-bg)]'
            "
            @click="selected = entry"
            @dblclick="openEntry(entry)"
          >
            <td class="px-3 py-2">
              <div class="flex min-w-0 items-center gap-2">
                <Folder
                  v-if="entry.is_dir"
                  :size="15"
                  class="shrink-0 text-[var(--color-warning)]"
                />
                <FileIcon
                  v-else
                  :size="15"
                  class="shrink-0 text-[var(--text-tertiary)]"
                />
                <span class="truncate" :title="entry.name">{{
                  entry.name
                }}</span>
              </div>
            </td>
            <td class="px-2 py-2 font-mono text-[var(--text-secondary)]">
              {{ entry.is_dir ? "--" : formatSize(entry.size) }}
            </td>
            <td class="truncate px-2 py-2 text-[var(--text-tertiary)]">
              {{ formatTime(entry.modified_at) }}
            </td>
          </tr>
        </tbody>
      </table>
      <div
        v-if="!loading && !entries.length"
        class="p-8 text-center text-xs text-[var(--text-tertiary)]"
      >
        此目录为空，可拖放文件到这里上传。
      </div>
    </div>

    <footer
      class="flex min-h-10 shrink-0 items-center border-t border-[var(--border-default)] px-3 py-2"
    >
      <p class="min-w-0 truncate text-[11px] text-[var(--text-secondary)]">
        {{
          message ||
          (selected
            ? `已选择：${selected.name}`
            : `${entries.length} 项 · 双击目录打开，双击文件下载`)
        }}
      </p>
    </footer>

    <Modal
      :open="!!action"
      :title="
        action === 'create'
          ? '新建远端文件'
          : action === 'mkdir'
            ? '新建远端目录'
            : '重命名远端文件或目录'
      "
      size="sm"
      :closable="!actionSaving"
      @close="!actionSaving && (action = null)"
    >
      <label>
        <span class="aw-modal-label text-xs">名称</span>
        <input
          v-model="actionName"
          class="aw-input mt-1 w-full"
          maxlength="255"
          autocomplete="off"
          :disabled="actionSaving"
          @keyup.enter="saveAction"
        />
      </label>
      <template #footer>
        <Button :disabled="actionSaving" @click="action = null">取消</Button>
        <Button variant="primary" :loading="actionSaving" @click="saveAction"
          >保存</Button
        >
      </template>
    </Modal>

    <ConfirmDialog
      :open="!!deleting"
      title="删除远端文件"
      :message="`确认删除 ${deleting?.name || ''}？${deleting?.is_dir ? '目录及其全部内容都会被永久删除。' : '此操作无法撤销。'}`"
      confirm-text="删除"
      danger
      @confirm="removeEntry"
      @cancel="deleting = null"
    />
    <ConfirmDialog
      :open="pendingOverwrite.length > 0"
      title="覆盖远端文件"
      :message="`所选文件中有 ${pendingOverwrite.filter((file) => entries.some((entry) => entry.name === file.name)).length} 个名称已存在。确认覆盖？上传失败时会保留原文件。`"
      confirm-text="确认覆盖"
      danger
      @confirm="confirmOverwrite"
      @cancel="pendingOverwrite = []"
    />
  </section>
</template>
