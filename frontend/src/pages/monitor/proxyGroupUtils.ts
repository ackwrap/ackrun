import type { ProxyGroup, ProxyNode } from "@/services/clash";

export type ProxyMap = Record<string, ProxyGroup | ProxyNode>;

export function latestDelay(proxy?: ProxyGroup | ProxyNode) {
  const history = proxy?.history || [];
  return Number(history[history.length - 1]?.delay || 0);
}

export function latencyTextClass(delay: number) {
  if (!delay) return "text-[var(--text-tertiary)]";
  if (delay < 200) return "text-[var(--color-success)]";
  if (delay < 800) return "text-[var(--color-warning)]";
  return "text-[var(--color-error)]";
}

export function latencyBackgroundClass(delay: number) {
  if (!delay) return "bg-[var(--text-tertiary)]";
  if (delay < 200) return "bg-[var(--color-success)]";
  if (delay < 800) return "bg-[var(--color-warning)]";
  return "bg-[var(--color-error)]";
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
