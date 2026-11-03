import test from "node:test";
import assert from "node:assert/strict";
import { subscriptionExpiry } from "../src/utils/subscriptionExpiry.ts";

const now = Date.parse("2026-11-03T10:00:00+08:00");
const day = 86_400_000;

test("subscription expiry consumes milliseconds without multiplying by 1000", () => {
  const timestamp = Date.parse("2026-11-13T21:38:12+08:00");
  const result = subscriptionExpiry(timestamp, now);
  assert.equal(result.date.getTime(), timestamp);
  assert.equal(result.date.getUTCFullYear(), 2026);
  assert.equal(result.remainingDays, 11);
});

test("remaining days round partial days up and clamp expired dates to zero", () => {
  assert.equal(subscriptionExpiry(now + 1, now).remainingDays, 1);
  assert.equal(subscriptionExpiry(now + day, now).remainingDays, 1);
  assert.equal(subscriptionExpiry(now + day + 1, now).remainingDays, 2);
  assert.equal(subscriptionExpiry(now, now).remainingDays, 0);
  assert.equal(subscriptionExpiry(now - day, now).remainingDays, 0);
});

test("unknown and invalid expiry dates are unavailable", () => {
  for (const value of [0, -1, NaN, Infinity, 9e15]) {
    assert.equal(subscriptionExpiry(value, now), null);
  }
});
