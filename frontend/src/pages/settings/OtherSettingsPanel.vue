<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { SlidersHorizontal } from "lucide-vue-next";
import { api } from "@/services/api";
import type { GeneralSettings, GeoSource } from "@/services/types";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const autoStartCore = ref(true);
const dnsmasqTakeoverEnabled = ref(true);
const dnsmasqTakeoverSupported = ref(false);
const realIPPingBypassEnabled = ref(false);
const geoSource = ref<GeoSource>("sagernet");
const savedGeoSource = ref<GeoSource>("sagernet");
const loading = ref(true);
const saving = ref(false);
const savingGeoSource = ref(false);
const available = ref(false);
const switchesUnconfirmed = ref(false);
const protection = ref<
  Pick<
    GeneralSettings,
    | "device_protection_enabled"
    | "device_protection_supported"
    | "device_protection_status"
    | "device_protection_error"
    | "device_protection_count"
  >
>({
  device_protection_enabled: false,
  device_protection_supported: false,
  device_protection_status: "disabled",
  device_protection_count: 0,
});
const protectionLabel = computed(
  () =>
    ({
      disabled: "未开启",
      protected: "保护中",
      blocked: "已阻断",
      error: "异常",
    })[protection.value.device_protection_status],
);
let protectionPoll: ReturnType<typeof setInterval> | undefined;

function updateProtection(settings: GeneralSettings) {
  protection.value = {
    device_protection_enabled: settings.device_protection_enabled,
    device_protection_supported: settings.device_protection_supported,
    device_protection_status: settings.device_protection_status,
    device_protection_error: settings.device_protection_error,
    device_protection_count: settings.device_protection_count,
  };
}

function updateSwitches(settings: GeneralSettings) {
  updateProtection(settings);
  autoStartCore.value = settings.auto_start_core !== false;
  dnsmasqTakeoverEnabled.value = settings.dnsmasq_takeover_enabled !== false;
  dnsmasqTakeoverSupported.value = settings.dnsmasq_takeover_supported === true;
  realIPPingBypassEnabled.value = settings.real_ip_ping_bypass_enabled === true;
  switchesUnconfirmed.value = false;
}

async function refreshProtection() {
  if (saving.value || loading.value || !available.value) return;
  try {
    updateSwitches(await api.getGeneralSettings());
  } catch {
    protection.value.device_protection_status = "error";
    protection.value.device_protection_error =
      "无法确认防护状态，请检查后端连接。";
  }
}

async function saveProtection(enabled: boolean) {
  if (saving.value || loading.value || !available.value) return;
  saving.value = true;
  try {
    await api.setGeneralSettings({ device_protection_enabled: enabled });
    updateProtection(await api.getGeneralSettings());
    emit(
      "notify",
      enabled
        ? "设备流量防泄漏已开启，启动核心后恢复代理访问"
        : "设备流量防泄漏已解除",
      "success",
    );
  } catch (cause: any) {
    emit("notify", `流量防泄漏设置失败：${cause.message}`, "error");
    try {
      updateProtection(await api.getGeneralSettings());
    } catch {
      protection.value.device_protection_status = "error";
    }
  } finally {
    saving.value = false;
  }
}

function changeProtection(event: Event) {
  const input = event.target as HTMLInputElement;
  const enabled = input.checked;
  // Keep the last confirmed state until the backend returns. A rejected save
  // does not change the bound value, so Vue alone may not reset native checked.
  input.checked = protection.value.device_protection_enabled;
  void saveProtection(enabled);
}

async function load() {
  loading.value = true;
  available.value = false;
  try {
    const settings = await api.getGeneralSettings();
    updateSwitches(settings);
    geoSource.value = settings.geo_source || "sagernet";
    savedGeoSource.value = geoSource.value;
    available.value = true;
  } catch (cause: any) {
    emit("notify", `其他开关加载失败: ${cause.message}`, "error");
    protection.value.device_protection_status = "error";
    protection.value.device_protection_error =
      "无法读取防护状态，请刷新页面重试。";
  } finally {
    loading.value = false;
  }
}

async function saveSetting(kind: "autoStart" | "dnsmasq" | "ping") {
  if (
    loading.value ||
    saving.value ||
    !available.value ||
    switchesUnconfirmed.value
  )
    return;
  saving.value = true;
  const setting = {
    autoStart: {
      value: autoStartCore,
      key: "auto_start_core",
      label: "核心自动启动",
    },
    dnsmasq: {
      value: dnsmasqTakeoverEnabled,
      key: "dnsmasq_takeover_enabled",
      label: "dnsmasq 接管",
    },
    ping: {
      value: realIPPingBypassEnabled,
      key: "real_ip_ping_bypass_enabled",
      label: "真实 IP Ping 直连",
    },
  }[kind];
  const next = setting.value.value;
  try {
    await api.setGeneralSettings({ [setting.key]: next });
    emit("notify", `已${next ? "开启" : "关闭"}${setting.label}`, "success");
  } catch (cause: any) {
    switchesUnconfirmed.value = true;
    emit("notify", `其他开关保存失败: ${cause.message}`, "error");
    try {
      updateSwitches(await api.getGeneralSettings());
    } catch {
      // The write may have succeeded before the response was lost. Keep the
      // state unconfirmed until the existing poll can read it back.
    }
  } finally {
    saving.value = false;
  }
}

async function saveGeoSource() {
  if (loading.value || saving.value || !available.value) return;
  if (geoSource.value === savedGeoSource.value) return;
  saving.value = true;
  savingGeoSource.value = true;
  const next = geoSource.value;
  try {
    await api.setGeneralSettings({ geo_source: next });
    savedGeoSource.value = next;
    emit(
      "notify",
      "Geo 来源已切换，配置将自动重新生成；请按现有流程应用到核心",
      "success",
    );
  } catch (cause: any) {
    geoSource.value = savedGeoSource.value;
    emit("notify", `Geo 数据来源切换失败: ${cause.message}`, "error");
  } finally {
    saving.value = false;
    savingGeoSource.value = false;
  }
}

onMounted(() => {
  void load();
  protectionPoll = setInterval(refreshProtection, 5000);
});
onBeforeUnmount(() => clearInterval(protectionPoll));
</script>

<template>
  <section
    class="self-start rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="mb-4 flex items-center gap-2">
      <SlidersHorizontal :size="18" class="text-[var(--color-primary)]" />
      <h2 class="font-semibold">其他开关</h2>
    </div>
    <p
      v-if="switchesUnconfirmed"
      role="status"
      class="mb-3 text-xs text-[var(--text-secondary)]"
    >
      开关状态未确认，正在重新读取…
    </p>

    <label
      class="flex cursor-pointer items-center justify-between gap-4 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
      :class="(loading || saving || !available) && 'cursor-wait opacity-70'"
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium">启动时运行核心</span>
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          下次启动 AckWrap 时生效，需已安装核心并有有效配置。
        </span>
      </span>
      <input
        v-model="autoStartCore"
        type="checkbox"
        class="h-4 w-4 shrink-0"
        :disabled="loading || saving || !available || switchesUnconfirmed"
        @change="saveSetting('autoStart')"
      />
    </label>
    <label
      class="mt-3 flex cursor-pointer items-center justify-between gap-4 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
      :class="
        (loading || saving || !available || !dnsmasqTakeoverSupported) &&
        'cursor-wait opacity-70'
      "
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium">接管 dnsmasq</span>
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          TUN 模式接管 OpenWrt
          DNS；停止核心后恢复，受保护设备除外。切换前请停止核心。
        </span>
      </span>
      <input
        v-model="dnsmasqTakeoverEnabled"
        type="checkbox"
        class="h-4 w-4 shrink-0"
        :disabled="
          loading ||
          saving ||
          !available ||
          !dnsmasqTakeoverSupported ||
          switchesUnconfirmed
        "
        @change="saveSetting('dnsmasq')"
      />
    </label>
    <label
      class="mt-3 flex cursor-pointer items-center justify-between gap-4 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
      :class="(loading || saving || !available) && 'cursor-wait opacity-70'"
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium">真实 IP Ping 直连</span>
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          真实 IPv4/IPv6 Ping 绕过 TUN，设备规则优先，FakeIP
          除外。切换前请停止核心。
        </span>
      </span>
      <input
        v-model="realIPPingBypassEnabled"
        type="checkbox"
        class="h-4 w-4 shrink-0"
        :disabled="loading || saving || !available || switchesUnconfirmed"
        @change="saveSetting('ping')"
      />
    </label>
    <div
      class="mt-3 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
      :class="(loading || saving || !available) && 'opacity-70'"
    >
      <label
        for="geo-source"
        class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4"
      >
        <span class="shrink-0 text-sm font-medium">Geo 数据来源</span>
        <select
          id="geo-source"
          v-model="geoSource"
          class="w-full sm:max-w-52"
          :disabled="loading || saving || !available"
          aria-describedby="geo-source-description"
          @change="saveGeoSource"
        >
          <option value="sagernet">SagerNet（默认）</option>
          <option value="loyalsoldier">Loyalsoldier</option>
        </select>
      </label>
      <p
        id="geo-source-description"
        class="mt-2 text-xs leading-5 text-[var(--text-secondary)]"
      >
        缺失分类自动回退 SagerNet，切换失败保留原来源。
      </p>
      <p
        v-if="savingGeoSource"
        role="status"
        class="mt-2 text-xs leading-5 text-[var(--color-primary)]"
      >
        正在下载并校验 Geo 数据，请稍候…
      </p>
    </div>
    <div
      class="mt-3 rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-base)] px-4 py-3"
    >
      <label class="flex cursor-pointer items-center justify-between gap-4">
        <span class="min-w-0">
          <span class="block text-sm font-medium">流量防泄漏</span>
          <span
            class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]"
          >
            核心停止时阻断已绑定设备外网，保留局域网。IP 规则排除接口可直连；MAC
            规则须常驻保护，不支持排除。修改前请停止核心。
          </span>
        </span>
        <input
          type="checkbox"
          class="h-4 w-4 shrink-0"
          :checked="protection.device_protection_enabled"
          :disabled="
            loading ||
            saving ||
            !available ||
            (!protection.device_protection_supported &&
              !protection.device_protection_enabled)
          "
          @change="changeProtection"
        />
      </label>
      <p
        role="status"
        class="mt-2 text-xs leading-5 text-[var(--text-secondary)]"
      >
        {{ saving ? "正在保存…" : protectionLabel }}
        <span v-if="protection.device_protection_count">
          · {{ protection.device_protection_count }} 个 MAC / 地址范围</span
        >
      </p>
      <p
        v-if="!loading && !protection.device_protection_supported"
        class="mt-1 text-xs leading-5 text-[var(--text-secondary)]"
      >
        需要安装含防火墙加载文件的 OpenWrt/fw4 版本，并关闭软/硬件流量卸载。
      </p>
      <p
        v-if="protection.device_protection_error"
        role="alert"
        class="mt-1 text-xs leading-5 text-[var(--text-secondary)]"
      >
        {{ protection.device_protection_error }}
      </p>
      <button
        v-if="
          available &&
          !protection.device_protection_enabled &&
          protection.device_protection_status === 'error'
        "
        class="aw-action-button mt-2"
        :disabled="saving"
        @click="saveProtection(false)"
      >
        解除残留保护
      </button>
    </div>
  </section>
</template>
