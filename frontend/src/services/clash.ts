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

export interface Connection {
  id: string;
  metadata: {
    network: string;
    type: string;
    sourceIP: string;
    destinationIP: string;
    sourcePort: string;
    destinationPort: string;
    host: string;
    dnsMode: string;
    processPath?: string;
  };
  upload: number;
  download: number;
  start: string;
  chains: string[];
  rule: string;
  rulePayload: string;
}

export interface Rule {
  type: string;
  payload: string;
  proxy: string;
}

export interface ClashConfig {
  port: number;
  "socks-port": number;
  "redir-port": number;
  "tproxy-port": number;
  "mixed-port": number;
  mode: "rule" | "global" | "direct";
  "log-level": string;
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
  async getProxies(): Promise<{
    proxies: Record<string, ProxyNode | ProxyGroup>;
  }> {
    return this.request("/proxies");
  }

  // 获取单个代理
  async getProxy(name: string): Promise<ProxyNode | ProxyGroup> {
    return this.request(`/proxies/${encodeURIComponent(name)}`);
  }

  // 切换节点（仅对 Selector 类型有效）
  async selectProxy(group: string, proxy: string): Promise<void> {
    await this.request(`/proxies/${encodeURIComponent(group)}`, {
      method: "PUT",
      body: JSON.stringify({ name: proxy }),
    });
  }

  // 测速
  async delayTest(
    proxy: string,
    testURL: string = "https://www.gstatic.com/generate_204",
    timeout: number = 5000,
  ): Promise<{ delay: number }> {
    try {
      return await this.request(
        `/proxies/${encodeURIComponent(proxy)}/delay?timeout=${timeout}&url=${encodeURIComponent(testURL)}`,
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      if (
        message === "An error occurred in the delay test" ||
        message.includes("503 Service Unavailable")
      ) {
        throw new Error(
          "真实出站测试失败：节点协议握手、TLS/认证或测试地址访问失败（DNS 成功不代表代理可用）",
        );
      }
      throw error;
    }
  }

  // 获取连接列表
  async getConnections(): Promise<{
    connections: Connection[];
    downloadTotal: number;
    uploadTotal: number;
    memory?: number;
  }> {
    return this.request("/connections");
  }

  // 关闭连接
  async closeConnection(id: string): Promise<void> {
    await this.request(`/connections/${id}`, { method: "DELETE" });
  }

  // 关闭所有连接
  async closeAllConnections(): Promise<void> {
    await this.request("/connections", { method: "DELETE" });
  }

  // 获取规则
  async getRules(): Promise<{ rules: Rule[] }> {
    return this.request("/rules");
  }

  // 获取配置
  async getConfig(): Promise<ClashConfig> {
    return this.request("/configs");
  }

  // 更新配置
  async updateConfig(config: Partial<ClashConfig>): Promise<void> {
    await this.request("/configs", {
      method: "PATCH",
      body: JSON.stringify(config),
    });
  }

  // 切换模式
  async setMode(mode: "rule" | "global" | "direct"): Promise<void> {
    await this.updateConfig({ mode });
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
