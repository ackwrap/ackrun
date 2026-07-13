export type RuntimeStatus =
  "not_installed" | "no_config" | "stopped" | "running" | "error";

export interface RuntimeResponse {
  status: RuntimeStatus;
  pid?: number;
  version?: string;
  platform?: string;
  proxy_port?: number;
}

export type InstallStatusType =
  "idle" | "downloading" | "extracting" | "done" | "failed";

export interface InstallStateResponse {
  status: InstallStatusType;
  version?: string;
  latest_version?: string;
  progress?: number;
  message?: string;
  error?: string;
}

export interface InstallerProgressData {
  percent: number;
  downloaded_bytes: number;
  total_bytes: number;
  speed_bps: number;
}

export interface ConfigStatus {
  has_config: boolean;
  valid: boolean;
  file_name?: string;
  updated_at?: number;
  error?: string;
}

export interface ConfigFileItem {
  name: string;
  path: string;
  active: boolean;
  size_bytes: number;
  updated_at: number;
  valid: boolean;
  error?: string;
}

export interface CoreStatus {
  status: "starting" | "running" | "stopping" | "stopped" | "error";
  pid: number;
  error?: string;
}

export interface CoreLogEntry {
  id: number;
  time: number;
  source: "stdout" | "stderr" | string;
  line: string;
}

export interface MaintenanceCheck {
  key: string;
  label: string;
  status: "pass" | "warn" | "fail";
  message: string;
}

export interface MaintenanceCheckResponse {
  success: boolean;
  checks: MaintenanceCheck[];
}

export interface CoreDiagnosticsResponse {
  generated_at: number;
  platform: string;
  architecture: string;
  version?: string;
  running: boolean;
  pid?: number;
  binary_path: string;
  config_path?: string;
  config_present: boolean;
  config_valid: boolean;
  network: MaintenanceCheckResponse;
  logs: {
    total: number;
    stdout: number;
    stderr: number;
    error_lines: number;
  };
}

export interface UpdateSettings {
  acceleration: string;
  custom_mirror_url?: string;
  github_token?: string;
  proxy_url?: string;
}

export interface UpdateSettingsResponse {
  acceleration: string;
  custom_mirror_url: string;
  github_token: string;
  proxy_url: string;
}

export interface LogSettings {
  level: string;
  timestamp: boolean;
}

export interface LogSettingsResponse {
  level: string;
  timestamp: boolean;
}

export interface NTPSettings {
  enabled: boolean;
  server?: string;
  server_port?: number;
  interval?: string;
  detour?: string;
}

export interface NTPSettingsResponse {
  enabled: boolean;
  server: string;
  server_port: number;
  interval: string;
  detour: string;
}

export interface DNSSettings {
  enabled: boolean;
  proxy_server?: string;
  direct_server?: string;
  resolver?: string;
  final?: string;
  strategy?: string;
  address_strategy?: string;
  disable_cache: boolean;
  disable_expire: boolean;
  independent_cache: boolean;
  reverse_mapping: boolean;
  client_subnet?: string;
  fakeip_enabled: boolean;
  fakeip_inet4_range?: string;
  fakeip_inet6_range?: string;
  route_cn: boolean;
  route_non_cn: boolean;
  block_ads: boolean;
}

export interface DNSSettingsResponse {
  enabled: boolean;
  proxy_server: string;
  direct_server: string;
  resolver: string;
  final: string;
  strategy: string;
  address_strategy: string;
  disable_cache: boolean;
  disable_expire: boolean;
  independent_cache: boolean;
  reverse_mapping: boolean;
  client_subnet: string;
  fakeip_enabled: boolean;
  fakeip_inet4_range: string;
  fakeip_inet6_range: string;
  route_cn: boolean;
  route_non_cn: boolean;
  block_ads: boolean;
}

export interface ExperimentalSettings {
  clash_api_enabled: boolean;
  clash_api_port: string;
  clash_api_secret?: string;
  clash_api_external_ui?: string;
  clash_api_external_ui_download_url?: string;
  cache_file_enabled: boolean;
  cache_file_store_fakeip: boolean;
  cache_file_store_dns: boolean;
}

export interface ExperimentalSettingsResponse {
  clash_api_enabled: boolean;
  clash_api_port: string;
  clash_api_secret?: string;
  clash_api_external_ui?: string;
  clash_api_external_ui_download_url?: string;
  cache_file_enabled: boolean;
  cache_file_store_fakeip: boolean;
  cache_file_store_dns: boolean;
}

export interface NodeFilter {
  id: number;
  name: string;
  target: "all" | "name" | "type" | "server" | "raw" | "raw_json" | string;
  pattern: string;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface NodeFilterRequest {
  name: string;
  target: string;
  pattern: string;
  enabled: boolean;
}

export interface ActionResponse {
  success: boolean;
  message: string;
}

export interface Subscription {
  id: number;
  name: string;
  url: string;
  user_agent: string;
  sync_interval_minutes: number;
  sync_mode: "off" | "daily" | "weekly" | "monthly" | string;
  sync_time: string;
  sync_weekday: number;
  sync_status: "updated" | "syncing" | "failed" | string;
  sync_progress: number;
  sync_timeout_seconds: number;
  node_count: number;
  traffic_used_bytes: number;
  traffic_total_bytes: number;
  expire_at: number;
  last_sync_at: number;
  created_at: number;
  updated_at: number;
}

export interface SubscriptionRequest {
  name: string;
  url: string;
  user_agent?: string;
  expire_at?: number;
  sync_interval_minutes?: number;
  sync_mode?: "off" | "daily" | "weekly" | "monthly";
  sync_time?: string;
  sync_weekday?: number;
  sync_timeout_seconds?: number;
}

export interface NodeItem {
  id: number;
  uid: string;
  subscription_id: number;
  subscription_name: string;
  name: string;
  name_overridden: boolean;
  type: string;
  server: string;
  server_port: number;
  raw: string;
  raw_json: string;
  enabled: boolean;
  preferred: boolean;
  latency_ms: number;
  status: string;
  last_test_at: number;
  test_latency_ms: number;
  test_success: boolean;
  created_at: number;
  updated_at: number;
}

export interface NodeListResponse {
  items: NodeItem[];
  total: number;
}

export interface NodeListParams {
  subscription_id?: number;
  keyword?: string;
  type?: string;
  status?: string;
  enabled?: boolean;
  preferred?: boolean;
  limit?: number;
  offset?: number;
}

export interface NodeFacetItem {
  value: string;
  label: string;
  count: number;
}

export interface NodeFacetsResponse {
  total: number;
  types: NodeFacetItem[];
  subscriptions: NodeFacetItem[];
}

export interface NodeBatchResult {
  success: number;
  failed: number;
}

export interface NodeTCPingResult {
  uid: string;
  success: boolean;
  latency_ms: number;
  error?: string;
}

export interface NodeFlagRequest {
  name: string;
  server?: string;
}

export interface NodeFlagResponse {
  flag: string;
}

export interface NodeFlagBatchItem {
  key: string;
  name: string;
  server?: string;
}

export interface NodeFlagBatchResponse {
  items: Array<{ key: string; flag: string }>;
}

export interface NodeBatchRenameRequest {
  uids: string[];
  mode: "lines" | "replace" | "prefix" | "suffix";
  names?: string[];
  find?: string;
  replace?: string;
  prefix?: string;
  suffix?: string;
}

export interface NodeImportRequest {
  content: string;
}

export interface NodeImportResponse {
  imported: number;
  subscription_id: number;
}

export interface NodeImportPreviewItem {
  name: string;
  type: string;
  server: string;
  server_port: number;
  uid: string;
  raw_json: string;
}

export interface NodeImportPreviewResponse {
  count: number;
  items: NodeImportPreviewItem[];
}

export interface UserAgentOption {
  label: string;
  value: string;
  description: string;
}

export interface RouteRule {
  id: number;
  name: string;
  enabled: boolean;
  priority: number;
  rule_type: string;
  values: string[];
  outbound: string;
  invert: boolean;
  system_key?: string;
  is_system: boolean;
  created_at: number;
  updated_at: number;
}

export interface RouteRuleRequest {
  name: string;
  enabled: boolean;
  priority: number;
  rule_type: string;
  values: string[];
  outbound: string;
  invert: boolean;
}

export interface RouteRulePreviewResponse {
  rules: Array<Record<string, unknown>>;
  rule_sets: Array<Record<string, unknown>>;
}

export interface RouteRuleSubscription {
  id: number;
  name: string;
  enabled: boolean;
  tag: string;
  url: string;
  format: string;
  use_proxy: boolean;
  sync_mode: string;
  sync_time: string;
  sync_weekday: number;
  sync_status: string;
  sync_progress: number;
  sync_error: string;
  last_sync_at: number;
  cached_path: string;
  cached_updated_at: number;
  created_at: number;
  updated_at: number;
}

export interface RouteRuleSubscriptionRequest {
  name: string;
  enabled: boolean;
  tag: string;
  url: string;
  format: string;
  use_proxy: boolean;
  sync_mode: string;
  sync_time: string;
  sync_weekday: number;
}

export interface GeoAsset {
  id: number;
  name: string;
  type: string;
  url: string;
  use_proxy: boolean;
  sync_mode: string;
  sync_time: string;
  sync_weekday: number;
  sync_status: string;
  sync_error: string;
  last_sync_at: number;
  local_path: string;
  cached_updated_at: number;
  created_at: number;
  updated_at: number;
}

export interface GeoAssetRequest {
  url: string;
  use_proxy: boolean;
  sync_mode: string;
  sync_time: string;
  sync_weekday: number;
}

export interface GeoLookupResponse {
  target: string;
  target_type: string;
  dns_server: string;
  resolved_ips: string[];
  geo_assets: Array<{
    type: string;
    name: string;
    ready: boolean;
    local_path: string;
    updated_at: number;
    error: string;
  }>;
  capabilities: string[];
  geoip_matches: string[];
  geosite_matches: string[];
  message: string;
}

export interface GeoDomainsResponse {
  tag: string;
  items: Array<{ type: string; value: string }>;
  suggestions: string[];
  total: number;
  limit: number;
  offset: number;
  ready: boolean;
  message: string;
}

export interface ApiError {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}

export interface WSEvent<T = unknown> {
  type: string;
  time: number;
  data: T;
}

export interface ProxyCollection {
  id: number;
  name: string;
  type: "selector" | "urltest";
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface ProxyCollectionWithNodes extends ProxyCollection {
  node_uids: string[];
  route_rule_ids: number[];
}

export interface ProxyCollectionRequest {
  name: string;
  type: "selector" | "urltest";
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  route_rule_ids: number[];
  node_uids: string[];
}

export interface CollectionTestNodeResult {
  uid: string;
  success: boolean;
  latency_ms: number;
  error?: string;
}

export interface CollectionTestResponse {
  collection_id: number;
  tested: number;
  available: number;
  fastest_uid?: string;
  fastest_latency?: number;
  error?: string;
  results: CollectionTestNodeResult[];
}

export interface ConfigGenerateRequest {
  default_outbound: string;
  inbound_listen?: string;
  inbound_port?: number;
  log_level?: string;
}

export interface ConfigGenerateResponse {
  config: Record<string, any>;
  valid: boolean;
  error: string;
  file_path: string;
}

export interface ConfigApplyRequest {
  restart_core: boolean;
}
