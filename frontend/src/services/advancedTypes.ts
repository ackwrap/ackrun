export interface NodeExposure {
  id: number;
  name: string;
  subscription_id: number;
  node_uid: string;
  inbound_type: string;
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
  inbound_type: "mixed";
  listen: string;
  listen_port: number;
  username: string;
  password?: string;
  clear_password?: boolean;
  enabled: boolean;
}
