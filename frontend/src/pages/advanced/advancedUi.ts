import type { AdvancedRouteRef } from "@/services/advancedTypes";
import type { AdvancedTargetFields } from "@/services/advancedTypes";

export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "请求失败";
}

export function formatTime(value?: number | string) {
  if (value === undefined || value === null || value === "") return "--";
  const normalized =
    typeof value === "number" && value < 10_000_000_000 ? value * 1000 : value;
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? "--" : date.toLocaleString();
}

export function dateTimeInput(value: number) {
  const date = new Date(value);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export function lines(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function lineText(value?: string[]) {
  return (value || []).join("\n");
}

export function routeRefKey(value?: AdvancedRouteRef | null) {
  if (!value) return "";
  if (value.type === "node") {
    return `node:${value.subscription_id || 0}:${value.node_uid || ""}`;
  }
  if (value.type === "collection") return `collection:${value.collection_id || 0}`;
  return value.type;
}

export function parseRouteRef(value: string): AdvancedRouteRef {
  const [type, first, ...rest] = value.split(":");
  if (type === "node") {
    return {
      type: "node",
      subscription_id: Number(first),
      node_uid: rest.join(":"),
    };
  }
  if (type === "collection") {
    return { type: "collection", collection_id: Number(first) };
  }
  return { type: "direct" };
}

export function routeRefFields(value: string) {
  const ref = parseRouteRef(value);
  return {
    type: ref.type,
    subscriptionID: ref.subscription_id,
    nodeUID: ref.node_uid,
    collectionID: ref.collection_id,
  };
}

export function targetRef(item: AdvancedTargetFields, fallback = false) {
  return fallback
    ? {
        type: item.fallback_type,
        subscription_id: item.fallback_subscription_id,
        node_uid: item.fallback_node_uid,
        collection_id: item.fallback_collection_id,
      }
    : {
        type: item.target_type,
        subscription_id: item.target_subscription_id,
        node_uid: item.target_node_uid,
        collection_id: item.target_collection_id,
      };
}

export function healthBadge(status: string) {
  if (
    [
      "healthy",
      "active",
      "success",
      "recovered",
      "probe_succeeded",
      "route",
      "route_fallback",
      "lease",
      "lease_fallback",
      "node_exposure",
    ].includes(status)
  ) {
    return "online" as const;
  }
  if (
    [
      "unhealthy",
      "circuit_open",
      "expired",
      "error",
      "failed",
      "probe_failed",
      "route_failed",
      "lease_failed",
      "node_exposure_failed",
    ].includes(status)
  ) {
    return "error" as const;
  }
  if (status === "probing" || status === "expiring") return "pending" as const;
  return "offline" as const;
}

export function statusLabel(status: string) {
  const labels: Record<string, string> = {
    healthy: "健康",
    unhealthy: "异常",
    circuit_open: "熔断中",
    probing: "探测中",
    unknown: "未知",
    active: "生效中",
    expiring: "即将到期",
    expired: "已到期",
    disabled: "已停用",
    route: "平台路由",
    route_fallback: "平台路由回退",
    route_failed: "平台路由失败",
    lease: "租约路由",
    lease_fallback: "租约路由回退",
    lease_failed: "租约路由失败",
    node_exposure: "入口固定路由",
    node_exposure_failed: "入口固定路由失败",
    probe_succeeded: "探测成功",
    probe_failed: "探测失败",
    recovered: "已恢复",
    error: "错误",
  };
  return labels[status] || status || "--";
}
