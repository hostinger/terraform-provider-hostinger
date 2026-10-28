package hostinger

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
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

func TestDiffVMSSHKeys(t *testing.T) {
	desired := map[string]bool{"a": true, "b": true}
	previous := map[string]bool{"b": true, "c": true, "gone": true}
	onVM := map[string]bool{"b": true, "c": true, "unmanaged": true}

	toAdd, toRemove := diffVMSSHKeys(desired, previous, onVM)

	// "b" already on the VM; "gone" not on the VM (nothing to remove); "unmanaged" never touched
	if !reflect.DeepEqual(toAdd, []string{"a"}) {
		t.Errorf("unexpected toAdd: %v", toAdd)
	}
	if !reflect.DeepEqual(toRemove, []string{"c"}) {
		t.Errorf("unexpected toRemove: %v", toRemove)
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

// fakeKeyAPI serves account public keys and the VM's authorized keys, recording add/remove calls.
type fakeKeyAPI struct {
	account map[int]string
	onVM    []string
	added   []string
	removed []string
}

func (f *fakeKeyAPI) serve(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/vps/v1/public-keys":
			var data []SSHKey
			for id, key := range f.account {
				data = append(data, SSHKey{ID: id, Key: key})
			}
			_ = json.NewEncoder(w).Encode(map[string][]SSHKey{"data": data})
		case "/api/billing/v1/catalog":
			_, _ = w.Write([]byte(`[{"prices": [{"id": "kvm2"}]}]`))
		case "/api/vps/v1/templates":
			_, _ = w.Write([]byte(`[{"id": 1002}]`))
		case "/api/vps/v1/data-centers":
			_, _ = w.Write([]byte(`[{"id": 1}]`))
		case "/api/vps/v1/virtual-machines":
			_, _ = w.Write([]byte(`{"virtual_machine": {"id": 123}}`))
		case "/api/vps/v1/virtual-machines/123":
			_, _ = w.Write([]byte(`{"id": 123, "hostname": "vps.example.com", "state": "running"}`))
		case "/api/vps/v1/virtual-machines/123/ssh-keys":
			if r.Method == http.MethodGet {
				var keys []VMSSHKey
				for _, k := range f.onVM {
					keys = append(keys, VMSSHKey{Key: k})
				}
				_ = json.NewEncoder(w).Encode(keys)
				return
			}
			var body struct {
				Keys []string `json:"keys"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method == http.MethodPost {
				f.added = append(f.added, body.Keys...)
			} else {
				f.removed = append(f.removed, body.Keys...)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func vpsState(attrs map[string]string) *terraform.InstanceState {
	base := map[string]string{
		"id": "123", "vps_id": "123", "plan": "kvm2", "data_center_id": "1", "template_id": "1002",
		"hostname": "vps.example.com",
	}
	for k, v := range attrs {
		base[k] = v
	}
	return &terraform.InstanceState{ID: "123", Attributes: base}
}

func vpsConfig(sshKeyIDs ...interface{}) *terraform.ResourceConfig {
	return terraform.NewResourceConfigRaw(map[string]interface{}{
		"plan": "kvm2", "data_center_id": 1, "template_id": 1002, "hostname": "vps.example.com",
		"ssh_key_ids": sshKeyIDs,
	})
}

func applySSHKeyIDs(t *testing.T, api *fakeKeyAPI, state *terraform.InstanceState, sshKeyIDs ...interface{}) (*terraform.InstanceState, diag.Diagnostics) {
	t.Helper()
	server := api.serve(t)
	defer server.Close()
	client := newTestClient(server)

	r := resourceHostingerVPS()
	diff, err := r.Diff(context.Background(), state, vpsConfig(sshKeyIDs...), client)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	return r.Apply(context.Background(), state, diff, client)
}

func TestVPSUpdate_RevokesRotatedSSHKeyIDDeletedFromAccount(t *testing.T) {
	// Key 7 was rotated to key 8: the account key 7 is already gone when the VPS update runs.
	api := &fakeKeyAPI{
		account: map[int]string{8: "ssh-ed25519 NEW"},
		onVM:    []string{"ssh-ed25519 OLD", "ssh-ed25519 UNMANAGED"},
	}
	state := vpsState(map[string]string{
		"ssh_key_ids.#": "1", "ssh_key_ids.0": "7",
		"ssh_key_ids_resolved.#": "1", "ssh_key_ids_resolved.0": "ssh-ed25519 OLD",
	})

	newState, diags := applySSHKeyIDs(t, api, state, 8)
	if diags.HasError() {
		t.Fatalf("apply failed: %v", diags)
	}
	if !reflect.DeepEqual(api.added, []string{"ssh-ed25519 NEW"}) {
		t.Errorf("unexpected added keys: %v", api.added)
	}
	if !reflect.DeepEqual(api.removed, []string{"ssh-ed25519 OLD"}) {
		t.Errorf("unexpected removed keys: %v", api.removed)
	}
	if got := newState.Attributes["ssh_key_ids_resolved.0"]; got != "ssh-ed25519 NEW" {
		t.Errorf("expected resolved key to be updated, got %q", got)
	}
}

func TestVPSUpdate_RemovingAllSSHKeyIDsRevokesThem(t *testing.T) {
	api := &fakeKeyAPI{onVM: []string{"ssh-ed25519 OLD"}}
	state := vpsState(map[string]string{
		"ssh_key_ids.#": "1", "ssh_key_ids.0": "7",
		"ssh_key_ids_resolved.#": "1", "ssh_key_ids_resolved.0": "ssh-ed25519 OLD",
	})

	newState, diags := applySSHKeyIDs(t, api, state)
	if diags.HasError() {
		t.Fatalf("apply failed: %v", diags)
	}
	if !reflect.DeepEqual(api.removed, []string{"ssh-ed25519 OLD"}) {
		t.Errorf("unexpected removed keys: %v", api.removed)
	}
	if got := newState.Attributes["ssh_key_ids_resolved.#"]; got != "0" {
		t.Errorf("expected resolved keys to be cleared, got count %q", got)
	}
}

func TestVPSUpdate_LegacyStateFallsBackToAccountLookup(t *testing.T) {
	api := &fakeKeyAPI{
		account: map[int]string{7: "ssh-ed25519 OLD", 8: "ssh-ed25519 NEW"},
		onVM:    []string{"ssh-ed25519 OLD"},
	}
	state := vpsState(map[string]string{"ssh_key_ids.#": "1", "ssh_key_ids.0": "7"})

	if _, diags := applySSHKeyIDs(t, api, state, 8); diags.HasError() {
		t.Fatalf("apply failed: %v", diags)
	}
	if !reflect.DeepEqual(api.removed, []string{"ssh-ed25519 OLD"}) {
		t.Errorf("unexpected removed keys: %v", api.removed)
	}
}

func TestVPSUpdate_UnknownNewSSHKeyIDFails(t *testing.T) {
	api := &fakeKeyAPI{onVM: []string{}}
	state := vpsState(nil)

	_, diags := applySSHKeyIDs(t, api, state, 9)
	if !diags.HasError() || !strings.Contains(diags[0].Summary, "SSH key with ID 9 not found") {
		t.Fatalf("expected not-found error, got %v", diags)
	}
	if len(api.added) != 0 || len(api.removed) != 0 {
		t.Errorf("expected no VM key changes, got added %v removed %v", api.added, api.removed)
	}
}

func TestVPSRead_BackfillsResolvedSSHKeysForLegacyState(t *testing.T) {
	api := &fakeKeyAPI{account: map[int]string{7: "ssh-ed25519 OLD"}}
	server := api.serve(t)
	defer server.Close()

	// Key 99 no longer exists in the account and is skipped.
	state := vpsState(map[string]string{"ssh_key_ids.#": "2", "ssh_key_ids.0": "7", "ssh_key_ids.1": "99"})
	newState, diags := resourceHostingerVPS().RefreshWithoutUpgrade(context.Background(), state, newTestClient(server))
	if diags.HasError() {
		t.Fatalf("refresh failed: %v", diags)
	}
	if newState.Attributes["ssh_key_ids_resolved.#"] != "1" || newState.Attributes["ssh_key_ids_resolved.0"] != "ssh-ed25519 OLD" {
		t.Errorf("unexpected resolved keys: %v", newState.Attributes)
	}
}

func TestVPSRead_KeepsExistingResolvedSSHKeys(t *testing.T) {
	// No public-keys endpoint call expected: the account no longer has key 7, yet the stored key must survive.
	api := &fakeKeyAPI{}
	server := api.serve(t)
	defer server.Close()

	state := vpsState(map[string]string{
		"ssh_key_ids.#": "1", "ssh_key_ids.0": "7",
		"ssh_key_ids_resolved.#": "1", "ssh_key_ids_resolved.0": "ssh-ed25519 OLD",
	})
	newState, diags := resourceHostingerVPS().RefreshWithoutUpgrade(context.Background(), state, newTestClient(server))
	if diags.HasError() {
		t.Fatalf("refresh failed: %v", diags)
	}
	if newState.Attributes["ssh_key_ids_resolved.0"] != "ssh-ed25519 OLD" {
		t.Errorf("resolved key lost on refresh: %v", newState.Attributes)
	}
}

func TestVPSCreate_StoresResolvedSSHKeyIDs(t *testing.T) {
	api := &fakeKeyAPI{account: map[int]string{7: "ssh-ed25519 OLD"}}

	newState, diags := applySSHKeyIDs(t, api, nil, 7)
	if diags.HasError() {
		t.Fatalf("apply failed: %v", diags)
	}
	if !reflect.DeepEqual(api.added, []string{"ssh-ed25519 OLD"}) {
		t.Errorf("unexpected added keys: %v", api.added)
	}
	if newState.Attributes["ssh_key_ids_resolved.0"] != "ssh-ed25519 OLD" {
		t.Errorf("unexpected resolved keys: %v", newState.Attributes)
	}
}

func TestVPSCreate_UnknownSSHKeyIDFails(t *testing.T) {
	api := &fakeKeyAPI{}

	_, diags := applySSHKeyIDs(t, api, nil, 9)
	if !diags.HasError() || !strings.Contains(diags[0].Summary, "SSH key with ID 9 not found") {
		t.Fatalf("expected not-found error, got %v", diags)
	}
}
