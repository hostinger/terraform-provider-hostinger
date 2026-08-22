package hostinger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// zoneCacheTTL is how long a fetched zone stays usable.
//
// The DNS resource reads the whole zone once per record, so refreshing a
// 25-record zone issues 25 identical requests within a second or two of each
// other. A few seconds is enough to collapse one such burst into a single call
// while still being far shorter than the gap between separate operations, so no
// operation starts from records fetched during an earlier one.
const zoneCacheTTL = 10 * time.Second

// zoneCache holds recently fetched DNS zones, keyed by zone name. Its zero
// value is ready to use.
type zoneCache struct {
	mu      sync.Mutex
	entries map[string]*zoneCacheEntry
}

// zoneCacheEntry holds one zone's records. Its mutex is held for the whole of a
// fetch, so when Terraform reads several records of the same zone in parallel
// one goroutine fetches and the others wait for its result, rather than all
// missing the cache and fetching at once.
type zoneCacheEntry struct {
	mu      sync.Mutex
	records []DNSEntry
	fetched time.Time
}

// entry returns the cache slot for a zone, creating it if this is the first
// time the zone has been seen.
func (c *zoneCache) entry(zone string) *zoneCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[string]*zoneCacheEntry)
	}

	entry, ok := c.entries[zone]
	if !ok {
		entry = &zoneCacheEntry{}
		c.entries[zone] = entry
	}
	return entry
}

// GetZoneRecords returns every record in a zone, fetching it from the API at
// most once per zoneCacheTTL.
//
// The returned slice is shared with the cache and callers must treat it as
// read-only.
func (c *HostingerClient) GetZoneRecords(zone string) ([]DNSEntry, error) {
	entry := c.zoneCache.entry(zone)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if !entry.fetched.IsZero() && time.Since(entry.fetched) < zoneCacheTTL {
		return entry.records, nil
	}

	records, err := c.fetchZoneRecords(zone)
	if err != nil {
		return nil, err
	}

	entry.records = records
	entry.fetched = time.Now()
	return records, nil
}

// InvalidateZone drops any cached copy of a zone.
//
// Every write must call it. The create path in particular polls Read for the
// record it has just written, and would otherwise keep being handed the
// pre-write zone until the TTL expired.
func (c *HostingerClient) InvalidateZone(zone string) {
	entry := c.zoneCache.entry(zone)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.records = nil
	entry.fetched = time.Time{}
}

// fetchZoneRecords reads a zone from the API, bypassing the cache.
func (c *HostingerClient) fetchZoneRecords(zone string) ([]DNSEntry, error) {
	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", c.BaseURL, zone)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read DNS records: %s", body)
	}

	var entries []DNSEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal read response: %w", err)
	}
	return entries, nil
}
