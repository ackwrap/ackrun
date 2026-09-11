<script setup lang="ts">
import { toRefs } from "vue";
import type { SimpleDashboard } from "./useSimpleDashboard";

const props = defineProps<{ state: SimpleDashboard }>();
const { options, optionsDisabled, devicesText, unmanaged, professional } =
  toRefs(props.state);
</script>

<template>
  <section
    id="simple-panel-network"
    role="tabpanel"
    aria-labelledby="simple-tab-network"
    class="simple-card"
  >
    <h2 class="text-xl font-semibold">网络设置</h2>
    <p class="mb-6 mt-2 text-sm leading-6 text-[var(--text-secondary)]">
      常用设置开箱即用，需要调整时再展开高级选项。
    </p>
    <fieldset
      v-if="options"
      :disabled="optionsDisabled"
      class="min-w-0 space-y-5"
    >
      <label
        class="flex items-center justify-between gap-4 rounded-xl border border-[var(--border-light)] p-4"
      >
        <span
          ><span class="block font-medium">自动启动代理</span
          ><span class="mt-1 block text-xs text-[var(--text-secondary)]"
            >路由器或管理服务重启后恢复代理</span
          ></span
        >
        <input
          v-model="options.auto_start_core"
          type="checkbox"
          class="simple-checkbox"
        />
      </label>
      <details class="simple-details">
        <summary>高级 DNS 与 IPv6 设置</summary>
        <div class="mt-5 grid gap-5 sm:grid-cols-2">
          <label class="space-y-2 text-sm"
            ><span class="block font-medium">直连 DNS</span
            ><input
              v-model="options.local_dns"
              list="simple-local-dns"
              type="text"
              spellcheck="false"
              class="simple-input w-full"
            /><span class="block text-xs leading-5 text-[var(--text-secondary)]"
              >用于国内域名，填写 DNS 服务器的 IP。</span
            ></label
          >
          <datalist id="simple-local-dns">
            <option value="223.5.5.5">阿里 DNS</option>
            <option value="119.29.29.29">腾讯 DNS</option>
          </datalist>
          <label class="space-y-2 text-sm"
            ><span class="block font-medium">代理 DNS</span
            ><input
              v-model="options.proxy_dns"
              list="simple-proxy-dns"
              type="text"
              spellcheck="false"
              class="simple-input w-full"
            /><span class="block text-xs leading-5 text-[var(--text-secondary)]"
              >通过代理访问，服务器须支持 HTTPS DNS。</span
            ></label
          >
          <datalist id="simple-proxy-dns">
            <option value="1.1.1.1">Cloudflare</option>
            <option value="8.8.8.8">Google</option>
          </datalist>
          <label class="space-y-2 text-sm sm:col-span-2"
            ><span class="block font-medium">IPv6 解析策略</span
            ><select v-model="options.dns_strategy" class="simple-input w-full">
              <option value="prefer_ipv4">IPv4 优先（推荐）</option>
              <option value="prefer_ipv6">IPv6 优先</option>
              <option value="ipv4_only">仅解析 IPv4</option></select
            ><span class="block text-xs leading-5 text-[var(--text-secondary)]"
              >影响域名解析和连接选择，不会关闭路由器本身的 IPv6。</span
            ></label
          >
        </div>
      </details>
      <details class="simple-details">
        <summary>高级设备设置 · 指定设备直连</summary>
        <label class="mt-5 block space-y-2 text-sm"
          ><span class="block font-medium">不使用代理的设备</span
          ><textarea
            v-model="devicesText"
            rows="5"
            spellcheck="false"
            placeholder="192.168.1.50&#10;192.168.1.64/28"
            class="simple-input w-full font-mono"
          ></textarea
          ><span class="block text-xs leading-6 text-[var(--text-secondary)]"
            >每行一个设备 IP 或网段，建议在 DHCP
            中固定设备地址。设备流量直连，DNS 仍由路由器处理。</span
          ></label
        >
      </details>
    </fieldset>
    <p v-if="unmanaged" class="simple-alert">
      当前沿用专业模式的网络设置。
      <RouterLink to="/dns" class="underline" @click="professional"
        >管理现有 DNS</RouterLink
      >
      ·
      <RouterLink to="/settings" class="underline" @click="professional"
        >网络与运行设置</RouterLink
      >
    </p>
  </section>
</template>
