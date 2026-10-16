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
const geoSource = ref<GeoSource>("sagernet");
const savedGeoSource = ref<GeoSource>("sagernet");
const loading = ref(true);
const saving = ref(false);
const savingGeoSource = ref(false);
const available = ref(false);
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

async function refreshProtection() {
  if (saving.value || loading.value || !available.value) return;
  try {
    updateProtection(await api.getGeneralSettings());
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
    updateProtection(settings);
    autoStartCore.value = settings.auto_start_core !== false;
    dnsmasqTakeoverEnabled.value = settings.dnsmasq_takeover_enabled !== false;
    dnsmasqTakeoverSupported.value =
      settings.dnsmasq_takeover_supported === true;
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

async function saveSetting(kind: "autoStart" | "dnsmasq") {
  if (loading.value || saving.value || !available.value) return;
  saving.value = true;
  const next =
    kind === "autoStart" ? autoStartCore.value : dnsmasqTakeoverEnabled.value;
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
      :class="
        (loading || saving || !available || !dnsmasqTakeoverSupported) &&
        'cursor-wait opacity-70'
      "
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium">接管 OpenWrt dnsmasq 上游</span>
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          TUN 模式下将 dnsmasq 转发到本机 sing-box DNS
          端口；停止核心时恢复普通设备 DNS，受防泄漏保护的设备继续阻断。
          切换前请先停止核心。
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
        Loyalsoldier 缺失分类时自动回退 SagerNet；切换失败保留原来源。
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
            保护已绑定设备。核心停止或异常退出后，阻断外网与 DNS
            直连；本地网络仍可访问，FakeIP 地址池不作为本地网络放行。
            DNS（包括发往路由器的查询）继续由节点处理，
            核心停止时阻断。关闭后解除保护。
            切换开关、修改设备地址或启停设备规则前，请先停止核心。
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
          · {{ protection.device_protection_count }} 个地址范围</span
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
