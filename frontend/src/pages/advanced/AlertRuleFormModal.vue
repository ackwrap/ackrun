<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import type {
  AlertChannel,
  AlertEventType,
  AlertRule,
  AlertRuleRequest,
} from "@/services/advancedTypes";
import { alertChannelLabel, alertEventOptions } from "./alertUi";

const props = defineProps<{
  open: boolean;
  editing: AlertRule | null;
  channels: AlertChannel[];
  saving: boolean;
}>();
const emit = defineEmits<{ close: []; save: [AlertRuleRequest] }>();

const form = reactive({
  name: "",
  enabled: true,
  eventTypes: [] as AlertEventType[],
  channelIDs: [] as number[],
  cooldownMinutes: 30,
});
const error = ref("");

function reset() {
  const item = props.editing;
  Object.assign(form, {
    name: item?.name || "",
    enabled: item?.enabled ?? true,
    eventTypes: [...(item?.event_types || [])],
    channelIDs: [...(item?.channel_ids || [])],
    cooldownMinutes: item?.cooldown_minutes ?? 30,
  });
  error.value = "";
}

function submit() {
  const name = form.name.trim();
  if (!name) error.value = "请输入规则名称";
  else if (!form.eventTypes.length) error.value = "请至少选择一种事件";
  else if (!form.channelIDs.length) error.value = "请至少选择一个目标渠道";
  else if (!Number.isInteger(form.cooldownMinutes) || form.cooldownMinutes < 0 || form.cooldownMinutes > 10080) {
    error.value = "冷却期必须是 0 到 10080 的整数";
  } else {
    error.value = "";
    emit("save", {
      name,
      enabled: form.enabled,
      event_types: [...form.eventTypes],
      channel_ids: form.channelIDs.map(Number),
      cooldown_minutes: form.cooldownMinutes,
    });
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) reset();
  },
);
</script>

<template>
  <Modal :open="open" :title="editing ? '编辑告警规则' : '新建告警规则'" size="lg" :closable="!saving" @close="emit('close')">
    <form class="space-y-5" @submit.prevent="submit">
      <fieldset class="space-y-5" :disabled="saving">
        <div class="grid gap-4 sm:grid-cols-2">
          <label><span class="aw-modal-label text-xs">规则名称</span><input v-model="form.name" class="aw-input mt-1 w-full" maxlength="100" placeholder="例如：出口故障通知" /></label>
          <label><span class="aw-modal-label text-xs">去重冷却期（分钟）</span><input v-model.number="form.cooldownMinutes" class="aw-input mt-1 w-full" type="number" min="0" max="10080" /></label>
        </div>

        <fieldset>
          <legend class="aw-modal-label text-xs">事件类型（可多选）</legend>
          <div class="mt-2 grid gap-2 sm:grid-cols-3">
            <label v-for="item in alertEventOptions" :key="item.value" class="flex items-center gap-2 rounded-lg border border-[var(--border-default)] p-3 text-sm"><input v-model="form.eventTypes" type="checkbox" :value="item.value" />{{ item.label }}</label>
          </div>
        </fieldset>

        <fieldset>
          <legend class="aw-modal-label text-xs">目标渠道（可多选）</legend>
          <div class="mt-2 grid max-h-56 gap-2 overflow-y-auto rounded-lg border border-[var(--border-default)] p-3 sm:grid-cols-2">
            <label v-for="channel in channels" :key="channel.id" class="flex items-start gap-2 rounded-lg p-2 text-sm hover:bg-[var(--bg-sidebar-hover)]"><input v-model="form.channelIDs" class="mt-0.5" type="checkbox" :value="channel.id" /><span><span class="block">{{ channel.name }}</span><span class="mt-0.5 block text-xs text-[var(--text-tertiary)]">{{ alertChannelLabel(channel.type) }} · {{ channel.enabled ? "已启用" : "已停用" }}</span></span></label>
            <p v-if="!channels.length" class="text-xs text-[var(--text-tertiary)]">请先创建通知渠道</p>
          </div>
        </fieldset>

        <label class="flex items-center gap-2 rounded-lg border border-[var(--border-default)] p-3 text-sm"><input v-model="form.enabled" type="checkbox" />保存后启用此规则</label>
      </fieldset>
    </form>
    <p class="mt-4 text-xs leading-5 text-[var(--text-tertiary)]">冷却期按“规则 + 事件类型 + 事件对象”去重；设置为 0 表示每次事件都投递。</p>
    <p v-if="error" class="mt-3 text-sm text-[var(--color-error)]" role="alert">{{ error }}</p>
    <template #footer><Button :disabled="saving" @click="emit('close')">取消</Button><Button variant="primary" :loading="saving" @click="submit">保存</Button></template>
  </Modal>
</template>
