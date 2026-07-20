export interface DNSBindingServer {
  tag: string;
  enabled: boolean;
  server_type: string;
  detour: string;
}

export interface DNSBindingRule {
  id: number;
  enabled: boolean;
  priority: number;
  rule_type: string;
  conditions_json: string;
  server: string;
  disable_cache: boolean;
  rewrite_ttl: number;
  client_subnet: string;
}

type JSONRequest = (url: string, init?: RequestInit) => Promise<unknown>;
type DNSRuleWithConditions = Pick<DNSBindingRule, "conditions_json">;

function ruleConditions(rule: DNSRuleWithConditions): Record<string, unknown> {
  try {
    const conditions = JSON.parse(rule.conditions_json || "{}");
    return conditions && typeof conditions === "object" ? conditions : {};
  } catch {
    return {};
  }
}

export function dnsRuleOutbounds(rule: DNSRuleWithConditions): string[] {
  const outbound = ruleConditions(rule).outbound;
  return Array.isArray(outbound)
    ? outbound.filter((value): value is string => typeof value === "string")
    : typeof outbound === "string"
      ? [outbound]
      : [];
}

export function isDNSOutboundBinding(rule: DNSRuleWithConditions): boolean {
  const conditions = ruleConditions(rule);
  return Object.keys(conditions).length === 1 && dnsRuleOutbounds(rule).length > 0;
}

export function findDNSOutboundBinding(
  rules: DNSBindingRule[],
  outbound: string,
) {
  return rules.find(
    (rule) => isDNSOutboundBinding(rule) && dnsRuleOutbounds(rule).includes(outbound),
  );
}

function ruleBody(
  rule: DNSBindingRule,
  conditions: Record<string, unknown>,
  server = rule.server,
  enabled = rule.enabled,
) {
  return {
    enabled,
    priority: rule.priority || 0,
    rule_type: rule.rule_type || "default",
    conditions,
    server,
    disable_cache: rule.disable_cache || false,
    rewrite_ttl: rule.rewrite_ttl || 0,
    client_subnet: rule.client_subnet || "",
  };
}

async function updateRule(
  request: JSONRequest,
  rule: DNSBindingRule,
  conditions: Record<string, unknown>,
  server = rule.server,
  enabled = rule.enabled,
) {
  await request(`/api/v1/dns/rules/${rule.id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(ruleBody(rule, conditions, server, enabled)),
  });
}

async function createRule(
  request: JSONRequest,
  outbound: string,
  server: string,
  template?: DNSBindingRule,
) {
  await request("/api/v1/dns/rules", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      enabled: true,
      priority: template?.priority || 0,
      rule_type: "default",
      conditions: { outbound: [outbound] },
      server,
      disable_cache: template?.disable_cache || false,
      rewrite_ttl: template?.rewrite_ttl || 0,
      client_subnet: template?.client_subnet || "",
    }),
  });
}

export async function saveDNSOutboundBinding(
  request: JSONRequest,
  rules: DNSBindingRule[],
  previousOutbound: string,
  outbound: string,
  server: string,
) {
  const previous = previousOutbound.trim();
  const next = outbound.trim();
  const old =
    findDNSOutboundBinding(rules, previous) ||
    findDNSOutboundBinding(rules, next);

  if (!old) {
    if (server) await createRule(request, next, server);
    return;
  }

  const conditions = ruleConditions(old);
  const remaining = dnsRuleOutbounds(old).filter(
    (value) => value !== previous && value !== next,
  );
  if (remaining.length) conditions.outbound = remaining;
  else delete conditions.outbound;

  if (!server) {
    if (Object.keys(conditions).length) {
      await updateRule(request, old, conditions);
    } else {
      await request(`/api/v1/dns/rules/${old.id}`, { method: "DELETE" });
    }
    return;
  }

  const hasOtherConditions = Object.keys(conditions).length > 0;
  if (old.server === server || !hasOtherConditions) {
    conditions.outbound = [...new Set([...remaining, next])];
    await updateRule(request, old, conditions, server, true);
    return;
  }

  await updateRule(request, old, conditions);
  await createRule(request, next, server, old);
}
