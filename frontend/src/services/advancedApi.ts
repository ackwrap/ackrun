import { request } from "./api";
import type {
  AdvancedAccessLogParams,
  AdvancedAccessLogResponse,
  AdvancedActionResponse,
  AdvancedHealthResponse,
  AdvancedSettings,
  AlertChannel,
  AlertChannelRequest,
  AlertDelivery,
  AlertDeliveryPage,
  AlertDeliveryParams,
  AlertRule,
  AlertRuleRequest,
  NodeExposure,
  NodeExposureRequest,
  PlatformRoute,
  PlatformRouteRequest,
  SessionLease,
  SessionLeaseRequest,
} from "./advancedTypes";

function queryString<T extends object>(params: T) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") {
      query.set(key, String(value));
    }
  });
  const value = query.toString();
  return value ? `?${value}` : "";
}

export const advancedApi = {
  getPlatformRoutes: () =>
    request<PlatformRoute[]>("/advanced/platform-routes"),
  createPlatformRoute: (body: PlatformRouteRequest) =>
    request<PlatformRoute>("/advanced/platform-routes", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updatePlatformRoute: (id: number, body: PlatformRouteRequest) =>
    request<PlatformRoute>(`/advanced/platform-routes/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deletePlatformRoute: (id: number) =>
    request<AdvancedActionResponse>(`/advanced/platform-routes/${id}`, {
      method: "DELETE",
    }),
  reorderPlatformRoutes: (ids: number[]) =>
    request<AdvancedActionResponse>("/advanced/platform-routes/reorder", {
      method: "POST",
      body: JSON.stringify({ ids }),
    }),

  getSessionLeases: () =>
    request<SessionLease[]>("/advanced/session-leases"),
  createSessionLease: (body: SessionLeaseRequest) =>
    request<SessionLease>("/advanced/session-leases", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateSessionLease: (id: number, body: SessionLeaseRequest) =>
    request<SessionLease>(`/advanced/session-leases/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteSessionLease: (id: number) =>
    request<AdvancedActionResponse>(`/advanced/session-leases/${id}`, {
      method: "DELETE",
    }),
  renewSessionLease: (id: number, durationMinutes: number) =>
    request<SessionLease>(`/advanced/session-leases/${id}/renew`, {
      method: "POST",
      body: JSON.stringify({ duration_minutes: durationMinutes }),
    }),

  getHealthScheduling: () =>
    request<AdvancedHealthResponse>("/advanced/health-scheduling"),
  runHealthScheduling: () =>
    request<AdvancedActionResponse>("/advanced/health-scheduling/run", {
      method: "POST",
    }),

  getAccessLogs: (params: AdvancedAccessLogParams) =>
    request<AdvancedAccessLogResponse>(
      `/advanced/access-logs${queryString(params)}`,
    ),
  clearAccessLogs: () =>
    request<AdvancedActionResponse>("/advanced/access-logs", {
      method: "DELETE",
    }),

  getNodeExposures: () =>
    request<NodeExposure[]>("/advanced/node-exposures"),
  createNodeExposure: (body: NodeExposureRequest) =>
    request<NodeExposure>("/advanced/node-exposures", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateNodeExposure: (id: number, body: NodeExposureRequest) =>
    request<NodeExposure>(`/advanced/node-exposures/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteNodeExposure: (id: number) =>
    request<AdvancedActionResponse>(`/advanced/node-exposures/${id}`, {
      method: "DELETE",
    }),

  getSettings: () => request<AdvancedSettings>("/advanced/settings"),
  updateSettings: (body: AdvancedSettings) =>
    request<AdvancedSettings>("/advanced/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  getAlertChannels: () =>
    request<AlertChannel[]>("/advanced/alerts/channels"),
  createAlertChannel: (body: AlertChannelRequest) =>
    request<AlertChannel>("/advanced/alerts/channels", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateAlertChannel: (id: number, body: AlertChannelRequest) =>
    request<AlertChannel>(`/advanced/alerts/channels/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteAlertChannel: (id: number) =>
    request<AdvancedActionResponse>(`/advanced/alerts/channels/${id}`, {
      method: "DELETE",
    }),
  testAlertChannel: (id: number) =>
    request<AlertDelivery>(`/advanced/alerts/channels/${id}/test`, {
      method: "POST",
    }),

  getAlertRules: () => request<AlertRule[]>("/advanced/alerts/rules"),
  createAlertRule: (body: AlertRuleRequest) =>
    request<AlertRule>("/advanced/alerts/rules", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateAlertRule: (id: number, body: AlertRuleRequest) =>
    request<AlertRule>(`/advanced/alerts/rules/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteAlertRule: (id: number) =>
    request<AdvancedActionResponse>(`/advanced/alerts/rules/${id}`, {
      method: "DELETE",
    }),

  getAlertDeliveries: (params: AlertDeliveryParams) =>
    request<AlertDeliveryPage>(
      `/advanced/alerts/deliveries${queryString(params)}`,
    ),
  clearAlertDeliveries: () =>
    request<AdvancedActionResponse>("/advanced/alerts/deliveries", {
      method: "DELETE",
    }),
};
