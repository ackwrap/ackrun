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

export interface ConfigBackup {
  id: number;
  config_name: string;
  file_name: string;
  path: string;
  backup_date: string;
  size_bytes: number;
  created_at: number;
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

export interface ToolLogEntry {
  id: number;
  time: number;
  level: "info" | "error" | string;
  tag: string;
  message: string;
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
}

export interface UpdateSettingsResponse {
  acceleration: string;
  custom_mirror_url: string;
}

export interface AppUpdateStatus {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  can_install: boolean;
  platform: string;
  architecture: string;
  release_url?: string;
  published_at?: string;
  asset_name?: string;
  message?: string;
  updating: boolean;
  update_error?: string;
}

export interface AppUpdateInstallResponse {
  success: boolean;
  message: string;
  version: string;
}

export type TrafficBypassRuleType =
  | "process_name"
  | "interface"
  | "ip_cidr"
  | "source_ip_cidr"
  | "domain_suffix";

export interface TrafficBypassRule {
  type: TrafficBypassRuleType;
  value: string;
}

export interface TrafficBypassSettings {
  rules: TrafficBypassRule[];
}

export interface LogSettings {
  level: string;
  timestamp: boolean;
}

export interface LogSettingsResponse {
  level: string;
  timestamp: boolean;
}

export interface ConnectivitySettings {
  test_url: string;
  interval_seconds: number;
}

export interface ConnectivityTarget {
  id: number;
  name: string;
  url: string;
  enabled: boolean;
  builtin: boolean;
  created_at: number;
  updated_at: number;
}

export interface ConnectivityTargetRequest {
  name: string;
  url: string;
  enabled: boolean;
}

export interface GeoIPFieldMapping {
  asnumber?: string;
  country?: string;
  country_code?: string;
  country_en?: string;
  prov?: string;
  prov_en?: string;
  city?: string;
  city_en?: string;
  district?: string;
  owner?: string;
  isp?: string;
  domain?: string;
  whois?: string;
  lat?: string;
  lng?: string;
  prefix?: string;
}

export interface GeoIPProvider {
  id: number;
  name: string;
  key: string;
  template: string;
  url?: string;
  ip_parameter?: string;
  mapping: GeoIPFieldMapping;
  enabled: boolean;
  is_default: boolean;
  builtin: boolean;
  created_at: number;
  updated_at: number;
}

export interface GeoIPProviderRequest {
  name: string;
  template: string;
  url?: string;
  ip_parameter?: string;
  mapping: GeoIPFieldMapping;
  enabled: boolean;
  is_default: boolean;
}

export interface GeoIPProviderTemplate {
  key: string;
  name: string;
  url?: string;
  ip_parameter?: string;
  mapping: GeoIPFieldMapping;
}

export interface GeoIPProviderListResponse {
  items: GeoIPProvider[];
  templates: GeoIPProviderTemplate[];
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
  clash_api_dashboard?: string;
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
  clash_api_dashboard?: string;
  cache_file_enabled: boolean;
  cache_file_store_fakeip: boolean;
  cache_file_store_dns: boolean;
}

export interface Dashboard {
  id: string;
  name: string;
  description: string;
  installed: boolean;
  selected: boolean;
  local_path?: string;
  updated_at?: number;
  current_version?: string;
  latest_version?: string;
  update_available: boolean;
  check_error?: string;
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

export interface NodeExitIPResponse {
  uid: string;
  node_name: string;
  node_ip: string;
  exit_ip: string;
  matched: boolean;
  resolution: "literal" | "alidns_doh";
  geo_provider: string;
  geo?: NodeTracerouteGeo;
  geo_error?: string;
}

export interface NodeTracerouteAttempt {
  success: boolean;
  ip?: string;
  hostname?: string;
  rtt_ms?: number;
  reached?: boolean;
  geo?: NodeTracerouteGeo;
  geo_error?: string;
}

export interface NodeTracerouteGeo {
  asnumber?: string;
  country?: string;
  country_en?: string;
  prov?: string;
  prov_en?: string;
  city?: string;
  city_en?: string;
  district?: string;
  owner?: string;
  isp?: string;
  domain?: string;
  whois?: string;
  lat?: number;
  lng?: number;
  prefix?: string;
  source?: string;
}

export interface NodeTracerouteHop {
  ttl: number;
  attempts: NodeTracerouteAttempt[];
}

export interface NodeTracerouteResponse {
  uid: string;
  node_name: string;
  target: string;
  resolved_ip: string;
  protocol: string;
  ip_version: number;
  reached: boolean;
  duration_ms: number;
  geo_provider: string;
  hops: NodeTracerouteHop[];
}

export interface NodeTracerouteStartResponse {
  trace_id: string;
  uid: string;
  status: "started";
  geo_provider: string;
}

export interface NodeTracerouteEvent {
  trace_id: string;
  uid: string;
  node_name: string;
  status: "started" | "hop" | "completed" | "failed" | "canceled";
  target: string;
  resolved_ip?: string;
  protocol?: string;
  ip_version?: number;
  reached: boolean;
  duration_ms: number;
  geo_provider?: string;
  hop?: NodeTracerouteHop;
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
  sync_progress?: number;
  sync_error: string;
  last_sync_at: number;
  local_path: string;
  available: boolean;
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

export interface GeoTagsResponse {
  type: "geoip" | "geosite";
  tags: string[];
  total: number;
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
  route_rule_id: number;
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
  route_rule_id: number;
  route_rule_ids: number[];
  node_uids: string[];
}

export interface StrategyItem {
  rule_id: number;
  name: string;
  priority: number;
  kind: "reject" | "direct" | "proxy" | "final";
  enabled: boolean;
  read_only: boolean;
  outbound_tag: string;
  collection?: ProxyCollectionWithNodes;
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
  tun_ipv4_address?: string;
  tun_ipv6_address?: string;
  log_level?: string;
}

export interface ConfigGenerateResponse {
  config: Record<string, any>;
  valid: boolean;
  error: string;
  file_path: string;
}

export interface ConfigApplyRequest {
  file_name: string;
  restart_core: boolean;
}

export interface CoreRestartSettings {
  mode: "off" | "daily" | "weekly";
  time: string;
  weekday: number;
}
