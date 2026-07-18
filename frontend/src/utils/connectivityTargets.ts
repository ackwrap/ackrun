export const defaultConnectivityTestURL = "http://www.gstatic.com/generate_204";

export const connectivityTestTargets = [
  {
    label: "Google HTTP（推荐）",
    value: defaultConnectivityTestURL,
  },
  {
    label: "Cloudflare HTTP",
    value: "http://cp.cloudflare.com/generate_204",
  },
  {
    label: "Apple HTTP",
    value: "http://captive.apple.com/generate_204",
  },
  {
    label: "Google HTTPS",
    value: "https://www.gstatic.com/generate_204",
  },
  {
    label: "Cloudflare HTTPS",
    value: "https://cp.cloudflare.com/generate_204",
  },
] as const;

export const connectivityTestTargetValues = new Set<string>(
  connectivityTestTargets.map((target) => target.value),
);
