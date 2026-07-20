import { onBeforeUnmount, onMounted, shallowRef, type Ref } from "vue";
import type { WSEvent } from "@/services/types";

type RealtimeListener = (event: WSEvent) => void;

const connected = shallowRef(false);
const listeners = new Set<RealtimeListener>();
let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
let mountedConsumers = 0;

function scheduleReconnect() {
  if (mountedConsumers === 0 || reconnectTimer) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = undefined;
    connect();
  }, 2000);
}

function connect() {
  if (
    mountedConsumers === 0 ||
    socket?.readyState === WebSocket.CONNECTING ||
    socket?.readyState === WebSocket.OPEN
  )
    return;

  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  const nextSocket = new WebSocket(
    `${protocol}//${location.host}/api/v1/realtime/ws`,
  );
  socket = nextSocket;
  nextSocket.onopen = () => {
    if (socket === nextSocket) connected.value = true;
  };
  nextSocket.onmessage = (event) => {
    if (socket !== nextSocket) return;
    try {
      const parsed = JSON.parse(event.data) as WSEvent;
      listeners.forEach((listener) => {
        try {
          listener(parsed);
        } catch (error) {
          console.error("[WS] listener error", error);
        }
      });
    } catch {
      // Ignore malformed events without dropping the socket.
    }
  };
  nextSocket.onerror = (error) => console.error("[WS] error", error);
  nextSocket.onclose = () => {
    if (socket !== nextSocket) return;
    socket = null;
    connected.value = false;
    scheduleReconnect();
  };
}

function releaseConnection() {
  if (mountedConsumers > 0) return;
  if (reconnectTimer) clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
  const currentSocket = socket;
  socket = null;
  connected.value = false;
  currentSocket?.close();
}

export function useRealtimeSocket(onEvent: (event: WSEvent) => void): {
  connected: Ref<boolean>;
} {
  const listener: RealtimeListener = (event) => onEvent(event);
  onMounted(() => {
    listeners.add(listener);
    mountedConsumers++;
    connect();
  });
  onBeforeUnmount(() => {
    listeners.delete(listener);
    mountedConsumers = Math.max(0, mountedConsumers - 1);
    releaseConnection();
  });
  return { connected };
}

export function useRealtime() {
  const listeners = new Map<string, Set<(data: unknown) => void>>();
  const on = (type: string, fn: (data: unknown) => void) => {
    if (!listeners.has(type)) listeners.set(type, new Set());
    listeners.get(type)!.add(fn);
    return () => listeners.get(type)?.delete(fn);
  };
  const emit = (event: WSEvent) =>
    listeners.get(event.type)?.forEach((fn) => fn(event.data));
  return { on, emit };
}
