import { createRouter, createWebHistory } from "vue-router";
import AppLayout from "@/components/layout/AppLayout.vue";

const pages = {
  "/": () => import("@/pages/ControlPage.vue"),
  "/control": () => import("@/pages/ControlPage.vue"),
  "/subscriptions": () => import("@/pages/SubscriptionsPage.vue"),
  "/nodes": () => import("@/pages/NodesPage.vue"),
  "/rules": () => import("@/pages/RulesPage.vue"),
  "/dns": () => import("@/pages/DNSPage.vue"),
  "/config": () => import("@/pages/ConfigPage.vue"),
  "/logs": () => import("@/pages/LogsPage.vue"),
  "/settings": () => import("@/pages/SettingsPage.vue"),
  "/advanced/routing": () => import("@/pages/MixedBindingsPage.vue"),
  "/advanced/ssh": () => import("@/pages/SSHHostsPage.vue"),
};

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/",
      component: AppLayout,
      children: [
        ...Object.entries(pages).map(([path, component]) => ({
          path: path === "/" ? "" : path.slice(1),
          component,
        })),
        { path: "advanced/node-exposures", redirect: "/advanced/routing" },
        {
          path: "collections",
          redirect: (to) =>
            to.hash === "#collections"
              ? "/rules"
              : { path: "/nodes", hash: "#node-groups" },
        },
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
