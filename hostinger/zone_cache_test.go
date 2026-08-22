package hostinger

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// withExtraRecord is testZoneExample plus the record the create test writes.
const withExtraRecord = `[
	{"name": "@", "type": "A", "ttl": 14400, "records": [{"content": "192.0.2.1", "is_disabled": false}]},
	{"name": "new", "type": "A", "ttl": 3600, "records": [{"content": "192.0.2.55", "is_disabled": false}]}
]`

// TestGetZoneRecords_CollapsesConcurrentReads is the case that matters: a plan
// calls Read once per record, in parallel up to -parallelism, and every one of
// those reads needs the whole zone. Without a cache that is one API call per
// record, which is what trips Hostinger's rate limiter.
func TestGetZoneRecords_CollapsesConcurrentReads(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	const readers = 26

	var wg sync.WaitGroup
	results := make([][]DNSEntry, readers)
	errs := make([]error, readers)

	// A gate so the readers start together, rather than trickling in one at a
	// time and each finding the previous one's result already cached.
	start := make(chan struct{})

	for i := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = client.GetZoneRecords("example.com")
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("reader %d failed: %v", i, err)
		}
		if len(results[i]) != len(results[0]) {
			t.Errorf("reader %d got %d records, reader 0 got %d", i, len(results[i]), len(results[0]))
		}
	}

	if n := server.countRequests("GET", zonePath); n != 1 {
		t.Errorf("expected %d concurrent reads to make 1 zone GET, got %d", readers, n)
	}
}

func TestGetZoneRecords_CachesSequentialReads(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	for range 5 {
		if _, err := client.GetZoneRecords("example.com"); err != nil {
			t.Fatalf("read failed: %v", err)
		}
	}

	if n := server.countRequests("GET", zonePath); n != 1 {
		t.Errorf("expected 1 zone GET, got %d", n)
	}
}

func TestGetZoneRecords_CachesPerZone(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{
		"example.com": testZoneExample,
		"other.com":   testZoneExample,
	})
	client := server.client()

	for range 3 {
		if _, err := client.GetZoneRecords("example.com"); err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if _, err := client.GetZoneRecords("other.com"); err != nil {
			t.Fatalf("read failed: %v", err)
		}
	}

	if n := server.countRequests("GET", zonePath); n != 1 {
		t.Errorf("expected 1 GET for example.com, got %d", n)
	}
	if n := server.countRequests("GET", "/api/dns/v1/zones/other.com"); n != 1 {
		t.Errorf("expected 1 GET for other.com, got %d", n)
	}
}

func TestGetZoneRecords_ExpiresAfterTTL(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	if _, err := client.GetZoneRecords("example.com"); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// Age the entry past the TTL rather than sleeping for it.
	entry := client.zoneCache.entry("example.com")
	entry.mu.Lock()
	entry.fetched = time.Now().Add(-zoneCacheTTL - time.Second)
	entry.mu.Unlock()

	if _, err := client.GetZoneRecords("example.com"); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if n := server.countRequests("GET", zonePath); n != 2 {
		t.Errorf("expected an expired entry to be refetched, got %d GETs", n)
	}
}

// TestInvalidateZone_ForcesRefetch covers the correctness risk the cache
// introduces: a write must not leave a later read looking at the old zone.
func TestInvalidateZone_ForcesRefetch(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	records, err := client.GetZoneRecords("example.com")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	before := len(records)

	server.setZone("example.com", `[]`)

	// Still cached, so the change is not visible yet.
	if records, err = client.GetZoneRecords("example.com"); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(records) != before {
		t.Fatalf("expected the cached zone, got %d records", len(records))
	}

	client.InvalidateZone("example.com")

	if records, err = client.GetZoneRecords("example.com"); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected the refetched zone to be empty, got %d records", len(records))
	}
	if n := server.countRequests("GET", zonePath); n != 2 {
		t.Errorf("expected 2 zone GETs, got %d", n)
	}
}

func TestGetZoneRecords_ErrorIsNotCached(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	if _, err := client.GetZoneRecords("nosuchzone.com"); err == nil {
		t.Fatal("expected an error, got none")
	}
	if _, err := client.GetZoneRecords("nosuchzone.com"); err == nil {
		t.Fatal("expected an error, got none")
	}

	if n := server.countRequests("GET", "/api/dns/v1/zones/nosuchzone.com"); n != 2 {
		t.Errorf("expected a failed read to be retried, got %d GETs", n)
	}
}

// TestDNSRecordRead_SharesOneZoneFetch is the end-to-end shape of a refresh:
// one Read per record, all against the same zone, one API call in total.
func TestDNSRecordRead_SharesOneZoneFetch(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	ids := []string{
		"@|A|192.0.2.1",
		"www|CNAME|example.com.",
		"@|MX|10 mx1.example.com",
		"@|MX|20 mx2.example.com",
		`@|TXT|v=spf1 include:_spf.example.com ~all`,
	}

	for _, id := range ids {
		d := testDNSRecordData(t, "example.com", id)
		if err := resourceHostingerDNSRecordRead(d, client); err != nil {
			t.Fatalf("read of %q failed: %v", id, err)
		}
		if d.Id() == "" {
			t.Errorf("expected %q to be found in the zone", id)
		}
	}

	if n := server.countRequests("GET", zonePath); n != 1 {
		t.Errorf("expected %d reads to share 1 zone GET, got %d", len(ids), n)
	}
}

// TestDNSRecordCreate_InvalidatesBeforePolling covers the interaction that
// would break if a write did not invalidate: create polls Read until the new
// record shows up, and a cached zone would never contain it.
func TestDNSRecordCreate_InvalidatesBeforePolling(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	// Warm the cache, as a refresh preceding the apply would.
	if _, err := client.GetZoneRecords("example.com"); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// The write lands: from now on the zone also holds the new record.
	server.setIntercept(func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPut {
			server.setZone("example.com", withExtraRecord)
		}
		return false
	})

	d := resourceHostingerDNSRecord().TestResourceData()
	for key, value := range map[string]interface{}{
		"zone":  "example.com",
		"name":  "new",
		"type":  "A",
		"value": "192.0.2.55",
		"ttl":   3600,
	} {
		if err := d.Set(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	if err := resourceHostingerDNSRecordCreate(d, client); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if d.Id() != "new|A|192.0.2.55" {
		t.Errorf("expected the new record to be found after create, got id %q", d.Id())
	}
}

// TestDNSRecordDelete_InvalidatesZone checks that a read after a delete does not
// come back from the cache the delete itself populated.
func TestDNSRecordDelete_InvalidatesZone(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	d := testDNSRecordData(t, "example.com", "www|CNAME|example.com.")
	if err := resourceHostingerDNSRecordDelete(d, client); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	entry := client.zoneCache.entry("example.com")
	entry.mu.Lock()
	fetched := entry.fetched
	entry.mu.Unlock()

	if !fetched.IsZero() {
		t.Error("expected delete to leave no cached copy of the zone")
	}
}

func TestGetZoneRecords_ErrorMentionsTheZoneRead(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})

	_, err := server.client().GetZoneRecords("nosuchzone.com")
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if !strings.Contains(err.Error(), "failed to read DNS records") {
		t.Errorf("unexpected error: %v", err)
	}
}
