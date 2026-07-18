package traceroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultGeoProvider  = "disable-geoip"
	defaultGeoEndpoint  = "https://api.ipapi.is/"
	maxGeoResponseBytes = 1 << 20
)

var asNumberPattern = regexp.MustCompile(`[0-9]+`)

type geoProvider interface {
	Name() string
	Lookup(context.Context, string) (GeoData, error)
}

type geoProviderFunc struct {
	name   string
	lookup func(context.Context, string) (GeoData, error)
}

func (p geoProviderFunc) Name() string {
	return p.name
}

func (p geoProviderFunc) Lookup(ctx context.Context, ip string) (GeoData, error) {
	return p.lookup(ctx, ip)
}

func NormalizeGeoProvider(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return DefaultGeoProvider, nil
	case "ipapi.is", "ipapiis":
		return "ipapi.is", nil
	case "leomoeapi", "leo":
		return "leomoeapi", nil
	case "ip.sb", "ipsb":
		return "ip.sb", nil
	case "ipinfo":
		return "ipinfo", nil
	case "ipinsight":
		return "ipinsight", nil
	case "ipapi.com", "ip-api.com", "ipapi":
		return "ip-api.com", nil
	case "baidu", "baidu-ip":
		return "baidu", nil
	case "songzixian", "songzi":
		return "songzixian", nil
	case "ipinfolocal":
		return "ipinfolocal", nil
	case "chunzhen":
		return "chunzhen", nil
	case "ipdb.one", "ipdbone":
		return "ipdb.one", nil
	case "dn42":
		return "dn42", nil
	case "disable-geoip", "disable", "none":
		return "disable-geoip", nil
	default:
		return "", fmt.Errorf("unsupported Geo provider %q", name)
	}
}

func ValidateGeoProvider(name string) (string, error) {
	normalized, err := NormalizeGeoProvider(name)
	if err != nil {
		return "", err
	}
	provider, err := newGeoProvider(normalized)
	if err != nil {
		return "", err
	}
	if closeable, ok := provider.(interface{ Close() error }); ok {
		_ = closeable.Close()
	}
	return normalized, nil
}

func newGeoProvider(name string) (geoProvider, error) {
	normalized, err := NormalizeGeoProvider(name)
	if err != nil {
		return nil, err
	}
	switch normalized {
	case "ipapi.is":
		return newIPAPIISProvider(endpointFromEnv("NEXTTRACE_MINI_IPAPIIS_BASE", defaultGeoEndpoint)), nil
	case "leomoeapi":
		return newLeoMoeProvider(), nil
	case "ip.sb":
		return newIPSBProvider(), nil
	case "ipinfo":
		return newIPInfoProvider(), nil
	case "ipinsight":
		return newIPInsightProvider(), nil
	case "ip-api.com":
		return newIPAPIComProvider(), nil
	case "baidu":
		return newBaiduIPProvider(), nil
	case "songzixian":
		return newSongzixianIPProvider(), nil
	case "ipinfolocal":
		return newIPInfoLocalProvider()
	case "chunzhen":
		return newChunzhenProvider(), nil
	case "ipdb.one":
		return newIPDBOneProvider()
	case "dn42":
		return newDN42Provider()
	default:
		return geoProviderFunc{name: "disable-geoip", lookup: func(context.Context, string) (GeoData, error) {
			return GeoData{Source: "disable-geoip"}, nil
		}}, nil
	}
}

type ipAPIISProvider struct {
	endpoint string
	client   *http.Client
}

func newIPAPIISProvider(endpoint string) *ipAPIISProvider {
	return &ipAPIISProvider{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 3 * time.Second},
	}
}

func (p *ipAPIISProvider) Name() string {
	return "ipapi.is"
}

func (p *ipAPIISProvider) Lookup(ctx context.Context, ip string) (GeoData, error) {
	var body struct {
		IsBogon bool `json:"is_bogon"`
		Company struct {
			Name    string `json:"name"`
			Domain  string `json:"domain"`
			Netname string `json:"netname"`
		} `json:"company"`
		ASN struct {
			Number int    `json:"asn"`
			Org    string `json:"org"`
			Domain string `json:"domain"`
			Route  string `json:"route"`
		} `json:"asn"`
		Location struct {
			Country     string  `json:"country"`
			CountryCode string  `json:"country_code"`
			State       string  `json:"state"`
			City        string  `json:"city"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
		} `json:"location"`
	}
	endpoint, err := url.Parse(p.endpoint)
	if err != nil {
		return GeoData{}, err
	}
	query := endpoint.Query()
	query.Set("q", ip)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return GeoData{}, err
	}
	request.Header.Set("User-Agent", "Ackwrap/1")
	response, err := p.client.Do(request)
	if err != nil {
		return GeoData{}, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxGeoResponseBytes+1))
	if err != nil {
		return GeoData{}, err
	}
	if len(content) > maxGeoResponseBytes {
		return GeoData{}, errors.New("Geo response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return GeoData{}, fmt.Errorf("Geo API returned %s", response.Status)
	}
	if err := json.Unmarshal(content, &body); err != nil {
		return GeoData{}, fmt.Errorf("decode Geo response: %w", err)
	}
	if body.IsBogon {
		return GeoData{Whois: "Reserved", Source: "ipapi.is"}, nil
	}
	return GeoData{
		ASN:        numberString(body.ASN.Number),
		Country:    localizeLocation(body.Location.CountryCode, body.Location.Country),
		CountryEn:  body.Location.Country,
		Province:   localizeLocation("", body.Location.State),
		ProvinceEn: body.Location.State,
		City:       localizeLocation("", body.Location.City),
		CityEn:     body.Location.City,
		Owner:      firstNonEmpty(body.Company.Domain, body.ASN.Domain, body.Company.Name, body.ASN.Org),
		Domain:     firstNonEmpty(body.Company.Domain, body.ASN.Domain),
		Whois:      body.Company.Netname,
		Latitude:   body.Location.Latitude,
		Longitude:  body.Location.Longitude,
		Prefix:     body.ASN.Route,
		Source:     "ipapi.is",
	}, nil
}

func newIPSBProvider() geoProvider {
	client := newGeoHTTPClient()
	base := endpointFromEnv("NEXTTRACE_IPAPI_BASE", "https://api.ip.sb/geoip/")
	return geoProviderFunc{name: "IP.SB", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		var body struct {
			ASN             int     `json:"asn"`
			Country         string  `json:"country"`
			CountryCode     string  `json:"country_code"`
			Region          string  `json:"region"`
			City            string  `json:"city"`
			ISP             string  `json:"isp"`
			Organization    string  `json:"organization"`
			ASNOrganization string  `json:"asn_organization"`
			Latitude        float64 `json:"latitude"`
			Longitude       float64 `json:"longitude"`
		}
		if err := fetchJSON(ctx, client, appendPath(base, ip), browserHeaders(), &body); err != nil {
			return GeoData{}, err
		}
		if body.Country == "" && body.ASN == 0 {
			return GeoData{}, errors.New("IP.SB returned no Geo or ASN data")
		}
		return GeoData{
			ASN:        numberString(body.ASN),
			Country:    localizeLocation(body.CountryCode, body.Country),
			CountryEn:  body.Country,
			Province:   localizeLocation("", body.Region),
			ProvinceEn: body.Region,
			City:       localizeLocation("", body.City),
			CityEn:     body.City,
			Owner:      firstNonEmpty(body.ISP, body.Organization, body.ASNOrganization),
			ISP:        body.ISP,
			Latitude:   body.Latitude,
			Longitude:  body.Longitude,
			Source:     "IP.SB",
		}, nil
	}}
}

func newIPInfoProvider() geoProvider {
	client := newGeoHTTPClient()
	base := endpointFromEnv("NEXTTRACE_IPAPI_BASE", "https://ipinfo.io/")
	token := strings.TrimSpace(os.Getenv("NEXTTRACE_IPINFO_TOKEN"))
	return geoProviderFunc{name: "IPInfo", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		var body struct {
			City    string `json:"city"`
			Region  string `json:"region"`
			Country string `json:"country"`
			Org     string `json:"org"`
			Anycast bool   `json:"anycast"`
			Loc     string `json:"loc"`
			Error   any    `json:"error"`
		}
		rawURL := appendPath(base, ip)
		if token != "" {
			rawURL += "?token=" + url.QueryEscape(token)
		}
		if err := fetchJSON(ctx, client, rawURL, nil, &body); err != nil {
			return GeoData{}, err
		}
		if body.Error != nil {
			return GeoData{}, fmt.Errorf("IPInfo: %v", body.Error)
		}
		asn, owner := splitASNOrg(body.Org)
		countryName := countryNameFromCode(body.Country)
		geo := GeoData{
			ASN: asn, Country: localizeLocation(body.Country, countryName), CountryEn: countryName,
			Province: localizeLocation("", body.Region), ProvinceEn: body.Region,
			City: localizeLocation("", body.City), CityEn: body.City,
			Owner: owner, Source: "IPInfo",
		}
		if body.Anycast {
			geo.Country, geo.CountryEn, geo.Province, geo.ProvinceEn = "ANYCAST", "ANYCAST", "ANYCAST", "ANYCAST"
		}
		if parts := strings.Split(body.Loc, ","); len(parts) == 2 {
			geo.Latitude, _ = strconv.ParseFloat(parts[0], 64)
			geo.Longitude, _ = strconv.ParseFloat(parts[1], 64)
		}
		return geo, nil
	}}
}

func newIPInsightProvider() geoProvider {
	client := newGeoHTTPClient()
	base := endpointFromEnv("NEXTTRACE_IPAPI_BASE", "https://api.ipinsight.io/ip/")
	token := strings.TrimSpace(os.Getenv("NEXTTRACE_IPINSIGHT_TOKEN"))
	return geoProviderFunc{name: "IPInsight", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		var body struct {
			Country string `json:"country_name"`
			Region  string `json:"region_name"`
			City    string `json:"city_name"`
		}
		rawURL := appendPath(base, ip)
		if token != "" {
			rawURL += "?token=" + url.QueryEscape(token)
		}
		if err := fetchJSON(ctx, client, rawURL, nil, &body); err != nil {
			return GeoData{}, err
		}
		return GeoData{
			Country: localizeLocation("", body.Country), CountryEn: body.Country,
			Province: localizeLocation("", body.Region), ProvinceEn: body.Region,
			City: localizeLocation("", body.City), CityEn: body.City,
			Source: "IPInsight",
		}, nil
	}}
}

func newIPAPIComProvider() geoProvider {
	client := newGeoHTTPClient()
	base := endpointFromEnv("NEXTTRACE_IPAPI_BASE", "http://ip-api.com/json/")
	return geoProviderFunc{name: "IP-API.com", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		var body struct {
			Status      string  `json:"status"`
			Message     string  `json:"message"`
			Country     string  `json:"country"`
			CountryCode string  `json:"countryCode"`
			Region      string  `json:"regionName"`
			City        string  `json:"city"`
			District    string  `json:"district"`
			ISP         string  `json:"isp"`
			AS          string  `json:"as"`
			Latitude    float64 `json:"lat"`
			Longitude   float64 `json:"lon"`
		}
		rawURL := appendPath(base, ip) + "?fields=status,message,country,countryCode,regionName,city,isp,district,as,lat,lon"
		if err := fetchJSON(ctx, client, rawURL, browserHeaders(), &body); err != nil {
			return GeoData{}, err
		}
		if body.Status != "success" {
			return GeoData{}, fmt.Errorf("IP-API.com: %s", body.Message)
		}
		return GeoData{
			ASN: firstMatch(asNumberPattern, body.AS), Country: localizeLocation(body.CountryCode, body.Country), CountryEn: body.Country,
			Province: localizeLocation("", body.Region), ProvinceEn: body.Region,
			City: localizeLocation("", body.City), CityEn: body.City,
			District: body.District, Owner: body.ISP, ISP: body.ISP,
			Latitude: body.Latitude, Longitude: body.Longitude, Source: "IP-API.com",
		}, nil
	}}
}

func newBaiduIPProvider() geoProvider {
	client := newGeoHTTPClient()
	endpoint := endpointFromEnv("NEXTTRACE_BAIDU_IP_BASE", "https://opendata.baidu.com/api.php")
	return geoProviderFunc{name: "百度 IP", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		rawURL, err := url.Parse(endpoint)
		if err != nil {
			return GeoData{}, err
		}
		query := rawURL.Query()
		query.Set("query", ip)
		query.Set("resource_id", "6006")
		query.Set("oe", "utf8")
		rawURL.RawQuery = query.Encode()
		var body struct {
			Status string `json:"status"`
			Data   []struct {
				Location string `json:"location"`
			} `json:"data"`
		}
		if err := fetchJSON(ctx, client, rawURL.String(), browserHeaders(), &body); err != nil {
			return GeoData{}, err
		}
		if body.Status != "0" || len(body.Data) == 0 || strings.TrimSpace(body.Data[0].Location) == "" {
			return GeoData{}, errors.New("百度 IP 未返回归属信息")
		}
		return GeoData{Country: body.Data[0].Location, Source: "百度 IP"}, nil
	}}
}

func newSongzixianIPProvider() geoProvider {
	client := newDirectGeoHTTPClient()
	endpoint := endpointFromEnv("NEXTTRACE_SONGZIXIAN_IP_BASE", "https://api.songzixian.com/api/ip")
	return geoProviderFunc{name: "松子 IP", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		rawURL, err := url.Parse(endpoint)
		if err != nil {
			return GeoData{}, err
		}
		query := rawURL.Query()
		query.Set("ip", ip)
		rawURL.RawQuery = query.Encode()
		var body struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				CountryCode string  `json:"countryCode"`
				Country     string  `json:"country"`
				Region      string  `json:"region"`
				Province    string  `json:"province"`
				City        string  `json:"city"`
				District    string  `json:"district"`
				Longitude   float64 `json:"longitude"`
				Latitude    float64 `json:"latitude"`
				ISP         string  `json:"isp"`
			} `json:"data"`
		}
		if err := fetchJSON(ctx, client, rawURL.String(), browserHeaders(), &body); err != nil {
			return GeoData{}, err
		}
		if body.Code != http.StatusOK || strings.TrimSpace(body.Data.Country) == "" {
			return GeoData{}, fmt.Errorf("松子 IP 查询失败: %s", body.Message)
		}
		province := firstNonEmpty(body.Data.Province, body.Data.Region)
		return GeoData{
			Country: localizeLocation(body.Data.CountryCode, body.Data.Country), CountryEn: body.Data.Country,
			Province: localizeLocation("", province), ProvinceEn: province,
			City: localizeLocation("", body.Data.City), CityEn: body.Data.City,
			District: body.Data.District, Owner: body.Data.ISP, ISP: body.Data.ISP,
			Latitude: body.Data.Latitude, Longitude: body.Data.Longitude, Source: "松子 IP",
		}, nil
	}}
}

func newLeoMoeProvider() geoProvider {
	return newLegacyLeoProvider()
}

func newChunzhenProvider() geoProvider {
	client := newGeoHTTPClient()
	base := endpointFromEnv("NEXTTRACE_CHUNZHENURL", "http://127.0.0.1:2060")
	return geoProviderFunc{name: "chunzhen", lookup: func(ctx context.Context, ip string) (GeoData, error) {
		rawURL, err := url.Parse(base)
		if err != nil {
			return GeoData{}, err
		}
		query := rawURL.Query()
		query.Set("ip", ip)
		rawURL.RawQuery = query.Encode()
		var body map[string]map[string]any
		if err := fetchJSON(ctx, client, rawURL.String(), nil, &body); err != nil {
			return GeoData{}, err
		}
		entry, ok := body[ip]
		if !ok {
			return GeoData{}, fmt.Errorf("chunzhen returned no entry for %s", ip)
		}
		country := anyString(entry["country"])
		city := anyString(entry["area"])
		if containsChineseProvince(country) {
			city = country + city
			country = "中国"
		}
		return GeoData{ASN: anyString(entry["asn"]), Country: country, City: city, Source: "chunzhen"}, nil
	}}
}

type ipdbOneProvider struct {
	client    *http.Client
	baseURL   string
	apiID     string
	apiKey    string
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newIPDBOneProvider() (geoProvider, error) {
	provider := &ipdbOneProvider{
		client: newGeoHTTPClient(), baseURL: endpointFromEnv("IPDBONE_BASE_URL", "https://api.ipdb.one"),
		apiID: strings.TrimSpace(os.Getenv("IPDBONE_API_ID")), apiKey: strings.TrimSpace(os.Getenv("IPDBONE_API_KEY")),
	}
	if provider.apiID == "" || provider.apiKey == "" {
		return nil, errors.New("IPDB.One requires IPDBONE_API_ID and IPDBONE_API_KEY")
	}
	return provider, nil
}

func (p *ipdbOneProvider) Name() string {
	return "ipdb.one"
}

func (p *ipdbOneProvider) Lookup(ctx context.Context, ip string) (GeoData, error) {
	token, err := p.authToken(ctx)
	if err != nil {
		return GeoData{}, err
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Geo struct {
				Country    string    `json:"country"`
				Region     string    `json:"region"`
				City       string    `json:"city"`
				Coordinate []float64 `json:"coordinate"`
			} `json:"geo"`
			Routing struct {
				ASN struct {
					Number int    `json:"number"`
					Name   string `json:"name"`
					Domain string `json:"domain"`
					ASName string `json:"asname"`
				} `json:"asn"`
			} `json:"routing"`
		} `json:"data"`
	}
	rawURL := appendPath(p.baseURL, "query/"+ip) + "?lang=zh-CN"
	if err := fetchJSON(ctx, p.client, rawURL, map[string]string{
		"Authorization": "Bearer " + token, "User-Agent": "Ackwrap/1",
	}, &body); err != nil {
		return GeoData{}, err
	}
	if body.Code != 200 {
		return GeoData{}, fmt.Errorf("IPDB.One: %s", body.Message)
	}
	geo := GeoData{
		ASN: numberString(body.Data.Routing.ASN.Number), Country: body.Data.Geo.Country,
		Province: body.Data.Geo.Region, City: body.Data.Geo.City,
		Owner:  firstNonEmpty(body.Data.Routing.ASN.Domain, body.Data.Routing.ASN.Name),
		Domain: body.Data.Routing.ASN.Domain, Whois: body.Data.Routing.ASN.ASName, Source: "ipdb.one",
	}
	if len(body.Data.Geo.Coordinate) >= 2 {
		geo.Latitude, geo.Longitude = body.Data.Geo.Coordinate[0], body.Data.Geo.Coordinate[1]
	}
	return geo, nil
}

func (p *ipdbOneProvider) authToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Before(p.expiresAt) {
		return p.token, nil
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := fetchJSON(ctx, p.client, appendPath(p.baseURL, "auth/requestToken/query"), map[string]string{
		"x-api-id": p.apiID, "x-api-key": p.apiKey, "User-Agent": "Ackwrap/1",
	}, &body); err != nil {
		return "", err
	}
	if body.Code != 200 || body.Data.Token == "" {
		return "", fmt.Errorf("IPDB.One authentication failed: %s", body.Message)
	}
	p.token, p.expiresAt = body.Data.Token, time.Now().Add(30*time.Second)
	return p.token, nil
}

func newGeoHTTPClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Second}
}

func newDirectGeoHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}

func fetchJSON(ctx context.Context, client *http.Client, rawURL string, headers map[string]string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxGeoResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxGeoResponseBytes {
		return errors.New("Geo response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Geo API returned %s", response.Status)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Geo response: %w", err)
	}
	return nil
}

func endpointFromEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func appendPath(base, value string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(value, "/")
}

func browserHeaders() map[string]string {
	return map[string]string{"User-Agent": "Mozilla/5.0 (compatible; Ackwrap/1)"}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func numberString(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func splitASNOrg(value string) (string, string) {
	fields := strings.Fields(value)
	if len(fields) == 0 || !strings.HasPrefix(strings.ToUpper(fields[0]), "AS") {
		return "", value
	}
	return strings.TrimPrefix(strings.ToUpper(fields[0]), "AS"), strings.TrimSpace(strings.TrimPrefix(value, fields[0]))
}

func firstMatch(pattern *regexp.Regexp, value string) string {
	return pattern.FindString(value)
}

func countryNameFromCode(code string) string {
	if value := map[string]string{
		"CN": "China", "HK": "Hong Kong", "TW": "Taiwan", "MO": "Macao", "US": "United States",
		"JP": "Japan", "SG": "Singapore", "KR": "South Korea", "GB": "United Kingdom", "DE": "Germany",
		"FR": "France", "AU": "Australia", "CA": "Canada", "RU": "Russia",
	}[strings.ToUpper(code)]; value != "" {
		return value
	}
	return code
}

func GeoDataFromCountryCode(code, source string) GeoData {
	code = strings.ToUpper(strings.TrimSpace(code))
	country := countryNameFromCode(code)
	return GeoData{
		Country:   localizeLocation(code, country),
		CountryEn: country,
		Source:    source,
	}
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func containsChineseProvince(value string) bool {
	for _, province := range []string{
		"北京", "天津", "河北", "山西", "内蒙古", "辽宁", "吉林", "黑龙江", "上海", "江苏", "浙江",
		"安徽", "福建", "江西", "山东", "河南", "湖北", "湖南", "广东", "广西", "海南", "重庆", "四川",
		"贵州", "云南", "西藏", "陕西", "甘肃", "青海", "宁夏", "新疆", "台湾", "香港", "澳门",
	} {
		if strings.Contains(value, province) {
			return true
		}
	}
	return false
}
