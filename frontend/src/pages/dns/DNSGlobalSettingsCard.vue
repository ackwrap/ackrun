<script setup lang="ts">
import { computed } from "vue";
import { Save, ServerCog } from "lucide-vue-next";

interface GlobalSettings {
  enabled: boolean;
  final: string;
  proxy_final: string;
  strategy: string;
  disable_cache: boolean;
  disable_expire: boolean;
  independent_cache: boolean;
  independent_cache_supported: boolean;
  reverse_mapping: boolean;
  cache_capacity: number;
  client_subnet: string;
  fakeip_enabled: boolean;
  fakeip_inet4_range: string;
  fakeip_inet6_range: string;
}

interface ServerOption {
  tag: string;
  enabled: boolean;
  server_type: string;
  address: string;
}

const global = defineModel<GlobalSettings>({ required: true });
const props = defineProps<{
  servers: ServerOption[];
  strategies: string[];
}>();
defineEmits<{ save: [] }>();

const remoteTypes = new Set(["udp", "tcp", "tls", "https", "quic", "h3"]);
const finalServers = computed(() =>
  props.servers.filter(
    (server) => server.enabled && server.server_type !== "fakeip",
  ),
);
const proxyFinalServers = computed(() =>
  props.servers.filter(
    (server) => server.enabled && remoteTypes.has(server.server_type),
  ),
);
const proxyFinalUnavailable = computed(
  () =>
    !!global.value.proxy_final &&
    !proxyFinalServers.value.some(
      (server) => server.tag === global.value.proxy_final,
    ),
);
</script>

<template>
  <section
    class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <div class="flex justify-between">
      <h3><ServerCog class="inline" /> 全局设置</h3>
      <label
        ><input v-model="global.enabled" type="checkbox" /> 启用 DNS 管理</label
      >
    </div>
    <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      <label
        >默认 Server<select v-model="global.final">
          <option value="">请选择</option>
          <option
            v-for="server in finalServers"
            :key="server.tag"
            :value="server.tag"
          >
            {{ server.tag }}
          </option>
        </select></label
      >
      <label
        >代理 DNS Final<select v-model="global.proxy_final">
          <option value="">自动选择（兼容旧配置）</option>
          <option
            v-if="proxyFinalUnavailable"
            :value="global.proxy_final"
            disabled
          >
            {{ global.proxy_final }}（不可用）
          </option>
          <option
            v-for="server in proxyFinalServers"
            :key="server.tag"
            :value="server.tag"
          >
            {{ server.tag }} · {{ server.server_type }} · {{ server.address }}
          </option>
        </select></label
      >
      <label
        >IP 返回策略<select v-model="global.strategy">
          <option v-for="strategy in strategies" :key="strategy">
            {{ strategy }}
          </option>
        </select></label
      >
      <label
        >缓存容量<input v-model.number="global.cache_capacity" type="number"
      /></label>
      <label>Client Subnet<input v-model="global.client_subnet" /></label>
      <button
        class="aw-action-button aw-action-success self-end justify-self-end px-4"
        @click="$emit('save')"
      >
        <Save :size="13" />保存全局设置
      </button>
    </div>
    <p class="mt-2 text-xs text-[var(--text-tertiary)]">
      代理 DNS Final 用于规则或全局代理模式下未命中显式 DNS 规则的真实查询。
      未设置 proxy detour 的远程 Server 会由 Ackwrap 强制通过 proxy 发送。
    </p>
    <p
      v-if="proxyFinalUnavailable"
      class="mt-2 text-xs text-[var(--color-warning)]"
    >
      当前代理 DNS Final 已失效，请重新选择后保存。
    </p>
    <div class="mt-3 flex flex-wrap gap-4">
      <label
        ><input v-model="global.disable_cache" type="checkbox" />
        disable_cache</label
      >
      <label
        ><input v-model="global.disable_expire" type="checkbox" />
        disable_expire</label
      >
      <label v-if="global.independent_cache_supported"
        ><input v-model="global.independent_cache" type="checkbox" />
        independent_cache</label
      >
      <label
        ><input v-model="global.reverse_mapping" type="checkbox" />
        reverse_mapping</label
      >
    </div>
    <p
      v-if="!global.independent_cache_supported"
      class="mt-2 text-xs text-[var(--text-tertiary)]"
    >
      当前核心已按 DNS transport 隔离缓存，无需 independent_cache。
    </p>
  </section>
</template>
