package geoquery

import (
	"fmt"
	"net/netip"

	"github.com/oschwald/maxminddb-golang"
)

type GeoIPReader struct {
	reader *maxminddb.Reader
}

func OpenGeoIP(path string) (*GeoIPReader, error) {
	database, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	if database.Metadata.DatabaseType != "sing-geoip" {
		database.Close()
		return nil, fmt.Errorf("incorrect database type, expected sing-geoip, got %s", database.Metadata.DatabaseType)
	}
	return &GeoIPReader{reader: database}, nil
}

func (r *GeoIPReader) Lookup(addr netip.Addr) string {
	var code string
	_ = r.reader.Lookup(addr.AsSlice(), &code)
	if code != "" {
		return code
	}
	return "unknown"
}

func (r *GeoIPReader) Close() error {
	return r.reader.Close()
}
