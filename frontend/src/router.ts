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
  "/advanced/routing": () => import("@/pages/AdvancedPage.vue"),
  "/advanced/ssh": () => import("@/pages/SSHHostsPage.vue"),
};

const legacyAdvancedViews = {
  "advanced/node-exposures": "node-exposures",
  "advanced/platform-routing": "platform-routing",
  "advanced/session-leases": "session-leases",
  "advanced/health-scheduling": "health-scheduling",
  "advanced/access-logs": "access-logs",
  "advanced/settings": "settings",
} as const;

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/simple",
      component: () => import("@/pages/SimplePage.vue"),
    },
    {
      path: "/",
      component: AppLayout,
      children: [
        ...Object.entries(pages).map(([path, component]) => ({
          path: path === "/" ? "" : path.slice(1),
          component,
        })),
        ...Object.entries(legacyAdvancedViews).map(([path, view]) => ({
          path,
          redirect: () => ({ path: "/advanced/routing", query: { view } }),
        })),
        { path: "advanced", redirect: "/advanced/routing" },
      ],
    },
    {
      path: "/ssh-terminal/:hostId",
      component: () => import("@/pages/SSHTerminalWorkspacePage.vue"),
    },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});

router.beforeEach((to) => {
  if (to.path === "/" && localStorage.getItem("ackwrap.ui-mode") === "simple")
    return "/simple";
});
