<script setup lang="ts">
import { Upload } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";

defineProps<{
  open: boolean;
  code: string;
  importing: boolean;
}>();

const emit = defineEmits<{
  close: [];
  import: [];
  "update:code": [string];
}>();

function updateCode(event: Event) {
  emit("update:code", (event.target as HTMLTextAreaElement).value);
}
</script>

<template>
  <Modal :open="open" title="导入规则分享码" size="lg" @close="$emit('close')">
    <p class="mb-3 text-xs leading-5 text-[var(--text-secondary)]">
      导入会按名称合并规则：同名规则更新，缺失规则新增，其他现有规则保留。分享码不包含节点、策略组或规则订阅。
    </p>
    <textarea
      :value="code"
      class="aw-input min-h-56 w-full resize-y font-mono text-xs"
      placeholder="粘贴 Base64 规则分享码"
      spellcheck="false"
      :disabled="importing"
      @input="updateCode"
    />
    <template #footer>
      <button
        class="aw-action-button aw-action-neutral"
        :disabled="importing"
        @click="$emit('close')"
      >
        取消
      </button>
      <button
        class="aw-action-button aw-action-success"
        :disabled="importing || !code.trim()"
        @click="$emit('import')"
      >
        <Upload :size="13" />{{ importing ? "导入中..." : "导入并合并" }}
      </button>
    </template>
  </Modal>
</template>
