<script setup lang="ts">
import { ref } from "vue";
import Modal from "@/components/ui/Modal.vue";
import type { SSHDeviceDetails, SSHHost } from "@/services/sshTypes";
import SSHSingboxDeployPanel from "./SSHSingboxDeployPanel.vue";

defineProps<{
  host: SSHHost | null;
  details: SSHDeviceDetails | null;
  loading: boolean;
  error: string;
}>();
const emit = defineEmits<{ close: []; retry: [] }>();
const deploymentBusy = ref(false);
</script>

<template>
  <Modal
    :open="Boolean(host)"
    :title="`sing-box 服务端部署 · ${host?.name || ''}`"
    :width="1480"
    :closable="!deploymentBusy"
    :close-on-backdrop="false"
    :close-on-escape="false"
    @close="emit('close')"
  >
    <SSHSingboxDeployPanel
      v-if="host"
      :host="host"
      :details="details"
      :details-error="error"
      :details-loading="loading"
      @busy="deploymentBusy = $event"
      @refresh="emit('retry')"
    />
  </Modal>
</template>
