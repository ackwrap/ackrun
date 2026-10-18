export function splitDeviceRuleValues(values: string[]) {
  const macAddresses: string[] = [];
  const ipCidrs: string[] = [];
  for (const raw of values) {
    const value = raw.trim();
    if (!value) continue;
    if (
      /^(?:[\da-f]{2}:){5}[\da-f]{2}$/i.test(value) ||
      /^(?:[\da-f]{2}-){5}[\da-f]{2}$/i.test(value) ||
      /^(?:[\da-f]{4}\.){2}[\da-f]{4}$/i.test(value)
    ) {
      macAddresses.push(
        value
          .replace(/[:.\-]/g, "")
          .toLowerCase()
          .match(/../g)!
          .join(":"),
      );
    } else {
      ipCidrs.push(value);
    }
  }
  return {
    macAddresses: [...new Set(macAddresses)],
    ipCidrs: [...new Set(ipCidrs)],
  };
}

export function deviceRulePreview(
  values: string[],
  directDomains: string[],
  outbound: string,
) {
  const { macAddresses, ipCidrs } = splitDeviceRuleValues(values);
  const conditions: Array<Record<string, unknown>> = [];
  if (macAddresses.length)
    conditions.push({
      inbound: ["ackwrap-device-mac-in", "ackwrap-device-mac-dns-in"],
      source_mac_address: macAddresses,
    });
  if (ipCidrs.length) conditions.push({ source_ip_cidr: ipCidrs });
  if (!conditions.length) return [];

  const deviceCondition =
    conditions.length === 1
      ? conditions[0]
      : { type: "logical", mode: "or", rules: conditions };
  const domains = [
    ...new Set(
      directDomains
        .map((value) => value.trim().toLowerCase().replace(/\.$/, ""))
        .filter(Boolean),
    ),
  ];
  return [
    ...(domains.length
      ? [
          {
            type: "logical",
            mode: "and",
            rules: [deviceCondition, { domain_suffix: domains }],
            action: "route",
            outbound: "direct",
          },
        ]
      : []),
    { ...deviceCondition, action: "route", outbound },
  ];
}
