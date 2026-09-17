package service

import (
	"context"
	"net/netip"
	"sort"
	"strings"

	"github.com/ackwrap/ackrun/internal/geoquery"
	"github.com/ackwrap/ackrun/internal/model"
)

// Queries follow the same category ownership as generated rules. A domain or
// address missing from an existing primary category must not match that same
// category through the fallback database.
func (svc *RouteRuleService) appendFallbackGeoLookup(resp *model.GeoLookupResponse, assets []model.GeoAsset) {
	var used []string
	for _, asset := range assets {
		if asset.Source != model.GeoSourceLoyalsoldier || !asset.Available {
			continue
		}
		if asset.Type == "geosite" && resp.TargetType != "domain" {
			continue
		}
		catalog, err := svc.geoCategoryCatalog([]model.GeoAsset{asset})
		if err == nil {
			err = catalog.addFallback(asset.Type)
		}
		if err != nil {
			resp.Message = appendLookupMessage(resp.Message, "Geo 回退查询失败: "+err.Error())
			continue
		}
		var fallbackPath string
		for _, selected := range catalog.entries {
			if selected.source == model.GeoSourceSagerNet {
				fallbackPath = selected.path
				break
			}
		}
		if fallbackPath == "" {
			continue
		}
		isFallback := func(code string) bool {
			return catalog.entries[generatedGeoRuleSetTag(asset.Type, code)].source == model.GeoSourceSagerNet
		}
		if asset.Type == "geosite" {
			matches, err := lookupGeositeCodes(fallbackPath, resp.Target)
			if err != nil {
				resp.Message = appendLookupMessage(resp.Message, "GeoSite 回退查询失败: "+err.Error())
				continue
			}
			for _, match := range matches {
				code, _, _ := strings.Cut(match, " (")
				if isFallback(code) {
					resp.GeositeMatches = append(resp.GeositeMatches, match)
					used = append(used, "geosite:"+code)
				}
			}
			sort.Strings(resp.GeositeMatches)
			continue
		}
		reader, err := geoquery.OpenGeoIP(fallbackPath)
		if err != nil {
			resp.Message = appendLookupMessage(resp.Message, "GeoIP 回退查询失败: "+err.Error())
			continue
		}
		var privatePrefixes []netip.Prefix
		if isFallback("private") {
			privatePrefixes, err = svc.sagerNetPrivatePrefixes(context.Background())
			if err != nil {
				resp.Message = appendLookupMessage(resp.Message, "GeoIP private 回退查询失败: "+err.Error())
			}
		}
		for index, entry := range resp.GeoIPMatches {
			ip, value, ok := strings.Cut(entry, " => ")
			addr, err := netip.ParseAddr(ip)
			if !ok || err != nil {
				continue
			}
			codes := strings.Split(value, ", ")
			if value == "unknown" {
				codes = nil
			}
			for _, code := range reader.LookupCodes(addr) {
				if code != "private" && isFallback(code) {
					codes = append(codes, code)
					used = append(used, "geoip:"+code)
				}
			}
			for _, prefix := range privatePrefixes {
				if prefix.Contains(addr) {
					codes = append(codes, "private")
					used = append(used, "geoip:private")
					break
				}
			}
			if len(codes) > 0 {
				sort.Strings(codes)
				resp.GeoIPMatches[index] = ip + " => " + strings.Join(codes, ", ")
			}
		}
		_ = reader.Close()
	}
	if len(used) > 0 {
		sort.Strings(used)
		resp.Message = appendLookupMessage(resp.Message, "使用 SagerNet 回退分类: "+strings.Join(used, ", "))
	}
}
