import type {
  ClashConnection,
  ProxyGroup,
  ProxyMap,
  ProxyNode,
} from "../services/clash";

const initialSegmenter =
  typeof Intl.Segmenter === "function"
    ? new Intl.Segmenter(undefined, { granularity: "grapheme" })
    : null;

export function activityInitial(label: string): string {
  const name = label.trim();
  const initial = initialSegmenter
    ? initialSegmenter.segment(name)[Symbol.iterator]().next().value?.segment
    : Array.from(name)[0];
  return (initial || "?").toUpperCase();
}

export function isProxyGroup(
  proxy: ProxyNode | ProxyGroup,
): proxy is ProxyGroup {
  return Array.isArray(proxy.all);
}

export function resolveProxyName(name: string, proxies: ProxyMap): string {
  const visited = new Set<string>();
  let current = name;
  while (current && !visited.has(current)) {
    visited.add(current);
    const next = proxies[current];
    if (!next?.all || !next.now) break;
    current = next.now;
  }
  return current || name;
}

export function connectionSource(connection: ClashConnection): string {
  const metadata = connection.metadata;
  if (metadata.process) return metadata.process;
  const path = metadata.processPath?.replace(/ \([^)]*\)$/, "");
  return path?.split(/[\\/]/).pop() || metadata.sourceIP || "未知来源";
}

export function connectionTarget(connection: ClashConnection): string {
  const host =
    connection.metadata.host || connection.metadata.destinationIP || "未知目标";
  const port = connection.metadata.destinationPort;
  return port ? `${host.includes(":") ? `[${host}]` : host}:${port}` : host;
}

export function rememberConnections(
  history: ClashConnection[],
  incoming: ClashConnection[],
  limit = 200,
): ClashConnection[] {
  const records = new Map(history.map((item) => [item.id, item]));
  incoming.forEach((item) => records.set(item.id, item));
  return [...records.values()]
    .sort((a, b) => (Date.parse(b.start) || 0) - (Date.parse(a.start) || 0))
    .slice(0, limit);
}
