import type {
  ClashConnection,
  ProxyGroup,
  ProxyMap,
  ProxyNode,
} from "../services/clash";
import type { CoreLogEntry } from "../services/types";

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

export interface DNSActivity {
  id: string;
  context: string;
  time: number;
  domain: string;
  type: string;
  result: string;
  status: "pending" | "success" | "error";
}

export function parseDNSActivity(entry: CoreLogEntry): DNSActivity | null {
  const line = entry.line.replace(/\u001b\[[0-?]*[ -/]*[@-~]/g, "");
  const message = line.match(/\bdns(?:\/[^\s:]+)?:\s+(.+)$/)?.[1];
  if (!message) return null;
  let domain = "",
    type = "",
    result = "",
    status: DNSActivity["status"] = "pending";
  const question = message.match(/^exchange (\S+)\s+IN\s+([A-Z0-9]+)\b/);
  const answer = message.match(
    /^(cached|exchanged|optimistic|refreshed|rejected) ([A-Z0-9]+) (\S+)\s+\d+\s+IN\s+[A-Z0-9]+\s+(.+)$/,
  );
  const failure = message.match(
    /^exchange failed for (\S+)\s+IN\s+([A-Z0-9]+):?\s*(.*)$/,
  );
  if (question) {
    [, domain, type] = question;
    result = "查询中";
  } else if (answer) {
    type = answer[2];
    domain = answer[3];
    result = answer[4];
    status = answer[1] === "rejected" ? "error" : "success";
    if (answer[1] === "cached" || answer[1] === "optimistic")
      result = `缓存 · ${result}`;
  } else if (failure) {
    [, domain, type, result] = failure;
    type = type.replace(/:$/, "");
    status = "error";
  } else {
    const summary = message.match(
      /^(?:cached|exchanged|optimistic|refreshed) (\S+)\s+([A-Z]+)\s*\d*$/,
    );
    if (!summary) return null;
    domain = summary[1];
    result = summary[2];
    status = result === "NOERROR" ? "success" : "error";
  }
  domain = domain.replace(/\.$/, "");
  const context = line.match(/\[(\d+)\s+[^\]]+\]/)?.[1] || "";
  return {
    id: context ? `${context}:${domain}:${type}` : String(entry.id),
    context,
    time: entry.time,
    domain,
    type,
    result,
    status,
  };
}

export function rememberDNS(
  history: DNSActivity[],
  incoming: DNSActivity,
): DNSActivity[] {
  const index = history.findIndex(
    (item) =>
      item.id === incoming.id ||
      (!!incoming.context &&
        item.context === incoming.context &&
        item.domain === incoming.domain &&
        (!incoming.type || !item.type)),
  );
  if (index < 0)
    return [incoming, ...history].sort((a, b) => b.time - a.time).slice(0, 200);
  const previous = history[index];
  // A delayed initial log snapshot must not overwrite a newer live response.
  if (
    incoming.time < previous.time ||
    (incoming.status === "pending" && previous.status !== "pending")
  )
    return history;
  const record = {
    ...incoming,
    id: previous.id,
    type: incoming.type || previous.type,
  };
  if (
    incoming.status === "success" &&
    previous.status === "success" &&
    previous.type &&
    incoming.type &&
    previous.result !== incoming.result &&
    !/^(NOERROR|NXDOMAIN|SERVFAIL|REFUSED)$/.test(previous.result)
  ) {
    record.result = [
      ...new Set([
        ...previous.result.split(" · "),
        ...incoming.result.split(" · "),
      ]),
    ]
      .join(" · ")
      .slice(0, 1024);
  }
  return history.map((item, position) => (position === index ? record : item));
}
