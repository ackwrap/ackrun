package service

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGeoPrivateRuleSetPrefixes(t *testing.T) {
	source := []byte(`{"version":3,"rules":[{"ip_cidr":["10.0.0.1/8"]},{"type":"default","ip_cidr":["fc00::/7"]}]}`)
	prefixes, err := geoPrivateRuleSetPrefixes(source)
	want := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fc00::/7")}
	if err != nil || !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("prefixes = %v, %v", prefixes, err)
	}
	for name, source := range map[string]string{
		"invalid_json":    `{`,
		"unknown_version": `{"version":99,"rules":[{"ip_cidr":["10.0.0.0/8"]}]}`,
		"empty":           `{"version":3,"rules":[]}`,
		"empty_rule":      `{"version":3,"rules":[{}]}`,
		"invalid_cidr":    `{"version":3,"rules":[{"ip_cidr":["invalid"]}]}`,
		"inverse":         `{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"],"invert":true}]}`,
		"mixed_domain":    `{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"],"domain":["example.com"]}]}`,
		"logical":         `{"version":3,"rules":[{"type":"logical","mode":"and","rules":[{"ip_cidr":["10.0.0.0/8"]}]}]}`,
		"trailing_json":   `{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]} {}`,
		"trailing_data":   `{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]} broken`,
		"oversized":       strings.Repeat(" ", geoPrivateRuleSetMaxSize+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := geoPrivateRuleSetPrefixes([]byte(source)); err == nil {
				t.Fatal("invalid or non-CIDR source was accepted")
			}
		})
	}
}

func TestDecompileGeoPrivateRuleSetFailureCleansTemporaryFiles(t *testing.T) {
	directory := t.TempDir()
	if _, err := decompileGeoPrivateRuleSet(context.Background(), filepath.Join(directory, "missing-core"), directory, []byte("SRS")); err == nil {
		t.Fatal("missing core was accepted")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files remain: %v, %v", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := decompileGeoPrivateRuleSet(ctx, "unused", directory, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was lost: %v", err)
	}
	if _, err := decompileGeoPrivateRuleSet(context.Background(), "unused", directory, make([]byte, geoPrivateRuleSetMaxSize+1)); err == nil {
		t.Fatal("oversized SRS was accepted")
	}
}

// Uses the same opt-in local fixtures as the Geo source compatibility test.
// No network requests occur: the official private SRS is seeded into its cache.
func TestSagerNetPrivateRuleSetRealCompatibility(t *testing.T) {
	directory, core := os.Getenv("ACKWRAP_TEST_GEO_DIR"), os.Getenv("ACKWRAP_TEST_SING_BOX")
	if directory == "" || core == "" {
		t.Skip("set ACKWRAP_TEST_GEO_DIR and ACKWRAP_TEST_SING_BOX for private SRS compatibility testing")
	}
	svc, _ := geoSourceTestService(t)
	svc.paths.BinaryPath = core
	svc.ruleSetValidator = nil
	binary, err := os.ReadFile(filepath.Join(directory, "upstream-geoip-private.srs"))
	if err != nil {
		t.Fatal(err)
	}
	cacheDirectory := filepath.Join(svc.paths.RulesDir, "geo")
	if err := os.MkdirAll(cacheDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDirectory, "geoip-private.srs"), binary, 0644); err != nil {
		t.Fatal(err)
	}
	source, err := svc.sagerNetPrivateRuleSetSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prefixes, err := svc.sagerNetPrivatePrefixes(context.Background())
	if err != nil || len(prefixes) == 0 {
		t.Fatalf("prefixes = %v, %v", prefixes, err)
	}
	fromSource, err := geoPrivateRuleSetPrefixes(source)
	if err != nil || !reflect.DeepEqual(prefixes, fromSource) {
		t.Fatalf("JSON and lookup prefixes differ: %v", err)
	}
	for target, want := range map[string]bool{"10.0.0.1": true, "fc00::1": true, "8.8.8.8": false} {
		addr := netip.MustParseAddr(target)
		matched := false
		for _, prefix := range prefixes {
			matched = matched || prefix.Contains(addr)
		}
		if matched != want {
			t.Errorf("private match for %s = %v, want %v", target, matched, want)
		}
	}
	leftovers, err := filepath.Glob(filepath.Join(svc.paths.RulesDir, ".geoip-private-*.json"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary JSON remains: %v, %v", leftovers, err)
	}
	t.Logf("decoded official private SRS to %d bytes and %d prefixes", len(source), len(prefixes))
}
