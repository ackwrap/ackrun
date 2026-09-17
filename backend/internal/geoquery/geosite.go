package geoquery

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
)

type GeositeItemType = uint8

const (
	GeositeRuleTypeDomain GeositeItemType = iota
	GeositeRuleTypeDomainSuffix
	GeositeRuleTypeDomainKeyword
	GeositeRuleTypeDomainRegex
)

type GeositeItem struct {
	Type  GeositeItemType
	Value string
}

type GeositeReader struct {
	closer        io.Closer
	data          []byte
	metadataIndex int
	domainIndex   map[string]int
	domainLength  map[string]int
	datItems      map[string][]GeositeItem
}

func OpenGeosite(path string) (*GeositeReader, []string, error) {
	content, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	reader, codes, err := NewGeositeReader(content)
	if err != nil {
		content.Close()
		return nil, nil, err
	}
	return reader, codes, nil
}

func NewGeositeReader(readSeeker io.ReadSeeker) (*GeositeReader, []string, error) {
	data, err := readDatabase(readSeeker)
	if err != nil {
		return nil, nil, err
	}
	reader := &GeositeReader{data: data}
	reader.closer, _ = readSeeker.(io.Closer)
	var codes []string
	if data[0] == 0 {
		if err := reader.readMetadata(); err != nil {
			return nil, nil, err
		}
		for code := range reader.domainIndex {
			codes = append(codes, code)
		}
	} else {
		reader.datItems, err = parseGeositeDAT(data)
		if err != nil {
			return nil, nil, err
		}
		reader.data = nil
		for code := range reader.datItems {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)
	return reader, codes, nil
}

func (r *GeositeReader) readMetadata() error {
	reader := bytes.NewReader(r.data)
	version, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if version != 0 {
		return fmt.Errorf("unknown geosite version")
	}
	entryLength, err := binary.ReadUvarint(reader)
	if err != nil {
		return err
	}
	if entryLength > uint64(reader.Len()/3) {
		return fmt.Errorf("invalid geosite metadata entry count")
	}
	domainIndex := make(map[string]int)
	domainLength := make(map[string]int)
	for i := 0; i < int(entryLength); i++ {
		code, err := readString(reader)
		if err != nil {
			return err
		}
		codeIndex, err := binary.ReadUvarint(reader)
		if err != nil {
			return err
		}
		codeLength, err := binary.ReadUvarint(reader)
		if err != nil {
			return err
		}
		if codeIndex > uint64(len(r.data)) || codeLength > uint64(len(r.data)/2) {
			return fmt.Errorf("invalid geosite metadata offset or length")
		}
		if _, exists := domainIndex[code]; exists {
			return fmt.Errorf("duplicate geosite code %q", code)
		}
		domainIndex[code] = int(codeIndex)
		domainLength[code] = int(codeLength)
	}
	r.domainIndex = domainIndex
	r.domainLength = domainLength
	r.metadataIndex = len(r.data) - reader.Len()
	for code, offset := range domainIndex {
		if offset > reader.Len() || domainLength[code] > (reader.Len()-offset)/2 {
			return fmt.Errorf("invalid geosite data offset or length for %q", code)
		}
	}
	return nil
}

func (r *GeositeReader) Read(code string) ([]GeositeItem, error) {
	if r.datItems != nil {
		items, exists := r.datItems[strings.ToLower(code)]
		if !exists {
			return nil, fmt.Errorf("geosite code %q not exists", code)
		}
		return slices.Clone(items), nil
	}
	index, exists := r.domainIndex[code]
	if !exists {
		return nil, fmt.Errorf("geosite code %q not exists", code)
	}
	reader := bytes.NewReader(r.data[r.metadataIndex+index:])
	var items []GeositeItem
	for range r.domainLength[code] {
		typeByte, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if typeByte > GeositeRuleTypeDomainRegex {
			return nil, fmt.Errorf("unsupported geosite item type %d", typeByte)
		}
		value, err := readString(reader)
		if err != nil {
			return nil, err
		}
		items = append(items, GeositeItem{Type: typeByte, Value: value})
	}
	return items, nil
}

func (r *GeositeReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

func readString(reader *bytes.Reader) (string, error) {
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", err
	}
	if length > uint64(reader.Len()) {
		return "", io.ErrUnexpectedEOF
	}
	data := make([]byte, int(length))
	_, err = io.ReadFull(reader, data)
	return string(data), err
}

type GeositeMatcher struct {
	domainMap   map[string]bool
	suffixList  []string
	keywordList []string
	regexList   []string
}

func NewGeositeMatcher(items []GeositeItem) *GeositeMatcher {
	matcher := &GeositeMatcher{domainMap: make(map[string]bool)}
	for _, item := range items {
		switch item.Type {
		case GeositeRuleTypeDomain:
			matcher.domainMap[item.Value] = true
		case GeositeRuleTypeDomainSuffix:
			matcher.suffixList = append(matcher.suffixList, item.Value)
		case GeositeRuleTypeDomainKeyword:
			matcher.keywordList = append(matcher.keywordList, item.Value)
		case GeositeRuleTypeDomainRegex:
			matcher.regexList = append(matcher.regexList, item.Value)
		}
	}
	return matcher
}

func (m *GeositeMatcher) Match(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if m.domainMap[domain] {
		return "domain=" + domain
	}
	for _, suffix := range m.suffixList {
		// A leading dot in the legacy DB denotes subdomains only. A root
		// domain from DAT includes the apex as well as dot-delimited children.
		matches := strings.HasSuffix(domain, suffix)
		if !strings.HasPrefix(suffix, ".") {
			matches = domain == suffix || strings.HasSuffix(domain, "."+suffix)
		}
		if matches {
			return "domain_suffix=" + suffix
		}
	}
	for _, keyword := range m.keywordList {
		if strings.Contains(domain, keyword) {
			return "domain_keyword=" + keyword
		}
	}
	for _, regexStr := range m.regexList {
		regex, err := regexp.Compile(regexStr)
		if err == nil && regex.MatchString(domain) {
			return "domain_regex=" + regexStr
		}
	}
	return ""
}
