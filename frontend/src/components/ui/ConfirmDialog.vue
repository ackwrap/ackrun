<script setup lang="ts">
withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
    danger?: boolean;
  }>(),
  { confirmText: "确认", cancelText: "取消" },
);
defineEmits<{ confirm: []; cancel: [] }>();
</script>
<template>
  <div v-if="open" class="aw-modal-backdrop z-[80]">
    <div class="aw-modal-panel max-w-md">
      <div class="border-b border-[var(--border-default)] px-5 py-4">
        <h3 class="text-base font-semibold">{{ title }}</h3>
        <p class="mt-2 text-sm text-[var(--text-secondary)]">{{ message }}</p>
      </div>
      <div class="flex justify-end gap-2 px-5 py-4">
        <button class="aw-confirm-cancel" @click="$emit('cancel')">
          {{ cancelText }}</button
        ><button
          :class="danger ? 'aw-confirm-danger' : 'aw-confirm-primary'"
          @click="$emit('confirm')"
        >
          {{ confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>
