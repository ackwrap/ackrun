package traceroute

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

type ipInfoLocalProvider struct {
	reader *maxminddb.Reader
}

func newIPInfoLocalProvider() (geoProvider, error) {
	path := strings.TrimSpace(os.Getenv("NEXTTRACE_IPINFOLOCALPATH"))
	if path == "" {
		return nil, errors.New("IPInfoLocal requires NEXTTRACE_IPINFOLOCALPATH")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open IPInfoLocal database: %w", err)
	}
	return &ipInfoLocalProvider{reader: reader}, nil
}

func (p *ipInfoLocalProvider) Name() string {
	return "IPInfoLocal"
}

func (p *ipInfoLocalProvider) Close() error {
	return p.reader.Close()
}

func (p *ipInfoLocalProvider) Lookup(_ context.Context, ip string) (GeoData, error) {
	var record map[string]any
	if err := p.reader.Lookup(net.ParseIP(ip), &record); err != nil {
		return GeoData{}, err
	}
	country := anyString(record["country_name"])
	countryCode := anyString(record["country"])
	province := ""
	if countryCode == "HK" || countryCode == "TW" || countryCode == "MO" {
		country, province = "China", countryNameFromCode(countryCode)
	}
	return GeoData{
		ASN:       strings.TrimPrefix(anyString(record["asn"]), "AS"),
		Country:   localizeLocation(countryCode, country),
		CountryEn: country,
		Province:  localizeLocation("", province),
		Owner:     anyString(record["as_name"]),
		Source:    "IPInfoLocal",
	}, nil
}

type dn42Row struct {
	network *net.IPNet
	country string
	city    string
	asn     string
	owner   string
	ones    int
}

type dn42Provider struct {
	rows []dn42Row
}

func newDN42Provider() (geoProvider, error) {
	path := strings.TrimSpace(os.Getenv("NEXTTRACE_DN42_GEOFEED"))
	if path == "" {
		return nil, errors.New("DN42 requires NEXTTRACE_DN42_GEOFEED")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open DN42 geofeed: %w", err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read DN42 geofeed: %w", err)
	}
	rows := make([]dn42Row, 0, len(records))
	for _, record := range records {
		if len(record) < 4 {
			continue
		}
		_, network, err := net.ParseCIDR(strings.TrimSpace(record[0]))
		if err != nil {
			continue
		}
		ones, _ := network.Mask.Size()
		row := dn42Row{network: network, country: record[1], city: record[3], ones: ones}
		if len(record) >= 6 {
			row.asn, row.owner = record[4], record[5]
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("DN42 geofeed contains no valid rows")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ones > rows[j].ones })
	return &dn42Provider{rows: rows}, nil
}

func (p *dn42Provider) Name() string {
	return "DN42"
}

func (p *dn42Provider) Lookup(_ context.Context, ip string) (GeoData, error) {
	parsed := net.ParseIP(ip)
	for _, row := range p.rows {
		if row.network.Contains(parsed) {
			country := countryNameFromCode(row.country)
			return GeoData{
				ASN: row.asn, Country: localizeLocation(row.country, country), CountryEn: country,
				City: row.city, Owner: row.owner, Source: "DN42",
			}, nil
		}
	}
	return GeoData{Country: "Unknown", Source: "DN42"}, nil
}
