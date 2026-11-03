<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { Clock3 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type { CoreRestartSettings } from "@/services/types";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const open = ref(false),
  saving = ref(false),
  loading = ref(false),
  loaded = ref(false),
  error = ref(""),
  settings = ref<CoreRestartSettings>({
    mode: "daily",
    time: "04:00:00",
    weekday: 1,
  }),
  form = ref<CoreRestartSettings>({
    mode: "daily",
    time: "04:00",
    weekday: 1,
  });
const weekdayOptions = [
  [1, "周一"],
  [2, "周二"],
  [3, "周三"],
  [4, "周四"],
  [5, "周五"],
  [6, "周六"],
  [0, "周日"],
] as const;
const summary = computed(() => {
  if (settings.value.mode === "off") return "定时重启已关闭";
  const time = settings.value.time.slice(0, 5);
  if (settings.value.mode === "weekly") {
    const weekday =
      weekdayOptions.find((item) => item[0] === settings.value.weekday)?.[1] ||
      "每周";
    return `${weekday} ${time} 重启`;
  }
  return `每天 ${time} 重启`;
});

let disposed = false;
onBeforeUnmount(() => {
  disposed = true;
});

async function show() {
  if (loading.value || saving.value) return;
  open.value = true;
  loading.value = true;
  loaded.value = false;
  error.value = "";
  try {
    const value = await api.getCoreRestartSettings();
    if (disposed) return;
    settings.value = value;
    form.value = { ...value, time: value.time.slice(0, 5) };
    loaded.value = true;
  } catch (cause: any) {
    if (!disposed) error.value = `加载定时重启设置失败: ${cause.message}`;
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (saving.value || loading.value || !loaded.value) return;
  if (form.value.mode !== "off" && !form.value.time) {
    emit("notify", "请选择定时重启时间", "error");
    return;
  }
  saving.value = true;
  try {
    const value = await api.updateCoreRestartSettings({
      ...form.value,
      time: form.value.time || settings.value.time,
    });
    if (disposed) return;
    settings.value = value;
    open.value = false;
    emit("notify", `定时重启设置已保存：${summary.value}`);
  } catch (error: any) {
    emit("notify", `保存定时重启设置失败: ${error.message}`, "error");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Button
    size="sm"
    :title="loaded ? summary : '设置核心定时重启时间'"
    @click="show"
  >
    <Clock3 :size="14" />定时重启
  </Button>
  <Modal
    :open="open"
    title="核心定时重启"
    size="md"
    :closable="!saving"
    @close="open = false"
  >
    <p
      v-if="loading"
      role="status"
      class="text-sm text-[var(--text-secondary)]"
    >
      正在读取定时重启设置…
    </p>
    <div v-else-if="error" role="alert" class="space-y-3">
      <p class="text-sm text-[var(--color-error)]">{{ error }}</p>
      <Button size="sm" @click="show">重新加载</Button>
    </div>
    <fieldset
      v-else
      :disabled="saving || !loaded"
      class="grid gap-4 sm:grid-cols-3"
    >
      <label class="text-sm">
        周期
        <select v-model="form.mode" class="aw-input mt-1 w-full">
          <option value="off">关闭定时重启</option>
          <option value="daily">每天</option>
          <option value="weekly">每周</option>
        </select>
      </label>
      <label class="text-sm">
        时间
        <input
          v-model="form.time"
          type="time"
          class="aw-input mt-1 w-full"
          :disabled="form.mode === 'off'"
        />
      </label>
      <label class="text-sm">
        每周
        <select
          v-model.number="form.weekday"
          class="aw-input mt-1 w-full"
          :disabled="form.mode !== 'weekly'"
        >
          <option
            v-for="item in weekdayOptions"
            :key="item[0]"
            :value="item[0]"
          >
            {{ item[1] }}
          </option>
        </select>
      </label>
    </fieldset>
    <p class="mt-4 text-xs leading-5 text-[var(--text-tertiary)]">
      默认每天
      04:00。只有核心正在运行时才会重启；核心已停止时自动跳过，不会主动启动。
    </p>
    <template #footer>
      <Button :disabled="saving" @click="open = false">取消</Button>
      <Button
        variant="primary"
        :loading="saving"
        :disabled="loading || !loaded"
        @click="save"
        >保存</Button
      >
    </template>
  </Modal>
</template>
