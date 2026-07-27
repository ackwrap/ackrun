<script setup lang="ts">
import { ref, watch } from "vue";
import { useRoute } from "vue-router";
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
  Wrench,
  Share2,
  Route,
  KeyRound,
  HeartPulse,
  ScrollText,
  SlidersHorizontal,
  ChevronDown,
} from "lucide-vue-next";
const props = withDefaults(
  defineProps<{ collapsed: boolean; mobileOpen?: boolean }>(),
  { mobileOpen: false },
);
const emit = defineEmits<{ toggle: []; close: [] }>();
const route = useRoute();
const showAdvancedFeatures = true;
const advancedOpen = ref(route.path.startsWith("/advanced/"));
const items = [
  ["控制面板", LayoutDashboard, "/"],
  ["仪表盘", Gauge, "/dashboard"],
  ["订阅管理", RadioTower, "/subscriptions"],
  ["节点管理", Network, "/nodes"],
  ["规则管理", ListChecks, "/rules"],
  ["策略组管理", Layers, "/collections"],
  ["DNS 管理", ServerCog, "/dns"],
  ["配置生成", FileCode, "/config"],
  ["日志", Activity, "/logs"],
  ["设置", Settings, "/settings"],
] as const;
const advancedItems = [
  ["节点暴露", Share2, "/advanced/node-exposures"],
  ["平台路由", Route, "/advanced/platform-routing"],
  ["会话租约", KeyRound, "/advanced/session-leases"],
  ["健康调度", HeartPulse, "/advanced/health-scheduling"],
  ["访问日志", ScrollText, "/advanced/access-logs"],
  ["高级设置", SlidersHorizontal, "/advanced/settings"],
] as const;

watch(
  () => route.path,
  (path) => {
    if (path.startsWith("/advanced/")) advancedOpen.value = true;
  },
);

function toggleAdvanced() {
  if (props.collapsed && !props.mobileOpen) {
    emit("toggle");
    advancedOpen.value = true;
    return;
  }
  advancedOpen.value = !advancedOpen.value;
}
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
      <div v-if="showAdvancedFeatures">
        <button
          class="flex h-11 w-full items-center gap-3 rounded-[var(--radius-lg)] border border-transparent px-4 text-left"
          :class="[
            $route.path.startsWith('/advanced/')
              ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
              : 'hover:bg-[var(--bg-sidebar-hover)]',
            collapsed && 'lg:justify-center lg:px-0',
          ]"
          :aria-expanded="advancedOpen"
          @click="toggleAdvanced"
        >
          <Wrench :size="18" />
          <span :class="collapsed ? 'lg:hidden' : ''" class="min-w-0 flex-1"
            >高级功能</span
          >
          <ChevronDown
            v-if="!collapsed || mobileOpen"
            :size="15"
            class="transition-transform"
            :class="advancedOpen && 'rotate-180'"
          />
        </button>
        <div
          v-if="advancedOpen && (!collapsed || mobileOpen)"
          class="mt-1 space-y-1 border-l border-[var(--border-light)] pl-3"
        >
          <RouterLink
            v-for="[label, icon, path] in advancedItems"
            :key="path"
            :to="path"
            class="flex h-9 items-center gap-2.5 rounded-[var(--radius-md)] px-3 text-xs text-[var(--text-secondary)]"
            :class="
              $route.path === path
                ? 'bg-[var(--bg-sidebar-hover)] text-[var(--color-primary-hover)]'
                : 'hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--text-primary)]'
            "
            @click="$emit('close')"
          >
            <component :is="icon" :size="15" />
            <span>{{ label }}</span>
          </RouterLink>
        </div>
      </div>
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
