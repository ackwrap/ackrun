package geoquery

import (
	"bytes"
	"encoding/binary"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func datBytes(number protowire.Number, value []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, number, protowire.BytesType), value)
}

func datVarint(number protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, number, protowire.VarintType), value)
}

func datMessage(fields ...[]byte) []byte { return bytes.Join(fields, nil) }

func datCategory(code string, entries ...[]byte) []byte {
	entry := datBytes(1, []byte(code))
	for _, item := range entries {
		entry = append(entry, datBytes(2, item)...)
	}
	return datBytes(1, entry)
}

func datCIDR(value string) []byte {
	prefix := netip.MustParsePrefix(value)
	return datMessage(datBytes(1, prefix.Addr().AsSlice()), datVarint(2, uint64(prefix.Bits())))
}

func datDomain(kind uint64, value string, attributes ...[]byte) []byte {
	domain := datMessage(datVarint(1, kind), datBytes(2, []byte(value)))
	for _, attribute := range attributes {
		domain = append(domain, datBytes(3, attribute)...)
	}
	return domain
}

func writeFixture(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGeoIPDATOverlappingCategories(t *testing.T) {
	data := datMessage(
		datCategory("AU", datCIDR("1.0.0.0/8")),
		datCategory("CLOUDFLARE", datCIDR("1.1.1.0/24"), datCIDR("2001:db8::/32")),
		datCategory("OTHER", datCIDR("1.1.1.0/24"), datCIDR("1.1.1.1/32")),
		datCategory("PRIVATE", datCIDR("192.168.0.0/16")),
	)
	// The suffix deliberately differs from the format: detection is by content.
	reader, err := OpenGeoIP(writeFixture(t, "geoip.db", data))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	address := netip.MustParseAddr("1.1.1.1")
	if got := reader.LookupCodes(address); !reflect.DeepEqual(got, []string{"au", "cloudflare", "other"}) {
		t.Fatalf("overlapping categories = %v", got)
	}
	if got := reader.Lookup(address); got != "au" {
		t.Fatalf("country lookup = %q, want au", got)
	}
	if got := reader.Lookup(netip.MustParseAddr("192.168.1.2")); got != "private" {
		t.Fatalf("private lookup = %q", got)
	}
	if got := reader.LookupCodes(netip.MustParseAddr("2001:db8::1")); !reflect.DeepEqual(got, []string{"cloudflare"}) {
		t.Fatalf("IPv6 lookup = %v", got)
	}
	if got := reader.Lookup(netip.MustParseAddr("2.2.2.2")); got != "unknown" {
		t.Fatalf("unmatched lookup = %q", got)
	}
	if got := reader.LookupCodes(netip.Addr{}); len(got) != 0 {
		t.Fatalf("invalid address lookup = %v", got)
	}
	codes, err := reader.Codes()
	if err != nil || !reflect.DeepEqual(codes, []string{"au", "cloudflare", "other", "private"}) {
		t.Fatalf("Codes = %v, %v", codes, err)
	}
	prefixes, err := reader.Prefixes("CLOUDFLARE")
	if err != nil || !reflect.DeepEqual(prefixes, []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24"), netip.MustParsePrefix("2001:db8::/32")}) {
		t.Fatalf("Prefixes = %v, %v", prefixes, err)
	}
	prefixes[0] = netip.Prefix{}
	if again, _ := reader.Prefixes("cloudflare"); !again[0].IsValid() {
		t.Fatal("Prefixes exposed mutable internal state")
	}
	if _, err := reader.Prefixes("missing"); err == nil {
		t.Fatal("missing category succeeded")
	}
}

func TestGeositeDATDomainTypesAndAttributes(t *testing.T) {
	ads := datMessage(datBytes(1, []byte("ads")), datVarint(2, 1))
	// Attribute selection uses presence; false/int payloads do not erase a key.
	cn := datMessage(datBytes(1, []byte("cn")), datVarint(2, 0))
	nonCN := datMessage(datBytes(1, []byte("!cn")), datVarint(3, 7))
	data := datCategory("EXAMPLE",
		datDomain(0, "keyword", ads),
		datDomain(1, `^regexp[0-9]+\.test$`, nonCN),
		datDomain(2, "example.com", cn, cn),
		datDomain(3, "full.test", ads, cn),
	)
	reader, codes, err := OpenGeosite(writeFixture(t, "geosite.db", data))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if !reflect.DeepEqual(codes, []string{"example", "example@!cn", "example@ads", "example@cn"}) {
		t.Fatalf("codes = %v", codes)
	}
	items, err := reader.Read("EXAMPLE")
	if err != nil {
		t.Fatal(err)
	}
	want := []GeositeItem{
		{Type: GeositeRuleTypeDomainKeyword, Value: "keyword"},
		{Type: GeositeRuleTypeDomainRegex, Value: `^regexp[0-9]+\.test$`},
		{Type: GeositeRuleTypeDomainSuffix, Value: "example.com"},
		{Type: GeositeRuleTypeDomain, Value: "full.test"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, want %#v", items, want)
	}
	for code, expected := range map[string][]GeositeItem{
		"example@ads": {want[0], want[3]},
		"example@cn":  {want[2], want[3]},
		"example@!cn": {want[1]},
	} {
		got, err := reader.Read(code)
		if err != nil || !reflect.DeepEqual(got, expected) {
			t.Fatalf("Read(%q) = %#v, %v", code, got, err)
		}
	}
	matcher := NewGeositeMatcher(items)
	for _, domain := range []string{"has-keyword.test", "regexp42.test", "example.com", "www.example.com", "full.test"} {
		if matcher.Match(domain) == "" {
			t.Errorf("did not match %q", domain)
		}
	}
	for _, domain := range []string{"badexample.com", "full.test.bad", "child.full.test", "regexpno.test"} {
		if got := matcher.Match(domain); got != "" {
			t.Errorf("unexpected match %q: %q", domain, got)
		}
	}
	items[0].Value = "changed"
	if again, _ := reader.Read("example"); again[0].Value != "keyword" {
		t.Fatal("Read exposed mutable internal state")
	}
	if _, err := reader.Read("missing"); err == nil {
		t.Fatal("missing category succeeded")
	}
}

func TestDATRejectsMalformedOrUnsupportedSemantics(t *testing.T) {
	validIP := datCategory("cn", datCIDR("1.1.1.0/24"))
	validSite := datCategory("example", datDomain(2, "example.com"))
	oversize := protowire.AppendVarint([]byte{0x0a}, math.MaxUint64)
	for name, parse := range map[string]func([]byte) error{
		"geoip":   func(data []byte) error { _, err := parseGeoIPDAT(data); return err },
		"geosite": func(data []byte) error { _, err := parseGeositeDAT(data); return err },
	} {
		t.Run(name, func(t *testing.T) {
			data := validIP
			if name == "geosite" {
				data = validSite
			}
			for i := 0; i < len(data); i++ {
				if err := parse(data[:i]); err == nil {
					t.Errorf("truncated at byte %d succeeded", i)
				}
			}
			for _, bad := range [][]byte{oversize, {0xff}, {0}, datVarint(1, 1), datBytes(1, nil), []byte("not a database")} {
				if err := parse(bad); err == nil {
					t.Errorf("malformed input %x succeeded", bad)
				}
			}
		})
	}
	badIPs := [][]byte{
		datBytes(1, datMessage(datBytes(1, []byte("cn")), datVarint(3, 1))),
		datCategory("cn", datMessage(datBytes(1, []byte{1, 2, 3, 4}), datVarint(2, 33))),
		datCategory("cn", datMessage(datBytes(1, []byte{1, 2, 3}), datVarint(2, 24))),
		datCategory("cn", datBytes(1, []byte{1, 2, 3, 4, 5})),
		datCategory("cn", datMessage(datBytes(1, []byte{1, 2, 3, 4}), datVarint(2, math.MaxUint64))),
		datBytes(1, datMessage(datBytes(1, []byte("cn")), datBytes(4, []byte("external")))),
		datMessage(validIP, validIP),
		validSite,
		datCategory(strings.Repeat("x", 256), datCIDR("1.1.1.0/24")),
	}
	for i, data := range badIPs {
		if _, err := parseGeoIPDAT(data); err == nil {
			t.Errorf("bad GeoIP case %d succeeded", i)
		}
	}
	badSites := [][]byte{
		datCategory("example", datDomain(4, "example.com")),
		datCategory("example", datDomain(1, "[")),
		datCategory("example", datDomain(2, "")),
		datCategory("example", datDomain(2, "example.com", nil)),
		datCategory("example", datMessage(datVarint(1, 2), datVarint(2, 1))),
		datCategory("example", datMessage(datVarint(1, 2), datBytes(2, []byte{0xff}))),
		datBytes(1, datMessage(datBytes(1, []byte("example")), datBytes(3, []byte("external")))),
		datMessage(validSite, validSite),
		validIP,
		datCategory("example", datDomain(2, "example.com", datBytes(1, []byte(strings.Repeat("x", 256))))),
	}
	for i, data := range badSites {
		if _, err := parseGeositeDAT(data); err == nil {
			t.Errorf("bad GeoSite case %d succeeded", i)
		}
	}
}

func legacyGeositeFixture(items []GeositeItem) []byte {
	data := []byte{0, 1} // version and category count
	data = binary.AppendUvarint(data, uint64(len("example")))
	data = append(data, "example"...)
	data = binary.AppendUvarint(data, 0)
	data = binary.AppendUvarint(data, uint64(len(items)))
	for _, item := range items {
		data = append(data, item.Type)
		data = binary.AppendUvarint(data, uint64(len(item.Value)))
		data = append(data, item.Value...)
	}
	return data
}

func TestLegacyGeositeDatabaseCompatibility(t *testing.T) {
	want := []GeositeItem{
		{Type: GeositeRuleTypeDomain, Value: "example.com"},
		{Type: GeositeRuleTypeDomainSuffix, Value: ".example.com"},
		{Type: GeositeRuleTypeDomainKeyword, Value: "keyword"},
		{Type: GeositeRuleTypeDomainRegex, Value: `^regex\.test$`},
	}
	reader, codes, err := OpenGeosite(writeFixture(t, "geosite.dat", legacyGeositeFixture(want)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if !reflect.DeepEqual(codes, []string{"example"}) {
		t.Fatalf("codes = %v", codes)
	}
	items, err := reader.Read("example")
	if err != nil || !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, %v", items, err)
	}
	matcher := NewGeositeMatcher([]GeositeItem{{Type: GeositeRuleTypeDomainSuffix, Value: ".example.com"}})
	if matcher.Match("www.example.com") == "" || matcher.Match("example.com") != "" || matcher.Match("badexample.com") != "" {
		t.Fatal("leading-dot legacy suffix behavior changed")
	}
}

func TestLegacyGeositeRejectsInvalidLengths(t *testing.T) {
	largeCount := binary.AppendUvarint([]byte{0}, math.MaxUint64)
	largeString := binary.AppendUvarint([]byte{0, 1}, math.MaxUint64)
	largeOffset := binary.AppendUvarint([]byte{0, 1, 1, 'a'}, math.MaxUint64)
	largeOffset = append(largeOffset, 1, 0, 0)
	largeItemCount := binary.AppendUvarint([]byte{0, 1, 1, 'a', 0}, math.MaxUint64)
	largeItemCount = append(largeItemCount, 0, 0)
	for _, data := range [][]byte{largeCount, largeString, largeOffset, largeItemCount} {
		if _, _, err := NewGeositeReader(bytes.NewReader(data)); err == nil {
			t.Errorf("malformed metadata %x succeeded", data)
		}
	}
	badItem := []byte{0, 1, 1, 'a', 0, 1, 0}
	badItem = binary.AppendUvarint(badItem, math.MaxUint64)
	reader, _, err := NewGeositeReader(bytes.NewReader(badItem))
	if err == nil {
		if _, err := reader.Read("a"); err == nil {
			t.Fatal("oversized item string succeeded")
		}
	}
}

// A complete two-leaf MaxMind DB with string data records, the same record
// representation used by sing-geoip. It needs no fixture downloads or writer dependency.
func legacyGeoIPFixture(databaseType string) []byte {
	mmdbString := func(value string) []byte { return append([]byte{byte(2<<5 | len(value))}, value...) }
	data := []byte{0, 0, 17, 0, 0, 20} // 24-bit pointers into cn/us data records
	data = append(data, make([]byte, 16)...)
	data = append(data, mmdbString("cn")...)
	data = append(data, mmdbString("us")...)
	data = append(data, []byte("\xab\xcd\xefMaxMind.com")...)
	data = append(data, 7<<5|4) // metadata map with four fields
	data = append(data, mmdbString("database_type")...)
	data = append(data, mmdbString(databaseType)...)
	data = append(data, mmdbString("node_count")...)
	data = append(data, 6<<5|1, 1)
	data = append(data, mmdbString("record_size")...)
	data = append(data, 5<<5|1, 24)
	data = append(data, mmdbString("ip_version")...)
	data = append(data, 5<<5|1, 4)
	return data
}

func TestLegacyGeoIPDatabaseCompatibility(t *testing.T) {
	reader, err := OpenGeoIP(writeFixture(t, "geoip.dat", legacyGeoIPFixture("sing-geoip")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if got := reader.Lookup(netip.MustParseAddr("1.1.1.1")); got != "cn" {
		t.Fatalf("lookup = %q", got)
	}
	if got := reader.LookupCodes(netip.MustParseAddr("192.0.2.1")); !reflect.DeepEqual(got, []string{"us"}) {
		t.Fatalf("lookup codes = %v", got)
	}
	codes, err := reader.Codes()
	if err != nil || !reflect.DeepEqual(codes, []string{"cn", "us"}) {
		t.Fatalf("codes = %v, %v", codes, err)
	}
	prefixes, err := reader.Prefixes("cn")
	if err != nil || !reflect.DeepEqual(prefixes, []netip.Prefix{netip.MustParsePrefix("0.0.0.0/1")}) {
		t.Fatalf("prefixes = %v, %v", prefixes, err)
	}
	if _, err := OpenGeoIP(writeFixture(t, "wrong.db", legacyGeoIPFixture("wrong"))); err == nil {
		t.Fatal("wrong MMDB database type succeeded")
	}
}

func TestDatabaseSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.dat")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxDatabaseSize + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenGeoIP(path); err == nil {
		t.Error("oversized GeoIP database succeeded")
	}
	if _, _, err := OpenGeosite(path); err == nil {
		t.Error("oversized GeoSite database succeeded")
	}
}
