<script setup lang="ts">
import { Activity, List, Network, Shield } from "lucide-vue-next";
import type { MonitorTab } from "./monitorUtils";
defineProps<{ activeTab: MonitorTab }>();
defineEmits<{ change: [MonitorTab] }>();
const tabs = [
  ["overview", "概览", Activity],
  ["proxies", "策略组", Network],
  ["connections", "连接", List],
  ["rules", "规则", Shield],
] as const;
</script>
<template>
  <div
    class="flex gap-1 overflow-x-auto border-b border-[var(--border-default)]"
  >
    <button
      v-for="t in tabs"
      :key="t[0]"
      class="relative flex items-center gap-2 px-4 py-2.5 text-sm font-medium outline-none"
      :class="
        activeTab === t[0]
          ? 'text-[var(--color-primary)]'
          : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)]'
      "
      @click="$emit('change', t[0])"
    >
      <component :is="t[2]" :size="16" />{{ t[1] }}
    </button>
  </div>
</template>
