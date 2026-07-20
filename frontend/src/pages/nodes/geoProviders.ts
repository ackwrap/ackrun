import { api } from "@/services/api";

export interface GeoProviderOption {
  value: string;
  label: string;
}

export async function loadGeoProviderOptions(includeDisabled: boolean) {
  const response = await api.getGeoIPProviders();
  const enabled = response.items.filter((item) => item.enabled);
  const options: GeoProviderOption[] = enabled.map((item) => ({
    value: item.key,
    label: `${item.name}${item.is_default ? "（默认）" : ""}`,
  }));
  if (includeDisabled) {
    options.unshift({ value: "disable-geoip", label: "关闭 Geo 查询" });
  }
  return {
    options,
    defaultProvider:
      enabled.find((item) => item.is_default)?.key || enabled[0]?.key || "",
  };
}
