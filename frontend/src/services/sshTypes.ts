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

export interface SSHActionResponse {
  success: boolean;
  message: string;
}
