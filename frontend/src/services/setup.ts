import { request } from "./api";

export interface SetupStatus {
  status: "idle" | "running" | "succeeded" | "failed";
  stage: string;
  error?: string;
  configured: boolean;
  supported: boolean;
  has_existing_config: boolean;
  can_retry: boolean;
}

export const setupApi = {
  status: () => request<SetupStatus>("/setup"),
  start: (subscriptionURL: string) =>
    request<SetupStatus>("/setup", {
      method: "POST",
      body: JSON.stringify({ subscription_url: subscriptionURL }),
    }),
};
