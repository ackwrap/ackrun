import { ref } from "vue";

export const apiTokenRequired = ref(false);

export async function authenticatedFetch(
  input: RequestInfo | URL,
  init: RequestInit = {},
): Promise<Response> {
  const response = await fetch(input, { credentials: "same-origin", ...init });
  if (response.status === 401) apiTokenRequired.value = true;
  return response;
}

export async function establishAPISession(token: string): Promise<void> {
  const response = await fetch("/api/v1/runtime", {
    credentials: "same-origin",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (response.ok) return;
  const body = await response.json().catch(() => null);
  throw new Error(body?.error?.message || `API Token 验证失败 (${response.status})`);
}
