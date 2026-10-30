<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Copy, RefreshCw } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type { NodeItem } from "@/services/types";
import { writeClipboardText } from "@/utils/clipboard";

const props = defineProps<{ nodes: Pick<NodeItem, "id" | "name">[] }>();
const emit = defineEmits<{ close: [] }>();
const loading = ref(false);
const copying = ref(false);
const copied = ref(false);
const copyError = ref("");
const revision = ref(0);
const links = ref<string[]>([]);
const failures = ref<{ id: number; name: string; message: string }[]>([]);
const content = computed(() => links.value.join("\n"));
const completed = computed(() => links.value.length + failures.value.length);

watch(
  [() => props.nodes, revision],
  async ([nodes], _, onCleanup) => {
    const controller = new AbortController();
    onCleanup(() => controller.abort());
    loading.value = true;
    copied.value = false;
    copyError.value = "";
    links.value = [];
    failures.value = [];

    // Keep concurrent requests bounded on routers, preserving selection order.
    for (let offset = 0; offset < nodes.length; offset += 5) {
      const batch = nodes.slice(offset, offset + 5);
      const results = await Promise.allSettled(
        batch.map(async (node) => {
          const result = await api.getNodeShare(node.id, controller.signal);
          if (!result.uri?.trim()) throw new Error("未生成分享链接");
          return result.uri.trim();
        }),
      );
      if (controller.signal.aborted) return;
      results.forEach((result, index) => {
        if (result.status === "fulfilled") {
          links.value.push(result.value);
        } else {
          const node = batch[index];
          failures.value.push({
            id: node.id,
            name: node.name,
            message:
              result.reason instanceof Error
                ? result.reason.message
                : "分享链接生成失败",
          });
        }
      });
    }
    loading.value = false;
  },
  { immediate: true },
);

async function copyLinks() {
  if (loading.value || copying.value || !content.value) return;
  copying.value = true;
  copied.value = false;
  copyError.value = "";
  try {
    await writeClipboardText(content.value);
    copied.value = true;
  } catch {
    copyError.value = "自动复制失败，请在下方文本框中手动复制分享链接";
  } finally {
    copying.value = false;
  }
}
</script>

<template>
  <Modal
    :open="true"
    :title="`批量分享节点 (${nodes.length})`"
    size="lg"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p
        class="rounded-md bg-[var(--color-warning-bg)] p-3 text-sm text-[var(--text-secondary)]"
      >
        分享链接包含节点连接密码，请仅发送给可信任的人。每行一个链接，可直接用于批量导入。
      </p>
      <p role="status" aria-live="polite" class="text-sm">
        <template v-if="loading">
          正在生成分享链接：{{ completed }} / {{ nodes.length }}
        </template>
        <template v-else>
          生成完成：成功 {{ links.length }}，失败 {{ failures.length }}
        </template>
      </p>
      <div
        v-if="failures.length"
        role="alert"
        class="rounded-md border border-[var(--border-default)] p-3 text-sm text-[var(--color-error)]"
      >
        <p>以下节点分享失败，可重试或仅复制成功的链接：</p>
        <ul class="mt-2 max-h-40 space-y-1 overflow-y-auto">
          <li v-for="failure in failures" :key="failure.id" class="break-words">
            {{ failure.name }}：{{ failure.message }}
          </li>
        </ul>
      </div>
      <label v-if="links.length" class="block">
        <span class="aw-modal-label text-xs"
          >分享链接（{{ links.length }}）</span
        >
        <textarea
          :value="content"
          aria-label="节点分享链接"
          class="aw-input mt-1 min-h-48 w-full resize-y font-mono text-xs"
          readonly
          autocomplete="off"
          autocapitalize="off"
          :spellcheck="false"
        />
      </label>
      <p
        v-if="copyError"
        role="alert"
        class="text-sm text-[var(--color-error)]"
      >
        {{ copyError }}
      </p>
    </div>
    <template #footer>
      <button class="aw-action-button aw-action-neutral" @click="emit('close')">
        关闭
      </button>
      <button
        class="aw-action-button aw-action-neutral"
        :disabled="loading || copying"
        @click="revision++"
      >
        <RefreshCw :size="14" />重新生成
      </button>
      <button
        class="aw-action-button aw-action-success"
        :disabled="loading || copying || !links.length"
        @click="copyLinks"
      >
        <Copy :size="14" />{{
          copied
            ? "已复制"
            : failures.length
              ? `复制成功链接 (${links.length})`
              : "复制全部链接"
        }}
      </button>
    </template>
  </Modal>
</template>
