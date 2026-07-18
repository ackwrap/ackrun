package model

type GeoIPFieldMapping struct {
	ASN         string `json:"asnumber,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	CountryEn   string `json:"country_en,omitempty"`
	Province    string `json:"prov,omitempty"`
	ProvinceEn  string `json:"prov_en,omitempty"`
	City        string `json:"city,omitempty"`
	CityEn      string `json:"city_en,omitempty"`
	District    string `json:"district,omitempty"`
	Owner       string `json:"owner,omitempty"`
	ISP         string `json:"isp,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Whois       string `json:"whois,omitempty"`
	Latitude    string `json:"lat,omitempty"`
	Longitude   string `json:"lng,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
}

type GeoIPProvider struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Key         string            `json:"key"`
	Template    string            `json:"template"`
	URL         string            `json:"url,omitempty"`
	IPParameter string            `json:"ip_parameter,omitempty"`
	Mapping     GeoIPFieldMapping `json:"mapping"`
	Enabled     bool              `json:"enabled"`
	IsDefault   bool              `json:"is_default"`
	Builtin     bool              `json:"builtin"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

type GeoIPProviderRequest struct {
	Name        string            `json:"name" binding:"required"`
	Template    string            `json:"template"`
	URL         string            `json:"url,omitempty"`
	IPParameter string            `json:"ip_parameter,omitempty"`
	Mapping     GeoIPFieldMapping `json:"mapping"`
	Enabled     bool              `json:"enabled"`
	IsDefault   bool              `json:"is_default"`
}

type GeoIPProviderTemplate struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	URL         string            `json:"url,omitempty"`
	IPParameter string            `json:"ip_parameter,omitempty"`
	Mapping     GeoIPFieldMapping `json:"mapping"`
}

type GeoIPProviderListResponse struct {
	Items     []GeoIPProvider         `json:"items"`
	Templates []GeoIPProviderTemplate `json:"templates"`
}

type ConnectivityTarget struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	Builtin   bool   `json:"builtin"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type ConnectivityTargetRequest struct {
	Name    string `json:"name" binding:"required"`
	URL     string `json:"url" binding:"required"`
	Enabled bool   `json:"enabled"`
}
