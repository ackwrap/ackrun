<script setup lang="ts">
export interface SyncScheduleValue {
  sync_mode: "off" | "daily" | "weekly" | "monthly";
  sync_time: string;
  sync_weekday: number;
  use_proxy?: boolean;
}
const p = withDefaults(
  defineProps<{
    value: SyncScheduleValue;
    syncModes: { value: string; label: string }[];
    weekdays: string[];
    weekdayOptions?: { value: number; label: string }[];
    disabled?: boolean;
    showProxy?: boolean;
    saveText?: string;
  }>(),
  { saveText: "保存" },
);
const emit = defineEmits<{ change: [Partial<SyncScheduleValue>]; save: [] }>();
const days = () =>
  p.value.sync_mode === "monthly"
    ? Array.from({ length: 31 }, (_, i) => ({
        value: i + 1,
        label: `${i + 1} 号`,
      }))
    : p.weekdayOptions || p.weekdays.map((label, value) => ({ label, value }));
</script>
<template>
  <div class="grid gap-3 sm:grid-cols-3">
    <label
      >周期<select
        :value="value.sync_mode"
        :disabled="disabled"
        @change="
          emit('change', {
            sync_mode: ($event.target as HTMLSelectElement)
              .value as SyncScheduleValue['sync_mode'],
          })
        "
      >
        <option v-for="m in syncModes" :key="m.value" :value="m.value">
          {{ m.label }}
        </option>
      </select></label
    ><label
      >时间<input
        type="time"
        step="1"
        :value="value.sync_time || '03:30:00'"
        :disabled="disabled || value.sync_mode === 'off'"
        @input="
          emit('change', {
            sync_time: ($event.target as HTMLInputElement).value,
          })
        " /></label
    ><label
      >{{ value.sync_mode === "monthly" ? "每月" : "每周"
      }}<select
        :value="value.sync_weekday"
        :disabled="disabled || !['weekly', 'monthly'].includes(value.sync_mode)"
        @change="
          emit('change', {
            sync_weekday: Number(($event.target as HTMLSelectElement).value),
          })
        "
      >
        <option v-for="d in days()" :key="d.value" :value="d.value">
          {{ d.label }}
        </option>
      </select></label
    >
    <div v-if="showProxy || $attrs.onSave" class="sm:col-span-3">
      <label v-if="showProxy"
        ><input
          type="checkbox"
          :checked="value.use_proxy"
          @change="
            emit('change', {
              use_proxy: ($event.target as HTMLInputElement).checked,
            })
          "
        />代理</label
      ><button v-if="$attrs.onSave" @click="emit('save')">
        {{ saveText }}
      </button>
    </div>
  </div>
</template>
