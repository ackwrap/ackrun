package traceroute

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

type GeoData struct {
	ASN        string  `json:"asnumber,omitempty"`
	Country    string  `json:"country,omitempty"`
	CountryEn  string  `json:"country_en,omitempty"`
	Province   string  `json:"prov,omitempty"`
	ProvinceEn string  `json:"prov_en,omitempty"`
	City       string  `json:"city,omitempty"`
	CityEn     string  `json:"city_en,omitempty"`
	District   string  `json:"district,omitempty"`
	Owner      string  `json:"owner,omitempty"`
	ISP        string  `json:"isp,omitempty"`
	Domain     string  `json:"domain,omitempty"`
	Whois      string  `json:"whois,omitempty"`
	Latitude   float64 `json:"lat,omitempty"`
	Longitude  float64 `json:"lng,omitempty"`
	Prefix     string  `json:"prefix,omitempty"`
	Source     string  `json:"source,omitempty"`
}

type geoCacheEntry struct {
	data GeoData
	err  error
}

type metadataClient struct {
	provider geoProvider
	mu       sync.Mutex
	cache    map[string]geoCacheEntry
}

type GeoLookupFunc func(context.Context, net.IP) (GeoData, error)

func newMetadataClient(providerName string) (*metadataClient, error) {
	provider, err := newGeoProvider(providerName)
	if err != nil {
		return nil, err
	}
	return &metadataClient{
		provider: provider,
		cache:    make(map[string]geoCacheEntry),
	}, nil
}

func newMetadataClientWithLookup(providerName string, lookup GeoLookupFunc) *metadataClient {
	return &metadataClient{
		provider: geoProviderFunc{
			name: providerName,
			lookup: func(ctx context.Context, ip string) (GeoData, error) {
				return lookup(ctx, net.ParseIP(ip))
			},
		},
		cache: make(map[string]geoCacheEntry),
	}
}

func LookupGeo(ctx context.Context, ip net.IP, providerName string) (GeoData, error) {
	client, err := newMetadataClient(providerName)
	if err != nil {
		return GeoData{}, err
	}
	defer client.Close()
	return client.lookupGeo(ctx, ip)
}

func (c *metadataClient) ProviderName() string {
	return c.provider.Name()
}

func (c *metadataClient) Close() error {
	if closeable, ok := c.provider.(interface{ Close() error }); ok {
		return closeable.Close()
	}
	return nil
}

func (c *metadataClient) Enrich(ctx context.Context, ip net.IP, rdnsTimeout time.Duration) (GeoData, string, string) {
	var geo GeoData
	var geoErr error
	var hostname string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		geo, geoErr = c.lookupGeo(ctx, ip)
	}()
	go func() {
		defer wg.Done()
		hostname = lookupAddr(ctx, ip.String(), rdnsTimeout)
	}()
	wg.Wait()
	if geoErr != nil {
		return geo, hostname, geoErr.Error()
	}
	return geo, hostname, ""
}

func (c *metadataClient) lookupGeo(ctx context.Context, ip net.IP) (GeoData, error) {
	key := ip.String()
	c.mu.Lock()
	entry, exists := c.cache[key]
	c.mu.Unlock()
	if exists {
		return entry.data, entry.err
	}
	if label := reservedNetworkLabel(ip); label != "" {
		entry.data = GeoData{Whois: label, Source: "reserved"}
	} else {
		entry.data, entry.err = c.provider.Lookup(ctx, key)
		if entry.data.Source == "" {
			entry.data.Source = c.provider.Name()
		}
	}
	c.mu.Lock()
	c.cache[key] = entry
	c.mu.Unlock()
	return entry.data, entry.err
}

func lookupAddr(ctx context.Context, ip string, timeout time.Duration) string {
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

func reservedNetworkLabel(ip net.IP) string {
	if ip.IsLoopback() {
		return "RFC1122"
	}
	if ip.IsLinkLocalUnicast() {
		if ip.To4() != nil {
			return "RFC3927"
		}
		return "RFC4291"
	}
	if ip.IsPrivate() {
		if ip.To4() != nil {
			return "RFC1918"
		}
		return "RFC4193"
	}
	if inCIDR(ip, "100.64.0.0/10") {
		return "RFC6598"
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return "Reserved"
	}
	return ""
}

func inCIDR(ip net.IP, cidr string) bool {
	_, network, err := net.ParseCIDR(cidr)
	return err == nil && network.Contains(ip)
}

var chineseLocations = map[string]string{
	"CN": "中国", "China": "中国", "Hong Kong": "香港", "HK": "香港",
	"Taiwan": "台湾", "TW": "台湾", "Macao": "澳门", "MO": "澳门",
	"United States": "美国", "United States of America": "美国", "US": "美国",
	"Japan": "日本", "JP": "日本", "Singapore": "新加坡", "SG": "新加坡",
	"South Korea": "韩国", "Korea": "韩国", "KR": "韩国", "United Kingdom": "英国", "GB": "英国",
	"Germany": "德国", "DE": "德国", "France": "法国", "FR": "法国",
	"Australia": "澳大利亚", "AU": "澳大利亚", "Canada": "加拿大", "CA": "加拿大",
	"Russia": "俄罗斯", "Russian Federation": "俄罗斯", "RU": "俄罗斯",
	"Beijing": "北京", "Tianjin": "天津", "Shanghai": "上海", "Chongqing": "重庆",
	"Hebei": "河北", "Shanxi": "山西", "Liaoning": "辽宁", "Jilin": "吉林",
	"Heilongjiang": "黑龙江", "Jiangsu": "江苏", "Zhejiang": "浙江", "Anhui": "安徽",
	"Fujian": "福建", "Jiangxi": "江西", "Shandong": "山东", "Henan": "河南",
	"Hubei": "湖北", "Hunan": "湖南", "Guangdong": "广东", "Hainan": "海南",
	"Sichuan": "四川", "Guizhou": "贵州", "Yunnan": "云南", "Shaanxi": "陕西",
	"Gansu": "甘肃", "Qinghai": "青海", "Inner Mongolia": "内蒙古", "Guangxi": "广西",
	"Tibet": "西藏", "Ningxia": "宁夏", "Xinjiang": "新疆",
	"Guangzhou": "广州", "Shenzhen": "深圳", "Hangzhou": "杭州", "Nanjing": "南京",
	"Wuhan": "武汉", "Chengdu": "成都", "Xi'an": "西安", "Zhengzhou": "郑州",
}

func localizeLocation(code, value string) string {
	if translated := chineseLocations[code]; translated != "" {
		return translated
	}
	if translated := chineseLocations[value]; translated != "" {
		return translated
	}
	return value
}
