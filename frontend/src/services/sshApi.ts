import { ApiRequestError, request } from "./api";
import { apiTokenRequired, authenticatedFetch } from "./apiAuth";
import type {
  SSHActionResponse,
  SSHConnectionTestResult,
  SSHCredential,
  SSHCredentialRequest,
  SSHHost,
  SSHHostImportResponse,
  SSHHostKey,
  SSHHostRequest,
  SSHHostShareResponse,
  SSHDeviceDetails,
  SSHMonitorSnapshot,
  SSHSingboxDeployRequest,
  SSHSingboxDeployResponse,
  SSHSFTPListResponse,
  SSHSFTPTextFile,
  SSHSessionCreateResponse,
} from "./sshTypes";

const sftpEndpoint = (sessionID: string, suffix = "") =>
  `/advanced/ssh/sessions/${encodeURIComponent(sessionID)}/sftp${suffix}`;

async function parseSFTPError(
  response: Response | XMLHttpRequest,
): Promise<never> {
  let payload: any = null;
  try {
    payload = JSON.parse(
      response instanceof Response ? await response.text() : response.responseText,
    );
  } catch {
    // Keep the fallback below for non-JSON proxy and transport errors.
  }
  const status = response.status;
  throw new ApiRequestError(
    payload?.error?.message || `SFTP 请求失败 (${status})`,
    payload?.error?.code || "SSH_SFTP_FAILED",
    payload?.error?.details,
    status,
  );
}

export const sshApi = {
  getHosts: () => request<SSHHost[]>("/advanced/ssh/hosts"),
  getHost: (id: number) => request<SSHHost>(`/advanced/ssh/hosts/${id}`),
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
  shareHost: (id: number, password: string) =>
    request<SSHHostShareResponse>(`/advanced/ssh/hosts/${id}/share`, {
      method: "POST",
      body: JSON.stringify({ password }),
    }),
  shareHosts: (hostIDs: number[], password: string) =>
    request<SSHHostShareResponse>("/advanced/ssh/hosts/share", {
      method: "POST",
      body: JSON.stringify({ host_ids: hostIDs, password }),
    }),
  importHost: (code: string, password: string) =>
    request<SSHHostImportResponse>("/advanced/ssh/hosts/import", {
      method: "POST",
      body: JSON.stringify({ code, password }),
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
  getSessionMonitor: (
    sessionID: string,
    token: string,
    signal?: AbortSignal,
  ) =>
    request<SSHMonitorSnapshot>(
      `/advanced/ssh/sessions/${encodeURIComponent(sessionID)}/monitor`,
      { headers: { "X-SSH-Session-Token": token }, signal },
    ),
  getSessionDetails: (
    sessionID: string,
    token: string,
    signal?: AbortSignal,
  ) =>
    request<SSHDeviceDetails>(
      `/advanced/ssh/sessions/${encodeURIComponent(sessionID)}/details`,
      { headers: { "X-SSH-Session-Token": token }, signal },
    ),
  deploySingbox: (hostID: number, body: SSHSingboxDeployRequest) =>
    request<SSHSingboxDeployResponse>(
      `/advanced/ssh/hosts/${hostID}/sing-box/deploy`,
      { method: "POST", body: JSON.stringify(body) },
    ),
  listSFTP: (sessionID: string, token: string, path: string) =>
    request<SSHSFTPListResponse>(
      `${sftpEndpoint(sessionID)}?path=${encodeURIComponent(path)}`,
      { headers: { "X-SSH-Session-Token": token } },
    ),
  createSFTPDirectory: (sessionID: string, token: string, path: string) =>
    request<SSHActionResponse>(sftpEndpoint(sessionID, "/mkdir"), {
      method: "POST",
      headers: { "X-SSH-Session-Token": token },
      body: JSON.stringify({ path }),
    }),
  createSFTPFile: (sessionID: string, token: string, path: string) =>
    request<SSHActionResponse>(sftpEndpoint(sessionID, "/touch"), {
      method: "POST",
      headers: { "X-SSH-Session-Token": token },
      body: JSON.stringify({ path }),
    }),
  renameSFTP: (
    sessionID: string,
    token: string,
    oldPath: string,
    newPath: string,
  ) =>
    request<SSHActionResponse>(sftpEndpoint(sessionID, "/rename"), {
      method: "POST",
      headers: { "X-SSH-Session-Token": token },
      body: JSON.stringify({ old_path: oldPath, new_path: newPath }),
    }),
  copySFTP: (
    sessionID: string,
    token: string,
    sourcePath: string,
    targetPath: string,
  ) =>
    request<SSHActionResponse>(sftpEndpoint(sessionID, "/copy"), {
      method: "POST",
      headers: { "X-SSH-Session-Token": token },
      body: JSON.stringify({ source_path: sourcePath, target_path: targetPath }),
    }),
  readSFTPText: (sessionID: string, token: string, path: string) =>
    request<SSHSFTPTextFile>(
      `${sftpEndpoint(sessionID, "/text")}?path=${encodeURIComponent(path)}`,
      { headers: { "X-SSH-Session-Token": token } },
    ),
  writeSFTPText: (
    sessionID: string,
    token: string,
    path: string,
    content: string,
    expectedSHA256: string,
  ) =>
    request<SSHSFTPTextFile>(sftpEndpoint(sessionID, "/text"), {
      method: "PUT",
      headers: { "X-SSH-Session-Token": token },
      body: JSON.stringify({
        path,
        content,
        expected_sha256: expectedSHA256,
      }),
    }),
  deleteSFTP: (
    sessionID: string,
    token: string,
    path: string,
    recursive: boolean,
  ) =>
    request<SSHActionResponse>(sftpEndpoint(sessionID), {
      method: "DELETE",
      headers: { "X-SSH-Session-Token": token },
      body: JSON.stringify({ path, recursive }),
    }),
  downloadSFTP: async (
    sessionID: string,
    token: string,
    path: string,
  ): Promise<Blob> => {
    const response = await authenticatedFetch(
      `/api/v1${sftpEndpoint(sessionID, "/download")}?path=${encodeURIComponent(path)}`,
      { headers: { "X-SSH-Session-Token": token } },
    );
    if (!response.ok) return parseSFTPError(response);
    return response.blob();
  },
  uploadSFTP: (
    sessionID: string,
    token: string,
    path: string,
    file: File,
    overwrite: boolean,
    onProgress: (progress: number) => void,
  ) =>
    new Promise<SSHActionResponse>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open(
        "POST",
        `/api/v1${sftpEndpoint(sessionID, "/upload")}?path=${encodeURIComponent(path)}&overwrite=${overwrite}`,
      );
      xhr.withCredentials = true;
      xhr.setRequestHeader("Content-Type", "application/octet-stream");
      xhr.setRequestHeader("X-SSH-Session-Token", token);
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) {
          onProgress(Math.round((event.loaded / event.total) * 100));
        }
      };
      xhr.onerror = () => reject(new Error("SFTP 上传网络连接失败"));
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            resolve(JSON.parse(xhr.responseText) as SSHActionResponse);
          } catch {
            reject(new Error("SFTP 上传响应无效"));
          }
          return;
        }
        if (xhr.status === 401) apiTokenRequired.value = true;
        void parseSFTPError(xhr).catch(reject);
      };
      xhr.send(file);
    }),
};
