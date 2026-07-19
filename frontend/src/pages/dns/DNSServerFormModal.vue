<script setup lang="ts">
import Modal from "@/components/ui/Modal.vue";

interface ServerForm {
  id?: number;
  tag?: string;
  enabled?: boolean;
  server_type?: string;
  address?: string;
  address_resolver?: string;
  address_strategy?: string;
  strategy?: string;
  detour?: string;
  client_subnet?: string;
}

defineProps<{
  form: ServerForm;
  types: string[];
  strategies: string[];
  detours: string[];
}>();
defineEmits<{ close: []; save: [] }>();
</script>

<template>
  <Modal
    :open="true"
    :title="form.id ? '编辑 DNS 服务器' : '新增 DNS 服务器'"
    size="lg"
    @close="$emit('close')"
  >
    <div class="grid gap-3 md:grid-cols-3">
      <label>Tag<input v-model="form.tag" /></label>
      <label
        >类型<select v-model="form.server_type">
          <option v-for="item in types" :key="item">{{ item }}</option>
        </select></label
      >
      <label>地址<input v-model="form.address" /></label>
      <label>Address Resolver<input v-model="form.address_resolver" /></label>
      <label
        >Address Strategy<select v-model="form.address_strategy">
          <option value="">留空</option>
          <option v-for="item in strategies" :key="item">{{ item }}</option>
        </select></label
      >
      <label
        >Strategy<select v-model="form.strategy">
          <option value="">留空</option>
          <option v-for="item in strategies" :key="item">{{ item }}</option>
        </select></label
      >
      <label
        >Detour<select v-model="form.detour">
          <option v-for="item in detours" :key="item" :value="item">
            {{ item || "默认出站" }}
          </option>
        </select></label
      >
      <label>Client Subnet<input v-model="form.client_subnet" /></label>
      <label><input v-model="form.enabled" type="checkbox" />启用</label>
    </div>
    <template #footer>
      <button class="aw-action-button aw-action-neutral" @click="$emit('close')">
        取消
      </button>
      <button class="aw-action-button aw-action-success" @click="$emit('save')">
        保存
      </button>
    </template>
  </Modal>
</template>
