package traceroute

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tsosunchia/powclient"
)

const legacyLeoLookupTimeout = 10 * time.Second

type legacyLeoProvider struct {
	mu    sync.Mutex
	conn  *websocket.Conn
	token string
	host  string
	port  string
}

func newLegacyLeoProvider() geoProvider {
	host, port := legacyLeoEndpoint()
	return &legacyLeoProvider{host: host, port: port}
}

func (p *legacyLeoProvider) Name() string {
	return "LeoMoeAPI"
}

func (p *legacyLeoProvider) Lookup(ctx context.Context, ip string) (GeoData, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	lookupCtx, cancel := context.WithTimeout(ctx, legacyLeoLookupTimeout)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if p.conn == nil {
			if err := p.connect(lookupCtx); err != nil {
				lastErr = err
				p.reset()
				continue
			}
		}
		geo, err := p.lookupConnected(lookupCtx, ip)
		if err == nil {
			return geo, nil
		}
		lastErr = err
		p.reset()
	}
	return GeoData{}, fmt.Errorf("LeoMoeAPI legacy lookup failed: %w", lastErr)
}

func (p *legacyLeoProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

func (p *legacyLeoProvider) connect(ctx context.Context) error {
	token := strings.TrimSpace(os.Getenv("NEXTTRACE_TOKEN"))
	userAgent := legacyLeoUserAgent()
	if token == "" {
		var err error
		if p.token == "" {
			p.token, err = requestLegacyLeoToken(ctx, p.host, p.port)
			if err != nil {
				return err
			}
		}
		token = p.token
	} else {
		userAgent = "Privileged Client"
	}
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{ServerName: p.host, MinVersion: tls.VersionTLS12}
	rawURL := url.URL{Scheme: "wss", Host: net.JoinHostPort(p.host, p.port), Path: "/v3/ipGeoWs"}
	headers := http.Header{
		"Authorization": []string{"Bearer " + token},
		"User-Agent":    []string{userAgent},
	}
	conn, response, err := dialer.DialContext(ctx, rawURL.String(), headers)
	if err != nil {
		if response != nil {
			return fmt.Errorf("legacy WebSocket dial returned %s: %w", response.Status, err)
		}
		return fmt.Errorf("legacy WebSocket dial: %w", err)
	}
	p.conn = conn
	return nil
}

func (p *legacyLeoProvider) lookupConnected(ctx context.Context, ip string) (GeoData, error) {
	deadline := time.Now().Add(3 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := p.conn.SetWriteDeadline(deadline); err != nil {
		return GeoData{}, err
	}
	if err := p.conn.WriteMessage(websocket.TextMessage, []byte(ip)); err != nil {
		return GeoData{}, err
	}
	if err := p.conn.SetReadDeadline(deadline); err != nil {
		return GeoData{}, err
	}
	for {
		_, message, err := p.conn.ReadMessage()
		if err != nil {
			return GeoData{}, err
		}
		if string(message) == "pong" {
			continue
		}
		geo, responseIP, err := decodeLegacyLeoGeo(message)
		if err != nil {
			return GeoData{}, err
		}
		if responseIP == ip {
			return geo, nil
		}
	}
}

func (p *legacyLeoProvider) reset() {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	p.token = ""
}

func requestLegacyLeoToken(ctx context.Context, host, port string) (string, error) {
	params := powclient.NewGetTokenParams()
	rawURL := url.URL{Scheme: "https", Host: net.JoinHostPort(host, port), Path: "/v3/challenge"}
	params.BaseUrl = rawURL.String()
	params.SNI = host
	params.Host = host
	params.UserAgent = legacyLeoUserAgent()
	if request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL.String(), nil); err == nil {
		if proxyURL, proxyErr := http.ProxyFromEnvironment(request); proxyErr == nil {
			params.Proxy = proxyURL
		}
	}
	type tokenResult struct {
		token string
		err   error
	}
	resultCh := make(chan tokenResult, 1)
	go func() {
		token, err := powclient.RetToken(params)
		resultCh <- tokenResult{token: token, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			return "", fmt.Errorf("legacy PoW token: %w", result.err)
		}
		if strings.TrimSpace(result.token) == "" {
			return "", errors.New("legacy PoW token is empty")
		}
		return result.token, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func decodeLegacyLeoGeo(message []byte) (GeoData, string, error) {
	var body struct {
		IP         string  `json:"ip"`
		ASN        string  `json:"asnumber"`
		Country    string  `json:"country"`
		CountryEn  string  `json:"country_en"`
		Province   string  `json:"prov"`
		ProvinceEn string  `json:"prov_en"`
		City       string  `json:"city"`
		CityEn     string  `json:"city_en"`
		District   string  `json:"district"`
		Owner      string  `json:"owner"`
		ISP        string  `json:"isp"`
		Domain     string  `json:"domain"`
		Whois      string  `json:"whois"`
		Latitude   float64 `json:"lat"`
		Longitude  float64 `json:"lng"`
		Prefix     string  `json:"prefix"`
	}
	if err := json.Unmarshal(message, &body); err != nil {
		return GeoData{}, "", fmt.Errorf("decode legacy Geo response: %w", err)
	}
	if body.IP == "" {
		return GeoData{}, "", errors.New("legacy Geo response has no IP")
	}
	if body.ASN == "API Server Error" {
		return GeoData{}, body.IP, errors.New("legacy Geo API server error")
	}
	return GeoData{
		ASN: body.ASN, Country: body.Country, CountryEn: body.CountryEn,
		Province: body.Province, ProvinceEn: body.ProvinceEn, City: body.City, CityEn: body.CityEn,
		District: body.District, Owner: firstNonEmpty(body.Domain, body.Owner), ISP: body.ISP,
		Domain: body.Domain, Whois: body.Whois, Latitude: body.Latitude, Longitude: body.Longitude,
		Prefix: body.Prefix, Source: "LeoMoeAPI",
	}, body.IP, nil
}

func legacyLeoEndpoint() (string, string) {
	raw := strings.TrimSpace(os.Getenv("NEXTTRACE_HOSTPORT"))
	if raw == "" {
		return "api.nxtrace.org", "443"
	}
	if host, port, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(host, "[]"), port
	}
	return strings.Trim(raw, "[]"), "443"
}

func legacyLeoUserAgent() string {
	return fmt.Sprintf("NextTrace mini/%s/%s", runtime.GOOS, runtime.GOARCH)
}
