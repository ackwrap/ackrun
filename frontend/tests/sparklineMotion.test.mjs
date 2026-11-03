import test from "node:test";
import assert from "node:assert/strict";
import {
  createSparklineTransition,
  smoothTrafficPath,
  trafficProfile,
} from "../src/utils/sparklineMotion.ts";

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
      callbacks.forEach((callback) => callback(time));
    },
    pending,
  };
}

test("profiles preserve the latest value and remain bounded at a fixed complexity", () => {
  assert.deepEqual(trafficProfile([10]), []);
  for (const samples of [
    [0, 0],
    [0, 100, 10],
    [NaN, Infinity, -1],
    Array.from({ length: 1000 }, (_, i) => i),
  ]) {
    const profile = trafficProfile(samples);
    assert.equal(profile.length, 32);
    assert.ok(
      profile.every(
        (value) => Number.isFinite(value) && value >= 2 && value <= 11,
      ),
    );
  }
  assert.equal(trafficProfile([0, 100, 10]).at(-1), 10.1);
});

test("curves pass through samples with bounded control points, including sharp spikes", () => {
  for (const levels of [
    [11, 2, 11],
    [2, 2, 11, 11],
    [11, 9, 8, 2],
    [2, 9, 4, 11],
  ]) {
    const path = smoothTrafficPath(levels);
    const commands = path.split(" C ");
    assert.equal(commands.length, levels.length);
    assert.equal(commands[0], `M 0 ${levels[0].toFixed(3)}`);
    for (let i = 1; i < commands.length; i++) {
      const [x1, y1, x2, y2, x, y] = commands[i].split(" ").map(Number);
      assert.ok(x1 <= x2 && x2 <= x);
      assert.equal(y, levels[i]);
      const min = Math.min(levels[i - 1], levels[i]);
      const max = Math.max(levels[i - 1], levels[i]);
      assert.ok(y1 >= min && y1 <= max && y2 >= min && y2 <= max);
    }
  }
});

test("new traffic and scale changes morph from the current curve, then settle", () => {
  const clock = frames();
  let path;
  const motion = createSparklineTransition((value) => {
    path = value;
  }, clock);
  motion.set([0, 100, 10]);
  const initial = path;
  motion.set([0, 100, 10, 1000]);
  assert.equal(path, initial);
  assert.equal(clock.pending.size, 1);
  clock.step(120);
  const middle = path;
  assert.notEqual(middle, initial);
  motion.set([0, 100, 10, 1000, 1]);
  assert.equal(path, middle);
  assert.equal(clock.pending.size, 1);
  clock.step(360);
  assert.equal(path, smoothTrafficPath(trafficProfile([0, 100, 10, 1000, 1])));
  assert.equal(clock.pending.size, 0);
});

test("reduced motion, unavailable samples and unmount cancel curve frames", () => {
  const clock = frames();
  let path;
  const motion = createSparklineTransition((value) => {
    path = value;
  }, clock);
  motion.set([0, 100]);
  motion.set([100, 0]);
  motion.set([100, 0], false);
  assert.equal(path, smoothTrafficPath(trafficProfile([100, 0])));
  assert.equal(clock.pending.size, 0);
  motion.set([0, 200]);
  motion.set([]);
  assert.equal(path, "");
  assert.equal(clock.pending.size, 0);
  motion.set([0, 100]);
  const previous = path;
  motion.set([0, 200]);
  motion.dispose();
  assert.equal(clock.pending.size, 0);
  motion.set([100, 200]);
  clock.step(1000);
  assert.equal(path, previous);
});

test("unchanged traffic does not schedule redraw frames", () => {
  const clock = frames();
  let renders = 0;
  const motion = createSparklineTransition(() => {
    renders++;
  }, clock);
  motion.set([0, 0]);
  assert.equal(renders, 1);
  for (let index = 0; index < 100; index++) motion.set([0, 0, 0]);
  assert.equal(renders, 1);
  assert.equal(clock.pending.size, 0);
});
