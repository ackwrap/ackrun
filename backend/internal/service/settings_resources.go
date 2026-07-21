package service

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

var (
	ErrGeoIPProviderInvalid     = errors.New("GeoIP Provider 设置无效")
	ErrSettingsResourceNotFound = errors.New("设置资源不存在")
	jsonPathPattern             = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*$`)
	queryParameterPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

var geoIPProviderTemplates = []model.GeoIPProviderTemplate{
	{
		Key: "ip.sb", Name: "IP.SB", URL: "https://api.ip.sb/geoip/{ip}",
		Mapping: model.GeoIPFieldMapping{ASN: "asn", CountryCode: "country_code", Country: "country", Province: "region", City: "city", Latitude: "latitude", Longitude: "longitude", ISP: "isp", Owner: "organization"},
	},
	{
		Key: "baidu", Name: "百度 IP", URL: "https://opendata.baidu.com/api.php?resource_id=6006&oe=utf8", IPParameter: "query",
		Mapping: model.GeoIPFieldMapping{Country: "data.0.location"},
	},
	{Key: "custom", Name: "自定义 JSON 接口"},
}

func (svc *SettingsService) ListGeoIPProviders() (*model.GeoIPProviderListResponse, error) {
	items, err := svc.store.ListGeoIPProviders()
	if err != nil {
		return nil, err
	}
	return &model.GeoIPProviderListResponse{Items: items, Templates: geoIPProviderTemplates}, nil
}

func (svc *SettingsService) CreateGeoIPProvider(req *model.GeoIPProviderRequest) (*model.GeoIPProvider, error) {
	if err := applyAndValidateGeoIPProvider(req, true); err != nil {
		return nil, err
	}
	item, err := svc.store.CreateGeoIPProvider(req)
	if err == nil {
		logging.Info("settings.geoip_provider", "GeoIP Provider 已创建 id=%d template=%s", item.ID, item.Template)
	}
	return item, err
}

func (svc *SettingsService) UpdateGeoIPProvider(id int64, req *model.GeoIPProviderRequest) (*model.GeoIPProvider, error) {
	existing, err := svc.store.GetGeoIPProvider(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSettingsResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if existing.Builtin {
		req.Template = existing.Template
		req.URL = existing.URL
		req.IPParameter = existing.IPParameter
		req.Mapping = existing.Mapping
	}
	if err := applyAndValidateGeoIPProvider(req, false); err != nil {
		return nil, err
	}
	if existing.IsDefault && (!req.Enabled || !req.IsDefault) {
		return nil, fmt.Errorf("%w: 默认 Provider 必须保持启用；请先将其他 Provider 设为默认", ErrGeoIPProviderInvalid)
	}
	item, err := svc.store.UpdateGeoIPProvider(id, req)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSettingsResourceNotFound
	}
	if err == nil {
		logging.Info("settings.geoip_provider", "GeoIP Provider 已更新 id=%d enabled=%v default=%v", id, req.Enabled, req.IsDefault)
	}
	return item, err
}

func (svc *SettingsService) DeleteGeoIPProvider(id int64) (*model.ActionResponse, error) {
	item, err := svc.store.GetGeoIPProvider(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSettingsResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if item.Builtin {
		return nil, fmt.Errorf("%w: 内置 Provider 不能删除，可将其停用", ErrGeoIPProviderInvalid)
	}
	if item.IsDefault {
		return nil, fmt.Errorf("%w: 默认 Provider 不能删除", ErrGeoIPProviderInvalid)
	}
	if err := svc.store.DeleteGeoIPProvider(id); err != nil {
		return nil, err
	}
	logging.Info("settings.geoip_provider", "GeoIP Provider 已删除 id=%d", id)
	return &model.ActionResponse{Success: true, Message: "GeoIP provider deleted"}, nil
}

func applyAndValidateGeoIPProvider(req *model.GeoIPProviderRequest, applyTemplate bool) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Template = strings.TrimSpace(req.Template)
	req.URL = strings.TrimSpace(req.URL)
	req.IPParameter = strings.TrimSpace(req.IPParameter)
	if req.Name == "" {
		return fmt.Errorf("%w: 名称不能为空", ErrGeoIPProviderInvalid)
	}
	if req.Template == "" {
		req.Template = "custom"
	}
	if req.IsDefault && !req.Enabled {
		return fmt.Errorf("%w: 默认 Provider 必须启用", ErrGeoIPProviderInvalid)
	}
	if !applyTemplate && req.Template == "builtin" {
		return nil
	}
	if applyTemplate && req.Template != "custom" {
		var found *model.GeoIPProviderTemplate
		for i := range geoIPProviderTemplates {
			if geoIPProviderTemplates[i].Key == req.Template {
				found = &geoIPProviderTemplates[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("%w: 未知预置模板", ErrGeoIPProviderInvalid)
		}
		req.URL, req.IPParameter, req.Mapping = found.URL, found.IPParameter, found.Mapping
	}
	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: 自定义接口必须是无凭据和片段的 HTTPS URL", ErrGeoIPProviderInvalid)
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicGeoIP(ip) {
		return fmt.Errorf("%w: 自定义接口不能指向本机或私有网络", ErrGeoIPProviderInvalid)
	}
	if !strings.Contains(req.URL, "{ip}") && !queryParameterPattern.MatchString(req.IPParameter) {
		return fmt.Errorf("%w: URL 必须包含 {ip}，或提供有效的 IP 查询参数名", ErrGeoIPProviderInvalid)
	}
	paths := []string{req.Mapping.ASN, req.Mapping.Country, req.Mapping.CountryCode, req.Mapping.CountryEn, req.Mapping.Province, req.Mapping.ProvinceEn, req.Mapping.City, req.Mapping.CityEn, req.Mapping.District, req.Mapping.Owner, req.Mapping.ISP, req.Mapping.Domain, req.Mapping.Whois, req.Mapping.Latitude, req.Mapping.Longitude, req.Mapping.Prefix}
	if req.Mapping.Country == "" && req.Mapping.CountryCode == "" {
		return fmt.Errorf("%w: JSON 映射至少需要 country 或 country_code", ErrGeoIPProviderInvalid)
	}
	for _, path := range paths {
		if path != "" && !jsonPathPattern.MatchString(path) {
			return fmt.Errorf("%w: JSON 字段路径 %q 无效", ErrGeoIPProviderInvalid, path)
		}
	}
	return nil
}

func (svc *SettingsService) ListConnectivityTargets() ([]model.ConnectivityTarget, error) {
	return svc.store.ListConnectivityTargets()
}

func (svc *SettingsService) CreateConnectivityTarget(req *model.ConnectivityTargetRequest) (*model.ConnectivityTarget, error) {
	if err := validateConnectivityTarget(req); err != nil {
		return nil, err
	}
	if _, err := svc.store.GetConnectivityTargetByURL(req.URL); err == nil {
		return nil, fmt.Errorf("%w: 该连通性 URL 已存在", ErrConnectivitySettingsInvalid)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	item, err := svc.store.CreateConnectivityTarget(req)
	if err == nil {
		logging.Info("settings.connectivity_target", "连通性地址已创建 id=%d", item.ID)
	}
	return item, err
}

func (svc *SettingsService) UpdateConnectivityTarget(id int64, req *model.ConnectivityTargetRequest) (*model.ConnectivityTarget, error) {
	if err := validateConnectivityTarget(req); err != nil {
		return nil, err
	}
	existing, err := svc.store.GetConnectivityTarget(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSettingsResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if existing.Builtin {
		req.URL = existing.URL
	}
	if duplicate, err := svc.store.GetConnectivityTargetByURL(req.URL); err == nil && duplicate.ID != id {
		return nil, fmt.Errorf("%w: 该连通性 URL 已存在", ErrConnectivitySettingsInvalid)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	settings, err := svc.store.GetConnectivitySettings()
	if err != nil {
		return nil, err
	}
	if settings.TestURL == existing.URL && !req.Enabled {
		return nil, fmt.Errorf("%w: 当前使用的连通性地址不能停用", ErrConnectivitySettingsInvalid)
	}
	if settings.TestURL == existing.URL && req.URL != existing.URL {
		return nil, fmt.Errorf("%w: 当前使用的连通性地址不能修改 URL，请先切换到其他地址", ErrConnectivitySettingsInvalid)
	}
	item, err := svc.store.UpdateConnectivityTarget(id, req)
	if err != nil {
		return nil, err
	}
	logging.Info("settings.connectivity_target", "连通性地址已更新 id=%d enabled=%v", id, req.Enabled)
	return item, nil
}

func (svc *SettingsService) DeleteConnectivityTarget(id int64) (*model.ActionResponse, error) {
	item, err := svc.store.GetConnectivityTarget(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSettingsResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if item.Builtin {
		return nil, fmt.Errorf("%w: 内置连通性地址不能删除，可将其停用", ErrConnectivitySettingsInvalid)
	}
	settings, err := svc.store.GetConnectivitySettings()
	if err != nil {
		return nil, err
	}
	if settings.TestURL == item.URL {
		return nil, fmt.Errorf("%w: 当前使用的连通性地址不能删除", ErrConnectivitySettingsInvalid)
	}
	if err := svc.store.DeleteConnectivityTarget(id); err != nil {
		return nil, err
	}
	logging.Info("settings.connectivity_target", "连通性地址已删除 id=%d", id)
	return &model.ActionResponse{Success: true, Message: "connectivity target deleted"}, nil
}

func validateConnectivityTarget(req *model.ConnectivityTargetRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" {
		return fmt.Errorf("%w: 连通性地址名称不能为空", ErrConnectivitySettingsInvalid)
	}
	if err := validateUpdateURL(req.URL, "连通性地址"); err != nil {
		return fmt.Errorf("%w: %v", ErrConnectivitySettingsInvalid, err)
	}
	return nil
}
