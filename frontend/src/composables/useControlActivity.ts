import { onBeforeUnmount, ref, watch, type Ref } from "vue";
import {
  getClashClient,
  type ConnectionsSnapshot,
  type ProxyMap,
} from "@/services/clash";

export function useControlActivity(isRunning: Ref<boolean>) {
  const connections = ref<ConnectionsSnapshot | null>(null);
  const proxies = ref<ProxyMap | null>(null);
  const connectionError = ref("");
  const proxyError = ref("");
  const client = getClashClient();
  let generation = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let controller: AbortController | undefined;

  function stop() {
    generation++;
    clearTimeout(timer);
    controller?.abort();
    controller = undefined;
  }

  async function refresh() {
    if (!isRunning.value || controller) return;
    clearTimeout(timer);
    const run = generation;
    const request = new AbortController();
    controller = request;
    const [connectionResult, proxyResult] = await Promise.allSettled([
      client.getConnections(request.signal),
      client.getProxies(request.signal),
    ]);
    if (run !== generation) return;
    controller = undefined;
    connections.value =
      connectionResult.status === "fulfilled" ? connectionResult.value : null;
    connectionError.value =
      connectionResult.status === "rejected"
        ? connectionResult.reason?.message || "连接读取失败"
        : "";
    proxies.value =
      proxyResult.status === "fulfilled" ? proxyResult.value.proxies : null;
    proxyError.value =
      proxyResult.status === "rejected"
        ? proxyResult.reason?.message || "策略组读取失败"
        : "";
    timer = setTimeout(refresh, 2500);
  }

  watch(
    isRunning,
    () => {
      stop();
      connections.value = null;
      proxies.value = null;
      connectionError.value = "";
      proxyError.value = "";
      void refresh();
    },
    { immediate: true },
  );
  onBeforeUnmount(stop);

  return { connections, proxies, connectionError, proxyError, refresh };
}
