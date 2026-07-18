export const geoProviderOptions = [
  { value: "disable-geoip", label: "关闭 Geo 查询" },
  { value: "ipapi.is", label: "ipapi.is" },
  { value: "leomoeapi", label: "LeoMoeAPI" },
  { value: "ip.sb", label: "IP.SB" },
  { value: "ipinfo", label: "IPInfo" },
  { value: "ipinsight", label: "IPInsight" },
  { value: "ip-api.com", label: "IP-API.com" },
  { value: "ipinfolocal", label: "IPInfoLocal（本地 MMDB）" },
  { value: "chunzhen", label: "chunzhen（本地服务）" },
  { value: "ipdb.one", label: "ipdb.one（需要凭据）" },
  { value: "dn42", label: "DN42（本地 GeoFeed）" },
] as const;

export const defaultTracerouteGeoProvider = "disable-geoip";
export const defaultExitIPGeoProvider = "ipapi.is";
