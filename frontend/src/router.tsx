import React, { Suspense } from 'react';
import { createBrowserRouter, Navigate } from 'react-router-dom';
import { AppLayout } from '@/components/layout/AppLayout';
import { ErrorBoundary } from '@/components/layout/ErrorBoundary';
import { PageTransition } from '@/components/layout/PageTransition';
import { PageSkeleton } from '@/components/layout/PageSkeleton';

const MonitorPage = React.lazy(() => import('@/pages/MonitorPage'));
const ControlPage = React.lazy(() => import('@/pages/ControlPage'));
const SubscriptionsPage = React.lazy(() => import('@/pages/SubscriptionsPage'));
const NodesPage = React.lazy(() => import('@/pages/NodesPage'));
const CollectionsPage = React.lazy(() => import('@/pages/CollectionsPage'));
const RulesPage = React.lazy(() => import('@/pages/RulesPage'));
const DNSPage = React.lazy(() => import('@/pages/DNSPage'));
const ConfigPage = React.lazy(() => import('@/pages/ConfigPage'));
const LogsPage = React.lazy(() => import('@/pages/LogsPage'));
const SettingsPage = React.lazy(() => import('@/pages/SettingsPage'));

function LazyPage({ children }: { children: React.ReactNode }) {
  return (
    <ErrorBoundary>
      <Suspense fallback={<PageSkeleton />}>
        <PageTransition>{children}</PageTransition>
      </Suspense>
    </ErrorBoundary>
  );
}

export const router = createBrowserRouter([
  { path: '/', element: <AppLayout><LazyPage><MonitorPage /></LazyPage></AppLayout> },
  { path: '/control', element: <AppLayout><LazyPage><ControlPage /></LazyPage></AppLayout> },
  { path: '/subscriptions', element: <AppLayout><LazyPage><SubscriptionsPage /></LazyPage></AppLayout> },
  { path: '/nodes', element: <AppLayout><LazyPage><NodesPage /></LazyPage></AppLayout> },
  { path: '/collections', element: <AppLayout><LazyPage><CollectionsPage /></LazyPage></AppLayout> },
  { path: '/rules', element: <AppLayout><LazyPage><RulesPage /></LazyPage></AppLayout> },
  { path: '/dns', element: <AppLayout><LazyPage><DNSPage /></LazyPage></AppLayout> },
  { path: '/config', element: <AppLayout><LazyPage><ConfigPage /></LazyPage></AppLayout> },
  { path: '/logs', element: <AppLayout><LazyPage><LogsPage /></LazyPage></AppLayout> },
  { path: '/settings', element: <AppLayout><LazyPage><SettingsPage /></LazyPage></AppLayout> },
  { path: '*', element: <Navigate to="/" replace /> },
]);
