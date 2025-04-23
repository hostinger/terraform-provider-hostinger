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

	data.Set("zone", "example.com")
	data.Set("name", "test")
	data.Set("type", "CNAME")
	data.Set("value", "target.example.com")
	data.Set("ttl", 14400)

	// Just check that Set works without panic and has the right values
	if v := data.Get("zone"); v != "example.com" {
		t.Errorf("expected zone to be 'example.com', got %v", v)
	}
}
