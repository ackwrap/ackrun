import { ref } from "vue";
import { ApiRequestError } from "@/services/api";
import { sshApi } from "@/services/sshApi";
import type { SSHDeviceDetails, SSHHost } from "@/services/sshTypes";
import { errorMessage } from "../advanced/advancedUi";

interface SSHHostDetailsOptions {
  applyHostKeyError: (host: SSHHost, error: unknown) => boolean;
  notifyError: (message: string) => void;
}

export function useSSHHostDetails(options: SSHHostDetailsOptions) {
  const host = ref<SSHHost | null>(null);
  const details = ref<SSHDeviceDetails | null>(null);
  const error = ref("");
  const loadingID = ref(0);
  let requestGeneration = 0;
  let requestController: AbortController | null = null;
  let activeCleanup: (() => Promise<void>) | null = null;
  let retryAfterTrust = 0;
  let disposed = false;

  function open(item: SSHHost) {
    if (loadingID.value) return;
    host.value = item;
    details.value = null;
    error.value = "";
    retryAfterTrust = 0;
    void collect(item);
  }

  async function collect(item: SSHHost) {
    const generation = ++requestGeneration;
    requestController?.abort();
    requestController = null;
    if (activeCleanup) await activeCleanup();
    if (
      disposed ||
      generation !== requestGeneration ||
      host.value?.id !== item.id
    )
      return;
    loadingID.value = item.id;
    error.value = "";
    let releaseSession: (() => Promise<void>) | null = null;
    let controller: AbortController | null = null;
    try {
      const session = await sshApi.createSession(item.id, 80, 24);
      let released = false;
      releaseSession = async () => {
        if (released) return;
        released = true;
        if (activeCleanup === releaseSession) activeCleanup = null;
        try {
          await sshApi.closeSession(session.session_id);
        } catch (cause) {
          if (
            !disposed &&
            (!(cause instanceof ApiRequestError) ||
              cause.code !== "SSH_SESSION_NOT_FOUND")
          ) {
            options.notifyError(
              `设备详情临时会话回收失败：${errorMessage(cause)}`,
            );
          }
        }
      };
      activeCleanup = releaseSession;
      if (
        disposed ||
        generation !== requestGeneration ||
        host.value?.id !== item.id
      ) {
        await releaseSession();
        return;
      }
      controller = new AbortController();
      requestController = controller;
      const result = await sshApi.getSessionDetails(
        session.session_id,
        session.sftp_token,
        controller.signal,
      );
      if (
        disposed ||
        generation !== requestGeneration ||
        host.value?.id !== item.id
      )
        return;
      details.value = result;
    } catch (cause) {
      if (disposed || generation !== requestGeneration) return;
      if (cause instanceof DOMException && cause.name === "AbortError") return;
      if (options.applyHostKeyError(item, cause)) {
        retryAfterTrust = item.id;
        error.value = "请先核对并确认远端 Host Key，确认后将自动重新读取。";
      } else {
        error.value = `读取设备详情失败：${errorMessage(cause)}`;
      }
    } finally {
      if (requestController === controller) requestController = null;
      if (releaseSession) await releaseSession();
      if (!disposed && generation === requestGeneration) loadingID.value = 0;
    }
  }

  function retry() {
    if (host.value && !loadingID.value) {
      details.value = null;
      void collect(host.value);
    }
  }

  function retryTrustedHost(hostID: number, refreshed?: SSHHost) {
    if (retryAfterTrust !== hostID || host.value?.id !== hostID) return;
    retryAfterTrust = 0;
    void collect(refreshed || host.value);
  }

  function close() {
    requestGeneration++;
    retryAfterTrust = 0;
    requestController?.abort();
    requestController = null;
    const cleanup = activeCleanup;
    activeCleanup = null;
    if (cleanup) void cleanup();
    loadingID.value = 0;
    host.value = null;
    details.value = null;
    error.value = "";
  }

  function dispose() {
    disposed = true;
    requestGeneration++;
    requestController?.abort();
    requestController = null;
    const cleanup = activeCleanup;
    activeCleanup = null;
    if (cleanup) void cleanup();
  }

  return {
    detailsHost: host,
    deviceDetails: details,
    detailsError: error,
    loadingDetailsID: loadingID,
    openDetails: open,
    retryDetails: retry,
    retryDetailsAfterTrust: retryTrustedHost,
    closeDetails: close,
    disposeDetails: dispose,
  };
}
