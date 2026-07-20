<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
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

onMounted(async () => {
  try {
    settings.value = await api.getCoreRestartSettings();
  } catch (error: any) {
    emit("notify", `加载定时重启设置失败: ${error.message}`, "error");
  }
});

function show() {
  form.value = { ...settings.value, time: settings.value.time.slice(0, 5) };
  open.value = true;
}

async function save() {
  if (saving.value) return;
  if (!form.value.time) {
    emit("notify", "请选择定时重启时间", "error");
    return;
  }
  saving.value = true;
  try {
    settings.value = await api.updateCoreRestartSettings({ ...form.value });
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
  <button
    class="aw-action-button aw-action-neutral !h-[34px] shrink-0 px-2"
    :title="summary"
    @click="show"
  >
    <Clock3 :size="14" />
  </button>
  <Modal
    :open="open"
    title="核心定时重启"
    size="md"
    :closable="!saving"
    @close="open = false"
  >
    <div class="grid gap-4 sm:grid-cols-3">
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
          <option v-for="item in weekdayOptions" :key="item[0]" :value="item[0]">
            {{ item[1] }}
          </option>
        </select>
      </label>
    </div>
    <p class="mt-4 text-xs leading-5 text-[var(--text-tertiary)]">
      默认每天 04:00。只有核心正在运行时才会重启；核心已停止时自动跳过，不会主动启动。
    </p>
    <template #footer>
      <Button :disabled="saving" @click="open = false">取消</Button>
      <Button variant="primary" :loading="saving" @click="save">保存</Button>
    </template>
  </Modal>
</template>
