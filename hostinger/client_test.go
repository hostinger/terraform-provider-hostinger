package hostinger

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateTemplateID(t *testing.T) {
	// Simulate a valid API response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/vps/v1/templates" {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`[
				{"id": 1002, "name": "Debian 11"},
				{"id": 1034, "name": "Ubuntu 20.04"}
			]`)); err != nil {
				t.Fatalf("failed to write mock response: %v", err)
			}
		} else {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer mockServer.Close()

	client := &HostingerClient{
		BaseURL:    mockServer.URL,
		HTTPClient: http.DefaultClient,
		Token:      "test-token",
	}

	ok, err := client.ValidateTemplateID(1002)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Errorf("expected template ID 1002 to be valid")
	}

	ok, err = client.ValidateTemplateID(9999)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ok {
		t.Errorf("expected template ID 9999 to be invalid")
	}
}
