import type { SSHSingboxDeployRequest } from "@/services/sshTypes";

export type DeployProtocol = SSHSingboxDeployRequest["protocol"];

interface DeployProtocolOption {
  value: DeployProtocol;
  label: string;
  defaultPort: number;
  network: string;
  tlsMode: "reality" | "acme" | "none";
  parameterLabel: string;
  credentialLabel: string;
  generatedLabel: string;
  references: string[];
}

export const deployProtocolOptions: DeployProtocolOption[] = [
  {
    value: "vless-reality",
    label: "VLESS + REALITY",
    defaultPort: 443,
    network: "TCP",
    tlsMode: "reality",
    parameterLabel: "Reality 伪装域名",
    credentialLabel: "UUID / Reality 密钥",
    generatedLabel: "自动生成 UUID、密钥与 Short ID",
    references: ["无需域名证书", "客户端默认启用 uTLS Chrome 指纹"],
  },
  {
    value: "shadowsocks-2022",
    label: "Shadowsocks 2022",
    defaultPort: 8388,
    network: "TCP + UDP",
    tlsMode: "none",
    parameterLabel: "加密方法",
    credentialLabel: "访问密码",
    generatedLabel: "自动生成 2022 规范随机密码",
    references: ["AES-128-GCM", "同时生成 SIP002 分享链接"],
  },
  {
    value: "vmess-ws-tls",
    label: "VMess + WebSocket + TLS",
    defaultPort: 443,
    network: "TCP (WebSocket)",
    tlsMode: "acme",
    parameterLabel: "TLS 证书域名",
    credentialLabel: "UUID / WebSocket 路径",
    generatedLabel: "自动生成 UUID 与随机路径",
    references: ["使用内置 ACME 申请证书", "域名需解析到当前服务器"],
  },
  {
    value: "trojan-tls",
    label: "Trojan + TLS",
    defaultPort: 443,
    network: "TCP",
    tlsMode: "acme",
    parameterLabel: "TLS 证书域名",
    credentialLabel: "访问密码",
    generatedLabel: "自动生成高强度随机密码",
    references: ["使用内置 ACME 申请证书", "域名需解析到当前服务器"],
  },
  {
    value: "hysteria2",
    label: "Hysteria2",
    defaultPort: 443,
    network: "UDP (QUIC)",
    tlsMode: "acme",
    parameterLabel: "TLS 证书域名",
    credentialLabel: "密码 / Salamander 混淆",
    generatedLabel: "自动生成认证与混淆密码",
    references: ["使用 Salamander QUIC 混淆", "需放行 UDP 监听端口"],
  },
  {
    value: "tuic",
    label: "TUIC v5",
    defaultPort: 443,
    network: "UDP (QUIC)",
    tlsMode: "acme",
    parameterLabel: "TLS 证书域名",
    credentialLabel: "UUID / 访问密码",
    generatedLabel: "自动生成 UUID 与随机密码",
    references: ["默认使用 BBR 拥塞控制", "需放行 UDP 监听端口"],
  },
  {
    value: "anytls",
    label: "AnyTLS",
    defaultPort: 443,
    network: "TCP",
    tlsMode: "acme",
    parameterLabel: "TLS 证书域名",
    credentialLabel: "访问密码",
    generatedLabel: "自动生成高强度随机密码",
    references: ["需要 sing-box 1.12 或更高版本", "使用内置 ACME 申请证书"],
  },
];

export const deployProtocolMap = Object.fromEntries(
  deployProtocolOptions.map((item) => [item.value, item]),
) as Record<DeployProtocol, DeployProtocolOption>;

export function validSingboxDomain(value: string) {
  if (!value || value.length > 253 || value.startsWith(".") || value.endsWith(".")) {
    return false;
  }
  return value.split(".").every(
    (label) =>
      Boolean(label) &&
      label.length <= 63 &&
      !label.startsWith("-") &&
      !label.endsWith("-") &&
      /^[A-Za-z0-9-]+$/.test(label),
  );
}

export function validSingboxIPv4(value: string) {
  const parts = value.split(".");
  return (
    parts.length === 4 &&
    parts.every(
      (part) =>
        /^\d{1,3}$/.test(part) &&
        Number(part) >= 0 &&
        Number(part) <= 255,
    )
  );
}

export function validSingboxIPv6(value: string) {
  if (!value.includes(":")) return false;
  try {
    const parsed = new URL(`http://[${value}]/`);
    return parsed.hostname.startsWith("[") && parsed.hostname.endsWith("]");
  } catch {
    return false;
  }
}

export function validSingboxEmail(value: string) {
  return !value || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}
