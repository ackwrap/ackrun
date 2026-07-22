<script setup lang="ts">
import { onMounted, ref } from "vue";
import { SlidersHorizontal } from "lucide-vue-next";
import { api } from "@/services/api";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const autoStartCore = ref(true);
const dnsmasqTakeoverEnabled = ref(true);
const dnsmasqTakeoverSupported = ref(false);
const loading = ref(true);
const saving = ref(false);
const available = ref(false);

async function load() {
  loading.value = true;
  available.value = false;
  try {
    const settings = await api.getGeneralSettings();
    autoStartCore.value = settings.auto_start_core !== false;
    dnsmasqTakeoverEnabled.value = settings.dnsmasq_takeover_enabled !== false;
    dnsmasqTakeoverSupported.value = settings.dnsmasq_takeover_supported === true;
    available.value = true;
  } catch (cause: any) {
    emit("notify", `其他开关加载失败: ${cause.message}`, "error");
  } finally {
    loading.value = false;
  }
}

async function saveSetting(kind: "autoStart" | "dnsmasq") {
  if (saving.value) return;
  saving.value = true;
  const next = kind === "autoStart" ? autoStartCore.value : dnsmasqTakeoverEnabled.value;
  try {
    await api.setGeneralSettings(
      kind === "autoStart"
        ? { auto_start_core: autoStartCore.value }
        : { dnsmasq_takeover_enabled: dnsmasqTakeoverEnabled.value },
    );
    emit(
      "notify",
      kind === "autoStart"
        ? next
          ? "已开启 Ackwrap 启动后自动启动核心"
          : "已关闭 Ackwrap 启动后自动启动核心"
        : next
          ? "已开启 OpenWrt dnsmasq DNS 接管"
          : "已关闭 OpenWrt dnsmasq DNS 接管",
      "success",
    );
  } catch (cause: any) {
    if (kind === "autoStart") autoStartCore.value = !next;
    else dnsmasqTakeoverEnabled.value = !next;
    emit("notify", `其他开关保存失败: ${cause.message}`, "error");
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section
    class="self-start rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="mb-4 flex items-center gap-2">
      <SlidersHorizontal :size="18" class="text-[var(--color-primary)]" />
      <h2 class="font-semibold">其他开关</h2>
    </div>

    <label
      class="flex cursor-pointer items-center justify-between gap-4 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
      :class="(loading || saving || !available) && 'cursor-wait opacity-70'"
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium"
          >启动 Ackwrap 时自动启动核心</span
        >
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          仅在核心已安装且存在有效配置时生效；修改后从下次 Ackwrap
          启动开始执行。
        </span>
      </span>
      <input
        v-model="autoStartCore"
        type="checkbox"
        class="h-4 w-4 shrink-0"
        :disabled="loading || saving || !available"
        @change="saveSetting('autoStart')"
      />
    </label>
    <label
      class="mt-3 flex cursor-pointer items-center justify-between gap-4 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
      :class="(loading || saving || !available || !dnsmasqTakeoverSupported) && 'cursor-wait opacity-70'"
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium">接管 OpenWrt dnsmasq 上游</span>
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          TUN 模式下将 dnsmasq 转发到本机 sing-box DNS 端口；停止核心时自动恢复。切换前请先停止核心。
        </span>
      </span>
      <input
        v-model="dnsmasqTakeoverEnabled"
        type="checkbox"
        class="h-4 w-4 shrink-0"
        :disabled="loading || saving || !available || !dnsmasqTakeoverSupported"
        @change="saveSetting('dnsmasq')"
      />
    </label>
  </section>
</template>
