package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/traceroute"
)

const customGeoMaxResponse = 64 << 10

type resolvedNodeGeoProvider struct {
	Key    string
	Name   string
	Lookup traceroute.GeoLookupFunc
}

func (svc *NodeService) resolveNodeGeoProvider(key string) (resolvedNodeGeoProvider, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		item, err := svc.store.GetDefaultGeoIPProvider()
		if err != nil {
			return resolvedNodeGeoProvider{}, err
		}
		key = item.Key
	}
	if key == traceroute.DefaultGeoProvider {
		return resolvedNodeGeoProvider{Key: key, Name: key}, nil
	}
	item, err := svc.store.GetGeoIPProviderByKey(key)
	if errors.Is(err, sql.ErrNoRows) {
		return resolvedNodeGeoProvider{}, fmt.Errorf("unsupported Geo provider %q", key)
	}
	if err != nil {
		return resolvedNodeGeoProvider{}, err
	}
	if !item.Enabled {
		return resolvedNodeGeoProvider{}, fmt.Errorf("Geo provider %q is disabled", item.Name)
	}
	if item.Builtin {
		normalized, err := traceroute.ValidateGeoProvider(item.Key)
		if err != nil {
			return resolvedNodeGeoProvider{}, err
		}
		return resolvedNodeGeoProvider{Key: normalized, Name: item.Name}, nil
	}
	provider := *item
	return resolvedNodeGeoProvider{
		Key:  provider.Key,
		Name: provider.Name,
		Lookup: func(ctx context.Context, ip net.IP) (traceroute.GeoData, error) {
			return lookupCustomGeo(ctx, ip, provider)
		},
	}, nil
}

func (provider resolvedNodeGeoProvider) lookup(ctx context.Context, ip net.IP) (traceroute.GeoData, error) {
	if provider.Lookup != nil {
		return provider.Lookup(ctx, ip)
	}
	return traceroute.LookupGeo(ctx, ip, provider.Key)
}

func lookupCustomGeo(ctx context.Context, ip net.IP, provider model.GeoIPProvider) (traceroute.GeoData, error) {
	rawURL := provider.URL
	if strings.Contains(rawURL, "{ip}") {
		rawURL = strings.ReplaceAll(rawURL, "{ip}", url.PathEscape(ip.String()))
	} else {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return traceroute.GeoData{}, err
		}
		query := parsed.Query()
		query.Set(provider.IPParameter, ip.String())
		parsed.RawQuery = query.Encode()
		rawURL = parsed.String()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return traceroute.GeoData{}, errors.New("Geo API request configuration is invalid")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Ackwrap/1")
	response, err := customGeoHTTPClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return traceroute.GeoData{}, errors.New("Geo API request timed out")
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return traceroute.GeoData{}, errors.New("Geo API request timed out")
		}
		return traceroute.GeoData{}, errors.New("Geo API request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return traceroute.GeoData{}, fmt.Errorf("Geo API returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, customGeoMaxResponse+1))
	if err != nil {
		return traceroute.GeoData{}, errors.New("read Geo API response failed")
	}
	if len(body) > customGeoMaxResponse {
		return traceroute.GeoData{}, errors.New("Geo API response exceeds 64 KiB")
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return traceroute.GeoData{}, fmt.Errorf("decode Geo API response: %w", err)
	}
	mapping := provider.Mapping
	geo := traceroute.GeoData{
		ASN:        mappedString(payload, mapping.ASN),
		Country:    mappedString(payload, mapping.Country),
		CountryEn:  mappedString(payload, mapping.CountryEn),
		Province:   mappedString(payload, mapping.Province),
		ProvinceEn: mappedString(payload, mapping.ProvinceEn),
		City:       mappedString(payload, mapping.City),
		CityEn:     mappedString(payload, mapping.CityEn),
		District:   mappedString(payload, mapping.District),
		Owner:      mappedString(payload, mapping.Owner),
		ISP:        mappedString(payload, mapping.ISP),
		Domain:     mappedString(payload, mapping.Domain),
		Whois:      mappedString(payload, mapping.Whois),
		Prefix:     mappedString(payload, mapping.Prefix),
		Source:     provider.Name,
	}
	geo.ASN = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(geo.ASN), "AS"), "as")
	geo.Latitude, _ = strconv.ParseFloat(mappedString(payload, mapping.Latitude), 64)
	geo.Longitude, _ = strconv.ParseFloat(mappedString(payload, mapping.Longitude), 64)
	countryCode := mappedString(payload, mapping.CountryCode)
	if countryCode != "" {
		localized := traceroute.GeoDataFromCountryCode(countryCode, provider.Name)
		if geo.Country == "" {
			geo.Country = localized.Country
			geo.CountryEn = localized.CountryEn
		} else if localized.Country != countryCode {
			geo.CountryEn = geo.Country
			geo.Country = localized.Country
		}
	}
	if geo.Country == "" && geo.ASN == "" && geo.Owner == "" && geo.ISP == "" {
		return traceroute.GeoData{}, errors.New("Geo API response did not match the configured JSON fields")
	}
	return geo, nil
}

func mappedString(payload any, path string) string {
	if path == "" {
		return ""
	}
	current := payload
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	switch typed := current.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

var customGeoHTTPClient = newSafeDirectGeoHTTPClient()

func newSafeDirectGeoHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if !isPublicGeoIP(address.IP) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		}
		return nil, errors.New("Geo API host resolved only to local or private addresses")
	}
	return &http.Client{
		Transport: transport,
		Timeout:   4 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("Geo API redirected too many times")
			}
			if request.URL.Scheme != "https" || request.URL.Host == "" || request.URL.User != nil {
				return errors.New("Geo API redirect must remain on HTTPS")
			}
			return nil
		},
	}
}

func isPublicGeoIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	_, carrierGradeNAT, _ := net.ParseCIDR("100.64.0.0/10")
	return !carrierGradeNAT.Contains(ip)
}
