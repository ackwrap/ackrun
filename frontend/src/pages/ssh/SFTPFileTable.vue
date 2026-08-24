<script setup lang="ts">
import { File as FileIcon, Folder } from "lucide-vue-next";
import type { SSHSFTPEntry } from "@/services/sshTypes";

defineProps<{
  entries: SSHSFTPEntry[];
  selectedPaths: Set<string>;
  loading: boolean;
  busy: boolean;
}>();
defineEmits<{
  toggleAll: [];
  toggle: [entry: SSHSFTPEntry];
  select: [entry: SSHSFTPEntry, event: MouseEvent];
  open: [entry: SSHSFTPEntry];
  context: [entry: SSHSFTPEntry, event: MouseEvent];
}>();

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
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <table class="w-full table-fixed text-left text-xs">
      <thead
        class="sticky top-0 z-10 bg-[var(--bg-elevated)] text-[var(--text-tertiary)]"
      >
        <tr class="border-b border-[var(--border-default)]">
          <th class="w-8 px-2 py-2">
            <input
              type="checkbox"
              class="h-3.5 w-3.5 accent-[var(--color-primary)]"
              :checked="
                entries.length > 0 && selectedPaths.size === entries.length
              "
              :disabled="busy"
              aria-label="选择当前目录全部项目"
              @change="$emit('toggleAll')"
            />
          </th>
          <th class="w-[43%] px-2 py-2 font-medium">名称</th>
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
            selectedPaths.has(entry.path) && 'bg-[var(--color-primary-bg)]'
          "
          @click="$emit('select', entry, $event)"
          @dblclick="$emit('open', entry)"
          @contextmenu.prevent="$emit('context', entry, $event)"
        >
          <td class="px-2 py-2" @click.stop>
            <input
              type="checkbox"
              class="h-3.5 w-3.5 accent-[var(--color-primary)]"
              :checked="selectedPaths.has(entry.path)"
              :disabled="busy"
              :aria-label="`选择 ${entry.name}`"
              @change="$emit('toggle', entry)"
            />
          </td>
          <td class="px-2 py-2">
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
              <span class="truncate" :title="entry.name">{{ entry.name }}</span>
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
</template>
