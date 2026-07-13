import type {
  RuntimeResponse,
  InstallStateResponse,
  ConfigStatus,
  ConfigFileItem,
  ActionResponse,
  UpdateSettings,
  UpdateSettingsResponse,
  LogSettings,
  LogSettingsResponse,
  NTPSettings,
  NTPSettingsResponse,
  DNSSettings,
  DNSSettingsResponse,
  ExperimentalSettings,
  ExperimentalSettingsResponse,
  NodeFilter,
  NodeFilterRequest,
  Subscription,
  SubscriptionRequest,
  NodeListParams,
  NodeListResponse,
  NodeFacetsResponse,
  NodeBatchRenameRequest,
  NodeBatchResult,
  NodeTCPingResult,
  NodeFlagRequest,
  NodeFlagResponse,
  NodeFlagBatchItem,
  NodeFlagBatchResponse,
  NodeImportRequest,
  NodeImportPreviewResponse,
  NodeImportResponse,
  UserAgentOption,
  RouteRule,
  RouteRuleRequest,
  RouteRulePreviewResponse,
  RouteRuleSubscription,
  RouteRuleSubscriptionRequest,
  GeoAsset,
  GeoAssetRequest,
  GeoDomainsResponse,
  GeoLookupResponse,
  ProxyCollectionWithNodes,
  ProxyCollectionRequest,
  CollectionTestResponse,
  ConfigGenerateRequest,
  ConfigGenerateResponse,
  ConfigApplyRequest,
  CoreLogEntry,
  MaintenanceCheckResponse,
  CoreDiagnosticsResponse,
} from "./types";

const API_BASE = "/api/v1";

async function request<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const url = `${API_BASE}${endpoint}`;
  const res = await fetch(url, {
    headers: { "Content-Type": "application/json", ...options.headers },
    ...options,
  });
  if (!res.ok) {
    const err = await res
      .json()
      .catch(() => ({ error: { code: "UNKNOWN", message: res.statusText } }));
    throw new Error(err.error?.message || `API Error: ${res.status}`);
  }
  return res.json();
}

export const api = {
  getRuntime: () => request<RuntimeResponse>("/runtime"),

  getInstallerStatus: () =>
    request<InstallStateResponse>("/installer/sing-box"),
  install: () =>
    request<ActionResponse>("/installer/sing-box/install", { method: "POST" }),

  getConfigStatus: () => request<ConfigStatus>("/config/status"),
  getConfigFiles: () => request<ConfigFileItem[]>("/config/files"),
  generateDefaultConfig: () =>
    request<ActionResponse>("/config/default", { method: "POST" }),
  validateConfig: () =>
    request<ConfigStatus>("/config/validate", { method: "POST" }),
  updateRules: () =>
    request<ActionResponse>("/config/rules/update", { method: "POST" }),
  backupConfig: () =>
    request<ActionResponse>("/config/backup", { method: "POST" }),
  restoreConfig: () =>
    request<ActionResponse>("/config/restore", { method: "POST" }),

  startCore: () => request<ActionResponse>("/core/start", { method: "POST" }),
  stopCore: () => request<ActionResponse>("/core/stop", { method: "POST" }),
  restartCore: () =>
    request<ActionResponse>("/core/restart", { method: "POST" }),
  reloadConfig: () =>
    request<ActionResponse>("/core/reload-config", { method: "POST" }),
  closeConnections: () =>
    request<ActionResponse>("/core/close-connections", { method: "POST" }),
  flushCoreDNS: () =>
    request<ActionResponse>("/core/flush-core-dns", { method: "POST" }),
  flushFakeIP: () =>
    request<ActionResponse>("/core/flush-fakeip", { method: "POST" }),
  networkCheck: () =>
    request<MaintenanceCheckResponse>("/core/network-check", {
      method: "POST",
    }),
  getDiagnostics: () => request<CoreDiagnosticsResponse>("/core/diagnostics"),
  resetFirewall: () =>
    request<ActionResponse>("/core/reset-firewall", { method: "POST" }),
  flushDNS: () =>
    request<ActionResponse>("/core/flush-dns", { method: "POST" }),
  checkUpdate: () =>
    request<ActionResponse>("/core/check-update", { method: "POST" }),
  getCoreLogs: (limit = 500) =>
    request<CoreLogEntry[]>(`/logs/core?limit=${limit}`),
  clearCoreLogs: () =>
    request<ActionResponse>("/logs/core", { method: "DELETE" }),

  getUpdateSettings: () => request<UpdateSettingsResponse>("/settings/update"),
  setUpdateSettings: (body: UpdateSettings) =>
    request<ActionResponse>("/settings/update", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  getLogSettings: () => request<LogSettingsResponse>("/settings/log"),
  setLogSettings: (body: LogSettings) =>
    request<ActionResponse>("/settings/log", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  getNTPSettings: () => request<NTPSettingsResponse>("/settings/ntp"),
  setNTPSettings: (body: NTPSettings) =>
    request<ActionResponse>("/settings/ntp", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  getDNSSettings: () => request<DNSSettingsResponse>("/settings/dns"),
  setDNSSettings: (body: DNSSettings) =>
    request<ActionResponse>("/settings/dns", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  getInboundMode: () => request<{ mode: string }>("/settings/inbound-mode"),
  setInboundMode: (mode: string) =>
    request<ActionResponse>("/settings/inbound-mode", {
      method: "PUT",
      body: JSON.stringify({ mode }),
    }),
  getProxyMode: () => request<{ mode: string }>("/settings/proxy-mode"),
  setProxyMode: (mode: string) =>
    request<ActionResponse>("/settings/proxy-mode", {
      method: "PUT",
      body: JSON.stringify({ mode }),
    }),
  getExperimentalSettings: () =>
    request<ExperimentalSettingsResponse>("/settings/experimental"),
  setExperimentalSettings: (body: ExperimentalSettings) =>
    request<ActionResponse>("/settings/experimental", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  getNodeFilters: () => request<NodeFilter[]>("/settings/node-filters"),
  createNodeFilter: (body: NodeFilterRequest) =>
    request<NodeFilter>("/settings/node-filters", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateNodeFilter: (id: number, body: NodeFilterRequest) =>
    request<NodeFilter>(`/settings/node-filters/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteNodeFilter: (id: number) =>
    request<ActionResponse>(`/settings/node-filters/${id}`, {
      method: "DELETE",
    }),

  getSubscriptions: () => request<Subscription[]>("/subscriptions"),
  getSubscriptionUserAgents: () =>
    request<UserAgentOption[]>("/subscriptions/user-agents"),
  createSubscription: (body: SubscriptionRequest) =>
    request<Subscription>("/subscriptions", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateSubscription: (id: number, body: SubscriptionRequest) =>
    request<Subscription>(`/subscriptions/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteSubscription: (id: number) =>
    request<ActionResponse>(`/subscriptions/${id}`, { method: "DELETE" }),
  syncSubscription: (id: number) =>
    request<ActionResponse>(`/subscriptions/${id}/sync`, { method: "POST" }),
  syncAllSubscriptions: () =>
    request<ActionResponse>("/subscriptions/sync", { method: "POST" }),

  getNodes: (params: NodeListParams = {}) => {
    const search = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== "" && value !== null)
        search.set(key, String(value));
    });
    const query = search.toString();
    return request<NodeListResponse>(`/nodes${query ? `?${query}` : ""}`);
  },
  getNodeFacets: () => request<NodeFacetsResponse>("/nodes/facets"),
  previewImportNodes: (body: NodeImportRequest) =>
    request<NodeImportPreviewResponse>("/nodes/import/preview", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  importNodes: (body: NodeImportRequest) =>
    request<NodeImportResponse>("/nodes/import", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  tcpingNodes: (uids: string[]) =>
    request<NodeTCPingResult[]>("/nodes/tcping", {
      method: "POST",
      body: JSON.stringify({ uids }),
    }),
  addNodeEmoji: (uids: string[]) =>
    request<NodeBatchResult>("/nodes/add-emoji", {
      method: "POST",
      body: JSON.stringify({ uids }),
    }),
  inferNodeFlag: (body: NodeFlagRequest) =>
    request<NodeFlagResponse>("/nodes/flag", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  inferNodeFlags: (items: NodeFlagBatchItem[]) =>
    request<NodeFlagBatchResponse>("/nodes/flags", {
      method: "POST",
      body: JSON.stringify({ items }),
    }),
  batchRenameNodes: (body: NodeBatchRenameRequest) =>
    request<NodeBatchResult>("/nodes/batch-rename", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  batchDeleteNodes: (uids: string[]) =>
    request<NodeBatchResult>("/nodes/batch-delete", {
      method: "POST",
      body: JSON.stringify({ uids }),
    }),
  setNodeEnabled: (uid: string, value: boolean) =>
    request<ActionResponse>(`/nodes/${encodeURIComponent(uid)}/enabled`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),
  setNodePreferred: (uid: string, value: boolean) =>
    request<ActionResponse>(`/nodes/${encodeURIComponent(uid)}/preferred`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),

  getRouteRules: () => request<RouteRule[]>("/rules"),
  createRouteRule: (body: RouteRuleRequest) =>
    request<RouteRule>("/rules", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateRouteRule: (id: number, body: RouteRuleRequest) =>
    request<RouteRule>(`/rules/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteRouteRule: (id: number) =>
    request<ActionResponse>(`/rules/${id}`, { method: "DELETE" }),
  reorderRouteRules: (ids: number[]) =>
    request<ActionResponse>("/rules/reorder", {
      method: "POST",
      body: JSON.stringify({ ids }),
    }),
  previewRouteRules: () => request<RouteRulePreviewResponse>("/rules/preview"),
  getRouteRuleSubscriptions: () =>
    request<RouteRuleSubscription[]>("/rules/subscriptions"),
  createRouteRuleSubscription: (body: RouteRuleSubscriptionRequest) =>
    request<RouteRuleSubscription>("/rules/subscriptions", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateRouteRuleSubscription: (
    id: number,
    body: RouteRuleSubscriptionRequest,
  ) =>
    request<RouteRuleSubscription>(`/rules/subscriptions/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteRouteRuleSubscription: (id: number) =>
    request<ActionResponse>(`/rules/subscriptions/${id}`, { method: "DELETE" }),
  getRouteRuleSubscriptionContent: (id: number) =>
    request<unknown>(`/rules/subscriptions/${id}/content`),
  syncRouteRuleSubscription: (id: number) =>
    request<ActionResponse>(`/rules/subscriptions/${id}/sync`, {
      method: "POST",
    }),
  syncAllRouteRuleSubscriptions: () =>
    request<ActionResponse>("/rules/subscriptions/sync", { method: "POST" }),
  getGeoAssets: () => request<GeoAsset[]>("/rules/geo"),
  updateGeoAsset: (id: number, body: GeoAssetRequest) =>
    request<GeoAsset>(`/rules/geo/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  syncGeoAsset: (id: number) =>
    request<ActionResponse>(`/rules/geo/${id}/sync`, { method: "POST" }),
  syncAllGeoAssets: () =>
    request<ActionResponse>("/rules/geo/sync", { method: "POST" }),
  lookupGeo: (target: string, dnsServer?: string) =>
    request<GeoLookupResponse>(
      `/rules/geo/lookup?target=${encodeURIComponent(target)}${dnsServer ? `&dns_server=${encodeURIComponent(dnsServer)}` : ""}`,
    ),
  lookupGeositeDomains: (tag: string, limit = 100, offset = 0) =>
    request<GeoDomainsResponse>(
      `/rules/geo/domains?tag=${encodeURIComponent(tag)}&limit=${limit}&offset=${offset}`,
    ),

  generateConfig: (data: ConfigGenerateRequest) =>
    request<ConfigGenerateResponse>("/config/generate", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  getConfigGenerateRequest: () =>
    request<ConfigGenerateRequest>("/config/generate"),
  previewConfig: (defaultOutbound?: string) =>
    request<Record<string, any>>(
      `/config/preview${defaultOutbound ? `?default_outbound=${defaultOutbound}` : ""}`,
    ),
  applyConfig: (data: ConfigApplyRequest) =>
    request<ActionResponse>("/config/apply", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getProxyCollections: () =>
    request<ProxyCollectionWithNodes[]>("/collections"),
  getProxyCollection: (id: number) =>
    request<ProxyCollectionWithNodes>(`/collections/${id}`),
  createProxyCollection: (data: ProxyCollectionRequest) =>
    request<ProxyCollectionWithNodes>("/collections", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateProxyCollection: (id: number, data: ProxyCollectionRequest) =>
    request<ActionResponse>(`/collections/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteProxyCollection: (id: number) =>
    request<ActionResponse>(`/collections/${id}`, { method: "DELETE" }),
  toggleProxyCollectionEnabled: (id: number) =>
    request<ActionResponse>(`/collections/${id}/enabled`, { method: "PUT" }),
  testProxyCollection: (id: number) =>
    request<CollectionTestResponse>(`/collections/${id}/test`, {
      method: "POST",
    }),
};
