import { request } from "./api";

export interface SSHMCPSettings {
  enabled: boolean;
  token_configured: boolean;
  endpoint_path: string;
  token?: string;
}

export const sshMcpApi = {
  getSettings: () => request<SSHMCPSettings>("/advanced/ssh/mcp/settings"),
  updateSettings: (body: {
    enabled: boolean;
    token?: string;
    generate_token?: boolean;
  }) =>
    request<SSHMCPSettings>("/advanced/ssh/mcp/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
};
