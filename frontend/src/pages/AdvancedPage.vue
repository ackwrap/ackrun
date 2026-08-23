<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  Activity,
  KeyRound,
  Route,
  ScrollText,
  Share2,
  SlidersHorizontal,
} from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import AdvancedNodeExposurePage from "./advanced/AdvancedNodeExposurePage.vue";
import PlatformRoutingPage from "./advanced/PlatformRoutingPage.vue";
import SessionLeasesPage from "./advanced/SessionLeasesPage.vue";
import HealthSchedulingPage from "./advanced/HealthSchedulingPage.vue";
import AccessLogsPage from "./advanced/AccessLogsPage.vue";
import AdvancedSettingsPage from "./advanced/AdvancedSettingsPage.vue";

const route = useRoute();
const router = useRouter();

const modules = [
  {
    id: "node-exposures",
    label: "节点入口",
    icon: Share2,
    component: AdvancedNodeExposurePage,
  },
  {
    id: "platform-routing",
    label: "平台路由",
    icon: Route,
    component: PlatformRoutingPage,
  },
  {
    id: "session-leases",
    label: "会话租约",
    icon: KeyRound,
    component: SessionLeasesPage,
  },
  {
    id: "health-scheduling",
    label: "健康调度",
    icon: Activity,
    component: HealthSchedulingPage,
  },
  {
    id: "access-logs",
    label: "访问审计",
    icon: ScrollText,
    component: AccessLogsPage,
  },
  {
    id: "settings",
    label: "高级设置",
    icon: SlidersHorizontal,
    component: AdvancedSettingsPage,
  },
] as const;

const requestedView = computed(() =>
  typeof route.query.view === "string" ? route.query.view : "",
);
const activeModule = computed(
  () =>
    modules.find((item) => item.id === requestedView.value) || modules[0],
);

function selectView(view: string) {
  if (view === activeModule.value.id) return;
  void router.replace({ path: "/advanced/routing", query: { view } });
}
</script>

<template>
  <div class="space-y-5">
    <PageHeader
      title="路由与调度"
      description="集中管理高级流量入口、平台路由、会话绑定、健康状态和访问决策。"
    />

    <div
      class="flex gap-1 overflow-x-auto border-b border-[var(--border-default)]"
      role="tablist"
      aria-label="高级功能模块"
    >
      <button
        v-for="item in modules"
        :key="item.id"
        type="button"
        class="relative inline-flex h-11 shrink-0 items-center gap-2 px-4 text-sm font-medium transition-colors"
        :class="
          activeModule.id === item.id
            ? 'text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:bg-[var(--bg-sidebar-hover)] hover:text-[var(--text-primary)]'
        "
        role="tab"
        :aria-selected="activeModule.id === item.id"
        @click="selectView(item.id)"
      >
        <component :is="item.icon" :size="15" />{{ item.label }}
        <span
          v-if="activeModule.id === item.id"
          class="absolute inset-x-2 bottom-0 h-0.5 rounded-full bg-[var(--color-primary)]"
        />
      </button>
    </div>

    <KeepAlive>
      <AdvancedSettingsPage
        v-if="activeModule.id === 'settings'"
        embedded
      />
    </KeepAlive>
    <component
      :is="activeModule.component"
      v-if="activeModule.id !== 'settings'"
      :key="activeModule.id"
      embedded
    />
  </div>
</template>
