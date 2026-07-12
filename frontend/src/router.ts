import { createRouter, createWebHistory } from 'vue-router';
import AppLayout from '@/components/layout/AppLayout.vue';

const pages = {
  '/': () => import('@/pages/MonitorPage.vue'),
  '/control': () => import('@/pages/ControlPage.vue'),
  '/subscriptions': () => import('@/pages/SubscriptionsPage.vue'),
  '/nodes': () => import('@/pages/NodesPage.vue'),
  '/collections': () => import('@/pages/CollectionsPage.vue'),
  '/rules': () => import('@/pages/RulesPage.vue'),
  '/dns': () => import('@/pages/DNSPage.vue'),
  '/config': () => import('@/pages/ConfigPage.vue'),
  '/logs': () => import('@/pages/LogsPage.vue'),
  '/settings': () => import('@/pages/SettingsPage.vue'),
};

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: AppLayout, children: Object.entries(pages).map(([path, component]) => ({ path: path === '/' ? '' : path.slice(1), component })) },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
});
