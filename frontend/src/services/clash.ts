// Clash API Client for sing-box
import { authenticatedFetch } from "./apiAuth";

export interface TrafficData {
  up: number;
  down: number;
}

export interface ProxyNode {
  name: string;
  type: string;
  udp: boolean;
  history: { time: string; delay: number; meanDelay?: number }[];
  now?: string;
  all?: string[];
}

export interface ProxyGroup {
  name: string;
  type: "Selector" | "URLTest" | "Fallback" | "LoadBalance" | "Relay";
  now: string;
  all: string[];
  history: { time: string; delay: number }[];
}

export type ProxyMap = Record<string, ProxyNode | ProxyGroup>;

export interface ClashConnection {
  id: string;
  metadata: {
    network?: string;
    type?: string;
    sourceIP?: string;
    destinationIP?: string;
    sourcePort?: string | number;
    destinationPort?: string | number;
    host?: string;
    process?: string;
    processPath?: string;
  };
  upload: number;
  download: number;
  start: string;
  chains?: string[];
  rule?: string;
}

export interface ConnectionsSnapshot {
  connections: ClashConnection[];
  downloadTotal: number;
  uploadTotal: number;
  memory?: number;
}

class ClashAPIClient {
  private baseURL: string;
  private secret: string;
  private trafficWS: WebSocket | null = null;
  private trafficCallback: ((data: TrafficData) => void) | null = null;
  private trafficErrorCallback: ((message: string) => void) | null = null;
  private trafficReconnectTimer: number | null = null;

  constructor(baseURL: string = "/api/v1/clash", secret: string = "") {
    this.baseURL = baseURL;
    this.secret = secret;
  }

  private getHeaders(): HeadersInit {
    const headers: HeadersInit = {
      "Content-Type": "application/json",
    };
    if (this.secret) {
      headers["Authorization"] = `Bearer ${this.secret}`;
    }
    return headers;
  }

  private async request<T>(
    path: string,
    options: RequestInit = {},
  ): Promise<T> {
    const url = `${this.baseURL}${path}`;
    const response = await authenticatedFetch(url, {
      ...options,
      headers: {
        ...this.getHeaders(),
        ...options.headers,
      },
    });

    const body = await response.text();
    if (!response.ok) {
      let err: {
        message?: string;
        error?: string | { message?: string };
      } | null = null;
      try {
        err = body ? JSON.parse(body) : null;
      } catch {
        // Preserve the HTTP status when the upstream error is not JSON.
      }
      const nestedMessage =
        typeof err?.error === "string" ? err.error : err?.error?.message;
      throw new Error(
        err?.message ||
          nestedMessage ||
          `Clash API error: ${response.status} ${response.statusText}`,
      );
    }
    if (!body.trim()) return undefined as T;
    return JSON.parse(body) as T;
  }

  // 获取流量统计（WebSocket）
  connectTraffic(
    callback: (data: TrafficData) => void,
    onError?: (message: string) => void,
  ): void {
    this.disconnectTraffic();
    this.trafficCallback = callback;
    this.trafficErrorCallback = onError || null;
    this.openTrafficSocket();
  }

  private openTrafficSocket(): void {
    const wsURL = new URL(`${this.baseURL}/traffic`, window.location.href);
    wsURL.protocol = wsURL.protocol === "https:" ? "wss:" : "ws:";
    if (this.secret) wsURL.searchParams.set("token", this.secret);

    const socket = new WebSocket(wsURL);
    this.trafficWS = socket;

    socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as TrafficData;
        this.trafficCallback?.(data);
      } catch (e) {
        console.error("Failed to parse traffic data:", e);
      }
    };

    socket.onerror = (error) => {
      console.error("Traffic WebSocket error:", error);
      this.trafficErrorCallback?.("实时流量连接失败");
    };

    socket.onclose = () => {
      if (this.trafficWS === socket) this.trafficWS = null;
      if (!this.trafficCallback) return;
      this.trafficErrorCallback?.("实时流量已断开，正在重连");
      this.trafficReconnectTimer = window.setTimeout(() => {
        this.trafficReconnectTimer = null;
        if (this.trafficCallback) this.openTrafficSocket();
      }, 3000);
    };
  }

  disconnectTraffic(): void {
    this.trafficCallback = null;
    this.trafficErrorCallback = null;
    if (this.trafficReconnectTimer !== null) {
      window.clearTimeout(this.trafficReconnectTimer);
      this.trafficReconnectTimer = null;
    }
    if (this.trafficWS) {
      const socket = this.trafficWS;
      this.trafficWS = null;
      socket.onclose = null;
      socket.close();
    }
  }

  // 获取所有代理
  async getProxies(signal?: AbortSignal): Promise<{ proxies: ProxyMap }> {
    return this.request("/proxies", { signal });
  }

  async selectProxy(group: string, name: string): Promise<void> {
    return this.request(`/proxies/${encodeURIComponent(group)}`, {
      method: "PUT",
      body: JSON.stringify({ name }),
    });
  }

  // 获取连接列表
  async getConnections(signal?: AbortSignal): Promise<ConnectionsSnapshot> {
    const data = await this.request<ConnectionsSnapshot>("/connections", {
      signal,
    });
    return { ...data, connections: data.connections || [] };
  }
}

// 单例
let clashClient: ClashAPIClient | null = null;

export function getClashClient(
  baseURL?: string,
  secret?: string,
): ClashAPIClient {
  if (!clashClient || baseURL || secret) {
    clashClient = new ClashAPIClient(baseURL, secret);
  }
  return clashClient;
}

export default ClashAPIClient;
