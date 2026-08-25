export type SSHCredentialAuthType = "password" | "private_key";
export type SSHConnectionMode = "direct" | "node_exposure";

export interface SSHCredential {
  id: number;
  name: string;
  auth_type: SSHCredentialAuthType;
  has_secret: boolean;
  key_fingerprint?: string;
  created_at: number;
  updated_at: number;
}

export interface SSHCredentialRequest {
  name: string;
  auth_type: SSHCredentialAuthType;
  secret: string;
  passphrase: string;
}

export interface SSHHost {
  id: number;
  name: string;
  group_name: string;
  host: string;
  port: number;
  username: string;
  credential_id: number;
  credential_name: string;
  connection_mode: SSHConnectionMode;
  node_exposure_id?: number;
  node_exposure_name?: string;
  terminal_type: string;
  enabled: boolean;
  tags: string[];
  notes: string;
  host_key_status: "unknown" | "trusted";
  last_status: "unknown" | "available" | "unavailable" | "host_key_pending";
  last_latency_ms: number;
  last_error_code: string;
  last_error_message: string;
  last_checked_at: number;
  created_at: number;
  updated_at: number;
}

export interface SSHHostRequest {
  name: string;
  group_name: string;
  host: string;
  port: number;
  username: string;
  credential_id: number;
  connection_mode: SSHConnectionMode;
  node_exposure_id?: number;
  terminal_type: string;
  enabled: boolean;
  tags: string[];
  notes: string;
}

export interface SSHHostShareResponse {
  code: string;
  host_name?: string;
  host_count: number;
}

export interface SSHHostImportResponse {
  success: boolean;
  message: string;
  host: SSHHost;
  hosts: SSHHost[];
  host_count: number;
  credential_count: number;
  converted_to_direct: boolean;
  converted_to_direct_count: number;
}

export interface SSHHostKey {
  host_id: number;
  key_type: string;
  fingerprint_sha256: string;
  first_seen_at: number;
  last_seen_at: number;
  updated_at: number;
}

export interface SSHHostKeyChallenge {
  challenge_id: string;
  key_type: string;
  fingerprint_sha256: string;
  trusted_fingerprint?: string;
  expires_at: number;
}

export interface SSHConnectionTestResult {
  success: boolean;
  latency_ms: number;
  handshake_ms: number;
  server_version: string;
  key_type: string;
  fingerprint_sha256: string;
  connection_mode: SSHConnectionMode;
}

export interface SSHSessionCreateResponse {
  session_id: string;
  attach_token: string;
  sftp_token: string;
  expires_at: number;
}

export interface SSHMonitorSnapshot {
  hostname: string;
  username: string;
  cpu_total: number;
  cpu_idle: number;
  memory_total_bytes: number;
  memory_available_bytes: number;
  network_received_bytes: number;
  network_transmitted_bytes: number;
  uptime_seconds: number;
  login_sessions: number;
  disks: SSHMonitorDisk[];
  collected_at: number;
}

export interface SSHDeviceDetails {
  cpu_model: string;
  cpu_cores: number;
  memory_total_bytes: number;
  memory_available_bytes: number;
  swap_total_bytes: number;
  swap_available_bytes: number;
  uptime_seconds: number;
  load_average_1: number;
  load_average_5: number;
  load_average_15: number;
  software: SSHSoftware[];
  collected_at: number;
}

export interface SSHSoftware {
  key: string;
  installed: boolean;
  version: string;
}

export interface SSHSingboxDeployRequest {
  server_address: string;
  vless_reality_enabled: boolean;
  vless_reality_port: number;
  reality_server_name: string;
  shadowsocks_enabled: boolean;
  shadowsocks_port: number;
  replace_existing_config: boolean;
}

export interface SSHSingboxDeployResponse {
  success: boolean;
  message: string;
  version: string;
  config_path: string;
  backup_path?: string;
  nodes: SSHSingboxNode[];
  client_config: Record<string, unknown>;
}

export interface SSHSingboxNode {
  type: string;
  name: string;
  listen_port: number;
  network: string;
  share_uri: string;
  client_outbound: Record<string, unknown>;
}

export interface SSHMonitorDisk {
  mount_point: string;
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  usage_percent: number;
}

export interface SSHSFTPEntry {
  name: string;
  path: string;
  is_dir: boolean;
  is_symlink: boolean;
  size: number;
  mode: string;
  modified_at: number;
}

export interface SSHSFTPListResponse {
  path: string;
  parent: string;
  home: string;
  entries: SSHSFTPEntry[];
}

export interface SSHSFTPTextFile {
  path: string;
  name: string;
  content: string;
  sha256: string;
  size: number;
  modified_at: number;
}

export interface SSHActionResponse {
  success: boolean;
  message: string;
}
