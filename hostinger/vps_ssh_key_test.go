package hostinger

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestVMSSHKeyEndpoints(t *testing.T) {
	var lastMethod string
	var lastBody []byte

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/vps/v1/virtual-machines/123/ssh-keys" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		lastMethod = r.Method
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"key": "ssh-ed25519 AAAAC3NzaC test@host", "type": "ssh-ed25519", "data": "AAAAC3NzaC", "name": "test@host"}
		]`))
	}))
	defer mockServer.Close()

	client := &HostingerClient{
		BaseURL:    mockServer.URL,
		HTTPClient: http.DefaultClient,
		Token:      "test-token",
	}

	keys, err := client.ListVMSSHKeys(123)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(keys) != 1 || keys[0].Key != "ssh-ed25519 AAAAC3NzaC test@host" || keys[0].Data != "AAAAC3NzaC" {
		t.Errorf("unexpected keys: %+v", keys)
	}

	want := []string{"ssh-ed25519 AAAAC3NzaC test@host"}

	if err := client.AddVMSSHKeys(123, want); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if lastMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", lastMethod)
	}
	assertKeysBody(t, lastBody, want)

	if err := client.RemoveVMSSHKeys(123, want); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if lastMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", lastMethod)
	}
	assertKeysBody(t, lastBody, want)
}

func TestListPublicKeys(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/vps/v1/public-keys" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": 7, "name": "my key", "key": "ssh-rsa AAAA test@host"}]}`))
	}))
	defer mockServer.Close()

	client := &HostingerClient{
		BaseURL:    mockServer.URL,
		HTTPClient: http.DefaultClient,
		Token:      "test-token",
	}

	keys, err := client.ListPublicKeys()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(keys) != 1 || keys[0].ID != 7 || keys[0].Key != "ssh-rsa AAAA test@host" {
		t.Errorf("unexpected keys: %+v", keys)
	}
}

func assertKeysBody(t *testing.T, body []byte, want []string) {
	t.Helper()
	var req struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if !reflect.DeepEqual(req.Keys, want) {
		t.Errorf("expected keys %v, got %v", want, req.Keys)
	}
}
