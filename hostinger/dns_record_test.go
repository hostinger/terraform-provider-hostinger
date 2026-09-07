package hostinger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestResourceHostingerDNSRecord_Schema(t *testing.T) {
	resource := resourceHostingerDNSRecord()

	if err := resource.InternalValidate(resource.Schema, true); err != nil {
		t.Fatalf("schema validation failed: %s", err)
	}

	expectedFields := []string{"id", "zone", "name", "type", "value", "ttl"}

	for _, field := range expectedFields {
		if _, ok := resource.Schema[field]; !ok {
			t.Errorf("expected field %q not found in schema", field)
		}
	}
}

func TestResourceHostingerDNSRecord_BasicCreate(t *testing.T) {
	resource := resourceHostingerDNSRecord()
	data := resource.TestResourceData()

	if err := data.Set("zone", "example.com"); err != nil {
		t.Fatalf("failed to set zone: %v", err)
	}
	if err := data.Set("name", "test"); err != nil {
		t.Fatalf("failed to set name: %v", err)
	}
	if err := data.Set("type", "CNAME"); err != nil {
		t.Fatalf("failed to set type: %v", err)
	}
	if err := data.Set("value", "target.example.com"); err != nil {
		t.Fatalf("failed to set value: %v", err)
	}
	if err := data.Set("ttl", 14400); err != nil {
		t.Fatalf("failed to set ttl: %v", err)
	}

	// Just check that Set works without panic and has the right values
	if v := data.Get("zone"); v != "example.com" {
		t.Errorf("expected zone to be 'example.com', got %v", v)
	}
}

func TestDNSContentMatches(t *testing.T) {
	// The Hostinger API canonicalizes record content before storing it: hostnames
	// gain a trailing dot, TXT values are quoted and CAA values are re-quoted.
	// The terraform value is whatever the user wrote, so comparison must normalize.
	cases := []struct {
		name        string
		recordType  string
		apiContent  string
		configValue string
		want        bool
	}{
		{"CNAME trailing dot from API", "CNAME", "cname.vercel-dns.com.", "cname.vercel-dns.com", true},
		{"CNAME trailing dot in config", "CNAME", "cname.vercel-dns.com", "cname.vercel-dns.com.", true},
		{"CNAME different case", "CNAME", "CNAME.Vercel-DNS.com.", "cname.vercel-dns.com", true},
		{"CNAME different target", "CNAME", "other.vercel-dns.com.", "cname.vercel-dns.com", false},
		{"NS trailing dot", "NS", "ns1.dns-parking.com.", "ns1.dns-parking.com", true},
		{"ALIAS trailing dot", "ALIAS", "target.example.com.", "target.example.com", true},
		{"MX priority plus trailing dot", "MX", "10 feedback-smtp.ap-northeast-1.amazonses.com.", "10 feedback-smtp.ap-northeast-1.amazonses.com", true},
		{"MX extra whitespace", "MX", "10 mx.example.com.", "10   mx.example.com", true},
		{"MX different priority", "MX", "20 mx.example.com.", "10 mx.example.com", false},
		{"MX missing priority", "MX", "10 mx.example.com.", "mx.example.com", false},
		{"SRV trailing dot", "SRV", "10 5 5060 sip.example.com.", "10 5 5060 sip.example.com", true},
		{"SRV different port", "SRV", "10 5 5061 sip.example.com.", "10 5 5060 sip.example.com", false},
		{"TXT quoted by API", "TXT", "\"v=spf1 include:_spf.example.com ~all\"", "v=spf1 include:_spf.example.com ~all", true},
		{"TXT stays case sensitive", "TXT", "\"AbC\"", "abc", false},
		{"CAA quoted value", "CAA", "0 issue \"letsencrypt.org\"", "0 issue letsencrypt.org", true},
		{"CAA different tag", "CAA", "0 issuewild \"letsencrypt.org\"", "0 issue letsencrypt.org", false},
		{"A record exact", "A", "192.0.2.1", "192.0.2.1", true},
		{"A record different", "A", "192.0.2.2", "192.0.2.1", false},
		{"AAAA case insensitive", "AAAA", "2001:DB8::1", "2001:db8::1", true},
		{"lowercase record type still normalizes", "cname", "cname.vercel-dns.com.", "cname.vercel-dns.com", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dnsContentMatches(tc.recordType, tc.apiContent, tc.configValue); got != tc.want {
				t.Errorf("dnsContentMatches(%q, %q, %q) = %v, want %v",
					tc.recordType, tc.apiContent, tc.configValue, got, tc.want)
			}
		})
	}
}

func TestParseDNSRecordID(t *testing.T) {
	name, recordType, value, err := parseDNSRecordID("staging|CNAME|cname.vercel-dns.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "staging" || recordType != "CNAME" || value != "cname.vercel-dns.com" {
		t.Errorf("got (%q, %q, %q)", name, recordType, value)
	}

	// TXT values may contain pipes; everything after the type belongs to the value.
	_, _, value, err = parseDNSRecordID("@|TXT|v=spf1 a|b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "v=spf1 a|b" {
		t.Errorf("expected value to keep embedded pipes, got %q", value)
	}

	if _, _, _, err := parseDNSRecordID(""); !errors.Is(err, errEmptyDNSRecordID) {
		t.Errorf("expected errEmptyDNSRecordID for an empty ID, got %v", err)
	}

	if _, _, _, err := parseDNSRecordID("staging|CNAME"); err == nil {
		t.Error("expected an error for a truncated ID")
	}
}

func TestResourceHostingerDNSRecord_SupportsImport(t *testing.T) {
	r := resourceHostingerDNSRecord()
	if r.Importer == nil || r.Importer.StateContext == nil {
		t.Fatal("expected the resource to support terraform import")
	}

	d := r.TestResourceData()
	d.SetId("example.com|send|MX|10 mx.example.com")

	states, err := r.Importer.StateContext(context.Background(), d, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 imported state, got %d", len(states))
	}

	imported := states[0]
	if got := imported.Get("zone"); got != "example.com" {
		t.Errorf("zone = %v, want example.com", got)
	}
	if got := imported.Get("name"); got != "send" {
		t.Errorf("name = %v, want send", got)
	}
	if got := imported.Get("type"); got != "MX" {
		t.Errorf("type = %v, want MX", got)
	}
	if got := imported.Get("value"); got != "10 mx.example.com" {
		t.Errorf("value = %v, want '10 mx.example.com'", got)
	}
	if got := imported.Id(); got != "send|MX|10 mx.example.com" {
		t.Errorf("id = %v, want 'send|MX|10 mx.example.com'", got)
	}
}

func TestResourceHostingerDNSRecord_ImportRejectsBadID(t *testing.T) {
	r := resourceHostingerDNSRecord()
	d := r.TestResourceData()
	d.SetId("send|MX|10 mx.example.com")

	if _, err := r.Importer.StateContext(context.Background(), d, nil); err == nil {
		t.Error("expected an error when the import ID is missing the zone")
	}
}

// dnsZoneServer serves the Hostinger DNS zone API: it records the PUT payload and
// replies to GET with the canonicalized zone entries the real API would return.
func dnsZoneServer(t *testing.T, entries string, captured *map[string]interface{}) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if captured != nil {
				body, _ := io.ReadAll(r.Body)
				payload := map[string]interface{}{}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Errorf("bad PUT payload: %v", err)
				}
				*captured = payload
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":"Request accepted"}`))
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(entries))
		default:
			t.Errorf("unexpected %s request to %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func testDNSResourceData(t *testing.T, zone, name, recordType, value string, ttl int) *schema.ResourceData {
	t.Helper()

	d := resourceHostingerDNSRecord().TestResourceData()
	for key, val := range map[string]interface{}{
		"zone": zone, "name": name, "type": recordType, "value": value, "ttl": ttl,
	} {
		if err := d.Set(key, val); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}
	return d
}

func TestCreateDNSRecord_CanonicalizedContent(t *testing.T) {
	cases := []struct {
		name       string
		recordType string
		value      string
		ttl        int
		entries    string
	}{
		{
			name:       "CNAME gains a trailing dot",
			recordType: "CNAME",
			value:      "cname.vercel-dns.com",
			ttl:        14400,
			entries:    `[{"name":"staging","type":"CNAME","ttl":14400,"records":[{"content":"cname.vercel-dns.com.","is_disabled":false}]}]`,
		},
		{
			name:       "MX keeps its priority and gains a trailing dot",
			recordType: "MX",
			value:      "10 feedback-smtp.ap-northeast-1.amazonses.com",
			ttl:        900,
			entries:    `[{"name":"staging","type":"MX","ttl":900,"records":[{"content":"10 feedback-smtp.ap-northeast-1.amazonses.com.","is_disabled":false}]}]`,
		},
		{
			name:       "TXT gets quoted",
			recordType: "TXT",
			value:      "v=spf1 include:amazonses.com ~all",
			ttl:        300,
			entries:    `[{"name":"staging","type":"TXT","ttl":300,"records":[{"content":"\"v=spf1 include:amazonses.com ~all\"","is_disabled":false}]}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := dnsZoneServer(t, tc.entries, nil)
			defer server.Close()

			client := NewHostingerClient("token", "test")
			client.BaseURL = server.URL

			d := testDNSResourceData(t, "example.com", "staging", tc.recordType, tc.value, tc.ttl)

			if err := createDNSRecord(context.Background(), d, client); err != nil {
				t.Fatalf("createDNSRecord failed: %v", err)
			}

			wantID := fmt.Sprintf("staging|%s|%s", tc.recordType, tc.value)
			if d.Id() != wantID {
				t.Errorf("id = %q, want %q", d.Id(), wantID)
			}
			if got := d.Get("value"); got != tc.value {
				t.Errorf("value = %v, want %v (the configured form must be preserved)", got, tc.value)
			}
			if got := d.Get("ttl"); got != tc.ttl {
				t.Errorf("ttl = %v, want %v", got, tc.ttl)
			}
		})
	}
}

func TestCreateDNSRecord_MissingRecordDoesNotCorruptID(t *testing.T) {
	// Regression: when the zone read did not match, the retry loop cleared the ID
	// and the next iteration failed with "unexpected ID format" instead of retrying.
	server := dnsZoneServer(t, `[]`, nil)
	defer server.Close()

	client := NewHostingerClient("token", "test")
	client.BaseURL = server.URL

	original := dnsRecordCreateTimeout
	dnsRecordCreateTimeout = time.Second
	t.Cleanup(func() { dnsRecordCreateTimeout = original })

	d := testDNSResourceData(t, "example.com", "staging", "CNAME", "cname.vercel-dns.com", 14400)

	err := createDNSRecord(context.Background(), d, client)
	if err == nil {
		t.Fatal("expected createDNSRecord to fail when the record never appears")
	}
	if !strings.Contains(err.Error(), "error waiting for DNS record to be created") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "unexpected") {
		t.Errorf("retry loop leaked an ID parsing error: %v", err)
	}
	if d.Id() == "" {
		t.Error("expected the ID to be kept so the created record is not orphaned")
	}
}

func TestCreateDNSRecord_OverwriteIsConfigurable(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("overwrite=%v", overwrite), func(t *testing.T) {
			var payload map[string]interface{}
			entries := `[{"name":"staging","type":"A","ttl":300,"records":[{"content":"192.0.2.1","is_disabled":false}]}]`
			server := dnsZoneServer(t, entries, &payload)
			defer server.Close()

			client := NewHostingerClient("token", "test")
			client.BaseURL = server.URL

			d := testDNSResourceData(t, "example.com", "staging", "A", "192.0.2.1", 300)
			if err := d.Set("overwrite", overwrite); err != nil {
				t.Fatalf("failed to set overwrite: %v", err)
			}

			if err := createDNSRecord(context.Background(), d, client); err != nil {
				t.Fatalf("createDNSRecord failed: %v", err)
			}
			if payload["overwrite"] != overwrite {
				t.Errorf("payload overwrite = %v, want %v", payload["overwrite"], overwrite)
			}
		})
	}
}

func TestReadDNSRecord_EmptyIDIsNotFound(t *testing.T) {
	server := dnsZoneServer(t, `[]`, nil)
	defer server.Close()

	client := NewHostingerClient("token", "test")
	client.BaseURL = server.URL

	d := testDNSResourceData(t, "example.com", "staging", "CNAME", "cname.vercel-dns.com", 14400)
	d.SetId("")

	if err := readDNSRecord(context.Background(), d, client); err != nil {
		t.Fatalf("expected an empty ID to read as not found, got: %v", err)
	}
	if d.Id() != "" {
		t.Errorf("expected the ID to stay empty, got %q", d.Id())
	}
}

func TestReadDNSRecord_DisabledRecordIsNotFound(t *testing.T) {
	entries := `[{"name":"staging","type":"CNAME","ttl":14400,"records":[{"content":"cname.vercel-dns.com.","is_disabled":true}]}]`
	server := dnsZoneServer(t, entries, nil)
	defer server.Close()

	client := NewHostingerClient("token", "test")
	client.BaseURL = server.URL

	d := testDNSResourceData(t, "example.com", "staging", "CNAME", "cname.vercel-dns.com", 14400)
	d.SetId("staging|CNAME|cname.vercel-dns.com")

	if err := readDNSRecord(context.Background(), d, client); err != nil {
		t.Fatalf("readDNSRecord failed: %v", err)
	}
	if d.Id() != "" {
		t.Error("expected a disabled record to be treated as absent")
	}
}

// dnsZoneDeleteServer serves the zone API for delete flows, capturing the DELETE
// filter payload and any PUT payload used to recreate preserved records.
func dnsZoneDeleteServer(t *testing.T, entries string, deleted, recreated *map[string]interface{}) *httptest.Server {
	t.Helper()

	decode := func(r *http.Request, into *map[string]interface{}) {
		body, _ := io.ReadAll(r.Body)
		payload := map[string]interface{}{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("bad %s payload: %v", r.Method, err)
		}
		*into = payload
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(entries))
		case http.MethodDelete:
			decode(r, deleted)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			decode(r, recreated)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected %s request", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func TestDeleteDNSRecord_PreservesSiblingRecords(t *testing.T) {
	entries := `[{"name":"send","type":"MX","ttl":900,"records":[
		{"content":"10 mx1.example.com.","is_disabled":false},
		{"content":"20 mx2.example.com.","is_disabled":false}]}]`

	var deleted, recreated map[string]interface{}
	server := dnsZoneDeleteServer(t, entries, &deleted, &recreated)
	defer server.Close()

	client := NewHostingerClient("token", "test")
	client.BaseURL = server.URL

	d := testDNSResourceData(t, "example.com", "send", "MX", "10 mx1.example.com", 900)
	d.SetId("send|MX|10 mx1.example.com")

	if err := deleteDNSRecord(context.Background(), d, client); err != nil {
		t.Fatalf("deleteDNSRecord failed: %v", err)
	}
	if d.Id() != "" {
		t.Errorf("expected the ID to be cleared, got %q", d.Id())
	}

	filters, ok := deleted["filters"].([]interface{})
	if !ok || len(filters) != 1 {
		t.Fatalf("unexpected delete payload: %v", deleted)
	}
	filter := filters[0].(map[string]interface{})
	if filter["name"] != "send" || filter["type"] != "MX" {
		t.Errorf("unexpected delete filter: %v", filter)
	}

	zone, ok := recreated["zone"].([]interface{})
	if !ok || len(zone) != 1 {
		t.Fatalf("expected the sibling record to be recreated, got: %v", recreated)
	}
	kept := zone[0].(map[string]interface{})["records"].([]interface{})
	if len(kept) != 1 {
		t.Fatalf("expected exactly one preserved record, got %v", kept)
	}
	if got := kept[0].(map[string]interface{})["content"]; got != "20 mx2.example.com." {
		t.Errorf("preserved content = %v, want '20 mx2.example.com.'", got)
	}
}

func TestDeleteDNSRecord_OnlyRecordIsNotRecreated(t *testing.T) {
	entries := `[{"name":"staging","type":"CNAME","ttl":14400,"records":[{"content":"cname.vercel-dns.com.","is_disabled":false}]}]`

	var deleted, recreated map[string]interface{}
	server := dnsZoneDeleteServer(t, entries, &deleted, &recreated)
	defer server.Close()

	client := NewHostingerClient("token", "test")
	client.BaseURL = server.URL

	d := testDNSResourceData(t, "example.com", "staging", "CNAME", "cname.vercel-dns.com", 14400)
	d.SetId("staging|CNAME|cname.vercel-dns.com")

	if err := deleteDNSRecord(context.Background(), d, client); err != nil {
		t.Fatalf("deleteDNSRecord failed: %v", err)
	}
	if deleted == nil {
		t.Error("expected a DELETE request")
	}
	if recreated != nil {
		t.Errorf("expected no recreate request, got: %v", recreated)
	}
}
