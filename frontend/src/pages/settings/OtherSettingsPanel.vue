<script setup lang="ts">
import { onMounted, ref } from "vue";
import { SlidersHorizontal } from "lucide-vue-next";
import { api } from "@/services/api";
import type { GeoSource } from "@/services/types";

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

async function load() {
  loading.value = true;
  available.value = false;
  try {
    const settings = await api.getGeneralSettings();
    autoStartCore.value = settings.auto_start_core !== false;
    dnsmasqTakeoverEnabled.value = settings.dnsmasq_takeover_enabled !== false;
    dnsmasqTakeoverSupported.value =
      settings.dnsmasq_takeover_supported === true;
    geoSource.value = settings.geo_source || "sagernet";
    savedGeoSource.value = geoSource.value;
    available.value = true;
  } catch (cause: any) {
    emit("notify", `其他开关加载失败: ${cause.message}`, "error");
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
      :class="
        (loading || saving || !available || !dnsmasqTakeoverSupported) &&
        'cursor-wait opacity-70'
      "
    >
      <span class="min-w-0">
        <span class="block text-sm font-medium">接管 OpenWrt dnsmasq 上游</span>
        <span class="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">
          TUN 模式下将 dnsmasq 转发到本机 sing-box DNS
          端口；停止核心时自动恢复。切换前请先停止核心。
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
        默认使用 SagerNet。选择 Loyalsoldier 后优先使用其 GeoIP / GeoSite
        分类，缺少同名分类时自动使用 SagerNet；两边都不存在时会明确报错。
        下载并校验成功后才会切换，失败保留原来源；独立规则订阅不受影响。
        运行中的核心需按现有配置应用流程生效。
      </p>
      <p
        v-if="savingGeoSource"
        role="status"
        class="mt-2 text-xs leading-5 text-[var(--color-primary)]"
      >
        正在下载并校验 Geo 数据，请稍候…
      </p>
    </div>
  </section>
</template>
