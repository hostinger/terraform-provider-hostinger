package hostinger

import (
	"testing"
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
