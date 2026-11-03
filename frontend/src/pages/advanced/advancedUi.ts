export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "请求失败";
}

export function formatTime(value?: number | string) {
  if (value === undefined || value === null || value === "") return "--";
  const normalized =
    typeof value === "number" && value < 10_000_000_000 ? value * 1000 : value;
  const date = new Date(normalized);
  return Number.isNaN(date.getTime())
    ? "--"
    : date.toLocaleString(undefined, { hourCycle: "h23" });
}
