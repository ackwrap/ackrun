import test from "node:test";
import assert from "node:assert/strict";
import {
  createIPDetailsLookup,
  normalizeIP,
  parseIPDetails,
} from "../src/utils/ipDetails.ts";

const sample = {
  ip: "192.0.2.1",
  country_code: "US",
  country: "United States",
  region: "Example Region",
  city: "Example City",
  isp: "Example ISP",
  asn_organization: "Example Network",
  asn: 64500,
  timezone: "Example/City",
};

test("IP normalization rejects non-address responses and canonicalizes IPv6", () => {
  for (const value of [
    null,
    {},
    "",
    "error",
    "256.1.1.1",
    "1.1.1",
    "https://example.com",
    "::1/path",
    "fe80::1%eth0",
    "1:::2",
  ])
    assert.equal(normalizeIP(value), "");
  assert.equal(normalizeIP("192.0.2.1\n"), "192.0.2.1");
  assert.equal(normalizeIP("2001:0DB8:0:0:0:0:0:1"), "2001:db8::1");
});

test("metadata includes an emoji flag and location, ISP, ASN and timezone", () => {
  const result = parseIPDetails(sample, "192.0.2.1");
  assert.equal(result.flag, "🇺🇸");
  assert.equal(result.country, "美国");
  assert.equal(result.summary, "美国 · Example City · Example ISP");
  assert.match(result.description, /Example Region/);
  assert.match(result.description, /网络：Example Network/);
  assert.match(result.description, /ASN：AS64500/);
  assert.match(result.description, /时区：Example\/City/);
});

test("unconfirmed or mismatched exits never receive another IP's location", () => {
  for (const data of [
    null,
    [],
    "error",
    {},
    { ip: "192.0.2.1" },
    { ...sample, ip: "192.0.2.2" },
  ])
    assert.equal(parseIPDetails(data, "192.0.2.1"), null);
  const result = parseIPDetails(
    { ...sample, ip: "2001:0db8::1" },
    "2001:db8::1",
  );
  assert.equal(result.flag, "🇺🇸");
});

test("optional fields and unknown country codes do not create bogus flags", () => {
  const result = parseIPDetails(
    { ip: "192.0.2.1", country_code: "ZZ", organization: "Example\n ISP" },
    "192.0.2.1",
  );
  assert.equal(result.flag, "");
  assert.equal(result.summary, "Example ISP");
  assert.equal(
    parseIPDetails({ ...sample, country_code: "US<script>" }, "192.0.2.1").flag,
    "",
  );
});

test("simultaneous probes reuse requests, with distinct IPs and a 15-minute expiry", async () => {
  let time = 0;
  const requests = [];
  const lookup = createIPDetailsLookup(
    async (url) => {
      requests.push(url);
      return { ...sample, ip: decodeURIComponent(url.split("/").at(-1)) };
    },
    () => time,
  );
  const first = lookup("192.0.2.1");
  assert.equal(lookup("192.0.2.1"), first);
  await first;
  await lookup("192.0.2.1");
  assert.equal(requests.length, 1);
  await lookup("192.0.2.2");
  assert.equal(requests.length, 2);
  time = 15 * 60_000;
  await lookup("192.0.2.1");
  assert.equal(requests.length, 3);
  assert.equal(await lookup("not an IP"), null);
  assert.equal(requests.length, 3);
});

test("lookup failure can be retried and never poisons successful IP probes", async () => {
  let fail = true;
  const lookup = createIPDetailsLookup(async () => {
    if (fail) throw Error("offline");
    return sample;
  });
  await assert.rejects(lookup("192.0.2.1"), /offline/);
  fail = false;
  assert.equal((await lookup("192.0.2.1")).flag, "🇺🇸");
});

test("cache size stays bounded as exit addresses change", async () => {
  let requests = 0;
  const lookup = createIPDetailsLookup(async (url) => {
    requests++;
    return { ...sample, ip: url.split("/").at(-1) };
  });
  for (let i = 1; i <= 33; i++) await lookup(`192.0.2.${i}`);
  await lookup("192.0.2.1");
  assert.equal(requests, 34);
});
