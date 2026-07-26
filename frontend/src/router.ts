import { createRouter, createWebHistory } from "vue-router";
import AppLayout from "@/components/layout/AppLayout.vue";

const pages = {
  "/": () => import("@/pages/ControlPage.vue"),
  "/control": () => import("@/pages/ControlPage.vue"),
  "/dashboard": () => import("@/pages/MonitorPage.vue"),
  "/subscriptions": () => import("@/pages/SubscriptionsPage.vue"),
  "/nodes": () => import("@/pages/NodesPage.vue"),
  "/collections": () => import("@/pages/CollectionsPage.vue"),
  "/rules": () => import("@/pages/RulesPage.vue"),
  "/dns": () => import("@/pages/DNSPage.vue"),
  "/config": () => import("@/pages/ConfigPage.vue"),
  "/logs": () => import("@/pages/LogsPage.vue"),
  "/settings": () => import("@/pages/SettingsPage.vue"),
  "/advanced/node-exposures": () =>
    import("@/pages/AdvancedNodeExposurePage.vue"),
  "/advanced/platform-routing": () =>
    import("@/pages/AdvancedPlaceholderPage.vue"),
  "/advanced/session-leases": () =>
    import("@/pages/AdvancedPlaceholderPage.vue"),
  "/advanced/health-scheduling": () =>
    import("@/pages/AdvancedPlaceholderPage.vue"),
  "/advanced/access-logs": () =>
    import("@/pages/AdvancedPlaceholderPage.vue"),
  "/advanced/settings": () =>
    import("@/pages/AdvancedPlaceholderPage.vue"),
};

const advancedMeta: Record<string, { title: string; description: string }> = {
  "/advanced/platform-routing": {
    title: "平台路由",
    description: "按平台、租户和业务入口编排路由策略。",
  },
  "/advanced/session-leases": {
    title: "会话租约",
    description: "为客户端维持稳定的出口节点和租约周期。",
  },
  "/advanced/health-scheduling": {
    title: "健康调度",
    description: "结合探测、熔断和恢复状态执行节点调度。",
  },
  "/advanced/access-logs": {
    title: "访问日志",
    description: "提供高级入口的访问审计和流量观测。",
  },
  "/advanced/settings": {
    title: "高级设置",
    description: "集中管理高级数据面和控制面参数。",
  },
};

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/",
      component: AppLayout,
      children: Object.entries(pages).map(([path, component]) => {
        const meta = advancedMeta[path];
        return {
          path: path === "/" ? "" : path.slice(1),
          component,
          ...(meta ? { meta } : {}),
        };
      }),
    },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});
