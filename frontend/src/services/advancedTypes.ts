export type AdvancedTargetType = "node" | "collection" | "direct";

export interface AdvancedRouteRef {
  type: AdvancedTargetType;
  subscription_id?: number;
  node_uid?: string;
  collection_id?: number;
}

export interface AdvancedTargetFields {
  target_type: AdvancedTargetType;
  target_subscription_id?: number;
  target_node_uid?: string;
  target_collection_id?: number;
  fallback_type: AdvancedTargetType;
  fallback_subscription_id?: number;
  fallback_node_uid?: string;
  fallback_collection_id?: number;
}

export interface PlatformRoute extends AdvancedTargetFields {
  id: number;
  name: string;
  platform: string;
  enabled: boolean;
  priority: number;
  inbound_exposure_ids: number[];
  source_cidrs: string[];
  domains: string[];
  domain_suffixes: string[];
  domain_keywords: string[];
  destination_cidrs: string[];
  created_at?: number;
  updated_at?: number;
}

export type PlatformRouteRequest = Omit<
  PlatformRoute,
  "id" | "created_at" | "updated_at"
>;

export type SessionLeaseStatus = "active" | "disabled" | "expired" | string;

export interface SessionLease extends AdvancedTargetFields {
  id: number;
  name: string;
  enabled: boolean;
  client_cidr: string;
  inbound_exposure_ids: number[];
  platform_route_id?: number;
  expires_at: number;
  status?: SessionLeaseStatus;
  created_at?: number;
  updated_at?: number;
}

export interface SessionLeaseRequest extends AdvancedTargetFields {
  name: string;
  enabled: boolean;
  client_cidr: string;
  inbound_exposure_ids: number[];
  platform_route_id?: number;
  expires_at: number;
}

export type AdvancedHealthStatus =
  | "healthy"
  | "unhealthy"
  | "circuit_open"
  | "unknown"
  | string;

export interface AdvancedHealthState {
  target_key: string;
  target_type: string;
  target_ref: string;
  display_name: string;
  status: AdvancedHealthStatus;
  latency_ms: number;
  consecutive_failures: number;
  consecutive_successes: number;
  circuit_open_until: number;
  last_error: string;
  last_checked_at: number;
  updated_at: number;
}

export interface AdvancedHealthEvent {
  id: number;
  target_key: string;
  target_type: string;
  display_name: string;
  event_type: string;
  message: string;
  latency_ms: number;
  created_at: number;
}

export interface AdvancedSettings {
  routing_enabled: boolean;
  leases_enabled: boolean;
  health_enabled: boolean;
  health_interval_seconds: number;
  health_timeout_seconds: number;
  failure_threshold: number;
  recovery_threshold: number;
  circuit_open_seconds: number;
  access_logs_enabled: boolean;
  access_log_retention_days: number;
  access_log_max_entries: number;
  access_log_privacy_mode: "strict" | "balanced";
}

export interface AdvancedHealthResponse {
  settings: Partial<AdvancedSettings>;
  states: AdvancedHealthState[];
  events: AdvancedHealthEvent[];
  running: boolean;
}

export interface AdvancedAccessLog {
  id: number;
  core_event_id: string;
  event_time: number;
  network: string;
  inbound: string;
  source_hash: string;
  destination_summary: string;
  domain_summary: string;
  outbound_label: string;
  platform: string;
  platform_route_id?: number;
  session_lease_id?: number;
  decision: string;
  error_summary: string;
  created_at: number;
}

export interface AdvancedAccessLogParams {
  page?: number;
  page_size?: number;
  platform?: string;
  decision?: string;
  keyword?: string;
}

export interface AdvancedAccessLogResponse {
  items: AdvancedAccessLog[];
  total: number;
  page: number;
  page_size: number;
}

export interface AdvancedActionResponse {
  success: boolean;
  message: string;
}

export type AlertChannelType = "webhook" | "telegram" | "email";
export type AlertEventType =
  | "circuit_open"
  | "recovered"
  | "subscription_failed";

export interface AlertChannelConfig {
  webhook_allow_private?: boolean;
  telegram_chat_id?: string;
  smtp_host?: string;
  smtp_port?: number;
  smtp_username?: string;
  smtp_from?: string;
  smtp_recipients?: string[];
  smtp_tls_mode?: "starttls" | "tls" | "none";
}

export interface AlertChannel {
  id: number;
  name: string;
  type: AlertChannelType;
  enabled: boolean;
  config: AlertChannelConfig;
  destination: string;
  has_secret: boolean;
  last_status: "never" | "success" | "failed" | string;
  last_error: string;
  last_delivered_at: number;
  created_at: number;
  updated_at: number;
}

export interface AlertChannelRequest {
  name: string;
  type: AlertChannelType;
  enabled: boolean;
  config: AlertChannelConfig;
  secret?: string;
}

export interface AlertRule {
  id: number;
  name: string;
  enabled: boolean;
  event_types: AlertEventType[];
  channel_ids: number[];
  cooldown_minutes: number;
  created_at: number;
  updated_at: number;
}

export interface AlertRuleRequest {
  name: string;
  enabled: boolean;
  event_types: AlertEventType[];
  channel_ids: number[];
  cooldown_minutes: number;
}

export interface AlertDelivery {
  id: number;
  rule_id?: number;
  channel_id?: number;
  channel_name: string;
  channel_type: AlertChannelType;
  event_type: AlertEventType | "test" | string;
  event_title: string;
  success: boolean;
  status_code: number;
  error: string;
  is_test: boolean;
  delivered_at: number;
}

export interface AlertDeliveryParams {
  page?: number;
  page_size?: number;
  channel_id?: number;
  event_type?: string;
  status?: "success" | "failed" | "";
}

export interface AlertDeliveryPage {
  items: AlertDelivery[];
  total: number;
  page: number;
  page_size: number;
}

export type NodeExposureInboundType = "http" | "socks" | "mixed";

export interface NodeExposure {
  id: number;
  name: string;
  subscription_id: number;
  node_uid: string;
  inbound_type: NodeExposureInboundType;
  listen: string;
  listen_port: number;
  username: string;
  has_password: boolean;
  enabled: boolean;
  created_at: number;
  updated_at: number;
  node_name: string;
  node_type: string;
  subscription_name: string;
  node_exists: boolean;
  node_enabled: boolean;
}

export interface NodeExposureRequest {
  name: string;
  subscription_id: number;
  node_uid: string;
  inbound_type: NodeExposureInboundType;
  listen: string;
  listen_port: number;
  username: string;
  password?: string;
  clear_password?: boolean;
  enabled: boolean;
}
