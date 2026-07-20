<script setup lang="ts">
import { computed } from "vue";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";

interface RuleForm {
  id?: number;
  enabled: boolean;
  condition_type: string;
  values: string;
  server: string;
  disable_cache: boolean;
  rewrite_ttl: number;
  client_subnet: string;
}

interface ServerOption {
  tag: string;
}

const props = defineProps<{
  form: RuleForm;
  conditions: string[];
  servers: ServerOption[];
  fakeIPEnabled: boolean;
  saving: boolean;
}>();

defineEmits<{ close: []; save: [] }>();

const valuePlaceholder = computed(() => {
  switch (props.form.condition_type) {
    case "geosite":
      return "cn\ngeolocation-!cn";
    case "rule_set":
      return "geosite-cn";
    case "query_type":
      return "A\nAAAA";
    case "clash_mode":
      return "rule";
    default:
      return "baidu.com\nqq.com";
  }
});

const valueHelp = computed(() => {
  if (props.form.condition_type === "geosite")
    return "每行一个 GeoSite 分类名；保存后生成配置时自动转换为 geosite-* rule_set。";
  if (props.form.condition_type === "rule_set")
    return "每行一个 rule_set tag，例如 geosite-cn；该规则集必须已配置。";
  return "每行一个值。";
});
</script>

<template>
  <Modal
    :open="true"
    :title="form.id ? '编辑 DNS 规则' : '新增 DNS 规则'"
    size="lg"
    :closable="!saving"
    @close="$emit('close')"
  >
    <div class="grid gap-4 md:grid-cols-2">
      <label class="text-sm text-[var(--text-secondary)]">
        匹配条件
        <select v-model="form.condition_type" class="aw-input mt-1 w-full">
          <option v-for="condition in conditions" :key="condition" :value="condition">
            {{ condition }}
          </option>
        </select>
      </label>
      <label class="text-sm text-[var(--text-secondary)]">
        DNS Server
        <select v-model="form.server" class="aw-input mt-1 w-full">
          <option value="">请选择 Server</option>
          <option v-for="server in servers" :key="server.tag" :value="server.tag">
            {{ server.tag }}
          </option>
          <option
            v-if="fakeIPEnabled && !servers.some((server) => server.tag === 'fakeip')"
            value="fakeip"
          >
            fakeip
          </option>
        </select>
      </label>
      <label class="text-sm text-[var(--text-secondary)] md:col-span-2">
        匹配值
        <textarea
          v-model="form.values"
          rows="6"
          class="aw-input mt-1 w-full resize-y font-mono"
          :placeholder="valuePlaceholder"
        />
        <span class="mt-1 block text-xs text-[var(--text-tertiary)]">
          {{ valueHelp }}
        </span>
      </label>
      <label class="text-sm text-[var(--text-secondary)]">
        Rewrite TTL
        <input
          v-model.number="form.rewrite_ttl"
          type="number"
          min="0"
          class="aw-input mt-1 w-full"
          placeholder="0 表示不改写"
        />
      </label>
      <label class="text-sm text-[var(--text-secondary)]">
        Client Subnet
        <input
          v-model="form.client_subnet"
          class="aw-input mt-1 w-full"
          placeholder="例如 203.0.113.0/24，可留空"
        />
      </label>
    </div>
    <div
      class="mt-4 flex flex-wrap gap-x-6 gap-y-3 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-surface)] px-4 py-3"
    >
      <label class="inline-flex items-center gap-2 text-sm">
        <input v-model="form.enabled" type="checkbox" />
        启用规则
      </label>
      <label class="inline-flex items-center gap-2 text-sm">
        <input v-model="form.disable_cache" type="checkbox" />
        禁用此规则的 DNS 缓存
      </label>
    </div>
    <template #footer>
      <Button :disabled="saving" @click="$emit('close')">取消</Button>
      <Button variant="primary" :loading="saving" @click="$emit('save')">保存</Button>
    </template>
  </Modal>
</template>
