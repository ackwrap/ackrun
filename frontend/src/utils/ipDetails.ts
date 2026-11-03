export interface IPDetails {
  flag: string;
  country: string;
  summary: string;
  description: string;
}

export function normalizeIP(value: unknown): string {
  if (typeof value !== "string") return "";
  const ip = value.trim();
  if (!ip || ip.length > 45) return "";
  if (/^(\d{1,3}\.){3}\d{1,3}$/.test(ip)) {
    const octets = ip.split(".");
    return octets.every((part) => Number(part) <= 255)
      ? octets.map(Number).join(".")
      : "";
  }
  if (!/^[\da-f:]+$/i.test(ip) || !ip.includes(":")) return "";
  try {
    return new URL(`http://[${ip}]/`).hostname.slice(1, -1);
  } catch {
    return "";
  }
}

const countryNames = new Intl.DisplayNames(["zh-CN"], { type: "region" });
const field = (value: unknown) =>
  typeof value === "string" || typeof value === "number"
    ? String(value)
        .replace(/[\u0000-\u001f\u007f]/g, "")
        .trim()
        .slice(0, 120)
    : "";
const unique = (values: string[]) => [...new Set(values.filter(Boolean))];

/** Metadata is only usable when the provider confirms the exact probed IP. */
export function parseIPDetails(
  raw: unknown,
  expectedIP: string,
): IPDetails | null {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  const data = raw as Record<string, unknown>;
  const ip = normalizeIP(expectedIP);
  if (!ip || normalizeIP(data.ip) !== ip) return null;
  const code = field(data.country_code).toUpperCase();
  const validCode = /^[A-Z]{2}$/.test(code) && !["ZZ", "XX"].includes(code);
  const country = validCode
    ? countryNames.of(code) || field(data.country)
    : field(data.country);
  const flag = validCode
    ? String.fromCodePoint(
        ...[...code].map((char) => char.charCodeAt(0) + 127397),
      )
    : "";
  const region = field(data.region);
  const city = field(data.city);
  const isp =
    field(data.isp) || field(data.organization) || field(data.asn_organization);
  const location = unique([country, region, city]).join(" · ");
  const asn = field(data.asn);
  const organization = field(data.asn_organization) || field(data.organization);
  const timezone = field(data.timezone);
  if (!location && !isp && !asn && !organization && !timezone) return null;
  return {
    flag,
    country,
    summary: unique([country, city || region, isp]).join(" · "),
    description: [
      location,
      isp && `运营商：${isp}`,
      organization && organization !== isp && `网络：${organization}`,
      asn && `ASN：${asn.startsWith("AS") ? asn : `AS${asn}`}`,
      timezone && `时区：${timezone}`,
    ]
      .filter(Boolean)
      .join("\n"),
  };
}

/** Per-IP caching and in-flight deduplication keep the four probes inexpensive. */
export function createIPDetailsLookup(
  request: (url: string) => Promise<unknown>,
  now = Date.now,
) {
  const cache = new Map<string, { value: IPDetails; expires: number }>();
  const pending = new Map<string, Promise<IPDetails | null>>();
  return (value: string): Promise<IPDetails | null> => {
    const ip = normalizeIP(value);
    if (!ip) return Promise.resolve(null);
    const hit = cache.get(ip);
    if (hit && hit.expires > now()) return Promise.resolve(hit.value);
    const active = pending.get(ip);
    if (active) return active;
    const lookup = Promise.resolve()
      .then(() => request(`https://api.ip.sb/geoip/${encodeURIComponent(ip)}`))
      .then((raw) => {
        const details = parseIPDetails(raw, ip);
        if (details) {
          cache.delete(ip);
          cache.set(ip, { value: details, expires: now() + 15 * 60_000 });
          if (cache.size > 32) cache.delete(cache.keys().next().value!);
        }
        return details;
      })
      .finally(() => pending.delete(ip));
    pending.set(ip, lookup);
    return lookup;
  };
}
