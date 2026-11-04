import { useTableRowSort } from "@/composables/useTableRowSort";
import type { RouteRule } from "@/services/types";

export function isFixedRouteRule(rule: RouteRule) {
  return (
    rule.system_key === "ad_block" ||
    rule.system_key === "global_direct" ||
    ["fallback", "final"].includes(rule.rule_type) ||
    (rule.is_system && ["广告拦截", "全球直连", "最终策略"].includes(rule.name))
  );
}

export function useRuleSort(
  getRules: () => RouteRule[],
  isSaving: () => boolean,
  reorder: (ids: number[]) => void,
) {
  const { displayItems, ...sorting } = useTableRowSort(
    getRules,
    isSaving,
    reorder,
    isFixedRouteRule,
  );
  return { ...sorting, displayRules: displayItems };
}
