import test from "node:test";
import assert from "node:assert/strict";
import { attachCardSpotlight } from "../src/utils/cardSpotlight.ts";

function fixture() {
  const properties = new Map();
  const attributes = new Map();
  const classes = new Set();
  const frames = new Map();
  let id = 0;
  let reads = 0;
  const fine = Object.assign(new EventTarget(), { matches: true });
  const reduced = Object.assign(new EventTarget(), { matches: false });
  const view = Object.assign(new EventTarget(), {
    matchMedia: (query) => (query.includes("reduced-motion") ? reduced : fine),
    requestAnimationFrame: (callback) => {
      frames.set(++id, callback);
      return id;
    },
    cancelAnimationFrame: (key) => frames.delete(key),
  });
  const element = Object.assign(new EventTarget(), {
    ownerDocument: { defaultView: view },
    style: {
      setProperty: (key, value) => properties.set(key, value),
      removeProperty: (key) => properties.delete(key),
    },
    classList: {
      add: (name) => classes.add(name),
      remove: (name) => classes.delete(name),
    },
    setAttribute: (name, value) => attributes.set(name, value),
    removeAttribute: (name) => attributes.delete(name),
    getBoundingClientRect: () => {
      reads++;
      return { left: 10, top: 20, width: 400, height: 260 };
    },
  });
  return {
    element,
    view,
    properties,
    attributes,
    classes,
    frames,
    fine,
    reduced,
    get reads() {
      return reads;
    },
    pointer(type, x = 110, y = 120, pointerType = "mouse") {
      element.dispatchEvent(
        Object.assign(new Event(type), { clientX: x, clientY: y, pointerType }),
      );
    },
    flush() {
      const callbacks = [...frames.values()];
      frames.clear();
      callbacks.forEach((callback) => callback(0));
    },
  };
}

test("pointer bursts use one frame, shared CSS coordinates and a cached rectangle", () => {
  const state = fixture();
  const dispose = attachCardSpotlight(state.element);
  state.pointer("pointerenter");
  for (let x = 100; x <= 200; x++) state.pointer("pointermove", x, 25);
  assert.equal(state.frames.size, 1);
  assert.equal(state.reads, 0);
  state.flush();
  assert.equal(state.reads, 1);
  assert.equal(state.properties.get("--spotlight-x"), "190px");
  assert.equal(state.properties.get("--spotlight-y"), "5px");
  assert.ok(Number(state.properties.get("--spotlight-edge")) > 0.9);
  assert.ok(state.attributes.has("data-spotlight-active"));
  state.pointer("pointermove", 210, 150);
  state.flush();
  assert.equal(state.reads, 1);
  assert.equal(state.properties.get("--spotlight-edge"), "0");
  state.view.dispatchEvent(new Event("scroll"));
  state.pointer("pointermove");
  state.flush();
  assert.equal(state.reads, 2);
  dispose();
});

test("touch, coarse pointers and reduced motion never schedule tracking frames", () => {
  const state = fixture();
  const dispose = attachCardSpotlight(state.element);
  state.pointer("pointerenter", 110, 120, "touch");
  assert.equal(state.frames.size, 0);
  state.fine.matches = false;
  state.pointer("pointerenter");
  assert.equal(state.frames.size, 0);
  state.fine.matches = true;
  state.reduced.matches = true;
  state.pointer("pointermove");
  assert.equal(state.frames.size, 0);
  state.reduced.matches = false;
  state.pointer("pointerenter");
  state.flush();
  state.reduced.matches = true;
  state.reduced.dispatchEvent(new Event("change"));
  assert.equal(state.attributes.has("data-spotlight-active"), false);
  assert.equal(state.frames.size, 0);
  dispose();
});

test("leaving cancels queued frames, fades the effect and retains coordinates", () => {
  const state = fixture();
  const dispose = attachCardSpotlight(state.element);
  state.pointer("pointerenter");
  state.flush();
  state.pointer("pointermove", 300, 120);
  state.pointer("pointerleave");
  assert.equal(state.frames.size, 0);
  assert.equal(state.attributes.has("data-spotlight-active"), false);
  assert.equal(state.properties.get("--spotlight-x"), "100px");
  state.flush();
  assert.equal(state.attributes.has("data-spotlight-active"), false);
  dispose();
});

test("unmount clears styles, listeners and pending work", () => {
  const state = fixture();
  const dispose = attachCardSpotlight(state.element);
  state.pointer("pointerenter");
  state.flush();
  state.pointer("pointermove");
  dispose();
  assert.equal(state.frames.size, 0);
  assert.equal(state.properties.size, 0);
  assert.equal(state.classes.size, 0);
  state.pointer("pointerenter");
  state.pointer("pointermove");
  state.view.dispatchEvent(new Event("resize"));
  state.reduced.dispatchEvent(new Event("change"));
  state.flush();
  assert.equal(state.frames.size, 0);
  assert.equal(state.attributes.size, 0);
});
