<script setup lang="ts">
import { ref } from "vue";
import Modal from "@/components/ui/Modal.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import type { SSHSessionCreateResponse } from "@/services/sshTypes";
import SSHTerminalPane from "./SSHTerminalPane.vue";

defineProps<{
  open: boolean;
  hostName: string;
  session: SSHSessionCreateResponse;
}>();
defineEmits<{ close: [] }>();

type TerminalStatus = "connecting" | "online" | "offline" | "error";
const status = ref<TerminalStatus>("connecting");
const statusMessage = ref("正在建立 SSH 会话...");

function updateStatus(state: TerminalStatus, message: string) {
  status.value = state;
  statusMessage.value = message;
}
</script>

<template>
  <Modal
    :open="open"
    :title="`临时 Shell · ${hostName}`"
    size="xl"
    @close="$emit('close')"
  >
    <template #actions>
      <StatusBadge
        :status="
          status === 'online'
            ? 'online'
            : status === 'error'
              ? 'error'
              : status === 'offline'
                ? 'offline'
                : 'pending'
        "
        :label="statusMessage"
        size="sm"
      />
    </template>
    <div
      class="h-[min(68vh,720px)] min-h-[360px] overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--ssh-terminal-bg)]"
    >
      <SSHTerminalPane :session="session" @status="updateStatus" />
    </div>
  </Modal>
</template>
