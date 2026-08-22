package hostinger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// testZoneExample is a canned response for GET /api/dns/v1/zones/{zone}: a bare
// array of name/type groups, each holding one or more values. It deliberately
// covers the shapes matching has to cope with -- a multi-value MX group, a
// disabled record, TXT content that arrives wrapped in quotes, and a value
// containing the "|" character used as the resource ID separator.
const testZoneExample = `[
	{"name": "@", "type": "A", "ttl": 14400, "records": [{"content": "192.0.2.1", "is_disabled": false}]},
	{"name": "www", "type": "CNAME", "ttl": 3600, "records": [{"content": "example.com.", "is_disabled": false}]},
	{"name": "@", "type": "MX", "ttl": 14400, "records": [
		{"content": "10 mx1.example.com", "is_disabled": false},
		{"content": "20 mx2.example.com", "is_disabled": false}
	]},
	{"name": "@", "type": "TXT", "ttl": 14400, "records": [{"content": "\"v=spf1 include:_spf.example.com ~all\"", "is_disabled": false}]},
	{"name": "pipe", "type": "TXT", "ttl": 300, "records": [{"content": "\"key=a|b|c\"", "is_disabled": false}]},
	{"name": "disabled", "type": "A", "ttl": 14400, "records": [{"content": "192.0.2.9", "is_disabled": true}]}
]`

// zonePath is the API path for the canned zone above.
const zonePath = "/api/dns/v1/zones/example.com"

// testAPIServer is an httptest.Server standing in for the Hostinger API. It
// serves canned DNS zone payloads and records every request it receives, so
// tests can assert not just the resulting state but how many API calls were
// made to produce it.
type testAPIServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string
	zones    map[string]string

	// intercept, when set, handles a request instead of the default zone
	// routing. Returning false falls through to the default behaviour.
	intercept func(w http.ResponseWriter, r *http.Request) bool
}

// newTestAPIServer starts a server holding the given zones, keyed by zone name
// with the raw JSON body to return for a GET. It is closed when the test ends.
func newTestAPIServer(t *testing.T, zones map[string]string) *testAPIServer {
	t.Helper()

	s := &testAPIServer{zones: zones}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *testAPIServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.Method+" "+r.URL.Path)
	intercept := s.intercept
	s.mu.Unlock()

	if intercept != nil && intercept(w, r) {
		return
	}

	zone := strings.TrimPrefix(r.URL.Path, "/api/dns/v1/zones/")
	if zone == r.URL.Path {
		http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		body, ok := s.zones[zone]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "unknown zone: "+zone, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	case http.MethodPut, http.MethodDelete:
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	default:
		http.Error(w, "unexpected method: "+r.Method, http.StatusMethodNotAllowed)
	}
}

// client returns a HostingerClient pointed at this server.
func (s *testAPIServer) client() *HostingerClient {
	return &HostingerClient{
		BaseURL:    s.URL,
		HTTPClient: s.Client(),
		Token:      "test-token",
		Version:    "test",
	}
}

// setIntercept installs a handler that runs before the default zone routing.
func (s *testAPIServer) setIntercept(fn func(w http.ResponseWriter, r *http.Request) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.intercept = fn
}

// setZone replaces a zone's payload, simulating the zone changing underneath a
// client that may be holding a cached copy.
func (s *testAPIServer) setZone(zone, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones[zone] = body
}

// countRequests returns how many requests matched "METHOD /path" exactly.
func (s *testAPIServer) countRequests(method, path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	want := method + " " + path
	n := 0
	for _, req := range s.requests {
		if req == want {
			n++
		}
	}
	return n
}

// totalRequests returns how many requests the server has handled.
func (s *testAPIServer) totalRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// testDNSRecordData builds ResourceData for hostinger_dns_record with the given
// zone and synthetic "name|type|value" ID, as the CRUD functions expect it.
func testDNSRecordData(t *testing.T, zone, id string) *schema.ResourceData {
	t.Helper()

	d := resourceHostingerDNSRecord().TestResourceData()
	if err := d.Set("zone", zone); err != nil {
		t.Fatalf("failed to set zone: %v", err)
	}
	d.SetId(id)
	return d
}

// TestAPIServerHarness exercises the harness itself: a client built by client()
// reaches the canned zones, every request is counted, setZone changes what is
// served, and setIntercept can take over a response.
func TestAPIServerHarness(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	get := func() (string, int) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, client.BaseURL+"/api/dns/v1/zones/example.com", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		client.addStandardHeaders(req)

		resp, err := client.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		return string(body), resp.StatusCode
	}

	body, status := get()
	if status != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", status)
	}
	if !strings.Contains(body, "192.0.2.1") {
		t.Errorf("expected the canned zone, got %s", body)
	}

	server.setZone("example.com", `[]`)
	if body, _ = get(); strings.Contains(body, "192.0.2.1") {
		t.Errorf("expected setZone to replace the payload, got %s", body)
	}

	server.setIntercept(func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusTooManyRequests)
		return true
	})
	if _, status = get(); status != http.StatusTooManyRequests {
		t.Errorf("expected the intercept to answer 429, got %d", status)
	}

	if n := server.countRequests(http.MethodGet, "/api/dns/v1/zones/example.com"); n != 3 {
		t.Errorf("expected 3 counted zone GETs, got %d", n)
	}
	if n := server.totalRequests(); n != 3 {
		t.Errorf("expected 3 total requests, got %d", n)
	}

	d := testDNSRecordData(t, "example.com", "www|CNAME|example.com.")
	if got := d.Get("zone"); got != "example.com" {
		t.Errorf("expected zone example.com, got %v", got)
	}
	if got := d.Id(); got != "www|CNAME|example.com." {
		t.Errorf("expected the synthetic ID to be preserved, got %v", got)
	}
}
