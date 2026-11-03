import test from "node:test";
import assert from "node:assert/strict";
import {
  appendTrafficSample,
  createNumberTransition,
  trafficSparkline,
} from "../src/utils/metricMotion.ts";

function frames() {
  let now = 0;
  let id = 0;
  const pending = new Map();
  return {
    now: () => now,
    request: (callback) => {
      pending.set(++id, callback);
      return id;
    },
    cancel: (key) => pending.delete(key),
    step(time) {
      now = time;
      const callbacks = [...pending.values()];
      pending.clear();
      callbacks.forEach((callback) => callback(now));
    },
    pending,
  };
}

test("first sample appears immediately and later samples settle within 200ms", () => {
  const clock = frames();
  let shown;
  const motion = createNumberTransition((value) => {
    shown = value;
  }, clock);
  motion.set(1024);
  assert.equal(shown, 1024);
  assert.equal(clock.pending.size, 0);
  motion.set(2048);
  clock.step(100);
  assert.ok(shown > 1024 && shown < 2048);
  clock.step(200);
  assert.equal(shown, 2048);
  assert.equal(clock.pending.size, 0);
});

test("updates during interpolation continue from the displayed value, including zero", () => {
  const clock = frames();
  let shown;
  const motion = createNumberTransition((value) => {
    shown = value;
  }, clock);
  motion.set(100);
  motion.set(200);
  clock.step(50);
  const current = shown;
  motion.set(0);
  assert.equal(shown, current);
  assert.equal(clock.pending.size, 1);
  clock.step(100);
  assert.ok(shown > 0 && shown < current);
  clock.step(250);
  assert.equal(shown, 0);
});

test("reduced motion and unavailable data snap and cancel pending work", () => {
  const clock = frames();
  let shown;
  const motion = createNumberTransition((value) => {
    shown = value;
  }, clock);
  motion.set(100);
  motion.set(200);
  motion.set(200, false);
  assert.equal(shown, 200);
  assert.equal(clock.pending.size, 0);
  motion.set(400);
  motion.set(null);
  assert.equal(shown, null);
  assert.equal(clock.pending.size, 0);
  motion.set(500);
  assert.equal(shown, 500);
  motion.set(Number.NaN);
  assert.equal(shown, null);
  motion.set(-10);
  assert.equal(shown, 0);
});

test("disposing a metric cancels its frame and prevents later updates", () => {
  const clock = frames();
  let shown;
  const motion = createNumberTransition((value) => {
    shown = value;
  }, clock);
  motion.set(100);
  motion.set(200);
  motion.dispose();
  assert.equal(clock.pending.size, 0);
  motion.set(500);
  clock.step(1000);
  assert.equal(shown, 100);
});

test("traffic history stays bounded and curves stay within their viewport", () => {
  let samples = [];
  for (let index = 0; index < 1000; index++) {
    samples = appendTrafficSample(samples, index);
  }
  assert.equal(samples.length, 32);
  assert.equal(samples[0], 968);
  assert.equal(samples.at(-1), 999);
  assert.deepEqual(appendTrafficSample(samples, 10, 1), [10]);
  assert.equal(appendTrafficSample([], Infinity)[0], 0);
  assert.equal(trafficSparkline([1]), "");
  for (const history of [samples, [0, 0], [0, 100000, 0]]) {
    const points = trafficSparkline(history)
      .split(" ")
      .map((point) => point.split(",").map(Number));
    assert.equal(points[0][0], 0);
    assert.equal(points.at(-1)[0], 100);
    assert.ok(
      points.every(([x, y]) => x >= 0 && x <= 100 && y >= 2 && y <= 11),
    );
  }
});
