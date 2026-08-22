package hostinger

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// importRecord runs the importer over the canned zone and returns the imported
// ResourceData.
func importRecord(t *testing.T, client *HostingerClient, id string) (*schema.ResourceData, error) {
	t.Helper()

	d := resourceHostingerDNSRecord().TestResourceData()
	d.SetId(id)

	results, err := resourceHostingerDNSRecordImport(context.Background(), d, client)
	if err != nil {
		return nil, err
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 imported resource, got %d", len(results))
	}
	return results[0], nil
}

func TestResourceHostingerDNSRecordImport(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantName  string
		wantType  string
		wantValue string
		wantTTL   int
	}{
		{
			name:      "A record",
			id:        "example.com|@|A|192.0.2.1",
			wantName:  "@",
			wantType:  "A",
			wantValue: "192.0.2.1",
			wantTTL:   14400,
		},
		{
			name:      "CNAME with trailing dot",
			id:        "example.com|www|CNAME|example.com.",
			wantName:  "www",
			wantType:  "CNAME",
			wantValue: "example.com.",
			wantTTL:   3600,
		},
		{
			// One value out of a multi-value group imports on its own, which is
			// how the resource models a name/type holding several records.
			name:      "one value of a multi-value MX group",
			id:        "example.com|@|MX|20 mx2.example.com",
			wantName:  "@",
			wantType:  "MX",
			wantValue: "20 mx2.example.com",
			wantTTL:   14400,
		},
		{
			// The zone stores TXT content wrapped in quotes; config carries it
			// without. compareTXTContent bridges the two, and state keeps the
			// form the import ID used so no diff follows the import.
			name:      "TXT without the quotes the zone stores",
			id:        `example.com|@|TXT|v=spf1 include:_spf.example.com ~all`,
			wantName:  "@",
			wantType:  "TXT",
			wantValue: `v=spf1 include:_spf.example.com ~all`,
			wantTTL:   14400,
		},
		{
			name:      "TXT with embedded quotes",
			id:        `example.com|@|TXT|"v=spf1 include:_spf.example.com ~all"`,
			wantName:  "@",
			wantType:  "TXT",
			wantValue: `"v=spf1 include:_spf.example.com ~all"`,
			wantTTL:   14400,
		},
		{
			// SplitN with a limit of 4 keeps everything after the third
			// separator as the value, so "|" inside a value survives.
			name:      "value containing the ID separator",
			id:        `example.com|pipe|TXT|key=a|b|c`,
			wantName:  "pipe",
			wantType:  "TXT",
			wantValue: `key=a|b|c`,
			wantTTL:   300,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})

			d, err := importRecord(t, server.client(), tc.id)
			if err != nil {
				t.Fatalf("import failed: %v", err)
			}

			if got := d.Get("zone"); got != "example.com" {
				t.Errorf("zone: got %v, want example.com", got)
			}
			if got := d.Get("name"); got != tc.wantName {
				t.Errorf("name: got %v, want %v", got, tc.wantName)
			}
			if got := d.Get("type"); got != tc.wantType {
				t.Errorf("type: got %v, want %v", got, tc.wantType)
			}
			if got := d.Get("value"); got != tc.wantValue {
				t.Errorf("value: got %v, want %v", got, tc.wantValue)
			}
			if got := d.Get("ttl"); got != tc.wantTTL {
				t.Errorf("ttl: got %v, want %v", got, tc.wantTTL)
			}

			// The importer stores the 3-part synthetic ID the CRUD functions
			// use, not the 4-part ID it was given.
			wantID := tc.wantName + "|" + tc.wantType + "|" + tc.wantValue
			if got := d.Id(); got != wantID {
				t.Errorf("id: got %v, want %v", got, wantID)
			}

			if n := server.countRequests("GET", "/api/dns/v1/zones/example.com"); n != 1 {
				t.Errorf("expected import to make 1 zone GET, got %d", n)
			}
		})
	}
}

func TestResourceHostingerDNSRecordImport_Errors(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		{
			name:    "too few parts",
			id:      "example.com|www|CNAME",
			wantErr: "unexpected import ID format",
		},
		{
			name:    "no separators at all",
			id:      "example.com",
			wantErr: "unexpected import ID format",
		},
		{
			name:    "empty zone",
			id:      "|www|CNAME|example.com.",
			wantErr: "zone, name and type must all be set",
		},
		{
			name:    "record absent from the zone",
			id:      "example.com|nope|A|192.0.2.99",
			wantErr: `no A record named "nope" with value "192.0.2.99" found in zone "example.com"`,
		},
		{
			// A disabled record is not served, so it must not import either.
			name:    "disabled record",
			id:      "example.com|disabled|A|192.0.2.9",
			wantErr: "found in zone",
		},
		{
			name:    "unknown zone",
			id:      "nosuchzone.com|@|A|192.0.2.1",
			wantErr: "failed to read DNS records",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})

			_, err := importRecord(t, server.client(), tc.id)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err)
			}
		})
	}
}

// TestResourceHostingerDNSRecordImport_MalformedIDMakesNoRequest checks that a
// bad import ID is rejected before any API call, so a typo cannot cost a
// request against a rate-limited API.
func TestResourceHostingerDNSRecordImport_MalformedIDMakesNoRequest(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})

	if _, err := importRecord(t, server.client(), "example.com|www"); err == nil {
		t.Fatal("expected an error, got none")
	}
	if n := server.totalRequests(); n != 0 {
		t.Errorf("expected no API requests, got %d", n)
	}
}
