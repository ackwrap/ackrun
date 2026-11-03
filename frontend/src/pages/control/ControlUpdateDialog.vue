<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Download } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import ControlInstaller from "./ControlInstaller.vue";
import AppUpdatePanel from "../settings/AppUpdatePanel.vue";

defineProps<{
  currentVersion?: string;
  isRunning: boolean;
  disabled: boolean;
}>();
const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
  busy: [value: boolean];
  installed: [];
}>();
const open = ref(false);
const coreBusy = ref(false);
const appBusy = ref(false);
const updating = computed(() => coreBusy.value || appBusy.value);
watch(updating, (value) => emit("busy", value));
</script>

<template>
  <Button :disabled="disabled || updating" @click="open = true">
    <Download :size="14" />检查更新
  </Button>
  <Modal
    :open="open"
    title="检查更新"
    size="lg"
    :closable="!updating"
    @close="open = false"
  >
    <div class="space-y-4">
      <ControlInstaller
        :current-version="currentVersion"
        :is-running="isRunning"
        :disabled="disabled || appBusy"
        @notify="(message, type) => emit('notify', message, type)"
        @busy="coreBusy = $event"
        @installed="emit('installed')"
      />
      <AppUpdatePanel
        compact
        :disabled="disabled || coreBusy"
        @notify="(message, type) => emit('notify', message, type)"
        @busy="appBusy = $event"
      />
    </div>
    <template #footer>
      <Button :disabled="updating" @click="open = false">关闭</Button>
    </template>
  </Modal>
</template>
