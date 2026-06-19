import { useEffect, useRef, useCallback } from 'react';
import type { WSEvent } from '@/services/types';

export function useRealtimeSocket(onEvent: (event: WSEvent) => void) {
  const onEventRef = useRef(onEvent);
  useEffect(() => { onEventRef.current = onEvent; }, [onEvent]);

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let closed = false;
    let socket: WebSocket | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (closed) return;
      socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/realtime/ws`);

      socket.onopen = () => {
        console.log('[WS] connected');
      };

      socket.onmessage = (event) => {
        try {
          onEventRef.current(JSON.parse(event.data) as WSEvent);
        } catch { /* ignore */ }
      };

      socket.onclose = () => {
        if (!closed) {
          console.log('[WS] disconnected, reconnecting in 2s...');
          timer = setTimeout(connect, 2000);
        }
      };

      socket.onerror = (err) => {
        console.error('[WS] error', err);
      };
    };

    connect();
    return () => {
      closed = true;
      if (timer) clearTimeout(timer);
      socket?.close();
    };
  }, []);
}

export function useRealtime() {
  const listeners = useRef<Map<string, Set<(data: unknown) => void>>>(new Map());

  const on = useCallback((type: string, fn: (data: unknown) => void) => {
    if (!listeners.current.has(type)) listeners.current.set(type, new Set());
    listeners.current.get(type)!.add(fn);
    return () => { listeners.current.get(type)?.delete(fn); };
  }, []);

  const emit = useCallback((event: WSEvent) => {
    const fns = listeners.current.get(event.type);
    if (fns) fns.forEach(fn => fn(event.data));
  }, []);

  return { on, emit };
}