package model

import (
	"fmt"
	"strings"
)

const (
	GeoSourceSagerNet     = "sagernet"
	GeoSourceLoyalsoldier = "loyalsoldier"
)

// NormalizeGeoSource preserves the historical source for older settings.
func NormalizeGeoSource(source string) (string, error) {
	switch source = strings.ToLower(strings.TrimSpace(source)); source {
	case "", GeoSourceSagerNet:
		return GeoSourceSagerNet, nil
	case GeoSourceLoyalsoldier:
		return source, nil
	default:
		return "", fmt.Errorf("不支持的 Geo 数据来源: %s", source)
	}
}

func GeoSourceAssetURL(source, assetType string) string {
	source, err := NormalizeGeoSource(source)
	if err != nil {
		return ""
	}
	switch assetType {
	case "geoip":
		if source == GeoSourceLoyalsoldier {
			return "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/geoip.dat"
		}
		return "https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip.db"
	case "geosite":
		if source == GeoSourceLoyalsoldier {
			return "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geosite.dat"
		}
		return "https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite.db"
	default:
		return ""
	}
}
