import type { ProxyGroup, ProxyNode } from "@/services/clash";

export type ProxyMap = Record<string, ProxyGroup | ProxyNode>;
export type LatencyLevel = "unknown" | "low" | "medium" | "high";

const LOW_LATENCY_THRESHOLD = 400;
const MEDIUM_LATENCY_THRESHOLD = 800;
const LATENCY_TEXT_CLASSES: Record<LatencyLevel, string> = {
  unknown: "text-[var(--text-tertiary)]",
  low: "text-[var(--latency-low)]",
  medium: "text-[var(--latency-medium)]",
  high: "text-[var(--latency-high)]",
};
const LATENCY_BACKGROUND_CLASSES: Record<LatencyLevel, string> = {
  unknown: "bg-[var(--latency-unknown)]",
  low: "bg-[var(--latency-low)]",
  medium: "bg-[var(--latency-medium)]",
  high: "bg-[var(--latency-high)]",
};
const LATENCY_SURFACE_CLASSES: Record<LatencyLevel, string> = {
  unknown: "bg-[var(--button-secondary-bg)]",
  low: "bg-[var(--latency-low-bg)]",
  medium: "bg-[var(--latency-medium-bg)]",
  high: "bg-[var(--latency-high-bg)]",
};

export function latestDelay(proxy?: ProxyGroup | ProxyNode) {
  const history = proxy?.history || [];
  return Number(history[history.length - 1]?.delay || 0);
}

export function latencyLevel(delay: number): LatencyLevel {
  if (!delay) return "unknown";
  if (delay < LOW_LATENCY_THRESHOLD) return "low";
  if (delay < MEDIUM_LATENCY_THRESHOLD) return "medium";
  return "high";
}

export function latencyTextClass(delay: number) {
  return LATENCY_TEXT_CLASSES[latencyLevel(delay)];
}

export function latencyBackgroundClass(delay: number) {
  return LATENCY_BACKGROUND_CLASSES[latencyLevel(delay)];
}

export function latencyTagClass(delay: number) {
  const level = latencyLevel(delay);
  return `${LATENCY_TEXT_CLASSES[level]} ${LATENCY_SURFACE_CLASSES[level]}`;
}

export function proxyNodeDescription(node?: ProxyNode) {
  return [node?.type || "proxy", node?.udp ? "udp" : ""]
    .filter(Boolean)
    .join(" / ");
}

export function availableProxyCount(group: ProxyGroup, proxies: ProxyMap) {
  return (group.all || []).filter((name) => latestDelay(proxies[name]) > 0)
    .length;
}
