package geoquery

import (
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

type GeoIPReader struct {
	reader     *maxminddb.Reader
	prefixes   map[string][]netip.Prefix
	prefixOnce sync.Once
	prefixErr  error
}

func OpenGeoIP(path string) (*GeoIPReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := readDatabase(file)
	if err != nil {
		return nil, err
	}
	database, dbErr := maxminddb.FromBytes(data)
	if dbErr == nil {
		if database.Metadata.DatabaseType != "sing-geoip" {
			database.Close()
			return nil, fmt.Errorf("incorrect database type, expected sing-geoip, got %s", database.Metadata.DatabaseType)
		}
		return &GeoIPReader{reader: database}, nil
	}
	prefixes, err := parseGeoIPDAT(data)
	if err != nil {
		return nil, err
	}
	return &GeoIPReader{prefixes: prefixes}, nil
}

// Lookup keeps the country-oriented API used by node exit information. An ASN
// category must not shadow a matching country.
func (r *GeoIPReader) Lookup(addr netip.Addr) string {
	codes := r.LookupCodes(addr)
	for _, code := range codes {
		if len(code) == 2 && code[0] >= 'a' && code[0] <= 'z' && code[1] >= 'a' && code[1] <= 'z' {
			return code
		}
	}
	if len(codes) > 0 {
		return codes[0]
	}
	return "unknown"
}

func (r *GeoIPReader) LookupCodes(addr netip.Addr) []string {
	if !addr.IsValid() {
		return nil
	}
	addr = addr.Unmap()
	if r.reader != nil {
		var code string
		if r.reader.Lookup(addr.AsSlice(), &code) == nil && code != "" {
			return []string{strings.ToLower(code)}
		}
		return nil
	}
	var codes []string
	// Queries are occasional; scanning avoids a second full prefix index in
	// memory on routers, including during downloads and rule-set conversion.
	for code, prefixes := range r.prefixes {
		for _, prefix := range prefixes {
			if prefix.Contains(addr) {
				codes = append(codes, code)
				break
			}
		}
	}
	slices.Sort(codes)
	return codes
}

func (r *GeoIPReader) loadPrefixes() {
	if r.reader == nil {
		return
	}
	r.prefixes = make(map[string][]netip.Prefix)
	networks := r.reader.Networks(maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		var code string
		network, err := networks.Network(&code)
		if err != nil {
			r.prefixErr = err
			return
		}
		addr, ok := netip.AddrFromSlice(network.IP)
		bits, _ := network.Mask.Size()
		if !ok {
			r.prefixErr = fmt.Errorf("invalid GeoIP database network")
			return
		}
		code = strings.ToLower(code)
		r.prefixes[code] = append(r.prefixes[code], netip.PrefixFrom(addr.Unmap(), bits).Masked())
	}
	r.prefixErr = networks.Err()
}

func (r *GeoIPReader) Codes() ([]string, error) {
	r.prefixOnce.Do(r.loadPrefixes)
	if r.prefixErr != nil {
		return nil, r.prefixErr
	}
	codes := make([]string, 0, len(r.prefixes))
	for code := range r.prefixes {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes, nil
}

func (r *GeoIPReader) Prefixes(code string) ([]netip.Prefix, error) {
	r.prefixOnce.Do(r.loadPrefixes)
	if r.prefixErr != nil {
		return nil, r.prefixErr
	}
	prefixes, exists := r.prefixes[strings.ToLower(code)]
	if !exists {
		return nil, fmt.Errorf("geoip code %q not exists", code)
	}
	return slices.Clone(prefixes), nil
}

func (r *GeoIPReader) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	return nil
}
