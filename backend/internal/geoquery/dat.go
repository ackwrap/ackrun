package geoquery

import (
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

const MaxDatabaseSize int64 = 64 * 1024 * 1024

func readDatabase(reader io.Reader) ([]byte, error) {
	if file, ok := reader.(*os.File); ok {
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if info.Size() > MaxDatabaseSize {
			return nil, fmt.Errorf("geo database exceeds %d bytes", MaxDatabaseSize)
		}
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxDatabaseSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxDatabaseSize {
		return nil, fmt.Errorf("geo database exceeds %d bytes", MaxDatabaseSize)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty geo database")
	}
	return data, nil
}

// DAT uses the V2Fly routercommon GeoIPList / GeoSiteList protobuf schema:
// https://github.com/v2fly/v2ray-core/blob/master/app/router/routercommon/common.proto
// ConsumeBytes checks available input before slicing, so untrusted lengths never
// become unchecked allocation sizes.
type datField struct {
	number protowire.Number
	typeID protowire.Type
	bytes  []byte
	value  uint64
}

func walkDAT(data []byte, visit func(datField) error) error {
	for len(data) > 0 {
		number, typeID, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		if !number.IsValid() {
			return fmt.Errorf("invalid DAT field number %d", number)
		}
		data = data[n:]
		field := datField{number: number, typeID: typeID}
		switch typeID {
		case protowire.BytesType:
			field.bytes, n = protowire.ConsumeBytes(data)
		case protowire.VarintType:
			field.value, n = protowire.ConsumeVarint(data)
		default:
			return fmt.Errorf("unsupported DAT wire type %d in field %d", typeID, number)
		}
		if n < 0 {
			return protowire.ParseError(n)
		}
		if err := visit(field); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (f datField) require(typeID protowire.Type) error {
	if f.typeID != typeID {
		return fmt.Errorf("invalid DAT wire type %d for field %d", f.typeID, f.number)
	}
	return nil
}

func (f datField) text() (string, error) {
	if err := f.require(protowire.BytesType); err != nil {
		return "", err
	}
	if !utf8.Valid(f.bytes) {
		return "", fmt.Errorf("invalid UTF-8 in DAT field %d", f.number)
	}
	return string(f.bytes), nil
}

func normalizeCode(code string) (string, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "", fmt.Errorf("missing DAT category code")
	}
	if len(code) > 255 {
		return "", fmt.Errorf("DAT category code exceeds 255 bytes")
	}
	return code, nil
}

func parseGeoIPDAT(data []byte) (map[string][]netip.Prefix, error) {
	result := make(map[string][]netip.Prefix)
	err := walkDAT(data, func(field datField) error {
		if field.number != 1 {
			return fmt.Errorf("unsupported GeoIPList field %d", field.number)
		}
		if err := field.require(protowire.BytesType); err != nil {
			return err
		}
		var code string
		var prefixes []netip.Prefix
		seenPrefixes := make(map[netip.Prefix]bool)
		err := walkDAT(field.bytes, func(field datField) error {
			switch field.number {
			case 1:
				var err error
				code, err = field.text()
				return err
			case 2:
				if err := field.require(protowire.BytesType); err != nil {
					return err
				}
				prefix, err := parseDATCIDR(field.bytes)
				if err != nil {
					return err
				}
				if !seenPrefixes[prefix] {
					prefixes = append(prefixes, prefix)
					seenPrefixes[prefix] = true
				}
			case 3:
				if err := field.require(protowire.VarintType); err != nil {
					return err
				}
				if field.value != 0 {
					return fmt.Errorf("GeoIP inverse_match is not supported")
				}
			default:
				return fmt.Errorf("unsupported GeoIP field %d", field.number)
			}
			return nil
		})
		if err != nil {
			return err
		}
		code, err = normalizeCode(code)
		if err != nil {
			return err
		}
		if _, exists := result[code]; exists {
			return fmt.Errorf("duplicate GeoIP category %q", code)
		}
		result[code] = prefixes
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid GeoIP DAT: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("GeoIP DAT contains no categories")
	}
	return result, nil
}

func parseDATCIDR(data []byte) (netip.Prefix, error) {
	var address []byte
	var bits uint64
	err := walkDAT(data, func(field datField) error {
		switch field.number {
		case 1:
			if err := field.require(protowire.BytesType); err != nil {
				return err
			}
			address = field.bytes
		case 2:
			if err := field.require(protowire.VarintType); err != nil {
				return err
			}
			bits = field.value
		default:
			return fmt.Errorf("unsupported CIDR field %d", field.number)
		}
		return nil
	})
	if err != nil {
		return netip.Prefix{}, err
	}
	addr, ok := netip.AddrFromSlice(address)
	if !ok || bits > uint64(addr.BitLen()) {
		return netip.Prefix{}, fmt.Errorf("invalid DAT CIDR address or prefix length")
	}
	return netip.PrefixFrom(addr, int(bits)).Masked(), nil
}

func parseGeositeDAT(data []byte) (map[string][]GeositeItem, error) {
	result := make(map[string][]GeositeItem)
	err := walkDAT(data, func(field datField) error {
		if field.number != 1 {
			return fmt.Errorf("unsupported GeoSiteList field %d", field.number)
		}
		if err := field.require(protowire.BytesType); err != nil {
			return err
		}
		var code string
		var items []GeositeItem
		attributes := make(map[string][]GeositeItem)
		err := walkDAT(field.bytes, func(field datField) error {
			switch field.number {
			case 1:
				var err error
				code, err = field.text()
				return err
			case 2:
				if err := field.require(protowire.BytesType); err != nil {
					return err
				}
				item, keys, err := parseDATDomain(field.bytes)
				if err != nil {
					return err
				}
				items = append(items, item)
				for key := range keys {
					attributes[key] = append(attributes[key], item)
				}
			default:
				return fmt.Errorf("unsupported GeoSite field %d", field.number)
			}
			return nil
		})
		if err != nil {
			return err
		}
		code, err = normalizeCode(code)
		if err != nil {
			return err
		}
		if _, exists := result[code]; exists {
			return fmt.Errorf("duplicate GeoSite category %q", code)
		}
		result[code] = items
		for key, subset := range attributes {
			attributeCode := code + "@" + key
			if _, exists := result[attributeCode]; exists {
				return fmt.Errorf("duplicate GeoSite category %q", attributeCode)
			}
			result[attributeCode] = subset
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid GeoSite DAT: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("GeoSite DAT contains no categories")
	}
	return result, nil
}

func parseDATDomain(data []byte) (GeositeItem, map[string]bool, error) {
	var value string
	var domainType uint64
	attributes := make(map[string]bool)
	err := walkDAT(data, func(field datField) error {
		switch field.number {
		case 1:
			if err := field.require(protowire.VarintType); err != nil {
				return err
			}
			domainType = field.value
		case 2:
			var err error
			value, err = field.text()
			return err
		case 3:
			if err := field.require(protowire.BytesType); err != nil {
				return err
			}
			key, err := parseDATAttribute(field.bytes)
			if err != nil {
				return err
			}
			attributes[key] = true
		default:
			return fmt.Errorf("unsupported Domain field %d", field.number)
		}
		return nil
	})
	if err != nil {
		return GeositeItem{}, nil, err
	}
	if value == "" {
		return GeositeItem{}, nil, fmt.Errorf("empty DAT domain value")
	}
	item := GeositeItem{Value: value}
	switch domainType {
	case 0:
		item.Type = GeositeRuleTypeDomainKeyword
	case 1:
		item.Type = GeositeRuleTypeDomainRegex
		if _, err := regexp.Compile(value); err != nil {
			return GeositeItem{}, nil, fmt.Errorf("invalid DAT domain regex: %w", err)
		}
	case 2:
		item.Type = GeositeRuleTypeDomainSuffix
	case 3:
		item.Type = GeositeRuleTypeDomain
	default:
		return GeositeItem{}, nil, fmt.Errorf("unsupported DAT domain type %d", domainType)
	}
	return item, attributes, nil
}

func parseDATAttribute(data []byte) (string, error) {
	var key string
	err := walkDAT(data, func(field datField) error {
		switch field.number {
		case 1:
			var err error
			key, err = field.text()
			return err
		case 2, 3:
			// @key selects by presence, independent of its bool/int payload.
			return field.require(protowire.VarintType)
		default:
			return fmt.Errorf("unsupported Domain.Attribute field %d", field.number)
		}
	})
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", fmt.Errorf("empty DAT domain attribute")
	}
	// Attribute codes are materialized as category@key. Bound both parts to
	// prevent a large category name from multiplying across many attributes.
	if len(key) > 255 {
		return "", fmt.Errorf("DAT domain attribute exceeds 255 bytes")
	}
	return strings.ToLower(key), nil
}
