package service

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/ackwrap/ackrun/internal/geoquery"
)

const geoReadCacheMaxBytes = 8 * 1024 * 1024
const geoReadCacheMaxEntries = 32

type geoReadCacheKey struct {
	path, kind, code string
}

type geoReadCacheValue struct {
	codes []string
	items []geoquery.GeositeItem
}

type geoReadCacheEntry struct {
	info  os.FileInfo
	value geoReadCacheValue
	bytes int
	used  uint64
}

type geoDatabaseReadCache struct {
	mu      sync.Mutex
	entries map[geoReadCacheKey]geoReadCacheEntry
	loading map[geoReadCacheKey]chan struct{}
	bytes   int
	clock   uint64
}

// Config generation creates short-lived services, so share the bounded cache.
// Keep category names and recently browsed categories, never a decoded GeoIP DB.
var geoDatabaseReads geoDatabaseReadCache

func sameGeoFile(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) &&
		left.Size() == right.Size() && left.ModTime().Equal(right.ModTime()) && left.Mode() == right.Mode()
}

func (cache *geoDatabaseReadCache) load(key geoReadCacheKey, load func() (geoReadCacheValue, error)) (geoReadCacheValue, error) {
	path, err := filepath.Abs(key.path)
	if err != nil {
		return geoReadCacheValue{}, err
	}
	key.path = path
	// Serialize cold parses of one database without blocking any warm reads or
	// holding the cache mutex during I/O. Different categories share this flight.
	flightKey := geoReadCacheKey{path: path, kind: key.kind}
	var info os.FileInfo
	for {
		info, err = os.Stat(path)
		cache.mu.Lock()
		if err != nil {
			cache.remove(key)
			cache.mu.Unlock()
			return geoReadCacheValue{}, err
		}
		cache.clock++
		if entry, ok := cache.entries[key]; ok && sameGeoFile(entry.info, info) {
			entry.used = cache.clock
			cache.entries[key] = entry
			cache.mu.Unlock()
			return entry.value, nil
		}
		cache.remove(key)
		if done, ok := cache.loading[flightKey]; ok {
			cache.mu.Unlock()
			<-done
			continue
		}
		if cache.loading == nil {
			cache.loading = make(map[geoReadCacheKey]chan struct{})
		}
		cache.loading[flightKey] = make(chan struct{})
		cache.mu.Unlock()
		break
	}
	defer func() {
		cache.mu.Lock()
		close(cache.loading[flightKey])
		delete(cache.loading, flightKey)
		cache.mu.Unlock()
	}()
	value, err := load()
	if err != nil {
		return geoReadCacheValue{}, err
	}
	after, err := os.Stat(path)
	if err != nil {
		return geoReadCacheValue{}, err
	}
	if !sameGeoFile(info, after) {
		return geoReadCacheValue{}, fmt.Errorf("Geo database changed while reading")
	}
	cache.mu.Lock()
	cache.put(key, info, value)
	cache.mu.Unlock()
	return value, nil
}

func (cache *geoDatabaseReadCache) remove(key geoReadCacheKey) {
	if entry, ok := cache.entries[key]; ok {
		cache.bytes -= entry.bytes
		delete(cache.entries, key)
	}
}

func (cache *geoDatabaseReadCache) put(key geoReadCacheKey, info os.FileInfo, value geoReadCacheValue) {
	// Include slice backing storage and strings; allow headroom for map/entry metadata.
	size := 512 + len(key.path) + len(key.kind) + len(key.code) + cap(value.codes)*16 + cap(value.items)*24
	for _, code := range value.codes {
		size += len(code)
	}
	for _, item := range value.items {
		size += len(item.Value)
	}
	cache.remove(key)
	if size > geoReadCacheMaxBytes {
		return
	}
	if cache.entries == nil {
		cache.entries = make(map[geoReadCacheKey]geoReadCacheEntry)
	}
	for len(cache.entries) >= geoReadCacheMaxEntries || cache.bytes+size > geoReadCacheMaxBytes {
		var oldest geoReadCacheKey
		var age uint64 = ^uint64(0)
		for key, entry := range cache.entries {
			if entry.used < age {
				oldest, age = key, entry.used
			}
		}
		cache.remove(oldest)
	}
	cache.clock++
	cache.entries[key] = geoReadCacheEntry{info: info, value: value, bytes: size, used: cache.clock}
	cache.bytes += size
}

func cachedGeoAssetCodes(kind, path string) ([]string, error) {
	value, err := geoDatabaseReads.load(geoReadCacheKey{path: path, kind: kind}, func() (geoReadCacheValue, error) {
		codes, err := geoAssetCodes(kind, path)
		return geoReadCacheValue{codes: codes}, err
	})
	// Callers may sort or append category names without modifying the shared cache.
	return slices.Clone(value.codes), err
}

// Publication has already validated all entries. Reuse that work on the first
// request, including requests from short-lived config-generation services.
func rememberGeoAssetCodes(kind, path string, codes []string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return
	}
	geoDatabaseReads.mu.Lock()
	defer geoDatabaseReads.mu.Unlock()
	geoDatabaseReads.put(geoReadCacheKey{path: absPath, kind: kind}, info, geoReadCacheValue{codes: slices.Clone(codes)})
}

// Returned items are immutable. Pagination copies only the requested page into
// the response instead of cloning an entire category on every request.
func cachedGeositeItems(path, code string) ([]geoquery.GeositeItem, error) {
	if code == "" {
		return nil, fmt.Errorf("geosite code is empty")
	}
	value, err := geoDatabaseReads.load(geoReadCacheKey{path: path, kind: "geosite", code: code}, func() (geoReadCacheValue, error) {
		reader, _, err := geoquery.OpenGeosite(path)
		if err != nil {
			return geoReadCacheValue{}, err
		}
		defer reader.Close()
		items, err := reader.Read(code)
		return geoReadCacheValue{items: items}, err
	})
	return value.items, err
}
