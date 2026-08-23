import { request } from "./api";
import type {
  SSHActionResponse,
  SSHConnectionTestResult,
  SSHCredential,
  SSHCredentialRequest,
  SSHHost,
  SSHHostKey,
  SSHHostRequest,
  SSHSessionCreateResponse,
} from "./sshTypes";

export const sshApi = {
  getHosts: () => request<SSHHost[]>("/advanced/ssh/hosts"),
  createHost: (body: SSHHostRequest) =>
    request<SSHHost>("/advanced/ssh/hosts", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateHost: (id: number, body: SSHHostRequest) =>
    request<SSHHost>(`/advanced/ssh/hosts/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteHost: (id: number) =>
    request<SSHActionResponse>(`/advanced/ssh/hosts/${id}`, {
      method: "DELETE",
    }),
  testHost: (id: number) =>
    request<SSHConnectionTestResult>(`/advanced/ssh/hosts/${id}/test`, {
      method: "POST",
    }),
  getHostKey: (id: number) =>
    request<SSHHostKey>(`/advanced/ssh/hosts/${id}/host-key`),
  trustHostKey: (
    id: number,
    body: { challenge_id: string; fingerprint_sha256: string },
  ) =>
    request<SSHHostKey>(`/advanced/ssh/hosts/${id}/host-key/trust`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  rotateHostKey: (
    id: number,
    body: { challenge_id: string; fingerprint_sha256: string },
  ) =>
    request<SSHHostKey>(`/advanced/ssh/hosts/${id}/host-key/rotate`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  deleteHostKey: (id: number) =>
    request<SSHActionResponse>(`/advanced/ssh/hosts/${id}/host-key`, {
      method: "DELETE",
    }),
  getCredentials: () => request<SSHCredential[]>("/advanced/ssh/credentials"),
  createCredential: (body: SSHCredentialRequest) =>
    request<SSHCredential>("/advanced/ssh/credentials", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateCredential: (id: number, body: SSHCredentialRequest) =>
    request<SSHCredential>(`/advanced/ssh/credentials/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteCredential: (id: number) =>
    request<SSHActionResponse>(`/advanced/ssh/credentials/${id}`, {
      method: "DELETE",
    }),
  createSession: (id: number, columns: number, rows: number) =>
    request<SSHSessionCreateResponse>(`/advanced/ssh/hosts/${id}/sessions`, {
      method: "POST",
      body: JSON.stringify({ columns, rows }),
    }),
  closeSession: (sessionID: string) =>
    request<SSHActionResponse>(
      `/advanced/ssh/sessions/${encodeURIComponent(sessionID)}`,
      { method: "DELETE" },
    ),
};
