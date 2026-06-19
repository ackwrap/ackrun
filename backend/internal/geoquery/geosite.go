package geoquery

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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
	access         sync.Mutex
	reader         io.ReadSeeker
	bufferedReader *bufio.Reader
	metadataIndex  int64
	domainIndex    map[string]int
	domainLength   map[string]int
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
	reader := &GeositeReader{reader: readSeeker}
	if err := reader.readMetadata(); err != nil {
		return nil, nil, err
	}
	codes := make([]string, 0, len(reader.domainIndex))
	for code := range reader.domainIndex {
		codes = append(codes, code)
	}
	return reader, codes, nil
}

func (r *GeositeReader) readMetadata() error {
	counter := &readCounter{Reader: r.reader}
	reader := bufio.NewReader(counter)
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
		domainIndex[code] = int(codeIndex)
		domainLength[code] = int(codeLength)
	}
	r.domainIndex = domainIndex
	r.domainLength = domainLength
	r.metadataIndex = counter.count - int64(reader.Buffered())
	r.bufferedReader = reader
	return nil
}

func (r *GeositeReader) Read(code string) ([]GeositeItem, error) {
	r.access.Lock()
	defer r.access.Unlock()
	index, exists := r.domainIndex[code]
	if !exists {
		return nil, fmt.Errorf("geosite code %q not exists", code)
	}
	if _, err := r.reader.Seek(r.metadataIndex+int64(index), io.SeekStart); err != nil {
		return nil, err
	}
	r.bufferedReader.Reset(r.reader)
	items := make([]GeositeItem, r.domainLength[code])
	for i := range items {
		typeByte, err := r.bufferedReader.ReadByte()
		if err != nil {
			return nil, err
		}
		items[i].Type = GeositeItemType(typeByte)
		items[i].Value, err = readString(r.bufferedReader)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *GeositeReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type readCounter struct {
	io.Reader
	count int64
}

func (r *readCounter) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&r.count, int64(n))
	}
	return
}

func readString(reader io.ByteReader) (string, error) {
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", err
	}
	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i], err = reader.ReadByte()
		if err != nil {
			return "", err
		}
	}
	return string(bytes), nil
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
		if strings.HasSuffix(domain, suffix) {
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
