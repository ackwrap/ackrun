package traceroute

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReservedNetworkMetadata(t *testing.T) {
	client, err := newMetadataClient("ipapi.is")
	if err != nil {
		t.Fatal(err)
	}
	geo, err := client.lookupGeo(context.Background(), net.ParseIP("192.168.1.1"))
	if err != nil {
		t.Fatal(err)
	}
	if geo.Whois != "RFC1918" || geo.Source != "reserved" {
		t.Fatalf("unexpected reserved metadata: %+v", geo)
	}
}

func TestNormalizeGeoProvider(t *testing.T) {
	for input, expected := range map[string]string{
		"": "disable-geoip", "IP.SB": "ip.sb", "ipapi.com": "ip-api.com", "none": "disable-geoip",
	} {
		actual, err := NormalizeGeoProvider(input)
		if err != nil || actual != expected {
			t.Fatalf("NormalizeGeoProvider(%q) = %q, %v; want %q", input, actual, err, expected)
		}
	}
	if _, err := NormalizeGeoProvider("unknown"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestIPAPIISProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("q") != "203.0.113.1" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"company":{"domain":"example.net","netname":"EXAMPLE-NET"},"asn":{"asn":64500,"org":"Example","route":"203.0.113.0/24"},"location":{"country":"China","country_code":"CN","state":"Guangdong","city":"Guangzhou"}}`))
	}))
	defer server.Close()

	provider := newIPAPIISProvider(server.URL)
	geo, err := provider.Lookup(context.Background(), "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}
	if geo.ASN != "64500" || geo.Country != "中国" || geo.Province != "广东" || geo.City != "广州" || geo.Owner != "example.net" {
		t.Fatalf("unexpected Geo metadata: %+v", geo)
	}
}
