<script setup lang="ts">
import {
  LayoutDashboard,
  Settings,
  Activity,
  RadioTower,
  Network,
  Layers,
  ListChecks,
  ServerCog,
  FileCode,
  ChevronLeft,
  ChevronRight,
  Gauge,
} from "lucide-vue-next";
withDefaults(defineProps<{ collapsed: boolean; mobileOpen?: boolean }>(), {
  mobileOpen: false,
});
defineEmits<{ toggle: []; close: [] }>();
const items = [
  ["控制面板", LayoutDashboard, "/control"],
  ["仪表盘", Gauge, "/"],
  ["订阅管理", RadioTower, "/subscriptions"],
  ["节点管理", Network, "/nodes"],
  ["规则管理", ListChecks, "/rules"],
  ["策略组管理", Layers, "/collections"],
  ["DNS 管理", ServerCog, "/dns"],
  ["配置生成", FileCode, "/config"],
  ["日志", Activity, "/logs"],
  ["设置", Settings, "/settings"],
] as const;
</script>
<template>
  <aside
    class="h-screen shrink-0 flex-col bg-[var(--bg-sidebar)] text-[var(--text-sidebar)] transition-[width] duration-300"
    :class="[
      mobileOpen
        ? 'fixed inset-y-0 left-0 z-50 flex w-64 shadow-[var(--shadow-xl)]'
        : 'hidden lg:flex',
      collapsed ? 'lg:w-16' : 'lg:w-56',
    ]"
  >
    <div
      class="flex h-[62px] items-center justify-center border-b border-[var(--border-light)]"
    >
      <img src="/favicon.png" alt="AckWrap" class="h-9 w-9" /><b
        v-if="!collapsed"
        class="ml-3 text-lg text-[var(--text-primary)]"
        >AckWrap</b
      >
    </div>
    <nav class="flex-1 space-y-2 overflow-y-auto px-4 py-5">
      <RouterLink
        v-for="[label, icon, path] in items"
        :key="path"
        :to="path"
        class="flex h-11 items-center gap-3 rounded-[var(--radius-lg)] border border-transparent px-4"
        :class="[
          $route.path === path
            ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
            : 'hover:bg-[var(--bg-sidebar-hover)]',
          collapsed && 'lg:justify-center lg:px-0',
        ]"
        @click="$emit('close')"
        ><component :is="icon" :size="18" /><span
          :class="collapsed ? 'lg:hidden' : ''"
          >{{ label }}</span
        ></RouterLink
      >
    </nav>
    <button
      class="m-3 hidden h-8 items-center justify-center lg:flex"
      @click="$emit('toggle')"
    >
      <ChevronRight v-if="collapsed" :size="16" /><ChevronLeft
        v-else
        :size="16"
      />
    </button>
  </aside>
</template>
