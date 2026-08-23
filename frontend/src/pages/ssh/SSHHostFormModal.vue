<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { Plus } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import type { NodeExposure } from "@/services/advancedTypes";
import type {
  SSHConnectionMode,
  SSHCredential,
  SSHHost,
  SSHHostRequest,
} from "@/services/sshTypes";

interface HostForm {
  name: string;
  group_name: string;
  host: string;
  port: number;
  username: string;
  credential_id: number;
  connection_mode: SSHConnectionMode;
  node_exposure_id: number;
  terminal_type: string;
  enabled: boolean;
  tags: string;
  notes: string;
}

const props = defineProps<{
  open: boolean;
  editing: SSHHost | null;
  credentials: SSHCredential[];
  preferredCredentialId: number;
  exposures: NodeExposure[];
  saving: boolean;
}>();
const emit = defineEmits<{
  close: [];
  createCredential: [];
  save: [SSHHostRequest];
  invalid: [string];
}>();
const form = reactive<HostForm>(emptyForm());
const enabledExposures = computed(() =>
  props.exposures.filter(
    (item) => item.enabled && item.node_exists && item.node_enabled,
  ),
);

function emptyForm(): HostForm {
  return {
    name: "",
    group_name: "",
    host: "",
    port: 22,
    username: "",
    credential_id: props.credentials[0]?.id || 0,
    connection_mode: "direct",
    node_exposure_id: 0,
    terminal_type: "xterm-256color",
    enabled: true,
    tags: "",
    notes: "",
  };
}

function reset() {
  const host = props.editing;
  if (!host) {
    Object.assign(form, emptyForm());
    return;
  }
  Object.assign(form, {
    name: host.name,
    group_name: host.group_name,
    host: host.host,
    port: host.port,
    username: host.username,
    credential_id: host.credential_id,
    connection_mode: host.connection_mode,
    node_exposure_id: host.node_exposure_id || 0,
    terminal_type: host.terminal_type,
    enabled: host.enabled,
    tags: (host.tags || []).join(", "),
    notes: host.notes,
  });
}

function save() {
  if (
    !form.name.trim() ||
    !form.host.trim() ||
    !form.username.trim() ||
    !form.credential_id
  ) {
    emit("invalid", "请填写主机名称、地址、用户名并选择凭据");
    return;
  }
  if (form.connection_mode === "node_exposure" && !form.node_exposure_id) {
    emit("invalid", "请选择节点入口");
    return;
  }
  emit("save", {
    name: form.name.trim(),
    group_name: form.group_name.trim(),
    host: form.host.trim(),
    port: Number(form.port),
    username: form.username.trim(),
    credential_id: Number(form.credential_id),
    connection_mode: form.connection_mode,
    node_exposure_id:
      form.connection_mode === "node_exposure"
        ? Number(form.node_exposure_id)
        : undefined,
    terminal_type: form.terminal_type.trim(),
    enabled: form.enabled,
    tags: form.tags
      .split(/[,，]/)
      .map((item) => item.trim())
      .filter(Boolean),
    notes: form.notes.trim(),
  });
}

watch(
  () => props.open,
  (open) => {
    if (open) reset();
  },
  { immediate: true },
);
watch(
  () => props.preferredCredentialId,
  (credentialID) => {
    if (props.open && credentialID) form.credential_id = credentialID;
  },
);
</script>

<template>
  <Modal
    :open="open"
    :title="editing ? '编辑 SSH 主机' : '新增 SSH 主机'"
    size="lg"
    :closable="!saving"
    @close="!saving && $emit('close')"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <label>
        <span class="aw-modal-label text-xs">名称</span>
        <input
          v-model="form.name"
          class="aw-input mt-1 w-full"
          maxlength="128"
          placeholder="生产服务器"
        />
      </label>
      <label>
        <span class="aw-modal-label text-xs">分组</span>
        <input
          v-model="form.group_name"
          class="aw-input mt-1 w-full"
          maxlength="128"
          placeholder="可选"
        />
      </label>
      <label>
        <span class="aw-modal-label text-xs">主机地址</span>
        <input
          v-model="form.host"
          class="aw-input mt-1 w-full font-mono"
          autocomplete="off"
          placeholder="域名或 IP"
        />
      </label>
      <label>
        <span class="aw-modal-label text-xs">端口</span>
        <input
          v-model.number="form.port"
          class="aw-input mt-1 w-full"
          type="number"
          min="1"
          max="65535"
        />
      </label>
      <label>
        <span class="aw-modal-label text-xs">用户名</span>
        <input
          v-model="form.username"
          class="aw-input mt-1 w-full"
          autocomplete="off"
        />
      </label>
      <div>
        <div class="flex items-center justify-between gap-3">
          <span class="aw-modal-label text-xs">凭据</span>
          <button
            type="button"
            class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:text-[var(--color-primary-hover)]"
            @click="$emit('createCredential')"
          >
            <Plus :size="13" />新建凭据
          </button>
        </div>
        <select
          v-model.number="form.credential_id"
          class="aw-input mt-1 w-full"
          aria-label="凭据"
        >
          <option :value="0" disabled>
            {{ credentials.length ? "请选择" : "暂无凭据，请先新建" }}
          </option>
          <option v-for="item in credentials" :key="item.id" :value="item.id">
            {{ item.name }} ·
            {{ item.auth_type === "password" ? "密码" : "私钥" }}
          </option>
        </select>
      </div>
      <label>
        <span class="aw-modal-label text-xs">连接路径</span>
        <select v-model="form.connection_mode" class="aw-input mt-1 w-full">
          <option value="direct">直连</option>
          <option value="node_exposure">通过节点入口</option>
        </select>
      </label>
      <label v-if="form.connection_mode === 'node_exposure'">
        <span class="aw-modal-label text-xs">节点入口</span>
        <select
          v-model.number="form.node_exposure_id"
          class="aw-input mt-1 w-full"
        >
          <option :value="0" disabled>请选择</option>
          <option
            v-for="item in enabledExposures"
            :key="item.id"
            :value="item.id"
          >
            {{ item.name }} · {{ item.inbound_type.toUpperCase() }} ·
            {{ item.node_name }}
          </option>
        </select>
      </label>
      <label>
        <span class="aw-modal-label text-xs">终端类型</span>
        <input
          v-model="form.terminal_type"
          class="aw-input mt-1 w-full font-mono"
        />
      </label>
      <label>
        <span class="aw-modal-label text-xs">标签</span>
        <input
          v-model="form.tags"
          class="aw-input mt-1 w-full"
          placeholder="逗号分隔"
        />
      </label>
      <label class="sm:col-span-2">
        <span class="aw-modal-label text-xs">备注</span>
        <textarea
          v-model="form.notes"
          class="aw-input mt-1 min-h-20 w-full resize-y"
        />
      </label>
      <label class="flex items-center gap-2 text-sm sm:col-span-2">
        <input v-model="form.enabled" type="checkbox" />允许连接
      </label>
    </div>
    <p
      v-if="form.connection_mode === 'node_exposure'"
      class="mt-4 rounded-[var(--radius-lg)] bg-[var(--color-warning-bg)] p-3 text-xs text-[var(--text-secondary)]"
    >
      节点入口或核心不可用时连接会明确失败，不会静默回退直连。
    </p>
    <template #footer>
      <Button :disabled="saving" @click="$emit('close')">取消</Button>
      <Button variant="primary" :loading="saving" @click="save">保存</Button>
    </template>
  </Modal>
</template>
