package hostinger

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func newTestClient(server *httptest.Server) *HostingerClient {
	return &HostingerClient{
		BaseURL:    server.URL,
		HTTPClient: http.DefaultClient,
		Token:      "test-token",
	}
}

func TestResourceHostingerVPSFirewall_Schema(t *testing.T) {
	res := resourceHostingerVPSFirewall()

	if err := res.InternalValidate(res.Schema, true); err != nil {
		t.Fatalf("schema validation failed: %s", err)
	}

	for _, field := range []string{"name", "rule", "is_synced", "created_at", "updated_at"} {
		if _, ok := res.Schema[field]; !ok {
			t.Errorf("expected field %q not found in schema", field)
		}
	}

	if !res.Schema["name"].ForceNew {
		t.Errorf("expected name to be ForceNew")
	}

	ruleElem := res.Schema["rule"].Elem.(*schema.Resource)
	if !ruleElem.Schema["action"].Computed {
		t.Errorf("expected rule.action to be Computed")
	}
	if !ruleElem.Schema["id"].Computed {
		t.Errorf("expected rule.id to be Computed")
	}
}

func TestResourceHostingerVPSFirewallActivation_Schema(t *testing.T) {
	res := resourceHostingerVPSFirewallActivation()

	if err := res.InternalValidate(res.Schema, true); err != nil {
		t.Fatalf("schema validation failed: %s", err)
	}

	for _, field := range []string{"firewall_id", "virtual_machine_id", "triggers", "is_synced"} {
		if _, ok := res.Schema[field]; !ok {
			t.Errorf("expected field %q not found in schema", field)
		}
	}

	if !res.Schema["firewall_id"].ForceNew || !res.Schema["virtual_machine_id"].ForceNew {
		t.Errorf("expected firewall_id and virtual_machine_id to be ForceNew")
	}
	if !res.Schema["is_synced"].Computed {
		t.Errorf("expected is_synced to be Computed")
	}
}

func TestCreateFirewall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/vps/v1/firewall" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["name"] != "web-fw" {
			t.Errorf("expected name 'web-fw', got %v", body["name"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":65224,"name":"web-fw","is_synced":false,"rules":[]}`))
	}))
	defer server.Close()

	fw, err := newTestClient(server).CreateFirewall("web-fw")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if fw.ID != 65224 || fw.Name != "web-fw" {
		t.Errorf("unexpected firewall: %+v", fw)
	}
}

func TestGetFirewall_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := newTestClient(server).GetFirewall(999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateFirewallRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/vps/v1/firewall/65224/rules" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		// The create request must carry only the four user fields.
		for _, want := range []string{"protocol", "port", "source", "source_detail"} {
			if _, ok := body[want]; !ok {
				t.Errorf("expected request body to contain %q", want)
			}
		}
		// action and id are read-only and must not be sent.
		if _, ok := body["action"]; ok {
			t.Errorf("request body must not contain 'action'")
		}
		if _, ok := body["id"]; ok {
			t.Errorf("request body must not contain 'id'")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":24541,"action":"accept","protocol":"HTTPS","port":"443","source":"any","source_detail":"any"}`))
	}))
	defer server.Close()

	rule, err := newTestClient(server).CreateFirewallRule(65224, FirewallRule{
		Protocol:     "HTTPS",
		Port:         "443",
		Source:       "any",
		SourceDetail: "any",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rule.ID != 24541 || rule.Action != "accept" {
		t.Errorf("unexpected rule: %+v", rule)
	}
}

func TestDeleteFirewallRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/vps/v1/firewall/65224/rules/24541" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"deleted"}`))
	}))
	defer server.Close()

	if err := newTestClient(server).DeleteFirewallRule(65224, 24541); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestFirewallVMActions(t *testing.T) {
	for _, action := range []string{"activate", "deactivate", "sync"} {
		t.Run(action, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := "/api/vps/v1/firewall/65224/" + action + "/1268054"
				if r.Method != http.MethodPost || r.URL.Path != want {
					t.Fatalf("unexpected request: %s %s (want POST %s)", r.Method, r.URL.Path, want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":8123712,"name":"action_name","state":"success"}`))
			}))
			defer server.Close()

			client := newTestClient(server)
			var (
				res *FirewallAction
				err error
			)
			switch action {
			case "activate":
				res, err = client.ActivateFirewall(65224, 1268054)
			case "deactivate":
				res, err = client.DeactivateFirewall(65224, 1268054)
			case "sync":
				res, err = client.SyncFirewall(65224, 1268054)
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if res.State != "success" {
				t.Errorf("expected state 'success', got %q", res.State)
			}
		})
	}
}

func TestUpdateFirewallRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/vps/v1/firewall/65224/rules/24541" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["port"] != "8443" {
			t.Errorf("expected updated port '8443', got %v", body["port"])
		}
		// action and id are read-only and must not be sent.
		if _, ok := body["action"]; ok {
			t.Errorf("request body must not contain 'action'")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":24541,"action":"accept","protocol":"HTTPS","port":"8443","source":"any","source_detail":"any"}`))
	}))
	defer server.Close()

	rule, err := newTestClient(server).UpdateFirewallRule(65224, 24541, FirewallRule{
		Protocol:     "HTTPS",
		Port:         "8443",
		Source:       "any",
		SourceDetail: "any",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rule.ID != 24541 || rule.Port != "8443" {
		t.Errorf("unexpected rule: %+v", rule)
	}
}

func TestPlanFirewallRuleChanges(t *testing.T) {
	rule := func(id int, proto, port string) map[string]interface{} {
		return map[string]interface{}{
			"id":            id,
			"action":        "accept",
			"protocol":      proto,
			"port":          port,
			"source":        "any",
			"source_detail": "any",
		}
	}
	// State rules carry their server-assigned IDs; config rules (id 0) do not.
	old := []interface{}{rule(1, "SSH", "22"), rule(2, "HTTPS", "443")}

	// Edit in place: same length, one field changed -> one PUT keeping the ID.
	edited := []interface{}{rule(0, "SSH", "22"), rule(0, "HTTPS", "8443")}
	updates, creates, deletes := planFirewallRuleChanges(old, edited)
	if len(updates) != 1 || updates[0].ID != 2 || updates[0].Rule.Port != "8443" {
		t.Errorf("expected one in-place update of rule 2 to port 8443, got %+v", updates)
	}
	if len(creates) != 0 || len(deletes) != 0 {
		t.Errorf("expected no creates/deletes, got creates=%v deletes=%v", creates, deletes)
	}

	// Append: new list longer -> one create.
	grown := []interface{}{rule(0, "SSH", "22"), rule(0, "HTTPS", "443"), rule(0, "TCP", "8080")}
	updates, creates, deletes = planFirewallRuleChanges(old, grown)
	if len(creates) != 1 || creates[0].Port != "8080" || len(updates) != 0 || len(deletes) != 0 {
		t.Errorf("expected one create, got updates=%v creates=%v deletes=%v", updates, creates, deletes)
	}

	// Shrink: old list longer -> delete trailing rule by ID.
	shrunk := []interface{}{rule(0, "SSH", "22")}
	updates, creates, deletes = planFirewallRuleChanges(old, shrunk)
	if len(deletes) != 1 || deletes[0] != 2 || len(updates) != 0 || len(creates) != 0 {
		t.Errorf("expected one delete of rule 2, got updates=%v creates=%v deletes=%v", updates, creates, deletes)
	}

	// No change -> no API calls (computed id/action must not trigger an update).
	same := []interface{}{rule(0, "SSH", "22"), rule(0, "HTTPS", "443")}
	updates, creates, deletes = planFirewallRuleChanges(old, same)
	if len(updates) != 0 || len(creates) != 0 || len(deletes) != 0 {
		t.Errorf("expected no changes, got updates=%v creates=%v deletes=%v", updates, creates, deletes)
	}
}

func TestParseFirewallActivationID(t *testing.T) {
	fwID, vmID, err := parseFirewallActivationID("65224/1268054")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if fwID != 65224 || vmID != 1268054 {
		t.Errorf("unexpected parse result: fw=%d vm=%d", fwID, vmID)
	}

	for _, bad := range []string{"bad", "65224", "65224/1268054/3", "abc/1", "1/def"} {
		if _, _, err := parseFirewallActivationID(bad); err == nil {
			t.Errorf("expected error for invalid ID %q", bad)
		}
	}
}

func TestValidateFirewallPort(t *testing.T) {
	valid := []string{"1", "443", "65535", "1024:2048", "22:22"}
	for _, p := range valid {
		if _, errs := validateFirewallPort(p, "port"); len(errs) > 0 {
			t.Errorf("expected %q to be valid, got %v", p, errs)
		}
	}

	// 0 and 65536/99999 are out of range; reversed ranges and malformed inputs are rejected.
	invalid := []string{"0", "65536", "99999", "2048:1024", "", "abc", "1:2:3", "443:", ":443", "-1"}
	for _, p := range invalid {
		if _, errs := validateFirewallPort(p, "port"); len(errs) == 0 {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

func TestFirewallVMAction_ErrorState(t *testing.T) {
	// A 200 carrying an "error" action state must surface as an error, not a silent success.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":8123712,"name":"action_name","state":"error"}`))
	}))
	defer server.Close()

	if _, err := newTestClient(server).ActivateFirewall(65224, 1268054); err == nil {
		t.Fatal("expected an error when the action reports state 'error', got nil")
	}
}

func TestFirewallActivationRead_DriftDetection(t *testing.T) {
	const (
		firewallID = 65224
		vmID       = 1268054
		id         = "65224/1268054"
	)

	// handler serves the VM detail with the given firewall_group_id JSON literal
	// (e.g. "65224" or "null"), plus the firewall detail.
	newServer := func(fwGroupID string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api/vps/v1/virtual-machines/1268054":
				_, _ = w.Write([]byte(`{"id":1268054,"firewall_group_id":` + fwGroupID + `}`))
			case "/api/vps/v1/firewall/65224":
				_, _ = w.Write([]byte(`{"id":65224,"name":"web-fw","is_synced":true,"rules":[]}`))
			default:
				t.Errorf("unexpected request path %q", r.URL.Path)
			}
		}))
	}

	read := func(server *httptest.Server) *schema.ResourceData {
		d := resourceHostingerVPSFirewallActivation().TestResourceData()
		d.SetId(id)
		if diags := resourceHostingerVPSFirewallActivationRead(context.Background(), d, newTestClient(server)); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		return d
	}

	t.Run("active on this VM keeps the resource", func(t *testing.T) {
		server := newServer("65224")
		defer server.Close()
		d := read(server)
		if d.Id() != id {
			t.Errorf("expected ID %q to be retained, got %q", id, d.Id())
		}
		if !d.Get("is_synced").(bool) {
			t.Errorf("expected is_synced to be true")
		}
	})

	t.Run("disabled on this VM clears the resource", func(t *testing.T) {
		server := newServer("null")
		defer server.Close()
		if d := read(server); d.Id() != "" {
			t.Errorf("expected ID to be cleared on drift, got %q", d.Id())
		}
	})

	t.Run("different firewall active clears the resource", func(t *testing.T) {
		server := newServer("99999")
		defer server.Close()
		if d := read(server); d.Id() != "" {
			t.Errorf("expected ID to be cleared when another firewall is active, got %q", d.Id())
		}
	})
}
