// Subscription APIs expose Unix timestamps in milliseconds.
export function subscriptionExpiry(expireAt: number, now = Date.now()) {
  if (!Number.isFinite(expireAt) || expireAt <= 0) return null;
  const date = new Date(expireAt);
  if (!Number.isFinite(date.getTime())) return null;
  return {
    date,
    remainingDays: Math.max(0, Math.ceil((expireAt - now) / 86_400_000)),
  };
}
