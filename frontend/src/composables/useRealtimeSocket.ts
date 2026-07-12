import { onBeforeUnmount, onMounted, shallowRef, type Ref } from 'vue';
import type { WSEvent } from '@/services/types';

export function useRealtimeSocket(onEvent: (event: WSEvent) => void): { connected: Ref<boolean> } {
  const connected = shallowRef(false);
  let socket: WebSocket | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let closed = false;
  const connect = () => {
    if (closed) return;
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(`${protocol}//${location.host}/api/v1/realtime/ws`);
    socket.onopen = () => { connected.value = true; };
    socket.onmessage = event => { try { onEvent(JSON.parse(event.data) as WSEvent); } catch { /* ignore malformed events */ } };
    socket.onerror = error => console.error('[WS] error', error);
    socket.onclose = () => { connected.value = false; if (!closed) timer = setTimeout(connect, 2000); };
  };
  onMounted(connect);
  onBeforeUnmount(() => { closed = true; if (timer) clearTimeout(timer); socket?.close(); });
  return { connected };
}

export function useRealtime() {
  const listeners = new Map<string, Set<(data: unknown) => void>>();
  const on = (type: string, fn: (data: unknown) => void) => {
    if (!listeners.has(type)) listeners.set(type, new Set());
    listeners.get(type)!.add(fn);
    return () => listeners.get(type)?.delete(fn);
  };
  const emit = (event: WSEvent) => listeners.get(event.type)?.forEach(fn => fn(event.data));
  return { on, emit };
}
