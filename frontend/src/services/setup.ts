import { request } from "./api";

export interface SetupStatus {
  status: "idle" | "running" | "succeeded" | "failed";
  stage: string;
  error?: string;
  configured: boolean;
  supported: boolean;
  has_existing_config: boolean;
  can_retry: boolean;
  options_locked: boolean;
}

export interface SetupOptions {
  ad_block: boolean;
  cn_outbound: "bypass" | "direct";
  default_outbound: "proxy" | "direct";
  app_routing: Record<string, "proxy" | "direct">;
  local_dns: string;
  proxy_dns: string;
  dns_strategy: "prefer_ipv4" | "prefer_ipv6" | "ipv4_only";
  direct_devices: string[];
  auto_start_core: boolean;
}

export const setupApi = {
  status: () => request<SetupStatus>("/setup"),
  options: () => request<SetupOptions>("/setup/options"),
  saveOptions: (options: SetupOptions) =>
    request<SetupOptions>("/setup/options", {
      method: "PUT",
      body: JSON.stringify(options),
    }),
  start: (subscriptionURL: string) =>
    request<SetupStatus>("/setup", {
      method: "POST",
      body: JSON.stringify({ subscription_url: subscriptionURL }),
    }),
};
