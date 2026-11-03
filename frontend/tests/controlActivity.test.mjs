import test from "node:test";
import assert from "node:assert/strict";
import {
  connectionSource,
  connectionTarget,
  parseDNSActivity,
  rememberConnections,
  rememberDNS,
  resolveProxyName,
} from "../src/utils/controlActivity.ts";

const log = (id, line, time = id) => ({ id, time, source: "stdout", line });
const connection = (id, start = "2026-11-03T10:00:00Z") => ({
  id,
  start,
  metadata: {},
  upload: 0,
  download: 0,
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

test("DNS response records merge with the question and multiple answers", () => {
  const question = parseDNSActivity(
    log(1, "DEBUG [1234 0ms] dns: exchange example.test. IN A"),
  );
  const first = parseDNSActivity(
    log(
      2,
      "\u001b[32mINFO\u001b[0m [1234 10ms] dns: exchanged A example.test. 60 IN A 198.51.100.10",
    ),
  );
  const second = parseDNSActivity(
    log(
      3,
      "INFO [1234 11ms] dns: exchanged A example.test. 60 IN A 198.51.100.11",
    ),
  );
  let records = rememberDNS([], question);
  records = rememberDNS(records, first);
  records = rememberDNS(records, second);
  assert.equal(records.length, 1);
  assert.equal(records[0].domain, "example.test");
  assert.equal(records[0].status, "success");
  assert.match(records[0].result, /198\.51\.100\.10.*198\.51\.100\.11/);
  assert.deepEqual(rememberDNS(records, question), records);
});

test("negative DNS responses and failures remain visible", () => {
  const question = parseDNSActivity(
    log(1, "DEBUG [2345 0ms] dns: exchange missing.test. IN AAAA"),
  );
  const negative = parseDNSActivity(
    log(2, "DEBUG [2345 8ms] dns: exchanged missing.test NXDOMAIN 0"),
  );
  const records = rememberDNS(rememberDNS([], question), negative);
  assert.equal(records.length, 1);
  assert.equal(records[0].type, "AAAA");
  assert.equal(records[0].status, "error");
  assert.equal(records[0].result, "NXDOMAIN");
  const failed = parseDNSActivity(
    log(
      3,
      "ERROR [9999 1ms] dns: exchange failed for timeout.test. IN A: context deadline exceeded",
    ),
  );
  assert.equal(failed.type, "A");
  assert.equal(failed.status, "error");
  assert.match(failed.result, /deadline exceeded/);
  assert.equal(
    parseDNSActivity(
      log(4, "INFO outbound/direct: outbound connection to example.test:443"),
    ),
    null,
  );
});

test("loading older DNS logs cannot push recent live records out of the bounded list", () => {
  let records = [];
  for (let index = 300; index > 0; index--) {
    records = rememberDNS(records, {
      id: String(index),
      context: "",
      time: index,
      domain: "example.test",
      type: "A",
      result: "198.51.100.1",
      status: "success",
    });
  }
  assert.equal(records.length, 200);
  assert.equal(records[0].time, 300);
  assert.equal(records.at(-1).time, 101);
});
