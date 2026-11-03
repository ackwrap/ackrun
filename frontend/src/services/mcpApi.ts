import { request } from "./api";

export interface MCPSettings {
  enabled: boolean;
  token_configured: boolean;
  endpoint_path: string;
  token?: string;
}

export const mcpApi = {
  getSettings: () => request<MCPSettings>("/mcp/settings"),
  updateSettings: (body: { enabled: boolean; generate_token?: boolean }) =>
    request<MCPSettings>("/mcp/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
};
