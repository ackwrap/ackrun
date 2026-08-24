<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { sshApi } from "@/services/sshApi";
import type {
  SSHSessionCreateResponse,
  SSHSFTPEntry,
  SSHSFTPTextFile,
} from "@/services/sshTypes";
import SFTPContextMenu from "./SFTPContextMenu.vue";
import SFTPFileTable from "./SFTPFileTable.vue";
import SFTPTextEditorModal from "./SFTPTextEditorModal.vue";
import SFTPToolbar from "./SFTPToolbar.vue";

const props = defineProps<{ session: SSHSessionCreateResponse }>();

const path = ref(".");
const pathInput = ref(".");
const home = ref(".");
const parent = ref(".");
const entries = ref<SSHSFTPEntry[]>([]);
const selected = ref<SSHSFTPEntry | null>(null);
const selectedPaths = ref<Set<string>>(new Set());
const selectionAnchor = ref("");
const loading = ref(true);
const error = ref("");
const message = ref("");
const uploading = ref(false);
const uploadProgress = ref(0);
const uploadName = ref("");
const action = ref<"create" | "mkdir" | "rename" | null>(null);
const actionName = ref("");
const actionSaving = ref(false);
const deleting = ref<SSHSFTPEntry[]>([]);
const pendingOverwrite = ref<File[]>([]);
const working = ref(false);
const clipboard = ref<{
  mode: "copy" | "move";
  sourceDirectory: string;
  entries: SSHSFTPEntry[];
} | null>(null);
const contextMenu = ref({ open: false, x: 0, y: 0 });
const editorFile = ref<SSHSFTPTextFile | null>(null);
const editorSaving = ref(false);
const editorError = ref("");

const canGoUp = computed(
  () => path.value !== "/" && parent.value !== path.value,
);
const selectedEntries = computed(() =>
  entries.value.filter((entry) => selectedPaths.value.has(entry.path)),
);
const singleSelected = computed(() =>
  selectedEntries.value.length === 1 ? selectedEntries.value[0] : null,
);
const selectedFiles = computed(() =>
  selectedEntries.value.filter((entry) => !entry.is_dir),
);
const busy = computed(
  () =>
    uploading.value ||
    working.value ||
    actionSaving.value ||
    editorSaving.value,
);
const deleteSummary = computed(() => {
  if (deleting.value.length === 1) {
    const entry = deleting.value[0];
    return `确认永久删除${entry.is_dir ? "目录" : "文件"}「${entry.name}」？${entry.is_dir ? "目录及其全部内容都会被删除。" : "此操作无法撤销。"}`;
  }
  const names = deleting.value
    .slice(0, 3)
    .map((entry) => `「${entry.name}」`)
    .join("、");
  const total = deleting.value.length;
  const remaining = total > 3 ? `，其余 ${total - 3} 项` : "";
  const directoryWarning = deleting.value.some((entry) => entry.is_dir)
    ? "所选目录及其全部内容都会被删除。"
    : "";
  return `确认永久删除 ${names}${remaining}（共 ${total} 项）？${directoryWarning}此操作无法撤销。`;
});

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
    clearSelection();
  } catch (cause: any) {
    error.value = cause?.message || "SFTP 目录读取失败";
  } finally {
    loading.value = false;
  }
}

function clearSelection() {
  selectedPaths.value = new Set();
  selected.value = null;
  selectionAnchor.value = "";
  contextMenu.value.open = false;
}

function restoreSelection(paths: string[]) {
  const available = new Set(
    entries.value
      .filter((entry) => paths.includes(entry.path))
      .map((entry) => entry.path),
  );
  selectedPaths.value = available;
  selected.value =
    entries.value.find((entry) => available.has(entry.path)) || null;
}

function syncPrimarySelection(preferred?: SSHSFTPEntry) {
  if (preferred && selectedPaths.value.has(preferred.path)) {
    selected.value = preferred;
    return;
  }
  selected.value = selectedEntries.value[0] || null;
}

function selectEntry(entry: SSHSFTPEntry, event: MouseEvent) {
  if (busy.value) return;
  const paths = new Set(selectedPaths.value);
  const toggle = event.ctrlKey || event.metaKey;
  if (event.shiftKey && selectionAnchor.value) {
    const anchorIndex = entries.value.findIndex(
      (item) => item.path === selectionAnchor.value,
    );
    const currentIndex = entries.value.findIndex(
      (item) => item.path === entry.path,
    );
    if (anchorIndex >= 0 && currentIndex >= 0) {
      if (!toggle) paths.clear();
      const start = Math.min(anchorIndex, currentIndex);
      const end = Math.max(anchorIndex, currentIndex);
      for (const item of entries.value.slice(start, end + 1)) {
        paths.add(item.path);
      }
    }
  } else if (toggle) {
    if (paths.has(entry.path)) paths.delete(entry.path);
    else paths.add(entry.path);
    selectionAnchor.value = entry.path;
  } else {
    paths.clear();
    paths.add(entry.path);
    selectionAnchor.value = entry.path;
  }
  selectedPaths.value = paths;
  syncPrimarySelection(entry);
}

function toggleEntry(entry: SSHSFTPEntry) {
  if (busy.value) return;
  const paths = new Set(selectedPaths.value);
  if (paths.has(entry.path)) paths.delete(entry.path);
  else paths.add(entry.path);
  selectedPaths.value = paths;
  selectionAnchor.value = entry.path;
  syncPrimarySelection(entry);
}

function toggleAll() {
  if (busy.value) return;
  if (selectedPaths.value.size === entries.value.length) clearSelection();
  else {
    selectedPaths.value = new Set(entries.value.map((entry) => entry.path));
    selected.value = entries.value[0] || null;
  }
}

function openContextMenu(entry: SSHSFTPEntry, event: MouseEvent) {
  if (busy.value) return;
  if (!selectedPaths.value.has(entry.path)) {
    selectedPaths.value = new Set([entry.path]);
  }
  selected.value = entry;
  selectionAnchor.value = entry.path;
  contextMenu.value = {
    open: true,
    x: Math.max(8, Math.min(event.clientX, window.innerWidth - 205)),
    y: Math.max(8, Math.min(event.clientY, window.innerHeight - 330)),
  };
}

function closeContextMenu() {
  contextMenu.value.open = false;
}

function openSelectedEntry() {
  const entry = singleSelected.value;
  closeContextMenu();
  if (entry) openEntry(entry);
}

function openEntry(entry: SSHSFTPEntry) {
  if (busy.value) return;
  if (entry.is_dir) void load(entry.path);
  else void downloadEntry(entry);
}

function openAction(next: "create" | "mkdir" | "rename") {
  closeContextMenu();
  if (busy.value) return;
  action.value = next;
  actionName.value = next === "rename" ? singleSelected.value?.name || "" : "";
}

async function saveAction() {
  if (busy.value) return;
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
    } else if (action.value === "rename" && singleSelected.value) {
      await sshApi.renameSFTP(
        props.session.session_id,
        props.session.sftp_token,
        singleSelected.value.path,
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
  if (busy.value) return;
  const items = [...deleting.value];
  deleting.value = [];
  if (!items.length) return;
  working.value = true;
  let completed = 0;
  const failures: Array<{ entry: SSHSFTPEntry; message: string }> = [];
  for (const entry of items) {
    try {
      await sshApi.deleteSFTP(
        props.session.session_id,
        props.session.sftp_token,
        entry.path,
        entry.is_dir,
      );
      completed++;
    } catch (cause: any) {
      failures.push({ entry, message: cause?.message || "删除失败" });
    }
  }
  await load(path.value);
  if (failures.length) {
    restoreSelection(failures.map((failure) => failure.entry.path));
    const names = failures
      .slice(0, 3)
      .map((failure) => failure.entry.name)
      .join("、");
    message.value = `已删除 ${completed}/${items.length} 项；失败：${names}${failures.length > 3 ? ` 等 ${failures.length} 项` : ""}`;
  } else {
    message.value = `${completed} 项已删除`;
  }
  working.value = false;
}

async function downloadEntry(entry: SSHSFTPEntry) {
  if (busy.value || entry.is_dir) return;
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

async function downloadSelected() {
  closeContextMenu();
  if (busy.value || !selectedFiles.value.length) return;
  const items = [...selectedFiles.value];
  working.value = true;
  let completed = 0;
  const failures: Array<{ entry: SSHSFTPEntry; message: string }> = [];
  for (const entry of items) {
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
      completed++;
    } catch (cause: any) {
      failures.push({ entry, message: cause?.message || "下载失败" });
    }
  }
  if (failures.length) {
    const names = failures
      .slice(0, 3)
      .map((failure) => failure.entry.name)
      .join("、");
    message.value = `已下载 ${completed}/${items.length} 个文件；失败：${names}${failures.length > 3 ? ` 等 ${failures.length} 个` : ""}`;
  } else {
    message.value = `${completed} 个文件下载已开始`;
  }
  working.value = false;
}

function putSelectionOnClipboard(mode: "copy" | "move") {
  closeContextMenu();
  if (busy.value || !selectedEntries.value.length) return;
  clipboard.value = {
    mode,
    sourceDirectory: path.value,
    entries: selectedEntries.value.map((entry) => ({ ...entry })),
  };
  message.value = `已${mode === "copy" ? "复制" : "剪切"} ${selectedEntries.value.length} 项，进入目标目录后粘贴`;
}

async function pasteClipboard() {
  closeContextMenu();
  const current = clipboard.value;
  if (busy.value || !current?.entries.length) return;
  if (current.sourceDirectory === path.value) {
    message.value = "目标目录与来源目录相同";
    return;
  }
  working.value = true;
  let completed = 0;
  try {
    for (const entry of current.entries) {
      const targetPath = remoteJoin(path.value, entry.name);
      if (entry.is_dir && path.value.startsWith(`${entry.path}/`)) {
        throw new Error("不能粘贴到来源目录内部");
      }
      if (current.mode === "copy") {
        await sshApi.copySFTP(
          props.session.session_id,
          props.session.sftp_token,
          entry.path,
          targetPath,
        );
      } else {
        await sshApi.renameSFTP(
          props.session.session_id,
          props.session.sftp_token,
          entry.path,
          targetPath,
        );
      }
      completed++;
    }
    message.value = `${completed} 项已${current.mode === "copy" ? "复制" : "移动"}`;
    if (current.mode === "move") clipboard.value = null;
  } catch (cause: any) {
    if (completed > 0 && completed < current.entries.length) {
      clipboard.value = {
        ...current,
        entries: current.entries.slice(completed),
      };
    }
    message.value = `已完成 ${completed}/${current.entries.length} 项：${cause?.message || "粘贴失败"}`;
  } finally {
    working.value = false;
    await load(path.value);
  }
}

function requestDeleteSelection() {
  closeContextMenu();
  if (!busy.value && selectedEntries.value.length) {
    deleting.value = [...selectedEntries.value];
  }
}

async function openEditor(entry = singleSelected.value) {
  closeContextMenu();
  if (busy.value || !entry || entry.is_dir) return;
  working.value = true;
  editorError.value = "";
  message.value = `正在读取 ${entry.name}`;
  try {
    editorFile.value = await sshApi.readSFTPText(
      props.session.session_id,
      props.session.sftp_token,
      entry.path,
    );
    message.value = `${entry.name} 已在编辑器中打开`;
  } catch (cause: any) {
    message.value = cause?.message || "文本文件读取失败";
  } finally {
    working.value = false;
  }
}

async function reloadEditor() {
  const current = editorFile.value;
  if (!current || editorSaving.value) return;
  editorSaving.value = true;
  editorError.value = "";
  try {
    editorFile.value = await sshApi.readSFTPText(
      props.session.session_id,
      props.session.sftp_token,
      current.path,
    );
  } catch (cause: any) {
    editorError.value = cause?.message || "重新加载失败";
  } finally {
    editorSaving.value = false;
  }
}

async function saveEditor(content: string) {
  const current = editorFile.value;
  if (!current || editorSaving.value) return;
  editorSaving.value = true;
  editorError.value = "";
  try {
    editorFile.value = await sshApi.writeSFTPText(
      props.session.session_id,
      props.session.sftp_token,
      current.path,
      content,
      current.sha256,
    );
    message.value = `${current.name} 已保存`;
    await load(path.value);
  } catch (cause: any) {
    editorError.value = cause?.message || "保存失败";
  } finally {
    editorSaving.value = false;
  }
}

function uploadFiles(files: File[]) {
  if (!files.length || busy.value) return;
  const existingNames = new Set(entries.value.map((entry) => entry.name));
  if (files.some((file) => existingNames.has(file.name))) {
    pendingOverwrite.value = files;
    return;
  }
  void performUploads(files, false);
}

async function performUploads(files: File[], overwrite: boolean) {
  if (busy.value) return;
  const targetPath = path.value;
  let completed = 0;
  let failure = "";
  action.value = null;
  deleting.value = [];
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
    <SFTPToolbar
      :path-input="pathInput"
      :loading="loading"
      :busy="busy"
      :can-go-up="canGoUp"
      :entry-count="entries.length"
      :selected-count="selectedEntries.length"
      :selected-file-count="selectedFiles.length"
      :can-edit="!!singleSelected && !singleSelected.is_dir"
      :can-rename="!!singleSelected"
      :can-paste="!!clipboard"
      @update:path-input="pathInput = $event"
      @go="load(pathInput)"
      @home="load(home)"
      @parent="load(parent)"
      @refresh="load(path)"
      @upload="uploadFiles"
      @download="downloadSelected"
      @create-file="openAction('create')"
      @create-directory="openAction('mkdir')"
      @edit="openEditor()"
      @copy="putSelectionOnClipboard('copy')"
      @cut="putSelectionOnClipboard('move')"
      @paste="pasteClipboard"
      @rename="openAction('rename')"
      @delete="requestDeleteSelection"
    />

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

    <SFTPFileTable
      v-else
      :entries="entries"
      :selected-paths="selectedPaths"
      :loading="loading"
      :busy="busy"
      @toggle-all="toggleAll"
      @toggle="toggleEntry"
      @select="selectEntry"
      @open="openEntry"
      @context="openContextMenu"
    />

    <footer
      class="flex min-h-10 shrink-0 items-center border-t border-[var(--border-default)] px-3 py-2"
    >
      <p class="min-w-0 truncate text-[11px] text-[var(--text-secondary)]">
        {{
          message ||
          (selectedEntries.length
            ? `已选择 ${selectedEntries.length} 项${clipboard ? ` · 剪贴板 ${clipboard.entries.length} 项待粘贴` : ""}`
            : `${entries.length} 项 · Ctrl/Command 或 Shift 多选 · 右键查看更多操作`)
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
      :open="deleting.length > 0"
      title="删除远端文件或目录"
      :message="deleteSummary"
      confirm-text="删除"
      danger
      @confirm="removeEntry"
      @cancel="deleting = []"
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
    <SFTPContextMenu
      :open="contextMenu.open"
      :x="contextMenu.x"
      :y="contextMenu.y"
      :is-directory="!!singleSelected?.is_dir"
      :selection-count="selectedEntries.length"
      :download-count="selectedFiles.length"
      :can-paste="!!clipboard"
      :busy="busy"
      @close="closeContextMenu"
      @open-entry="openSelectedEntry"
      @download="downloadSelected"
      @edit="openEditor()"
      @copy="putSelectionOnClipboard('copy')"
      @cut="putSelectionOnClipboard('move')"
      @paste="pasteClipboard"
      @rename="openAction('rename')"
      @delete="requestDeleteSelection"
    />
    <SFTPTextEditorModal
      :file="editorFile"
      :saving="editorSaving"
      :error="editorError"
      @close="editorFile = null"
      @reload="reloadEditor"
      @save="saveEditor"
    />
  </section>
</template>
