import { request } from "./api";
import type {
  AdvancedAccessLogParams,
  AdvancedAccessLogResponse,
  AdvancedActionResponse,
  AdvancedHealthResponse,
  AdvancedSettings,
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

  getSettings: () => request<AdvancedSettings>("/advanced/settings"),
  updateSettings: (body: AdvancedSettings) =>
    request<AdvancedSettings>("/advanced/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
};
