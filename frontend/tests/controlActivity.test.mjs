import test from "node:test";
import assert from "node:assert/strict";
import {
  activityInitial,
  connectionSource,
  connectionTarget,
  rememberConnections,
  resolveProxyName,
} from "../src/utils/controlActivity.ts";

const connection = (id, start = "2026-11-03T10:00:00Z") => ({
  id,
  start,
  metadata: {},
  upload: 0,
  download: 0,
});

test("activity initials preserve whole emoji and composed characters", () => {
  for (const [name, expected] of [
    ["  proxy", "P"],
    ["🎯 Apple", "🎯"],
    ["🇭🇰 香港", "🇭🇰"],
    ["👩🏽‍💻 工作", "👩🏽‍💻"],
    ["e\u0301quipe", "E\u0301"],
    ["节点", "节"],
    [" ", "?"],
  ]) {
    assert.equal(activityInitial(name), expected);
  }
});

test("connection labels handle Windows processes and IPv6", () => {
  const item = {
    ...connection("one"),
    metadata: {
      processPath: "C:\\Apps\\browser.exe (1000)",
      destinationIP: "2001:db8::1",
      destinationPort: "443",
    },
  };
  assert.equal(connectionSource(item), "browser.exe");
  assert.equal(connectionTarget(item), "[2001:db8::1]:443");
});

test("recent requests update by identity, retain closed records and cap history", () => {
  const older = connection("old", "2026-11-03T09:00:00Z");
  const current = connection("current");
  const history = rememberConnections(
    [older, current],
    [{ ...current, download: 1024 }, connection("new", "2026-11-03T11:00:00Z")],
    2,
  );
  assert.deepEqual(
    history.map((item) => item.id),
    ["new", "current"],
  );
  assert.equal(history[1].download, 1024);
  assert.equal(rememberConnections([older], [current]).length, 2);
});

test("nested proxy selection resolves to the node and detects cycles", () => {
  assert.equal(
    resolveProxyName("Proxy", {
      Proxy: { all: ["Auto"], now: "Auto" },
      Auto: { all: ["test-node"], now: "test-node" },
    }),
    "test-node",
  );
  assert.equal(
    resolveProxyName("Proxy", {
      Proxy: { all: ["Auto"], now: "Auto" },
      Auto: { all: ["Proxy"], now: "Proxy" },
    }),
    "Proxy",
  );
});
