// Clash API Client for sing-box

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
  type: 'Selector' | 'URLTest' | 'Fallback' | 'LoadBalance' | 'Relay';
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
  'socks-port': number;
  'redir-port': number;
  'tproxy-port': number;
  'mixed-port': number;
  mode: 'rule' | 'global' | 'direct';
  'log-level': string;
}

class ClashAPIClient {
  private baseURL: string;
  private secret: string;
  private trafficWS: WebSocket | null = null;
  private trafficCallback: ((data: TrafficData) => void) | null = null;

  constructor(baseURL: string = '/api/v1/clash', secret: string = '') {
    this.baseURL = baseURL;
    this.secret = secret;
  }

  private getHeaders(): HeadersInit {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
    };
    if (this.secret) {
      headers['Authorization'] = `Bearer ${this.secret}`;
    }
    return headers;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseURL}${path}`;
    const response = await fetch(url, {
      ...options,
      headers: {
        ...this.getHeaders(),
        ...options.headers,
      },
    });

    if (!response.ok) {
      throw new Error(`Clash API error: ${response.status} ${response.statusText}`);
    }

    return response.json();
  }

  // 获取流量统计（WebSocket）
  connectTraffic(callback: (data: TrafficData) => void): void {
    this.trafficCallback = callback;
    // 使用相对路径，通过后端代理
    const wsURL = this.baseURL.replace('http://', 'ws://').replace('https://', 'wss://');
    const token = this.secret ? `?token=${this.secret}` : '';
    
    this.trafficWS = new WebSocket(`${wsURL}/traffic${token}`);
    
    this.trafficWS.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as TrafficData;
        this.trafficCallback?.(data);
      } catch (e) {
        console.error('Failed to parse traffic data:', e);
      }
    };

    this.trafficWS.onerror = (error) => {
      console.error('Traffic WebSocket error:', error);
    };

    this.trafficWS.onclose = () => {
      console.log('Traffic WebSocket closed');
      // 自动重连
      setTimeout(() => {
        if (this.trafficCallback) {
          this.connectTraffic(this.trafficCallback);
        }
      }, 3000);
    };
  }

  disconnectTraffic(): void {
    if (this.trafficWS) {
      this.trafficWS.close();
      this.trafficWS = null;
      this.trafficCallback = null;
    }
  }

  // 获取所有代理
  async getProxies(): Promise<{ proxies: Record<string, ProxyNode | ProxyGroup> }> {
    return this.request('/proxies');
  }

  // 获取单个代理
  async getProxy(name: string): Promise<ProxyNode | ProxyGroup> {
    return this.request(`/proxies/${encodeURIComponent(name)}`);
  }

  // 切换节点（仅对 Selector 类型有效）
  async selectProxy(group: string, proxy: string): Promise<void> {
    await this.request(`/proxies/${encodeURIComponent(group)}`, {
      method: 'PUT',
      body: JSON.stringify({ name: proxy }),
    });
  }

  // 测速
  async delayTest(proxy: string, testURL: string = 'https://www.gstatic.com/generate_204', timeout: number = 5000): Promise<{ delay: number }> {
    return this.request(`/proxies/${encodeURIComponent(proxy)}/delay?timeout=${timeout}&url=${encodeURIComponent(testURL)}`);
  }

  // 获取连接列表
  async getConnections(): Promise<{ connections: Connection[]; downloadTotal: number; uploadTotal: number }> {
    return this.request('/connections');
  }

  // 关闭连接
  async closeConnection(id: string): Promise<void> {
    await this.request(`/connections/${id}`, { method: 'DELETE' });
  }

  // 关闭所有连接
  async closeAllConnections(): Promise<void> {
    await this.request('/connections', { method: 'DELETE' });
  }

  // 获取规则
  async getRules(): Promise<{ rules: Rule[] }> {
    return this.request('/rules');
  }

  // 获取配置
  async getConfig(): Promise<ClashConfig> {
    return this.request('/configs');
  }

  // 更新配置
  async updateConfig(config: Partial<ClashConfig>): Promise<void> {
    await this.request('/configs', {
      method: 'PATCH',
      body: JSON.stringify(config),
    });
  }

  // 切换模式
  async setMode(mode: 'rule' | 'global' | 'direct'): Promise<void> {
    await this.updateConfig({ mode });
  }
}

// 单例
let clashClient: ClashAPIClient | null = null;

export function getClashClient(baseURL?: string, secret?: string): ClashAPIClient {
  if (!clashClient || baseURL || secret) {
    clashClient = new ClashAPIClient(baseURL, secret);
  }
  return clashClient;
}

export default ClashAPIClient;
