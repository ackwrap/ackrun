interface SubscriptionLabel {
  value: string;
  label: string;
}

export function subscriptionFilterLabel(
  value: string,
  subscriptions: readonly SubscriptionLabel[],
) {
  const ids = value
    .split(",")
    .map((id) => id.trim())
    .filter(Boolean);
  if (!ids.length) return "全部";
  return ids
    .map(
      (id) =>
        subscriptions.find((subscription) => subscription.value === id)
          ?.label || `订阅 #${id}`,
    )
    .join("、");
}
